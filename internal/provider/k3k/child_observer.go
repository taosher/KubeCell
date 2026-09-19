package k3k

import (
	"context"
	"fmt"
	"net"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/render"
)

type HostReader interface {
	GetSecret(context.Context, string, string) (*corev1.Secret, error)
	GetService(context.Context, string, string) (*corev1.Service, error)
	GetStorageClass(context.Context, string) (*storagev1.StorageClass, error)
}

type ChildClient interface {
	Ready(context.Context) error
	ListNodes(context.Context) (*corev1.NodeList, error)
	EnsureStorageClass(context.Context, *storagev1.StorageClass) error
	ListIngresses(context.Context) (*networkingv1.IngressList, error)
	GetService(ctx context.Context, namespace, name string) (*corev1.Service, error)
	GetEndpoints(ctx context.Context, namespace, name string) (*corev1.Endpoints, error)
}

type ChildClientFactory interface {
	New([]byte, string) (ChildClient, error)
}

type ResourceObservation struct {
	Kubeconfig        []byte
	SourceSecretName  string
	NodePort          int32
	Endpoint          string
	APIReady          bool
	LogicalNodeName   string
	LogicalNodeReady  bool
	StorageClassReady bool
	// Client is the connected child-cluster client (endpoint rewrite confirmed), reused by endpoint sync and later reads
	// to avoid reconnecting on every call.
	Client ChildClient
}

type ResourceObserver struct {
	Reader        HostReader
	ClientFactory ChildClientFactory
}

func (o ResourceObserver) ObserveResources(ctx context.Context, cluster *api.VirtualCluster) (ResourceObservation, error) {
	if o.Reader == nil {
		return ResourceObservation{}, fmt.Errorf("Host reader is not configured")
	}
	if cluster == nil || cluster.Status.Resolved.HostNamespace == "" || cluster.Status.Resolved.K3kClusterName == "" {
		return ResourceObservation{}, fmt.Errorf("resolved Host namespace and K3k cluster name are required")
	}
	namespace := cluster.Status.Resolved.HostNamespace
	name := cluster.Status.Resolved.K3kClusterName
	secretName := "k3k-" + name + "-kubeconfig"
	secret, err := o.Reader.GetSecret(ctx, namespace, secretName)
	if err != nil {
		return ResourceObservation{}, fmt.Errorf("read Child kubeconfig Secret %s/%s: %w", namespace, secretName, err)
	}
	kubeconfig, found := secret.Data["kubeconfig.yaml"]
	if !found || len(kubeconfig) == 0 {
		return ResourceObservation{}, fmt.Errorf("Child kubeconfig Secret %s/%s has no kubeconfig.yaml", namespace, secretName)
	}
	if uid := secret.Labels["kubecell.io/virtual-cluster-uid"]; uid != "" && uid != string(cluster.UID) {
		return ResourceObservation{}, fmt.Errorf("Child kubeconfig Secret %s/%s belongs to VirtualCluster UID %q, want %q", namespace, secretName, uid, cluster.UID)
	}
	if secret.Labels["kubecell.io/virtual-cluster-uid"] == "" && !isOwnedByK3kCluster(secret, name) {
		return ResourceObservation{}, fmt.Errorf("Child kubeconfig Secret %s/%s is not owned by K3k Cluster %q", namespace, secretName, name)
	}
	serviceName := "k3k-" + name + "-service"
	service, err := o.Reader.GetService(ctx, namespace, serviceName)
	if err != nil {
		return ResourceObservation{}, fmt.Errorf("read Child API Service %s/%s: %w", namespace, serviceName, err)
	}
	for _, port := range service.Spec.Ports {
		if port.Port == 443 && port.NodePort > 0 {
			return ResourceObservation{Kubeconfig: append([]byte(nil), kubeconfig...), SourceSecretName: secretName, NodePort: port.NodePort}, nil
		}
	}
	return ResourceObservation{}, fmt.Errorf("Child API Service %s/%s has no NodePort for port 443", namespace, serviceName)
}

func isOwnedByK3kCluster(secret *corev1.Secret, clusterName string) bool {
	if secret == nil {
		return false
	}
	for _, owner := range secret.OwnerReferences {
		if owner.Kind == "Cluster" && owner.Name == clusterName && strings.HasPrefix(owner.APIVersion, "k3k.io/") {
			return true
		}
	}
	return false
}

func (o ResourceObserver) Observe(ctx context.Context, cluster *api.VirtualCluster, addresses []string) (ResourceObservation, error) {
	observation, err := o.ObserveResources(ctx, cluster)
	if err != nil {
		return ResourceObservation{}, err
	}
	if o.ClientFactory == nil {
		return ResourceObservation{}, fmt.Errorf("Child client factory is not configured")
	}
	storageClass, err := render.RenderChildStorageClass(cluster.Status.Resolved.Storage.Class.ChildName)
	if err != nil {
		return ResourceObservation{}, fmt.Errorf("render Child StorageClass: %w", err)
	}
	hostStorageClass, err := o.Reader.GetStorageClass(ctx, cluster.Status.Resolved.Storage.Class.HostName)
	if err != nil {
		return ResourceObservation{}, fmt.Errorf("read Host TopoLVM StorageClass %q: %w", cluster.Status.Resolved.Storage.Class.HostName, err)
	}
	if hostStorageClass == nil || hostStorageClass.Provisioner != "topolvm.io" || hostStorageClass.VolumeBindingMode == nil || *hostStorageClass.VolumeBindingMode != storagev1.VolumeBindingWaitForFirstConsumer {
		return ResourceObservation{}, fmt.Errorf("Host StorageClass %q is not a certified TopoLVM WaitForFirstConsumer class", cluster.Status.Resolved.Storage.Class.HostName)
	}
	if len(addresses) == 0 {
		return ResourceObservation{}, fmt.Errorf("at least one Child API address is required")
	}
	var lastErr error
	for _, address := range addresses {
		endpoint := "https://" + net.JoinHostPort(address, fmt.Sprintf("%d", observation.NodePort))
		rewritten, rewriteErr := RewriteKubeconfigEndpoint(observation.Kubeconfig, endpoint)
		if rewriteErr != nil {
			return ResourceObservation{}, rewriteErr
		}
		child, clientErr := o.ClientFactory.New(rewritten, endpoint)
		if clientErr != nil {
			lastErr = clientErr
			continue
		}
		if readyErr := child.Ready(ctx); readyErr != nil {
			lastErr = readyErr
			continue
		}
		nodes, nodeErr := child.ListNodes(ctx)
		if nodeErr != nil {
			lastErr = nodeErr
			continue
		}
		node, nodeErr := readyNode(nodes)
		if nodeErr != nil {
			lastErr = nodeErr
			continue
		}
		if storageErr := child.EnsureStorageClass(ctx, storageClass); storageErr != nil {
			lastErr = storageErr
			continue
		}
		observation.Kubeconfig = rewritten
		observation.Endpoint = endpoint
		observation.APIReady = true
		observation.Client = child
		observation.LogicalNodeName = node.Name
		observation.LogicalNodeReady = true
		observation.StorageClassReady = true
		return observation, nil
	}
	return ResourceObservation{}, fmt.Errorf("Child API probe failed for all addresses: %w", lastErr)
}

type KubernetesChildClientFactory struct{}

func (KubernetesChildClientFactory) New(kubeconfig []byte, _ string) (ChildClient, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse Child kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Child Kubernetes client: %w", err)
	}
	return KubernetesChildClient{clientset: clientset}, nil
}

type KubernetesChildClient struct{ clientset kubernetes.Interface }

func (c KubernetesChildClient) Ready(ctx context.Context) error {
	request := c.clientset.Discovery().RESTClient().Get().AbsPath("/readyz")
	result := request.Do(ctx)
	return result.Error()
}

func (c KubernetesChildClient) GetNode(ctx context.Context, name string) (*corev1.Node, error) {
	return c.clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
}

func (c KubernetesChildClient) ListNodes(ctx context.Context) (*corev1.NodeList, error) {
	return c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
}

func (c KubernetesChildClient) ListIngresses(ctx context.Context) (*networkingv1.IngressList, error) {
	return c.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
}

func (c KubernetesChildClient) GetService(ctx context.Context, namespace, name string) (*corev1.Service, error) {
	return c.clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c KubernetesChildClient) GetEndpoints(ctx context.Context, namespace, name string) (*corev1.Endpoints, error) {
	return c.clientset.CoreV1().Endpoints(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c KubernetesChildClient) EnsureStorageClass(ctx context.Context, desired *storagev1.StorageClass) error {
	classes := c.clientset.StorageV1().StorageClasses()
	current, err := classes.Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = classes.Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if current.Provisioner != desired.Provisioner || current.VolumeBindingMode == nil || desired.VolumeBindingMode == nil || *current.VolumeBindingMode != *desired.VolumeBindingMode || current.ReclaimPolicy == nil || desired.ReclaimPolicy == nil || *current.ReclaimPolicy != *desired.ReclaimPolicy {
		return fmt.Errorf("Child StorageClass %q conflicts with approved mapping", desired.Name)
	}
	return nil
}

func nodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func readyNode(nodes *corev1.NodeList) (*corev1.Node, error) {
	if nodes == nil || len(nodes.Items) == 0 {
		return nil, fmt.Errorf("Child logical Node list is empty")
	}
	for index := range nodes.Items {
		if nodeReady(&nodes.Items[index]) {
			return &nodes.Items[index], nil
		}
	}
	return nil, fmt.Errorf("no Child logical Node is Ready")
}

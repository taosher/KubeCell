package ocm

import (
	"context"
	"fmt"
	"strings"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	extensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/platform"
	"github.com/kubecell/kubecell/internal/provider/host"
)

type HostReader interface {
	GetSecret(context.Context, string, string) (*corev1.Secret, error)
	GetService(context.Context, string, string) (*corev1.Service, error)
	GetResourceQuota(context.Context, string, string) (*corev1.ResourceQuota, error)
	ListPods(context.Context, string) (*corev1.PodList, error)
	GetPersistentVolumeClaim(context.Context, string, string) (*corev1.PersistentVolumeClaim, error)
	GetPersistentVolume(context.Context, string) (*corev1.PersistentVolume, error)
}

type NodeReader interface {
	ListNodes(context.Context) (*corev1.NodeList, error)
}

type InventoryStorageReader interface {
	ListPersistentVolumes(context.Context) (*corev1.PersistentVolumeList, error)
	ListPersistentVolumeClaims(context.Context) (*corev1.PersistentVolumeClaimList, error)
	ListStorageClasses(context.Context) (*storagev1.StorageClassList, error)
}

type InventoryServiceReader interface {
	ListServices(context.Context) (*corev1.ServiceList, error)
}

type RESTHostReader struct {
	client kubernetes.Interface
}

type RESTBaselineReader struct {
	client     kubernetes.Interface
	extensions extensionsclient.Interface
}

func NewRESTHostReader(config *rest.Config) (*RESTHostReader, error) {
	if config == nil {
		return nil, fmt.Errorf("Host proxy REST config is nil")
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Host proxy client: %w", err)
	}
	return &RESTHostReader{client: client}, nil
}

func NewRESTBaselineReader(config *rest.Config) (*RESTBaselineReader, error) {
	if config == nil {
		return nil, fmt.Errorf("Host proxy REST config is nil")
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Host baseline Kubernetes client: %w", err)
	}
	extensions, err := extensionsclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Host extensions client: %w", err)
	}
	return &RESTBaselineReader{client: client, extensions: extensions}, nil
}

func (r *RESTBaselineReader) ObserveBaseline(ctx context.Context, cell *api.Cell) (host.BaselineObservation, error) {
	if r == nil || r.client == nil || r.extensions == nil {
		return host.BaselineObservation{}, fmt.Errorf("Host baseline reader is not configured")
	}
	k3kNamespace := platform.K3kControllerNamespace
	namespace, err := r.client.CoreV1().Namespaces().Get(ctx, k3kNamespace, metav1.GetOptions{})
	if err != nil {
		return host.BaselineObservation{}, fmt.Errorf("read K3k namespace %s: %w", k3kNamespace, err)
	}
	crds, err := r.extensions.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return host.BaselineObservation{}, fmt.Errorf("list K3k CRDs: %w", err)
	}
	deployments, err := r.client.AppsV1().Deployments(k3kNamespace).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=k3k"})
	if err != nil {
		return host.BaselineObservation{}, fmt.Errorf("list K3k controller Deployments: %w", err)
	}
	topolvmNamespace, topolvmErr := r.client.CoreV1().Namespaces().Get(ctx, "topolvm-system", metav1.GetOptions{})
	if topolvmErr != nil && !apierrors.IsNotFound(topolvmErr) {
		return host.BaselineObservation{}, fmt.Errorf("read TopoLVM namespace: %w", topolvmErr)
	}
	var topolvmDeployments *apps.DeploymentList
	var topolvmNodeDaemonSets, topolvmLvmdDaemonSets *apps.DaemonSetList
	if topolvmErr == nil {
		topolvmDeployments, err = r.client.AppsV1().Deployments("topolvm-system").List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=topolvm"})
		if err != nil {
			return host.BaselineObservation{}, fmt.Errorf("list TopoLVM Deployments: %w", err)
		}
		topolvmDaemonSets, err := r.client.AppsV1().DaemonSets("topolvm-system").List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=topolvm"})
		if err != nil {
			return host.BaselineObservation{}, fmt.Errorf("list TopoLVM node DaemonSets: %w", err)
		}
		topolvmNodeDaemonSets, topolvmLvmdDaemonSets = splitTopoLVMDaemonSets(topolvmDaemonSets)
	}
	storageClasses, err := r.client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return host.BaselineObservation{}, fmt.Errorf("list StorageClasses: %w", err)
	}
	csiDrivers, err := r.client.StorageV1().CSIDrivers().List(ctx, metav1.ListOptions{})
	if err != nil {
		return host.BaselineObservation{}, fmt.Errorf("list CSIDrivers: %w", err)
	}
	nodes, err := r.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return host.BaselineObservation{}, fmt.Errorf("list Host Nodes: %w", err)
	}
	return host.EvaluateBaseline(host.BaselineInputs{
		K3kNamespaceExists:     namespace.Status.Phase == corev1.NamespaceActive,
		K3kCRDs:                filterK3kCRDs(crds.Items),
		K3kDeployments:         deployments.Items,
		TopoLVMNamespaceExists: topolvmErr == nil && topolvmNamespace.Status.Phase == corev1.NamespaceActive,
		TopoLVMDeployments:     deploymentItems(topolvmDeployments),
		TopoLVMNodeDaemonSets:  daemonSetItems(topolvmNodeDaemonSets),
		TopoLVMLvmdDaemonSets:  daemonSetItems(topolvmLvmdDaemonSets),
		TopoLVMStorageClasses:  storageClasses.Items,
		TopoLVMCSIDrivers:      csiDrivers.Items,
		Nodes:                  nodes.Items,
	}), nil
}

func splitTopoLVMDaemonSets(list *apps.DaemonSetList) (*apps.DaemonSetList, *apps.DaemonSetList) {
	node := &apps.DaemonSetList{}
	lvmd := &apps.DaemonSetList{}
	if list == nil {
		return node, lvmd
	}
	for _, daemonSet := range list.Items {
		if topoLVMComponent(daemonSet, "node") {
			node.Items = append(node.Items, daemonSet)
		}
		if topoLVMComponent(daemonSet, "lvmd") {
			lvmd.Items = append(lvmd.Items, daemonSet)
		}
	}
	return node, lvmd
}

func topoLVMComponent(daemonSet apps.DaemonSet, component string) bool {
	if daemonSet.Labels["app.kubernetes.io/component"] == component || daemonSet.Spec.Template.Labels["app.kubernetes.io/component"] == component {
		return true
	}
	if component == "node" {
		return daemonSet.Name == "topolvm-node"
	}
	return component == "lvmd" && strings.HasPrefix(daemonSet.Name, "topolvm-lvmd-")
}

func filterK3kCRDs(items []apiextensionsv1.CustomResourceDefinition) []apiextensionsv1.CustomResourceDefinition {
	result := make([]apiextensionsv1.CustomResourceDefinition, 0, 2)
	for _, item := range items {
		if item.Name == "clusters.k3k.io" || item.Name == "virtualclusterpolicies.k3k.io" {
			result = append(result, item)
		}
	}
	return result
}

func deploymentItems(list *apps.DeploymentList) []apps.Deployment {
	if list == nil {
		return nil
	}
	return list.Items
}

func daemonSetItems(list *apps.DaemonSetList) []apps.DaemonSet {
	if list == nil {
		return nil
	}
	return list.Items
}

func (r *RESTHostReader) GetSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	return r.client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (r *RESTHostReader) GetService(ctx context.Context, namespace, name string) (*corev1.Service, error) {
	return r.client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (r *RESTHostReader) GetStorageClass(ctx context.Context, name string) (*storagev1.StorageClass, error) {
	return r.client.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
}

func (r *RESTHostReader) GetResourceQuota(ctx context.Context, namespace, name string) (*corev1.ResourceQuota, error) {
	return r.client.CoreV1().ResourceQuotas(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (r *RESTHostReader) ListPods(ctx context.Context, namespace string) (*corev1.PodList, error) {
	return r.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
}

func (r *RESTHostReader) GetPersistentVolumeClaim(ctx context.Context, namespace, name string) (*corev1.PersistentVolumeClaim, error) {
	return r.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (r *RESTHostReader) GetPersistentVolume(ctx context.Context, name string) (*corev1.PersistentVolume, error) {
	return r.client.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
}

func (r *RESTHostReader) ListNodes(ctx context.Context) (*corev1.NodeList, error) {
	return r.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
}

func (r *RESTHostReader) ListAllPods(ctx context.Context) (*corev1.PodList, error) {
	return r.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
}

func (r *RESTHostReader) ListPersistentVolumes(ctx context.Context) (*corev1.PersistentVolumeList, error) {
	return r.client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
}

func (r *RESTHostReader) ListPersistentVolumeClaims(ctx context.Context) (*corev1.PersistentVolumeClaimList, error) {
	return r.client.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
}

func (r *RESTHostReader) ListStorageClasses(ctx context.Context) (*storagev1.StorageClassList, error) {
	return r.client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
}

func (r *RESTHostReader) ListServices(ctx context.Context) (*corev1.ServiceList, error) {
	return r.client.CoreV1().Services("").List(ctx, metav1.ListOptions{})
}

package k3k

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/kubecell/kubecell/api/v1alpha1"
)

func TestObserveChildResourcesReadsKubeconfigAndNodePort(t *testing.T) {
	reader := fakeHostReader{
		secret:      &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "k3k-demo-kubeconfig", Namespace: "host-ns", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "cluster-uid"}}, Data: map[string][]byte{"kubeconfig.yaml": []byte("apiVersion: v1\nclusters: []\n")}},
		service:     &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "k3k-demo-service", Namespace: "host-ns", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "cluster-uid"}}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "https", Port: 443, NodePort: 30443}}}},
		serviceName: "k3k-demo-service",
	}
	observer := ResourceObserver{Reader: reader}
	cluster := &api.VirtualCluster{ObjectMeta: metav1.ObjectMeta{UID: "cluster-uid"}, Status: api.VirtualClusterStatus{Resolved: api.ResolvedSnapshot{HostNamespace: "host-ns", K3kClusterName: "demo"}}}
	observation, err := observer.ObserveResources(context.Background(), cluster)
	if err != nil {
		t.Fatalf("ObserveResources() error = %v", err)
	}
	if observation.SourceSecretName != "k3k-demo-kubeconfig" || observation.NodePort != 30443 {
		t.Fatalf("observation = %#v", observation)
	}
}

func TestObserveChildResourcesAcceptsK3kOwnedSecretWithoutKubecellLabel(t *testing.T) {
	reader := fakeHostReader{
		secret: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "k3k-demo-kubeconfig",
				Namespace: "host-ns",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "k3k.io/v1beta1",
					Kind:       "Cluster",
					Name:       "demo",
					UID:        "k3k-cluster-uid",
				}},
			},
			Data: map[string][]byte{"kubeconfig.yaml": []byte("apiVersion: v1\nclusters: []\n")},
		},
		service: &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "k3k-demo-service", Namespace: "host-ns"}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 443, NodePort: 30443}}}},
	}
	observer := ResourceObserver{Reader: reader}
	cluster := &api.VirtualCluster{ObjectMeta: metav1.ObjectMeta{UID: "cluster-uid"}, Status: api.VirtualClusterStatus{Resolved: api.ResolvedSnapshot{HostNamespace: "host-ns", K3kClusterName: "demo"}}}

	if _, err := observer.ObserveResources(context.Background(), cluster); err != nil {
		t.Fatalf("ObserveResources() error = %v", err)
	}
}

func TestObserveChildResourcesRejectsUnownedSecretWithoutKubecellLabel(t *testing.T) {
	reader := fakeHostReader{
		secret:  &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "k3k-demo-kubeconfig", Namespace: "host-ns"}, Data: map[string][]byte{"kubeconfig.yaml": []byte("apiVersion: v1\nclusters: []\n")}},
		service: &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "k3k-demo-service", Namespace: "host-ns"}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 443, NodePort: 30443}}}},
	}
	observer := ResourceObserver{Reader: reader}
	cluster := &api.VirtualCluster{ObjectMeta: metav1.ObjectMeta{UID: "cluster-uid"}, Status: api.VirtualClusterStatus{Resolved: api.ResolvedSnapshot{HostNamespace: "host-ns", K3kClusterName: "demo"}}}

	if _, err := observer.ObserveResources(context.Background(), cluster); err == nil {
		t.Fatal("ObserveResources() error = nil, want unowned Secret rejection")
	}
}

func TestResourceObserverProbesChildAndEnsuresStorageClass(t *testing.T) {
	reader := fakeHostReader{
		secret:       &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "k3k-demo-kubeconfig", Namespace: "host-ns", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "cluster-uid"}}, Data: map[string][]byte{"kubeconfig.yaml": []byte("apiVersion: v1\nclusters:\n- name: child\n  cluster:\n    server: https://10.0.0.1:6443\n")}},
		service:      &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "k3k-demo-service", Namespace: "host-ns", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "cluster-uid"}}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "https", Port: 443, NodePort: 30443}}}},
		serviceName:  "k3k-demo-service",
		storageClass: &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "topolvm-provisioner"}, Provisioner: "topolvm.io", VolumeBindingMode: storageMode(storagev1.VolumeBindingWaitForFirstConsumer)},
	}
	child := fakeChildClient{ready: true, nodes: &corev1.NodeList{Items: []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "host-01"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}}}}
	factory := &fakeChildClientFactory{client: &child}
	observer := ResourceObserver{Reader: reader, ClientFactory: factory}
	cluster := &api.VirtualCluster{ObjectMeta: metav1.ObjectMeta{UID: "cluster-uid"}, Spec: api.VirtualClusterSpec{}, Status: api.VirtualClusterStatus{Resolved: api.ResolvedSnapshot{HostNamespace: "host-ns", K3kClusterName: "demo", Storage: api.StorageMappingSpec{Class: api.StorageClassMapping{ChildName: "topolvm-provisioner", HostName: "topolvm-provisioner", VolumeBindingMode: storagev1.VolumeBindingWaitForFirstConsumer, ReclaimPolicy: corev1.PersistentVolumeReclaimRetain}}}}}

	observation, err := observer.Observe(context.Background(), cluster, []string{"203.0.113.10"})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if !observation.APIReady || !observation.LogicalNodeReady || !observation.StorageClassReady {
		t.Fatalf("observation = %#v", observation)
	}
	if factory.endpoint != "https://203.0.113.10:30443" {
		t.Fatalf("Child client endpoint = %q", factory.endpoint)
	}
	if child.storageClass == nil || child.storageClass.Name != "topolvm-provisioner" || child.storageClass.Provisioner != "topolvm.io" {
		t.Fatalf("storage class = %#v", child.storageClass)
	}
}

func TestResourceObserverRejectsMissingHostTopoLVMStorageClass(t *testing.T) {
	reader := fakeHostReader{
		secret:  &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "k3k-demo-kubeconfig", Namespace: "host-ns", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "cluster-uid"}}, Data: map[string][]byte{"kubeconfig.yaml": []byte("apiVersion: v1\nclusters:\n- name: child\n  cluster:\n    server: https://10.0.0.1:6443\n")}},
		service: &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "k3k-demo-service", Namespace: "host-ns"}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 443, NodePort: 30443}}}},
	}
	child := fakeChildClient{ready: true, nodes: &corev1.NodeList{Items: []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "host-01"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}}}}
	observer := ResourceObserver{Reader: reader, ClientFactory: &fakeChildClientFactory{client: &child}}
	cluster := &api.VirtualCluster{ObjectMeta: metav1.ObjectMeta{UID: "cluster-uid"}, Status: api.VirtualClusterStatus{Resolved: api.ResolvedSnapshot{HostNamespace: "host-ns", K3kClusterName: "demo", Storage: api.StorageMappingSpec{Class: api.StorageClassMapping{ChildName: "topolvm-provisioner", HostName: "topolvm-provisioner", VolumeBindingMode: storagev1.VolumeBindingWaitForFirstConsumer, ReclaimPolicy: corev1.PersistentVolumeReclaimRetain}}}}}
	if _, err := observer.Observe(context.Background(), cluster, []string{"203.0.113.10"}); err == nil {
		t.Fatal("Observe() error = nil, want missing Host TopoLVM StorageClass rejection")
	}
}

func TestResourceObserverRejectsForeignKubeconfigSecret(t *testing.T) {
	reader := fakeHostReader{secret: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "k3k-demo-kubeconfig", Namespace: "host-ns", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "other"}}, Data: map[string][]byte{"kubeconfig.yaml": []byte("apiVersion: v1\nclusters: []\n")}}, service: &corev1.Service{}}
	observer := ResourceObserver{Reader: reader}
	cluster := &api.VirtualCluster{ObjectMeta: metav1.ObjectMeta{UID: "cluster-uid"}, Status: api.VirtualClusterStatus{Resolved: api.ResolvedSnapshot{HostNamespace: "host-ns", K3kClusterName: "demo"}}}
	if _, err := observer.ObserveResources(context.Background(), cluster); err == nil {
		t.Fatal("ObserveResources() error = nil, want foreign Secret rejection")
	}
}

type fakeChildClientFactory struct {
	client   ChildClient
	endpoint string
}

func (f *fakeChildClientFactory) New(_ []byte, endpoint string) (ChildClient, error) {
	f.endpoint = endpoint
	return f.client, nil
}

type fakeChildClient struct {
	ready        bool
	nodes        *corev1.NodeList
	storageClass *storagev1.StorageClass
}

func (f fakeChildClient) Ready(context.Context) error {
	if !f.ready {
		return fmt.Errorf("not ready")
	}
	return nil
}

func (f fakeChildClient) ListNodes(context.Context) (*corev1.NodeList, error) { return f.nodes, nil }

func (f fakeChildClient) ListIngresses(context.Context) (*networkingv1.IngressList, error) {
	return &networkingv1.IngressList{}, nil
}

func (f fakeChildClient) GetService(context.Context, string, string) (*corev1.Service, error) {
	return nil, fmt.Errorf("not found")
}

func (f fakeChildClient) GetEndpoints(context.Context, string, string) (*corev1.Endpoints, error) {
	return nil, fmt.Errorf("not found")
}

func (f *fakeChildClient) EnsureStorageClass(_ context.Context, desired *storagev1.StorageClass) error {
	f.storageClass = desired.DeepCopy()
	return nil
}

type fakeHostReader struct {
	secret       *corev1.Secret
	service      *corev1.Service
	storageClass *storagev1.StorageClass
	serviceName  string
}

func (f fakeHostReader) GetStorageClass(context.Context, string) (*storagev1.StorageClass, error) {
	return f.storageClass, nil
}

func storageMode(value storagev1.VolumeBindingMode) *storagev1.VolumeBindingMode { return &value }

func (f fakeHostReader) GetSecret(context.Context, string, string) (*corev1.Secret, error) {
	return f.secret, nil
}

func (f fakeHostReader) GetService(_ context.Context, _, name string) (*corev1.Service, error) {
	if f.serviceName != "" && name != f.serviceName {
		return nil, fmt.Errorf("unexpected Service name %q", name)
	}
	return f.service, nil
}

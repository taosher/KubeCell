package envtest_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeapi "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/controller"
	"github.com/kubecell/kubecell/internal/controller/helpers"
	"github.com/kubecell/kubecell/internal/platform"
)

func TestCRDLifecycleAndStatusSubresource(t *testing.T) {
	assets := os.Getenv("KUBEBUILDER_ASSETS")
	if assets == "" {
		t.Skip("KUBEBUILDER_ASSETS is not configured; run make setup-envtest first")
	}
	root := repositoryRoot(t)
	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join(root, "config", "crd", "bases")},
		BinaryAssetsDirectory: assets,
	}
	config, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer func() {
		if stopErr := testEnvironment.Stop(); stopErr != nil {
			t.Errorf("stop envtest: %v", stopErr)
		}
	}()

	clientset, err := kubernetesClient(config)
	if err != nil {
		t.Fatalf("create envtest client: %v", err)
	}
	ctx := context.Background()
	if err := clientset.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubecell-system"}}); err != nil {
		t.Fatalf("create test namespace: %v", err)
	}
	cell := newCellFixture("kubecell-system", "cell-a", "managed-a")
	if err := clientset.Create(ctx, cell); err != nil {
		t.Fatalf("create Cell: %v", err)
	}
	created := &api.Cell{}
	if err := clientset.Get(ctx, types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, created); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	created.Status.Phase = api.CellPhaseDegraded
	if err := clientset.Status().Update(ctx, created); err != nil {
		t.Fatalf("update Cell status: %v", err)
	}
	withoutStatus := &api.Cell{}
	if err := clientset.Get(ctx, client.ObjectKeyFromObject(created), withoutStatus); err != nil {
		t.Fatalf("get Cell after status update: %v", err)
	}
	if withoutStatus.Status.Phase != api.CellPhaseDegraded {
		t.Fatalf("status phase = %q, want Degraded", withoutStatus.Status.Phase)
	}

	class := newClassFixture("small")
	if err := clientset.Create(ctx, class); err != nil {
		t.Fatalf("create VirtualNodeClass: %v", err)
	}
	storedClass := &api.VirtualNodeClass{}
	if err := clientset.Get(ctx, types.NamespacedName{Name: class.Name}, storedClass); err != nil {
		t.Fatalf("get VirtualNodeClass: %v", err)
	}

	cluster := newVirtualClusterFixture("kubecell-system", "training", "kubecell-system", "cell-a", "small")
	if err := clientset.Create(ctx, cluster); err != nil {
		t.Fatalf("create VirtualCluster: %v", err)
	}
	storedCluster := &api.VirtualCluster{}
	if err := clientset.Get(ctx, client.ObjectKeyFromObject(cluster), storedCluster); err != nil {
		t.Fatalf("get VirtualCluster: %v", err)
	}
	storedCluster.Status.Phase = api.VirtualClusterPhaseDegraded
	if err := clientset.Status().Update(ctx, storedCluster); err != nil {
		t.Fatalf("update VirtualCluster status: %v", err)
	}

	plan := &api.VirtualClusterPlan{
		ObjectMeta: metav1.ObjectMeta{Name: "training-plan", Namespace: "kubecell-system"},
		Spec:       api.VirtualClusterPlanSpec{VirtualCluster: api.VirtualClusterSpec{CellRef: api.ObjectReference{Name: "cell-a", Namespace: "kubecell-system"}, ClassRef: api.ObjectReference{Name: "small"}}},
	}
	if err := clientset.Create(ctx, plan); err != nil {
		t.Fatalf("create VirtualClusterPlan: %v", err)
	}

	if err := clientset.Delete(ctx, plan); err != nil {
		t.Fatalf("delete VirtualClusterPlan: %v", err)
	}
	if err := clientset.Delete(ctx, storedCluster); err != nil {
		t.Fatalf("delete VirtualCluster: %v", err)
	}
	if err := clientset.Delete(ctx, storedClass); err != nil {
		t.Fatalf("delete VirtualNodeClass: %v", err)
	}
	if err := clientset.Delete(ctx, created); err != nil {
		t.Fatalf("delete Cell: %v", err)
	}
	if err := clientset.Get(ctx, client.ObjectKeyFromObject(created), &api.Cell{}); !apierrors.IsNotFound(err) {
		t.Fatalf("get deleted Cell error = %v, want NotFound", err)
	}
}

func TestControllerObjectsCanBeStoredWithOCMTypes(t *testing.T) {
	assets := os.Getenv("KUBEBUILDER_ASSETS")
	if assets == "" {
		t.Skip("KUBEBUILDER_ASSETS is not configured; run make setup-envtest first")
	}
	root := repositoryRoot(t)
	goModCache := os.Getenv("GOMODCACHE")
	if goModCache == "" {
		home := os.Getenv("HOME")
		if home == "" {
			t.Fatal("GOMODCACHE is not set and HOME is empty; set GOMODCACHE to the Go module cache directory")
		}
		goModCache = filepath.Join(home, "go", "pkg", "mod")
	}
	ocmCRDs := filepath.Join(goModCache, "open-cluster-management.io", "api@v1.3.0", "cluster", "v1", "0000_00_clusters.open-cluster-management.io_managedclusters.crd.yaml")
	workCRDs := filepath.Join(goModCache, "open-cluster-management.io", "api@v1.3.0", "work", "v1", "0000_00_work.open-cluster-management.io_manifestworks.crd.yaml")
	testEnvironment := &envtest.Environment{CRDDirectoryPaths: []string{filepath.Join(root, "config", "crd", "bases"), ocmCRDs, workCRDs}, BinaryAssetsDirectory: assets}
	config, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer testEnvironment.Stop()
	scheme := runtimeapi.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := clusterv1.Install(scheme); err != nil {
		t.Fatal(err)
	}
	if err := workv1.Install(scheme); err != nil {
		t.Fatal(err)
	}
	clientset, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := clientset.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubecell-system"}}); err != nil {
		t.Fatal(err)
	}
	if err := clientset.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "managed-a"}}); err != nil {
		t.Fatal(err)
	}
	managed := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "managed-a"}}
	if err := clientset.Create(ctx, managed); err != nil {
		t.Fatal(err)
	}
	cell := newCellFixture("kubecell-system", "cell-a", "managed-a")
	if err := clientset.Create(ctx, cell); err != nil {
		t.Fatal(err)
	}
	work := &workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: "foundation", Namespace: "managed-a"}}
	if err := clientset.Create(ctx, work); err != nil {
		t.Fatal(err)
	}
	stored := &workv1.ManifestWork{}
	if err := clientset.Get(ctx, types.NamespacedName{Name: "foundation", Namespace: "managed-a"}, stored); err != nil {
		t.Fatalf("get ManifestWork: %v", err)
	}
	if stored.Namespace != managed.Name {
		t.Fatalf("Work namespace = %q, want %q", stored.Namespace, managed.Name)
	}
}

func TestVirtualClusterReconcilerCreatesOrderedWorksInEnvtest(t *testing.T) {
	assets := os.Getenv("KUBEBUILDER_ASSETS")
	if assets == "" {
		t.Skip("KUBEBUILDER_ASSETS is not configured; run make setup-envtest first")
	}
	root := repositoryRoot(t)
	goModCache := os.Getenv("GOMODCACHE")
	if goModCache == "" {
		home := os.Getenv("HOME")
		if home == "" {
			t.Fatal("GOMODCACHE is not set and HOME is empty; set GOMODCACHE to the Go module cache directory")
		}
		goModCache = filepath.Join(home, "go", "pkg", "mod")
	}
	ocmCRDs := filepath.Join(goModCache, "open-cluster-management.io", "api@v1.3.0", "cluster", "v1", "0000_00_clusters.open-cluster-management.io_managedclusters.crd.yaml")
	workCRDs := filepath.Join(goModCache, "open-cluster-management.io", "api@v1.3.0", "work", "v1", "0000_00_work.open-cluster-management.io_manifestworks.crd.yaml")
	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join(root, "config", "crd", "bases"), ocmCRDs, workCRDs},
		BinaryAssetsDirectory: assets,
	}
	config, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer func() {
		if stopErr := testEnvironment.Stop(); stopErr != nil {
			t.Errorf("stop envtest: %v", stopErr)
		}
	}()

	scheme := runtimeapi.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := clusterv1.Install(scheme); err != nil {
		t.Fatal(err)
	}
	if err := workv1.Install(scheme); err != nil {
		t.Fatal(err)
	}
	clientset, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, namespace := range []string{"kubecell-system", "team-a", "managed-a"} {
		if err := clientset.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}); err != nil {
			t.Fatalf("create namespace %s: %v", namespace, err)
		}
	}
	managed := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "managed-a"}}
	if err := clientset.Create(ctx, managed); err != nil {
		t.Fatalf("create ManagedCluster: %v", err)
	}
	cell := newCellFixture("kubecell-system", "cell-a", "managed-a")
	if err := clientset.Create(ctx, cell); err != nil {
		t.Fatalf("create Cell: %v", err)
	}
	storedCell := &api.Cell{}
	if err := clientset.Get(ctx, client.ObjectKeyFromObject(cell), storedCell); err != nil {
		t.Fatalf("get Cell for status update: %v", err)
	}
	markCellReady(storedCell)
	if err := clientset.Status().Update(ctx, storedCell); err != nil {
		t.Fatalf("update Cell status: %v", err)
	}
	class := newClassFixture("small")
	if err := clientset.Create(ctx, class); err != nil {
		t.Fatalf("create VirtualNodeClass: %v", err)
	}
	cluster := newVirtualClusterFixture("team-a", "training", "kubecell-system", "cell-a", "small")
	if err := clientset.Create(ctx, cluster); err != nil {
		t.Fatalf("create VirtualCluster: %v", err)
	}
	if err := clientset.Get(ctx, client.ObjectKeyFromObject(cluster), cluster); err != nil {
		t.Fatalf("refresh VirtualCluster UID: %v", err)
	}
	hostNodes := &corev1.NodeList{Items: []corev1.Node{{
		ObjectMeta: metav1.ObjectMeta{Name: "host-node-a"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			Addresses:  []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "203.0.113.10"}},
		},
	}}}
	reconciler := &controller.VirtualClusterReconciler{Client: clientset, HostReaderFactory: fakeHostReaderFactory{nodes: hostNodes}}
	request := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cluster)}
	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	foundation := &workv1.ManifestWork{}
	if err := clientset.Get(ctx, types.NamespacedName{Name: names.FoundationWork, Namespace: "managed-a"}, foundation); err != nil {
		works := &workv1.ManifestWorkList{}
		if listErr := clientset.List(ctx, works, client.InNamespace("managed-a")); listErr == nil {
			t.Logf("VirtualCluster UID=%q expected foundation=%q actual works=%v", cluster.UID, names.FoundationWork, func() []string {
				result := make([]string, 0, len(works.Items))
				for _, item := range works.Items {
					result = append(result, item.Name)
				}
				return result
			}())
		}
		t.Fatalf("get Foundation Work: %v", err)
	}
	instance := &workv1.ManifestWork{}
	if err := clientset.Get(ctx, types.NamespacedName{Name: names.InstanceWork, Namespace: "managed-a"}, instance); err == nil {
		t.Fatal("Instance Work exists before Foundation is Applied")
	}
	foundation.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue, Reason: "Applied", LastTransitionTime: metav1.Now()}}
	if err := clientset.Status().Update(ctx, foundation); err != nil {
		t.Fatalf("update Foundation status: %v", err)
	}
	if _, err := reconciler.Reconcile(ctx, request); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if err := clientset.Get(ctx, types.NamespacedName{Name: names.InstanceWork, Namespace: "managed-a"}, instance); err != nil {
		t.Fatalf("get Instance Work: %v", err)
	}
	updated := &api.VirtualCluster{}
	if err := clientset.Get(ctx, client.ObjectKeyFromObject(cluster), updated); err != nil {
		t.Fatalf("get VirtualCluster status: %v", err)
	}
	if updated.Status.Resolved.K3kChildVersion != "v1.34.2-k3s1" {
		t.Fatalf("resolved K3k child version = %q, want v1.34.2-k3s1", updated.Status.Resolved.K3kChildVersion)
	}
	if !updated.Status.WorkRefs.Foundation.Applied {
		t.Fatalf("Foundation Work status = %#v, want Applied", updated.Status.WorkRefs.Foundation)
	}
}

func newCellFixture(namespace, name, managedCluster string) *api.Cell {
	return &api.Cell{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: api.CellSpec{
			ManagedClusterRef: api.ObjectReference{Name: managedCluster},
			MachineProfile: api.MachineProfile{
				Name:    "ascend-910b",
				Devices: []api.DeviceContract{{Name: "ascend", ResourceName: "huawei.com/Ascend910"}},
			},
		},
	}
}

func newClassFixture(name string) *api.VirtualNodeClass {
	return &api.VirtualNodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: api.VirtualNodeClassSpec{
			Entitlement: api.EntitlementSpec{WorkloadHard: corev1.ResourceList{
				"requests.cpu":                  resource.MustParse("1"),
				"limits.cpu":                    resource.MustParse("1"),
				"requests.memory":               resource.MustParse("2Gi"),
				"limits.memory":                 resource.MustParse("2Gi"),
				"requests.ephemeral-storage":    resource.MustParse("1Gi"),
				"limits.ephemeral-storage":      resource.MustParse("1Gi"),
				"requests.huawei.com/Ascend910": resource.MustParse("1"),
				"limits.huawei.com/Ascend910":   resource.MustParse("1"),
			}},
			StorageClassName: "topolvm-provisioner",
		},
	}
}

func newVirtualClusterFixture(namespace, name, cellNamespace, cellName, className string) *api.VirtualCluster {
	return &api.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: api.VirtualClusterSpec{
			CellRef:  api.ObjectReference{Name: cellName, Namespace: cellNamespace},
			ClassRef: api.ObjectReference{Name: className},
		},
	}
}

func markCellReady(cell *api.Cell) {
	free := resource.MustParse("100Gi")
	cell.Status.Phase = api.CellPhaseReady
	cell.Status.ManagedCluster = api.ManagedClusterStatus{Name: cell.Spec.ManagedClusterRef.Name, Joined: true, Available: true, LeaseFresh: true}
	cell.Status.Provider = api.ProviderStatus{
		K3kVersion:      platform.K3kVersion,
		NamespaceReady:  true,
		K3kCRDsReady:    true,
		ControllerReady: true,
		CNIReady:        true,
		TopoLVMReady:    true,
	}
	cell.Status.Conditions = []metav1.Condition{{Type: "InventoryFresh", Status: metav1.ConditionTrue, Reason: "HostInventoryObserved", LastTransitionTime: metav1.Now()}}
	cell.Status.Nodes = []api.CellNodeStatus{{
		Name:    "host-node-a",
		Profile: "ascend-910b",
		Ready:   true,
		Allocatable: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("16"),
			corev1.ResourceMemory:           resource.MustParse("64Gi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
			"huawei.com/Ascend910":          resource.MustParse("8"),
		},
		ObservedAt: metav1.Now(),
	}}
	cell.Status.Inventory = map[string]api.InventoryStatus{
		"cpu":                  {Allocatable: resource.MustParse("16"), AvailableEstimate: resource.MustParse("12")},
		"memory":               {Allocatable: resource.MustParse("64Gi"), AvailableEstimate: resource.MustParse("48Gi")},
		"ephemeral-storage":    {Allocatable: resource.MustParse("200Gi"), AvailableEstimate: resource.MustParse("160Gi")},
		"huawei.com/Ascend910": {Allocatable: resource.MustParse("8"), AvailableEstimate: resource.MustParse("8")},
	}
	cell.Status.StorageInventory = map[string]api.StorageInventoryStatus{
		"topolvm-provisioner": {
			Provisioner:       "topolvm.io",
			FreeCapacity:      &free,
			FreeCapacityKnown: true,
			Fresh:             true,
			Nodes: map[string]api.StorageNodeInventoryStatus{
				"host-node-a": {FreeCapacity: &free, FreeCapacityKnown: true, Fresh: true},
			},
			ObservedAt: metav1.Now(),
		},
	}
	cell.Status.NodePorts = api.NodePortInventoryStatus{ObservedAt: metav1.Now(), Fresh: true}
}

type fakeHostReader struct {
	nodes *corev1.NodeList
}

func (f fakeHostReader) ListNodes(context.Context) (*corev1.NodeList, error) {
	return f.nodes, nil
}

func (f fakeHostReader) ListPods(context.Context, string) (*corev1.PodList, error) {
	return &corev1.PodList{}, nil
}

func (f fakeHostReader) GetResourceQuota(context.Context, string, string) (*corev1.ResourceQuota, error) {
	return nil, nil
}

func (f fakeHostReader) GetPersistentVolumeClaim(context.Context, string, string) (*corev1.PersistentVolumeClaim, error) {
	return nil, nil
}

func (f fakeHostReader) GetPersistentVolume(context.Context, string) (*corev1.PersistentVolume, error) {
	return nil, nil
}

type fakeHostReaderFactory struct {
	nodes *corev1.NodeList
}

func (f fakeHostReaderFactory) ForCell(context.Context, *api.Cell) (controller.VirtualNodeHostReader, error) {
	return fakeHostReader{nodes: f.nodes}, nil
}

func kubernetesClient(config *rest.Config) (client.Client, error) {
	scheme := runtimeapi.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := api.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return client.New(config, client.Options{Scheme: scheme})
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

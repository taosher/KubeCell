package controller

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/platform"
	"github.com/kubecell/kubecell/internal/provider/host"
)

func testControllerScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kubecell scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add coordination scheme: %v", err)
	}
	if err := clusterv1.Install(scheme); err != nil {
		t.Fatalf("add OCM scheme: %v", err)
	}
	return scheme
}

func testCell() *api.Cell {
	return &api.Cell{
		ObjectMeta: metav1.ObjectMeta{Name: "cell-a", Namespace: "kubecell-system"},
		Spec: api.CellSpec{
			ManagedClusterRef: api.ObjectReference{Name: "cell-a"},
			MachineProfile: api.MachineProfile{
				Name:    "ascend-910b",
				Devices: []api.DeviceContract{{Name: "ascend", ResourceName: "huawei.com/Ascend910"}},
			},
		},
	}
}

func freshManagedClusterLease(clusterName string) *coordinationv1.Lease {
	renewTime := metav1.NewMicroTime(time.Now())
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: managedClusterLeaseName, Namespace: clusterName},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &renewTime},
	}
}

func readyNodeForTest(name, profile string) corev1.Node {
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{platform.ProfileLabelKey: profile}},
		Status: corev1.NodeStatus{
			Capacity:    corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("8")},
			Allocatable: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("8")},
			Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
		},
	}
}

func TestCellReconcilerPreservesFactoryErrorMessage(t *testing.T) {
	scheme := testControllerScheme(t)
	cell := testCell()
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a"}, Spec: clusterv1.ManagedClusterSpec{LeaseDurationSeconds: 60}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cell).WithObjects(cell, managedCluster, freshManagedClusterLease("cell-a")).Build()
	reconciler := &CellReconciler{
		Client:                 client,
		InventoryReaderFactory: failingInventoryFactory{err: errors.New("proxy dial refused")},
		BaselineReaderFactory:  failingBaselineFactory{err: errors.New("proxy dial refused")},
	}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ProviderReady", "InventoryFresh"} {
		condition := conditionByType(updated.Status.Conditions, want)
		if condition == nil {
			t.Fatalf("condition %s is missing", want)
		}
		if condition.Message != "proxy dial refused" {
			t.Fatalf("condition %s message = %q, want factory error (must not be overwritten by generic text)", want, condition.Message)
		}
	}
}

type failingInventoryFactory struct{ err error }

func (f failingInventoryFactory) ForCell(context.Context, *api.Cell) (InventoryReader, error) {
	return nil, f.err
}

type failingBaselineFactory struct{ err error }

func (f failingBaselineFactory) ForCell(context.Context, *api.Cell) (host.BaselineReader, error) {
	return nil, f.err
}

func conditionByType(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for index := range conditions {
		if conditions[index].Type == conditionType {
			return &conditions[index]
		}
	}
	return nil
}

func TestCellReconcilerReflectsOCMJoinedAndAvailable(t *testing.T) {
	managedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cell-a", UID: "managed-uid"},
		Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{
			{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue},
			{Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue},
		}},
	}
	cell := testCell()
	nodes := &corev1.NodeList{Items: []corev1.Node{readyNodeForTest("host-01", "ascend-910b")}}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster, freshManagedClusterLease(managedCluster.Name)).Build()
	reconciler := &CellReconciler{Client: client, InventoryReader: fakeInventoryReader{nodes: nodes}, BaselineReader: readyBaselineReader{}}

	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	if !updated.Status.ManagedCluster.Joined || !updated.Status.ManagedCluster.Available {
		t.Fatalf("managed cluster status = %#v", updated.Status.ManagedCluster)
	}
	if updated.Status.Phase != api.CellPhaseReady {
		t.Fatalf("phase = %q, want Ready", updated.Status.Phase)
	}
}

func TestCellReconcilerReportsLockedK3kVersion(t *testing.T) {
	managedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cell-a", UID: "managed-uid"},
		Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{
			{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue},
			{Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue},
		}},
	}
	cell := testCell()
	nodes := &corev1.NodeList{Items: []corev1.Node{readyNodeForTest("host-01", "ascend-910b")}}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster, freshManagedClusterLease(managedCluster.Name)).Build()
	reconciler := &CellReconciler{Client: client, InventoryReader: fakeInventoryReader{nodes: nodes}, BaselineReader: readyBaselineReader{}}

	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	if updated.Status.Provider.K3kVersion != platform.K3kVersion {
		t.Fatalf("provider k3kVersion = %q, want platform constant %q", updated.Status.Provider.K3kVersion, platform.K3kVersion)
	}
}

func TestCellReconcilerReportsUnavailableManagedCluster(t *testing.T) {
	cell := testCell()
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a"}, Spec: clusterv1.ManagedClusterSpec{LeaseDurationSeconds: 60}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster).Build()
	if _, err := (&CellReconciler{Client: client, BaselineReader: readyBaselineReader{}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	if updated.Status.Phase != api.CellPhaseDegraded {
		t.Fatalf("phase = %q, want Degraded", updated.Status.Phase)
	}
	if updated.Status.ManagedCluster.Available || updated.Status.ManagedCluster.LeaseFresh {
		t.Fatalf("managed cluster status = %#v, want unavailable with stale lease", updated.Status.ManagedCluster)
	}
	condition := cellCondition(updated, "Available")
	if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "ManagedClusterLeaseStale" {
		t.Fatalf("Available condition = %#v, want false with ManagedClusterLeaseStale", condition)
	}
}

func TestCellReconcilerRejectsStaleManagedClusterLease(t *testing.T) {
	cell := testCell()
	managedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cell-a"},
		Spec:       clusterv1.ManagedClusterSpec{LeaseDurationSeconds: 60},
		Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{
			{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue},
			{Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue},
		}},
	}
	staleRenewTime := metav1.NewMicroTime(time.Now().Add(-2 * time.Minute))
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: "managed-cluster-lease", Namespace: "cell-a"},
		Spec:       coordinationv1.LeaseSpec{RenewTime: &staleRenewTime},
	}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster, lease).Build()
	reconciler := &CellReconciler{Client: client, BaselineReader: readyBaselineReader{}}

	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	if updated.Status.Phase != api.CellPhaseDegraded {
		t.Fatalf("phase = %q, want Degraded for stale OCM lease", updated.Status.Phase)
	}
	if updated.Status.ManagedCluster.Available {
		t.Fatalf("managed cluster status = %#v, want unavailable and stale lease", updated.Status.ManagedCluster)
	}
	condition := cellCondition(updated, "Available")
	if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "ManagedClusterLeaseStale" {
		t.Fatalf("Available condition = %#v, want false with ManagedClusterLeaseStale", condition)
	}
}

func TestCellReconcilerRequiresHostBaselineReadiness(t *testing.T) {
	cell := testCell()
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a"}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster, freshManagedClusterLease(managedCluster.Name)).Build()
	reconciler := &CellReconciler{Client: client, BaselineReader: fakeBaselineReader{observation: host.BaselineObservation{NamespaceReady: false, K3kCRDsReady: true, ControllerReady: true, CNIReady: true, TopoLVMReady: true}}}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	if updated.Status.Phase == api.CellPhaseReady {
		t.Fatalf("phase = Ready, want degraded when Host baseline is not ready")
	}
	if updated.Status.Provider.NamespaceReady {
		t.Fatalf("provider namespaceReady = true, want false")
	}
	if !hasCellCondition(updated, "ProviderReady", metav1.ConditionFalse) {
		t.Fatalf("conditions = %#v, want ProviderReady=False", updated.Status.Conditions)
	}
}

func TestCellReconcilerPreservesHostBaselineReadFailure(t *testing.T) {
	cell := testCell()
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a"}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster, freshManagedClusterLease(managedCluster.Name)).Build()
	reconciler := &CellReconciler{Client: client, BaselineReader: failingBaselineReader{err: errors.New("proxy unavailable")}}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	condition := cellCondition(updated, "ProviderReady")
	if condition == nil || condition.Reason != "HostBaselineReadFailed" || condition.Message != "proxy unavailable" {
		t.Fatalf("ProviderReady = %#v, want preserved HostBaselineReadFailed condition", condition)
	}
}

func TestCellReconcilerProjectsProfileNodeInventory(t *testing.T) {
	cell := testCell()
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a", UID: "managed-uid"}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	nodes := &corev1.NodeList{Items: []corev1.Node{readyNodeForTest("host-01", "ascend-910b")}}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster, freshManagedClusterLease(managedCluster.Name)).Build()
	reconciler := &CellReconciler{Client: client, InventoryReader: fakeInventoryReader{nodes: nodes}, BaselineReader: readyBaselineReader{}}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	if len(updated.Status.Nodes) != 1 || !updated.Status.Nodes[0].Ready || updated.Status.Nodes[0].Name != "host-01" {
		t.Fatalf("nodes = %#v", updated.Status.Nodes)
	}
	if updated.Status.Nodes[0].Profile != "ascend-910b" {
		t.Fatalf("node profile = %q, want single MachineProfile name", updated.Status.Nodes[0].Profile)
	}
	inventory, found := updated.Status.Inventory["huawei.com/Ascend910"]
	if !found || inventory.Allocatable.Value() != 8 {
		t.Fatalf("inventory = %#v", updated.Status.Inventory)
	}
}

func TestCellReconcilerProjectsActiveAndPendingDeviceRequests(t *testing.T) {
	cell := testCell()
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a"}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	nodes := &corev1.NodeList{Items: []corev1.Node{readyNodeForTest("host-01", "ascend-910b")}}
	pods := &corev1.PodList{Items: []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "running", Namespace: "tenant-a", Labels: map[string]string{"k3k.io/clusterName": "kc-vc-a"}}, Spec: corev1.PodSpec{NodeName: "host-01", Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("2")}}}}}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "tenant-a", Labels: map[string]string{"k3k.io/clusterName": "kc-vc-a"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("1")}}}}}, Status: corev1.PodStatus{Phase: corev1.PodPending}},
		{ObjectMeta: metav1.ObjectMeta{Name: "foreign", Namespace: "tenant-b", Labels: map[string]string{"k3k.io/clusterName": "kc-foreign"}}, Spec: corev1.PodSpec{NodeName: "host-01", Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("4")}}}}}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	}}
	vc := &api.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "training", Namespace: "team-a"},
		Spec:       api.VirtualClusterSpec{CellRef: api.ObjectReference{Name: "cell-a", Namespace: "kubecell-system"}},
		Status:     api.VirtualClusterStatus{Resolved: api.ResolvedSnapshot{K3kClusterName: "kc-vc-a"}},
	}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell, vc).WithObjects(cell, vc, managedCluster, freshManagedClusterLease(managedCluster.Name)).Build()
	reconciler := &CellReconciler{Client: client, InventoryReader: fakeInventoryReaderWithPods{nodes: nodes, pods: pods}, BaselineReader: readyBaselineReader{}}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	node := updated.Status.Nodes[0]
	if got := node.ActiveRequested["huawei.com/Ascend910"]; !got.Equal(resource.MustParse("2")) {
		t.Fatalf("active node request = %s, want 2", got.String())
	}
	if len(node.PendingRequested) != 0 {
		t.Fatalf("node pending request = %#v, want no node attribution for unbound Pod", node.PendingRequested)
	}
	inventory := updated.Status.Inventory["huawei.com/Ascend910"]
	if !inventory.Requested.Equal(resource.MustParse("2")) || !inventory.Pending.Equal(resource.MustParse("1")) {
		t.Fatalf("inventory requests = %s pending = %s, want 2 and 1", inventory.Requested.String(), inventory.Pending.String())
	}
}

func TestCellReconcilerProjectsComputeAndStorageInventory(t *testing.T) {
	cell := testCell()
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a"}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	nodes := &corev1.NodeList{Items: []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "host-01", Labels: map[string]string{platform.ProfileLabelKey: "ascend-910b"}}, Status: corev1.NodeStatus{Capacity: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("16"), corev1.ResourceMemory: resource.MustParse("64Gi"), corev1.ResourceEphemeralStorage: resource.MustParse("500Gi"), "huawei.com/Ascend910": resource.MustParse("8")}, Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("15"), corev1.ResourceMemory: resource.MustParse("60Gi"), corev1.ResourceEphemeralStorage: resource.MustParse("450Gi"), "huawei.com/Ascend910": resource.MustParse("8")}, Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}}}
	pods := &corev1.PodList{Items: []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "running", Labels: map[string]string{"k3k.io/clusterName": "kc-vc-a"}}, Spec: corev1.PodSpec{NodeName: "host-01", Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("4Gi")}}}}}, Status: corev1.PodStatus{Phase: corev1.PodRunning}}}}
	storageClass := storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "topolvm-provisioner"}, Provisioner: "topolvm.io"}
	pv := corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-a"}, Spec: corev1.PersistentVolumeSpec{Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("20Gi")}, StorageClassName: storageClass.Name, ClaimRef: &corev1.ObjectReference{Name: "claim", Namespace: "tenant-a"}, NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "kubernetes.io/hostname", Operator: corev1.NodeSelectorOpIn, Values: []string{"host-01"}}}}}}}}, Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound}}
	reader := fakeInventoryReaderWithStorage{nodes: nodes, pods: pods, pvs: &corev1.PersistentVolumeList{Items: []corev1.PersistentVolume{pv}}, storageClasses: &storagev1.StorageClassList{Items: []storagev1.StorageClass{storageClass}}}
	vc := &api.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "training", Namespace: "team-a"},
		Spec:       api.VirtualClusterSpec{CellRef: api.ObjectReference{Name: "cell-a", Namespace: "kubecell-system"}},
		Status:     api.VirtualClusterStatus{Resolved: api.ResolvedSnapshot{K3kClusterName: "kc-vc-a"}},
	}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell, vc).WithObjects(cell, vc, managedCluster, freshManagedClusterLease(managedCluster.Name)).Build()
	if _, err := (&CellReconciler{Client: client, InventoryReader: reader, BaselineReader: readyBaselineReader{}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	node := updated.Status.Nodes[0]
	if !node.Capacity[corev1.ResourceCPU].Equal(resource.MustParse("16")) || !node.Allocatable[corev1.ResourceMemory].Equal(resource.MustParse("60Gi")) {
		t.Fatalf("node compute inventory = %#v", node)
	}
	if !updated.Status.Inventory[string(corev1.ResourceCPU)].Requested.Equal(resource.MustParse("2")) {
		t.Fatalf("cpu inventory = %#v", updated.Status.Inventory[string(corev1.ResourceCPU)])
	}
	storage, found := updated.Status.StorageInventory[storageClass.Name]
	if !found || !storage.AllocatedPV.Equal(resource.MustParse("20Gi")) || !storage.Nodes["host-01"].AllocatedPV.Equal(resource.MustParse("20Gi")) || storage.FreeCapacity != nil || storage.FreeCapacityKnown || storage.Fresh {
		t.Fatalf("storage inventory = %#v, want allocated PV and unknown physical free capacity", storage)
	}
	if hasCellCondition(updated, "InventoryFresh", metav1.ConditionTrue) {
		t.Fatalf("InventoryFresh = true with unknown physical capacity: %#v", updated.Status.Conditions)
	}
}

func TestCellReconcilerMarksInventoryDegradedWhenStorageSourceFails(t *testing.T) {
	cell := testCell()
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a"}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	nodes := &corev1.NodeList{Items: []corev1.Node{readyNodeForTest("host-01", "ascend-910b")}}
	reader := failingStorageInventoryReader{nodes: nodes, err: errors.New("proxy storage read failed")}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster, freshManagedClusterLease(managedCluster.Name)).Build()
	if _, err := (&CellReconciler{Client: client, InventoryReader: reader, BaselineReader: readyBaselineReader{}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	condition := cellCondition(updated, "InventoryFresh")
	if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "PersistentVolumeReadFailed" {
		t.Fatalf("InventoryFresh = %#v, want PersistentVolumeReadFailed", condition)
	}
	if updated.Status.Phase != api.CellPhaseDegraded {
		t.Fatalf("phase = %q, want Degraded", updated.Status.Phase)
	}
}

func TestCellReconcilerProjectsTopoLVMNodeAnnotationCapacity(t *testing.T) {
	cell := testCell()
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a"}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	nodes := &corev1.NodeList{Items: []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "host-01", Labels: map[string]string{platform.ProfileLabelKey: "ascend-910b"}, Annotations: map[string]string{"capacity.topolvm.io/00default": "100Gi"}}, Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("8")}, Allocatable: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("8")}, Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}}}
	storageClass := storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "topolvm-provisioner"}, Provisioner: "topolvm.io"}
	reader := fakeInventoryReaderWithStorage{nodes: nodes, pods: &corev1.PodList{}, pvs: &corev1.PersistentVolumeList{}, pvcs: &corev1.PersistentVolumeClaimList{}, storageClasses: &storagev1.StorageClassList{Items: []storagev1.StorageClass{storageClass}}}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster, freshManagedClusterLease(managedCluster.Name)).Build()
	if _, err := (&CellReconciler{Client: client, InventoryReader: reader, BaselineReader: readyBaselineReader{}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	storage := updated.Status.StorageInventory[storageClass.Name]
	if storage.FreeCapacity == nil || !storage.FreeCapacity.Equal(resource.MustParse("100Gi")) || !storage.FreeCapacityKnown || !storage.Fresh || storage.FreeCapacitySource != "TopoLVMNodeAnnotation:00default" {
		t.Fatalf("storage inventory = %#v, want TopoLVM annotation capacity", storage)
	}
	if !hasCellCondition(updated, "InventoryFresh", metav1.ConditionTrue) {
		t.Fatalf("InventoryFresh = %#v, want true", updated.Status.Conditions)
	}
}

func TestCellReconcilerUsesStorageClassTopoLVMDeviceClass(t *testing.T) {
	cell := testCell()
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a"}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	nodes := &corev1.NodeList{Items: []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "host-01", Labels: map[string]string{platform.ProfileLabelKey: "ascend-910b"}, Annotations: map[string]string{"capacity.topolvm.io/00default": "100Gi", "capacity.topolvm.io/fast": "20Gi"}}, Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("8")}, Allocatable: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("8")}, Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}}}
	storageClass := storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "topolvm-fast"}, Provisioner: "topolvm.io", Parameters: map[string]string{"topolvm.io/device-class": "fast"}}
	reader := fakeInventoryReaderWithStorage{nodes: nodes, pods: &corev1.PodList{}, pvs: &corev1.PersistentVolumeList{}, pvcs: &corev1.PersistentVolumeClaimList{}, storageClasses: &storagev1.StorageClassList{Items: []storagev1.StorageClass{storageClass}}}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster, freshManagedClusterLease(managedCluster.Name)).Build()
	if _, err := (&CellReconciler{Client: client, InventoryReader: reader, BaselineReader: readyBaselineReader{}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	storage := updated.Status.StorageInventory[storageClass.Name]
	if storage.FreeCapacity == nil || !storage.FreeCapacity.Equal(resource.MustParse("20Gi")) || storage.FreeCapacitySource != "TopoLVMNodeAnnotation:fast" {
		t.Fatalf("storage inventory = %#v, want fast device-class capacity", storage)
	}
}

func TestCellReconcilerDegradesWhenOneEligibleNodeLacksStorageCapacity(t *testing.T) {
	cell := testCell()
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a"}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	nodes := &corev1.NodeList{Items: []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{platform.ProfileLabelKey: "ascend-910b"}, Annotations: map[string]string{"capacity.topolvm.io/00default": "100Gi"}}, Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("8")}, Allocatable: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("8")}, Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-b", Labels: map[string]string{platform.ProfileLabelKey: "ascend-910b"}}, Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("8")}, Allocatable: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("8")}, Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
	}}
	storageClass := storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "topolvm-provisioner"}, Provisioner: "topolvm.io"}
	reader := fakeInventoryReaderWithStorage{nodes: nodes, pods: &corev1.PodList{}, pvs: &corev1.PersistentVolumeList{}, pvcs: &corev1.PersistentVolumeClaimList{}, storageClasses: &storagev1.StorageClassList{Items: []storagev1.StorageClass{storageClass}}}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster, freshManagedClusterLease(managedCluster.Name)).Build()
	if _, err := (&CellReconciler{Client: client, InventoryReader: reader, BaselineReader: readyBaselineReader{}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	condition := cellCondition(updated, "InventoryFresh")
	if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "TopoLVMPhysicalCapacityUnknown" {
		t.Fatalf("InventoryFresh = %#v, want TopoLVMPhysicalCapacityUnknown", condition)
	}
}

func TestCellReconcilerDoesNotInferDefaultStorageCapacityFromNonDefaultAnnotation(t *testing.T) {
	cell := testCell()
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a"}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	nodes := &corev1.NodeList{Items: []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "host-01", Labels: map[string]string{platform.ProfileLabelKey: "ascend-910b"}, Annotations: map[string]string{"capacity.topolvm.io/fast": "20Gi"}}, Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("8")}, Allocatable: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("8")}, Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}}}
	storageClass := storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "topolvm-provisioner"}, Provisioner: "topolvm.io"}
	reader := fakeInventoryReaderWithStorage{nodes: nodes, pods: &corev1.PodList{}, pvs: &corev1.PersistentVolumeList{}, pvcs: &corev1.PersistentVolumeClaimList{}, storageClasses: &storagev1.StorageClassList{Items: []storagev1.StorageClass{storageClass}}}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster, freshManagedClusterLease(managedCluster.Name)).Build()
	if _, err := (&CellReconciler{Client: client, InventoryReader: reader, BaselineReader: readyBaselineReader{}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	storage := updated.Status.StorageInventory[storageClass.Name]
	if storage.FreeCapacity != nil || storage.FreeCapacityKnown || storage.Fresh {
		t.Fatalf("storage inventory = %#v, want unknown default-class capacity", storage)
	}
}

func TestCellReconcilerExcludesUnreadyNodesFromAggregateInventory(t *testing.T) {
	cell := testCell()
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a"}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	nodes := &corev1.NodeList{Items: []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "ready", Labels: map[string]string{platform.ProfileLabelKey: "ascend-910b"}}, Status: corev1.NodeStatus{Capacity: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("16"), "huawei.com/Ascend910": resource.MustParse("8")}, Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("15"), "huawei.com/Ascend910": resource.MustParse("8")}, Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "not-ready", Labels: map[string]string{platform.ProfileLabelKey: "ascend-910b"}}, Status: corev1.NodeStatus{Capacity: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("16"), "huawei.com/Ascend910": resource.MustParse("8")}, Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("15"), "huawei.com/Ascend910": resource.MustParse("8")}, Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}}},
	}}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster, freshManagedClusterLease(managedCluster.Name)).Build()
	if _, err := (&CellReconciler{Client: client, InventoryReader: fakeInventoryReader{nodes: nodes}, BaselineReader: readyBaselineReader{}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	cpuInventory := updated.Status.Inventory[string(corev1.ResourceCPU)]
	if !cpuInventory.Allocatable.Equal(resource.MustParse("15")) {
		t.Fatalf("cpu aggregate allocatable = %s, want ready node only", cpuInventory.Allocatable.String())
	}
}

func TestCellReconcilerIgnoresNodesWithOtherProfileLabel(t *testing.T) {
	cell := testCell()
	nodes := &corev1.NodeList{Items: []corev1.Node{
		readyNodeForTest("node-a", "ascend-910b"),
		readyNodeForTest("node-b", "other-profile"),
	}}
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a"}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster, freshManagedClusterLease(managedCluster.Name)).Build()
	if _, err := (&CellReconciler{Client: client, InventoryReader: fakeInventoryReader{nodes: nodes}, BaselineReader: readyBaselineReader{}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	inventory := updated.Status.Inventory["huawei.com/Ascend910"]
	if !inventory.Allocatable.Equal(resource.MustParse("8")) {
		t.Fatalf("inventory allocatable = %s, want single-profile node only", inventory.Allocatable.String())
	}
	for _, node := range updated.Status.Nodes {
		if node.Name == "node-b" && len(node.ReadinessReasons) == 0 {
			t.Fatalf("unclassified node = %#v, want Unclassified reason", node)
		}
	}
}

func TestCellReconcilerProjectsNodePortInventory(t *testing.T) {
	cell := testCell()
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a"}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	nodes := &corev1.NodeList{Items: []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "host-01", Labels: map[string]string{platform.ProfileLabelKey: "ascend-910b"}, Annotations: map[string]string{"capacity.topolvm.io/00default": "100Gi"}}, Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("8")}, Allocatable: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("8")}, Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}}}
	storageClass := storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "topolvm-provisioner"}, Provisioner: "topolvm.io"}
	services := &corev1.ServiceList{Items: []corev1.Service{
		{ObjectMeta: metav1.ObjectMeta{Name: "svc-b", Namespace: "default"}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{NodePort: 30443}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "svc-a", Namespace: "default"}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{NodePort: 30080}}}},
	}}
	reader := fakeInventoryReaderWithServices{nodes: nodes, pods: &corev1.PodList{}, pvs: &corev1.PersistentVolumeList{}, pvcs: &corev1.PersistentVolumeClaimList{}, storageClasses: &storagev1.StorageClassList{Items: []storagev1.StorageClass{storageClass}}, services: services}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster, freshManagedClusterLease(managedCluster.Name)).Build()
	if _, err := (&CellReconciler{Client: client, InventoryReader: reader, BaselineReader: readyBaselineReader{}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	if !updated.Status.NodePorts.Fresh {
		t.Fatalf("nodePorts = %#v, want fresh inventory", updated.Status.NodePorts)
	}
	if len(updated.Status.NodePorts.Allocated) != 2 || updated.Status.NodePorts.Allocated[0] != 30080 || updated.Status.NodePorts.Allocated[1] != 30443 {
		t.Fatalf("nodePorts allocated = %v, want sorted [30080 30443]", updated.Status.NodePorts.Allocated)
	}
}

func TestProjectNodePortsSortsAllocated(t *testing.T) {
	services := &corev1.ServiceList{Items: []corev1.Service{
		{Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{NodePort: 30443}, {NodePort: 30080}}}},
		{Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}}},
	}}
	result := projectNodePorts(services)
	if !result.Fresh || len(result.Allocated) != 2 || result.Allocated[0] != 30080 || result.Allocated[1] != 30443 {
		t.Fatalf("projectNodePorts = %#v, want sorted allocated ports", result)
	}
	if empty := projectNodePorts(nil); empty.Fresh {
		t.Fatalf("projectNodePorts(nil) = %#v, want not fresh", empty)
	}
}

func TestCellReconcilerAddsFinalizerAndBlocksDeletionWhileVirtualClusterExists(t *testing.T) {
	cell := testCell()
	cell.UID = "cell-uid"
	deletionTime := metav1.Now()
	cell.DeletionTimestamp = &deletionTime
	cell.Finalizers = []string{"kubecell.io/cell-finalizer"}
	cluster := &api.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "training", Namespace: "team-a", UID: "cluster-uid"},
		Spec:       api.VirtualClusterSpec{CellRef: api.ObjectReference{Name: cell.Name, Namespace: cell.Namespace}},
	}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, cluster).Build()

	if _, err := (&CellReconciler{Client: client, BaselineReader: readyBaselineReader{}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	if !containsString(updated.Finalizers, cellFinalizer) {
		t.Fatalf("finalizers = %v, want %q", updated.Finalizers, cellFinalizer)
	}
	if !hasCellCondition(updated, conditionCleanupBlocked, metav1.ConditionTrue) {
		t.Fatalf("conditions = %#v, want CleanupBlocked=True", updated.Status.Conditions)
	}
}

func TestCellReconcilerRemovesFinalizerAfterVirtualClustersDrain(t *testing.T) {
	cell := testCell()
	cell.UID = "cell-uid"
	deletionTime := metav1.Now()
	cell.DeletionTimestamp = &deletionTime
	cell.Finalizers = []string{cellFinalizer}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell).Build()

	if _, err := (&CellReconciler{Client: client, BaselineReader: readyBaselineReader{}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, &api.Cell{}); !apierrors.IsNotFound(err) {
		t.Fatalf("get deleted Cell error = %v, want NotFound after finalizer removal", err)
	}
}

func TestCellReconcilerSortsAndReportsUnclassifiedNodes(t *testing.T) {
	cell := testCell()
	cell.Spec.MachineProfile = api.MachineProfile{Name: "ascend-910b"}
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a"}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	nodes := &corev1.NodeList{Items: []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "zeta", Labels: map[string]string{}}, Status: corev1.NodeStatus{}},
		{ObjectMeta: metav1.ObjectMeta{Name: "alpha", Labels: map[string]string{platform.ProfileLabelKey: "ascend-910b"}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
	}}
	client := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell, managedCluster).Build()
	if _, err := (&CellReconciler{Client: client, InventoryReader: fakeInventoryReader{nodes: nodes}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.Cell{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}, updated); err != nil {
		t.Fatalf("get Cell: %v", err)
	}
	if names := cellNodeNames(updated.Status.Nodes); !sort.StringsAreSorted(names) {
		t.Fatalf("node names = %v, want sorted order", names)
	}
	if len(updated.Status.Nodes) != 2 || updated.Status.Nodes[1].ReadinessReasons[0] != "Unclassified" {
		t.Fatalf("nodes = %#v, want unclassified node status", updated.Status.Nodes)
	}
}

func hasCellCondition(cell *api.Cell, conditionType string, status metav1.ConditionStatus) bool {
	for _, condition := range cell.Status.Conditions {
		if condition.Type == conditionType && condition.Status == status {
			return true
		}
	}
	return false
}

func cellCondition(cell *api.Cell, conditionType string) *metav1.Condition {
	for index := range cell.Status.Conditions {
		if cell.Status.Conditions[index].Type == conditionType {
			return &cell.Status.Conditions[index]
		}
	}
	return nil
}

func cellNodeNames(nodes []api.CellNodeStatus) []string {
	result := make([]string, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, node.Name)
	}
	return result
}

type fakeInventoryReader struct{ nodes *corev1.NodeList }

func (f fakeInventoryReader) ListNodes(context.Context) (*corev1.NodeList, error) {
	return f.nodes, nil
}

func (fakeInventoryReader) ListPersistentVolumes(context.Context) (*corev1.PersistentVolumeList, error) {
	return &corev1.PersistentVolumeList{}, nil
}

func (fakeInventoryReader) ListPersistentVolumeClaims(context.Context) (*corev1.PersistentVolumeClaimList, error) {
	return &corev1.PersistentVolumeClaimList{}, nil
}

func (fakeInventoryReader) ListStorageClasses(context.Context) (*storagev1.StorageClassList, error) {
	return &storagev1.StorageClassList{}, nil
}

type fakeInventoryReaderWithPods struct {
	nodes *corev1.NodeList
	pods  *corev1.PodList
}

type fakeInventoryReaderWithStorage struct {
	nodes          *corev1.NodeList
	pods           *corev1.PodList
	pvs            *corev1.PersistentVolumeList
	pvcs           *corev1.PersistentVolumeClaimList
	storageClasses *storagev1.StorageClassList
}

type fakeInventoryReaderWithServices struct {
	nodes          *corev1.NodeList
	pods           *corev1.PodList
	pvs            *corev1.PersistentVolumeList
	pvcs           *corev1.PersistentVolumeClaimList
	storageClasses *storagev1.StorageClassList
	services       *corev1.ServiceList
}

func (f fakeInventoryReaderWithServices) ListNodes(context.Context) (*corev1.NodeList, error) {
	return f.nodes, nil
}

func (f fakeInventoryReaderWithServices) ListAllPods(context.Context) (*corev1.PodList, error) {
	return f.pods, nil
}

func (f fakeInventoryReaderWithServices) ListPersistentVolumes(context.Context) (*corev1.PersistentVolumeList, error) {
	return f.pvs, nil
}

func (f fakeInventoryReaderWithServices) ListPersistentVolumeClaims(context.Context) (*corev1.PersistentVolumeClaimList, error) {
	return f.pvcs, nil
}

func (f fakeInventoryReaderWithServices) ListStorageClasses(context.Context) (*storagev1.StorageClassList, error) {
	return f.storageClasses, nil
}

func (f fakeInventoryReaderWithServices) ListServices(context.Context) (*corev1.ServiceList, error) {
	return f.services, nil
}

type failingStorageInventoryReader struct {
	nodes *corev1.NodeList
	err   error
}

func (f failingStorageInventoryReader) ListNodes(context.Context) (*corev1.NodeList, error) {
	return f.nodes, nil
}

func (f failingStorageInventoryReader) ListAllPods(context.Context) (*corev1.PodList, error) {
	return &corev1.PodList{}, nil
}

func (f failingStorageInventoryReader) ListPersistentVolumes(context.Context) (*corev1.PersistentVolumeList, error) {
	return nil, f.err
}

func (f failingStorageInventoryReader) ListPersistentVolumeClaims(context.Context) (*corev1.PersistentVolumeClaimList, error) {
	return nil, f.err
}

func (f failingStorageInventoryReader) ListStorageClasses(context.Context) (*storagev1.StorageClassList, error) {
	return nil, f.err
}

func (f fakeInventoryReaderWithStorage) ListNodes(context.Context) (*corev1.NodeList, error) {
	return f.nodes, nil
}

func (f fakeInventoryReaderWithStorage) ListAllPods(context.Context) (*corev1.PodList, error) {
	return f.pods, nil
}

func (f fakeInventoryReaderWithStorage) ListPersistentVolumes(context.Context) (*corev1.PersistentVolumeList, error) {
	return f.pvs, nil
}

func (f fakeInventoryReaderWithStorage) ListPersistentVolumeClaims(context.Context) (*corev1.PersistentVolumeClaimList, error) {
	return f.pvcs, nil
}

func (f fakeInventoryReaderWithStorage) ListStorageClasses(context.Context) (*storagev1.StorageClassList, error) {
	return f.storageClasses, nil
}

func (f fakeInventoryReaderWithPods) ListNodes(context.Context) (*corev1.NodeList, error) {
	return f.nodes, nil
}

func (f fakeInventoryReaderWithPods) ListAllPods(context.Context) (*corev1.PodList, error) {
	return f.pods, nil
}

func (fakeInventoryReaderWithPods) ListPersistentVolumes(context.Context) (*corev1.PersistentVolumeList, error) {
	return &corev1.PersistentVolumeList{}, nil
}

func (fakeInventoryReaderWithPods) ListPersistentVolumeClaims(context.Context) (*corev1.PersistentVolumeClaimList, error) {
	return &corev1.PersistentVolumeClaimList{}, nil
}

func (fakeInventoryReaderWithPods) ListStorageClasses(context.Context) (*storagev1.StorageClassList, error) {
	return &storagev1.StorageClassList{}, nil
}

type fakeBaselineReader struct{ observation host.BaselineObservation }

func (f fakeBaselineReader) ObserveBaseline(context.Context, *api.Cell) (host.BaselineObservation, error) {
	return f.observation, nil
}

type failingBaselineReader struct{ err error }

func (f failingBaselineReader) ObserveBaseline(context.Context, *api.Cell) (host.BaselineObservation, error) {
	return host.BaselineObservation{}, f.err
}

type readyBaselineReader struct{}

func (readyBaselineReader) ObserveBaseline(context.Context, *api.Cell) (host.BaselineObservation, error) {
	return host.BaselineObservation{NamespaceReady: true, K3kCRDsReady: true, ControllerReady: true, CNIReady: true, TopoLVMReady: true}, nil
}

func TestCellReconcilerRequeuesWhenManagedClusterMissing(t *testing.T) {
	scheme := testControllerScheme(t)
	cell := testCell()
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cell).WithObjects(cell).Build()
	result, err := (&CellReconciler{Client: client}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cell.Name, Namespace: cell.Namespace}})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("RequeueAfter = %v, want positive (KI-5: must retry until ManagedCluster registers)", result.RequeueAfter)
	}
}

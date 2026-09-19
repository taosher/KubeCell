package controller

import (
	"context"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/controller/helpers"
	"github.com/kubecell/kubecell/internal/platform"
)

func virtualClusterTestObjects(t *testing.T) (*api.VirtualCluster, *api.Cell, *api.VirtualNodeClass, *clusterv1.ManagedCluster, *runtime.Scheme) {
	t.Helper()
	cluster := &api.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "training", Namespace: "team-a", UID: "cluster-uid"},
		Spec:       api.VirtualClusterSpec{CellRef: api.ObjectReference{Name: "cell-a", Namespace: "kubecell-system"}, ClassRef: api.ObjectReference{Name: "small"}},
	}
	cell := &api.Cell{
		ObjectMeta: metav1.ObjectMeta{Name: "cell-a", Namespace: "kubecell-system", UID: "cell-uid"},
		Spec: api.CellSpec{
			ManagedClusterRef: api.ObjectReference{Name: "cell-a"},
			MachineProfile:    api.MachineProfile{Name: "ascend-910b", Devices: []api.DeviceContract{{Name: "ascend", ResourceName: "huawei.com/Ascend910"}}},
		},
		Status: api.CellStatus{
			Phase:          api.CellPhaseReady,
			ManagedCluster: api.ManagedClusterStatus{Name: "cell-a", UID: "managed-uid", Joined: true, Available: true, LeaseFresh: true},
			Provider:       api.ProviderStatus{K3kVersion: platform.K3kVersion, NamespaceReady: true, K3kCRDsReady: true, ControllerReady: true, CNIReady: true, TopoLVMReady: true},
			Nodes: []api.CellNodeStatus{{
				Name: "node-a", Profile: "ascend-910b", Ready: true,
				Allocatable: api.ResourceList{"huawei.com/Ascend910": resource.MustParse("1")},
			}},
			Inventory:        inventoryStatusMapForTest(),
			StorageInventory: storageInventoryMapForTest(),
			NodePorts:        api.NodePortInventoryStatus{Fresh: true},
			Conditions:       []metav1.Condition{{Type: "InventoryFresh", Status: metav1.ConditionTrue}},
		},
	}
	class := &api.VirtualNodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "small", UID: "class-uid"},
		Spec: api.VirtualNodeClassSpec{
			Entitlement:      api.EntitlementSpec{WorkloadHard: mapResourceList()},
			StorageClassName: "topolvm-provisioner",
		},
	}
	managedCluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cell-a", UID: "managed-uid"}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionJoined, Status: metav1.ConditionTrue}, {Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
	scheme := testControllerScheme(t)
	if err := workv1.Install(scheme); err != nil {
		t.Fatalf("add Work scheme: %v", err)
	}
	return cluster, cell, class, managedCluster, scheme
}

func mapResourceList() corev1.ResourceList {
	return corev1.ResourceList{"requests.cpu": resource.MustParse("1"), "limits.cpu": resource.MustParse("1")}
}

func inventoryStatusMapForTest() map[string]api.InventoryStatus {
	return map[string]api.InventoryStatus{
		"cpu":                  {Allocatable: resource.MustParse("16"), AvailableEstimate: resource.MustParse("12")},
		"memory":               {Allocatable: resource.MustParse("32Gi"), AvailableEstimate: resource.MustParse("24Gi")},
		"ephemeral-storage":    {Allocatable: resource.MustParse("100Gi"), AvailableEstimate: resource.MustParse("80Gi")},
		"huawei.com/Ascend910": {Allocatable: resource.MustParse("1"), AvailableEstimate: resource.MustParse("1")},
	}
}

func storageInventoryMapForTest() map[string]api.StorageInventoryStatus {
	free := resource.MustParse("100Gi")
	return map[string]api.StorageInventoryStatus{"topolvm-provisioner": {Provisioner: "topolvm.io", FreeCapacity: &free, FreeCapacityKnown: true, Fresh: true, Nodes: map[string]api.StorageNodeInventoryStatus{"node-a": {FreeCapacity: &free, FreeCapacityKnown: true, Fresh: true}}}}
}

func readyHostNodeList() *corev1.NodeList {
	return &corev1.NodeList{Items: []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "host-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}, Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "203.0.113.10"}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "host-b"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}, Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "203.0.113.11"}}}},
	}}
}

func TestDiscoverHostAPIAddressesSortsReadyNodesByName(t *testing.T) {
	nodes := &corev1.NodeList{Items: []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "host-b"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}, Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "203.0.113.11"}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "host-a"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}, Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "203.0.113.10"}}}},
	}}
	addresses := discoverHostAPIAddresses(nodes)
	if len(addresses) != 2 || addresses[0] != "203.0.113.10" || addresses[1] != "203.0.113.11" {
		t.Fatalf("addresses = %v, want sorted by node name", addresses)
	}
}

func TestDiscoverHostAPIAddressesIgnoresNotReadyAndMissingIP(t *testing.T) {
	nodes := &corev1.NodeList{Items: []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "not-ready"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}, Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "203.0.113.13"}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "no-ip"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}, Addresses: []corev1.NodeAddress{{Type: corev1.NodeExternalIP, Address: "203.0.113.1"}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "ready"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}, Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "203.0.113.12"}}}},
	}}
	addresses := discoverHostAPIAddresses(nodes)
	if len(addresses) != 1 || addresses[0] != "203.0.113.12" {
		t.Fatalf("addresses = %v, want only Ready InternalIP", addresses)
	}
	if got := discoverHostAPIAddresses(nil); len(got) != 0 {
		t.Fatalf("addresses for nil nodes = %v, want empty", got)
	}
	if got := discoverHostAPIAddresses(&corev1.NodeList{}); len(got) != 0 {
		t.Fatalf("addresses for empty nodes = %v, want empty", got)
	}
}

func TestVirtualClusterReconcilerBlocksWithoutDiscoveredHostAddress(t *testing.T) {
	cluster, cell, class, managedCluster, scheme := virtualClusterTestObjects(t)
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster, cell, class, &workv1.ManifestWork{}).WithObjects(cluster, cell, class, managedCluster).Build()
	reconciler := &VirtualClusterReconciler{Client: client, HostReaderFactory: fakeVirtualNodeHostReaderFactory{reader: &fakeVirtualNodeHostReader{nodes: &corev1.NodeList{}}}}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.VirtualCluster{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatal(err)
	}
	if !hasCondition(updated.Status.Conditions, "HostReadReady", metav1.ConditionFalse) {
		t.Fatalf("conditions = %#v, want HostReadReady=False without Ready host nodes", updated.Status.Conditions)
	}
	foundation := &workv1.ManifestWork{}
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.FoundationWork, Namespace: "cell-a"}, foundation); !apierrors.IsNotFound(err) {
		t.Fatalf("Foundation Work exists or unexpected error: %v", err)
	}
}

func TestVirtualClusterReconcilerCreatesFoundationBeforeInstance(t *testing.T) {
	cluster, cell, class, managedCluster, scheme := virtualClusterTestObjects(t)
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster, cell, class, &workv1.ManifestWork{}).WithObjects(cluster, cell, class, managedCluster).Build()
	reconciler := &VirtualClusterReconciler{Client: client, HostReaderFactory: fakeVirtualNodeHostReaderFactory{reader: &fakeVirtualNodeHostReader{nodes: readyHostNodeList()}}}
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	foundation := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.FoundationWork, Namespace: "cell-a"}, foundation); err != nil {
		t.Fatalf("get Foundation Work: %v", err)
	}
	instance := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.InstanceWork, Namespace: "cell-a"}, instance); err == nil {
		t.Fatal("Instance Work exists before Foundation Applied")
	}

	foundation.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue}}
	if err := client.Status().Update(context.Background(), foundation); err != nil {
		t.Fatalf("update Foundation status: %v", err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.InstanceWork, Namespace: "cell-a"}, instance); err != nil {
		t.Fatalf("get Instance Work: %v", err)
	}
	updated := &api.VirtualCluster{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Resolved.K3kVersion != platform.K3kVersion {
		t.Fatalf("resolved k3kVersion = %q, want platform constant %q", updated.Status.Resolved.K3kVersion, platform.K3kVersion)
	}
}

func TestVirtualClusterReconcilerBlocksFoundationWhenInventoryIsStale(t *testing.T) {
	cluster, cell, class, managedCluster, scheme := virtualClusterTestObjects(t)
	cell.Status.Conditions = []metav1.Condition{{Type: "InventoryFresh", Status: metav1.ConditionFalse, Reason: "HostReadFailed"}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster, cell, class, &workv1.ManifestWork{}).WithObjects(cluster, cell, class, managedCluster).Build()
	if _, err := (&VirtualClusterReconciler{Client: client, HostReaderFactory: fakeVirtualNodeHostReaderFactory{reader: &fakeVirtualNodeHostReader{nodes: readyHostNodeList()}}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	foundation := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.FoundationWork, Namespace: "cell-a"}, foundation); !apierrors.IsNotFound(err) {
		t.Fatalf("Foundation Work exists or returned unexpected error: %v", err)
	}
	updated := &api.VirtualCluster{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatal(err)
	}
	if !hasCondition(updated.Status.Conditions, "Feasible", metav1.ConditionFalse) {
		t.Fatalf("conditions = %#v, want Feasible=False for Unknown decision", updated.Status.Conditions)
	}
}

func TestVirtualClusterReconcilerRechecksPreviouslyAcceptedCapacity(t *testing.T) {
	cluster, cell, class, managedCluster, scheme := virtualClusterTestObjects(t)
	cell.Status.Inventory["cpu"] = api.InventoryStatus{Allocatable: resource.MustParse("16"), AvailableEstimate: resource.MustParse("0")}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster, cell, class, &workv1.ManifestWork{}).WithObjects(cluster, cell, class, managedCluster).Build()
	if _, err := (&VirtualClusterReconciler{Client: client, HostReaderFactory: fakeVirtualNodeHostReaderFactory{reader: &fakeVirtualNodeHostReader{nodes: readyHostNodeList()}}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	foundation := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.FoundationWork, Namespace: "cell-a"}, foundation); !apierrors.IsNotFound(err) {
		t.Fatalf("Foundation Work exists or returned unexpected error: %v", err)
	}
	updated := &api.VirtualCluster{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatal(err)
	}
	if !hasCondition(updated.Status.Conditions, "Feasible", metav1.ConditionFalse) || updated.Status.Phase != api.VirtualClusterPhaseFailed {
		t.Fatalf("status = %#v, want failed feasibility rejection", updated.Status)
	}
}

func TestVirtualClusterReconcilerProjectsFoundationFailureWithoutCreatingInstance(t *testing.T) {
	cluster, cell, class, managedCluster, scheme := virtualClusterTestObjects(t)
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster, cell, class, &workv1.ManifestWork{}).WithObjects(cluster, cell, class, managedCluster).Build()
	reconciler := &VirtualClusterReconciler{Client: client, HostReaderFactory: fakeVirtualNodeHostReaderFactory{reader: &fakeVirtualNodeHostReader{nodes: readyHostNodeList()}}}
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	foundation := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.FoundationWork, Namespace: "cell-a"}, foundation); err != nil {
		t.Fatalf("get Foundation Work: %v", err)
	}
	foundation.Status.Conditions = []metav1.Condition{{Type: workv1.WorkDegraded, Status: metav1.ConditionTrue, Message: "quota rejected"}}
	if err := client.Status().Update(context.Background(), foundation); err != nil {
		t.Fatalf("update Foundation status: %v", err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("failed Foundation reconcile: %v", err)
	}
	instance := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.InstanceWork, Namespace: "cell-a"}, instance); err == nil {
		t.Fatal("Instance Work exists after Foundation failure")
	}
	updated := &api.VirtualCluster{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != api.VirtualClusterPhaseDegraded {
		t.Fatalf("phase = %q, want Degraded", updated.Status.Phase)
	}
	if !hasCondition(updated.Status.Conditions, "FoundationReady", metav1.ConditionFalse) {
		t.Fatalf("conditions = %#v, want FoundationReady=False", updated.Status.Conditions)
	}
}

func TestVirtualClusterReconcilerProjectsWorkFeedbackIntoStatus(t *testing.T) {
	cluster, cell, class, managedCluster, scheme := virtualClusterTestObjects(t)
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster, cell, class, &workv1.ManifestWork{}).WithObjects(cluster, cell, class, managedCluster).Build()
	reconciler := &VirtualClusterReconciler{Client: client, HostReaderFactory: fakeVirtualNodeHostReaderFactory{reader: &fakeVirtualNodeHostReader{nodes: readyHostNodeList()}}, ChildObserver: fakeChildObserver{ready: false}}
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	foundation := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.FoundationWork, Namespace: "cell-a"}, foundation); err != nil {
		t.Fatal(err)
	}
	foundation.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue}}
	if err := client.Status().Update(context.Background(), foundation); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	instance := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.InstanceWork, Namespace: "cell-a"}, instance); err != nil {
		t.Fatal(err)
	}
	instance.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue}}
	if err := client.Status().Update(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	updated := &api.VirtualCluster{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatal(err)
	}
	if !updated.Status.WorkRefs.Foundation.Applied || !updated.Status.WorkRefs.Instance.Applied {
		t.Fatalf("Work refs = %#v, want both Applied", updated.Status.WorkRefs)
	}
	if updated.Status.Host.QuotaReady || updated.Status.Host.Namespace != updated.Status.Resolved.HostNamespace {
		t.Fatalf("Host status = %#v", updated.Status.Host)
	}
}

func TestProjectHostQuotaRequiresFeedbackAndReportsReservation(t *testing.T) {
	observation := api.HostClusterObservation{}
	hard := corev1.ResourceList{"requests.cpu": resource.MustParse("500m"), "limits.cpu": resource.MustParse("500m")}
	used := corev1.ResourceList{"requests.cpu": resource.MustParse("450m"), "limits.cpu": resource.MustParse("450m")}
	reservation := api.ReservationSpec{ReflectedSystem: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")}}
	projectHostQuota(&observation, hard, used, reservation)
	if !observation.QuotaReady || !observation.ReservationExceeded {
		t.Fatalf("observation = %#v, want ready and reservation exceeded", observation)
	}
	if !observation.TenantHeadroom["requests.cpu"].Equal(resource.MustParse("0")) {
		t.Fatalf("tenant headroom = %#v, want zero", observation.TenantHeadroom)
	}
}

func TestProjectHostQuotaWithoutFeedbackIsNotReady(t *testing.T) {
	observation := api.HostClusterObservation{}
	projectHostQuota(&observation, nil, nil, api.ReservationSpec{})
	if observation.QuotaReady || observation.TenantHeadroom != nil {
		t.Fatalf("observation = %#v, want not ready without quota feedback", observation)
	}
}

func TestProjectPlacementsFiltersForeignPodsAndSorts(t *testing.T) {
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "b", UID: "host-uid-b", Labels: map[string]string{helpers.K3kClusterNameLabel: "kc-vc-a"}, Annotations: map[string]string{"kubecell.io/child-uid": "child-b"}}, Spec: corev1.PodSpec{NodeName: "host-a"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: "host-uid-a", Labels: map[string]string{helpers.K3kClusterNameLabel: "kc-vc-a"}, Annotations: map[string]string{"kubecell.io/child-uid": "child-a"}}, Spec: corev1.PodSpec{NodeName: "host-a"}, Status: corev1.PodStatus{Phase: corev1.PodPending}},
		{ObjectMeta: metav1.ObjectMeta{Name: "foreign", UID: "host-uid-foreign", Labels: map[string]string{helpers.K3kClusterNameLabel: "kc-foreign"}}, Spec: corev1.PodSpec{NodeName: "host-a"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	}
	placements := projectPlacements(context.Background(), nil, "host-ns", "kc-vc-a", pods)
	if len(placements) != 2 {
		t.Fatalf("placements = %#v, want 2 matching pods", placements)
	}
	if placements[0].HostUID != "host-uid-a" || placements[1].HostUID != "host-uid-b" {
		t.Fatalf("placements = %#v, want sorted by HostUID", placements)
	}
	if placements[1].FailureLayer != "" {
		t.Fatalf("running placement = %#v, want empty failure layer", placements[1])
	}
	if placements[0].FailureLayer != "HostSchedulingBlocked" {
		t.Fatalf("pending placement = %#v, want HostSchedulingBlocked", placements[0])
	}
}

func TestVirtualClusterReconcilerProjectsPlacementsAfterChildReady(t *testing.T) {
	cluster, cell, class, managedCluster, scheme := virtualClusterTestObjects(t)
	k3kName := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID).K3kCluster
	hostPods := &corev1.PodList{Items: []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "workload-b", Namespace: "host-ns", UID: "host-uid-b", Labels: map[string]string{helpers.K3kClusterNameLabel: k3kName}, Annotations: map[string]string{"kubecell.io/child-uid": "child-b"}}, Spec: corev1.PodSpec{NodeName: "host-a"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		{ObjectMeta: metav1.ObjectMeta{Name: "workload-a", Namespace: "host-ns", UID: "host-uid-a", Labels: map[string]string{helpers.K3kClusterNameLabel: k3kName}, Annotations: map[string]string{"kubecell.io/child-uid": "child-a"}}, Spec: corev1.PodSpec{NodeName: "host-a"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster, cell, class, &workv1.ManifestWork{}).WithObjects(cluster, cell, class, managedCluster).Build()
	reconciler := &VirtualClusterReconciler{
		Client:            client,
		HostReaderFactory: fakeVirtualNodeHostReaderFactory{reader: &fakeVirtualNodeHostReader{nodes: readyHostNodeList(), pods: hostPods}},
		ChildObserver:     fakeChildObserver{ready: true},
	}
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	foundation := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.FoundationWork, Namespace: "cell-a"}, foundation); err != nil {
		t.Fatalf("get Foundation Work: %v", err)
	}
	foundation.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue}}
	if err := client.Status().Update(context.Background(), foundation); err != nil {
		t.Fatalf("update Foundation status: %v", err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	instance := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.InstanceWork, Namespace: "cell-a"}, instance); err != nil {
		t.Fatalf("get Instance Work: %v", err)
	}
	instance.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue}}
	if err := client.Status().Update(context.Background(), instance); err != nil {
		t.Fatalf("update Instance status: %v", err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("third Reconcile() error = %v", err)
	}
	updated := &api.VirtualCluster{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.Status.Placements) != 2 {
		t.Fatalf("placements = %#v, want reflected host pods", updated.Status.Placements)
	}
	if updated.Status.Placements[0].HostUID != "host-uid-a" || updated.Status.Placements[1].HostUID != "host-uid-b" {
		t.Fatalf("placements = %#v, want sorted by HostUID", updated.Status.Placements)
	}
}

func TestVirtualClusterReconcilerProjectsStandardReadyConditions(t *testing.T) {
	cluster, cell, class, managedCluster, scheme := virtualClusterTestObjects(t)
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster, cell, class, &workv1.ManifestWork{}).WithObjects(cluster, cell, class, managedCluster).Build()
	reconciler := &VirtualClusterReconciler{
		Client:            client,
		HostReaderFactory: fakeVirtualNodeHostReaderFactory{reader: &fakeVirtualNodeHostReader{nodes: readyHostNodeList()}},
		ChildObserver:     fakeChildObserverWithCredential{},
	}
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	foundation := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.FoundationWork, Namespace: "cell-a"}, foundation); err != nil {
		t.Fatal(err)
	}
	foundation.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue, Reason: "Applied", LastTransitionTime: metav1.Now()}}
	if err := client.Status().Update(context.Background(), foundation); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	instance := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.InstanceWork, Namespace: "cell-a"}, instance); err != nil {
		t.Fatal(err)
	}
	instance.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue, Reason: "Applied", LastTransitionTime: metav1.Now()}}
	if err := client.Status().Update(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	updated := &api.VirtualCluster{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatal(err)
	}
	for _, conditionType := range []string{"Validated", "CellSelected", "FoundationReady", "InstanceReady", "HostQuotaReady", "HostReadReady", "APIAccessible", "LogicalNodeReady", "StorageReady", "ChildIdentityReady"} {
		if !conditionPresent(updated.Status.Conditions, conditionType) {
			t.Fatalf("conditions = %#v, missing standard condition %q", updated.Status.Conditions, conditionType)
		}
	}
	for _, condition := range updated.Status.Conditions {
		if condition.Type == "ChildAPI" || condition.Type == "ChildIdentity" || condition.Type == "Credential" || condition.Type == "CellReady" {
			t.Fatalf("conditions = %#v, contains non-standard alias %q", updated.Status.Conditions, condition.Type)
		}
	}
	if updated.Status.Endpoint.Address != "203.0.113.10" || updated.Status.Endpoint.NodePort != 30443 {
		t.Fatalf("endpoint = %#v, want discovered host address with observed NodePort", updated.Status.Endpoint)
	}
}

func conditionPresent(conditions []metav1.Condition, conditionType string) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return true
		}
	}
	return false
}

func TestVirtualClusterReconcilerPublishesAdminKubeconfigUnconditionally(t *testing.T) {
	cluster, cell, class, managedCluster, scheme := virtualClusterTestObjects(t)
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster, cell, class, &workv1.ManifestWork{}).WithObjects(cluster, cell, class, managedCluster).Build()
	reconciler := &VirtualClusterReconciler{
		Client:            client,
		HostReaderFactory: fakeVirtualNodeHostReaderFactory{reader: &fakeVirtualNodeHostReader{nodes: readyHostNodeList()}},
		ChildObserver:     fakeChildObserverWithCredential{},
	}
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	foundation := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.FoundationWork, Namespace: "cell-a"}, foundation); err != nil {
		t.Fatalf("get Foundation Work: %v", err)
	}
	foundation.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue}}
	if err := client.Status().Update(context.Background(), foundation); err != nil {
		t.Fatalf("update Foundation status: %v", err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	instance := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.InstanceWork, Namespace: "cell-a"}, instance); err != nil {
		t.Fatalf("get Instance Work: %v", err)
	}
	instance.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue}}
	if err := client.Status().Update(context.Background(), instance); err != nil {
		t.Fatalf("update Instance status: %v", err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("third Reconcile() error = %v", err)
	}
	secret := &corev1.Secret{}
	secretName := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID).AdminKubeconfig
	if err := client.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: "team-a"}, secret); err != nil {
		t.Fatalf("get published credential: %v", err)
	}
	if !containsBytes(secret.Data["kubeconfig.yaml"], []byte("https://203.0.113.10:30443")) {
		t.Fatalf("published kubeconfig does not contain discovered endpoint: %q", secret.Data["kubeconfig.yaml"])
	}
	updatedCluster := &api.VirtualCluster{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updatedCluster); err != nil {
		t.Fatalf("get updated VirtualCluster: %v", err)
	}
	if updatedCluster.Status.Credential.SourceSecretName == "" {
		t.Fatalf("credential status = %#v", updatedCluster.Status.Credential)
	}
	if updatedCluster.Status.Credential.SecretName != secretName {
		t.Fatalf("credential secret = %q, want %q", updatedCluster.Status.Credential.SecretName, secretName)
	}
}

func TestVirtualClusterReconcilerDoesNotPublishCredentialWhenChildNotReady(t *testing.T) {
	cluster, cell, class, managedCluster, scheme := virtualClusterTestObjects(t)
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster, cell, class, &workv1.ManifestWork{}).WithObjects(cluster, cell, class, managedCluster).Build()
	reconciler := &VirtualClusterReconciler{
		Client:            client,
		HostReaderFactory: fakeVirtualNodeHostReaderFactory{reader: &fakeVirtualNodeHostReader{nodes: readyHostNodeList()}},
		ChildObserver:     fakeChildObserver{ready: false},
	}
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	foundation := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.FoundationWork, Namespace: "cell-a"}, foundation); err != nil {
		t.Fatal(err)
	}
	foundation.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue, Reason: "Applied", LastTransitionTime: metav1.Now()}}
	if err := client.Status().Update(context.Background(), foundation); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	instance := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.InstanceWork, Namespace: "cell-a"}, instance); err != nil {
		t.Fatal(err)
	}
	instance.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue, Reason: "Applied", LastTransitionTime: metav1.Now()}}
	if err := client.Status().Update(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	secret := &corev1.Secret{}
	secretName := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID).AdminKubeconfig
	if err := client.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: cluster.Namespace}, secret); !apierrors.IsNotFound(err) {
		t.Fatalf("credential Secret lookup error = %v, want NotFound", err)
	}
}

func TestVirtualClusterDeletionWaitsBetweenInstanceAndFoundation(t *testing.T) {
	cluster, cell, _, managedCluster, scheme := virtualClusterTestObjects(t)
	deletionTime := metav1.Now()
	cluster.DeletionTimestamp = &deletionTime
	cluster.Finalizers = []string{virtualClusterFinalizer}
	cluster.Status.Resolved.ManagedClusterName = "cell-a"
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	labels := helpers.OwnedHostLabels(cluster.Spec.CellRef.Name, cluster.Namespace, cluster.Name, string(cluster.UID), "profile")
	instance := &workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: names.InstanceWork, Namespace: "cell-a", Labels: labels}}
	foundation := &workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: names.FoundationWork, Namespace: "cell-a", Labels: labels}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: names.AdminKubeconfig, Namespace: cluster.Namespace, Labels: map[string]string{helpers.LabelVirtualClusterUID: string(cluster.UID)}}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster).WithObjects(cluster, cell, managedCluster, instance, foundation, secret).Build()
	reconciler := &VirtualClusterReconciler{Client: client}
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}}

	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("first deletion reconcile: %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.FoundationWork, Namespace: "cell-a"}, &workv1.ManifestWork{}); err != nil {
		t.Fatalf("foundation should remain while instance deletion settles: %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.AdminKubeconfig, Namespace: cluster.Namespace}, &corev1.Secret{}); err == nil {
		t.Fatal("published admin kubeconfig still exists")
	}
	updated := &api.VirtualCluster{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatalf("get cluster after first deletion reconcile: %v", err)
	}
	if !containsString(updated.Finalizers, virtualClusterFinalizer) {
		t.Fatal("finalizer removed before Foundation cleanup")
	}

	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("second deletion reconcile: %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.FoundationWork, Namespace: "cell-a"}, &workv1.ManifestWork{}); err == nil {
		t.Fatal("foundation Work still exists after second deletion reconcile")
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("third deletion reconcile: %v", err)
	}
	updated = &api.VirtualCluster{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err == nil {
		t.Fatal("VirtualCluster remains after finalizer removal")
	} else if !apierrors.IsNotFound(err) {
		t.Fatalf("get cluster after finalizer removal: %v", err)
	}
}

func TestVirtualClusterDeletionBlocksForeignInstanceWork(t *testing.T) {
	cluster, cell, _, managedCluster, scheme := virtualClusterTestObjects(t)
	deletionTime := metav1.Now()
	cluster.DeletionTimestamp = &deletionTime
	cluster.Finalizers = []string{virtualClusterFinalizer}
	cluster.Status.Resolved.ManagedClusterName = "cell-a"
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	foreign := &workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: names.InstanceWork, Namespace: "cell-a", Labels: map[string]string{helpers.LabelVirtualClusterUID: "foreign"}}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster).WithObjects(cluster, cell, managedCluster, foreign).Build()
	if _, err := (&VirtualClusterReconciler{Client: client}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}}); err != nil {
		t.Fatalf("foreign cleanup reconcile: %v", err)
	}
	updated := &api.VirtualCluster{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	if !containsString(updated.Finalizers, virtualClusterFinalizer) || !hasCondition(updated.Status.Conditions, "CleanupBlocked") {
		t.Fatalf("foreign cleanup was not blocked: finalizers=%v conditions=%v", updated.Finalizers, updated.Status.Conditions)
	}
}

func TestVirtualClusterReconcilerRejectsForeignExistingWork(t *testing.T) {
	cluster, cell, class, managedCluster, scheme := virtualClusterTestObjects(t)
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	foreign := &workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{
		Name:      names.FoundationWork,
		Namespace: "cell-a",
		Labels: map[string]string{
			helpers.LabelManagedBy:         "another-controller",
			helpers.LabelVirtualClusterUID: "different-uid",
		},
	}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster, cell, class, &workv1.ManifestWork{}).WithObjects(cluster, cell, class, managedCluster, foreign).Build()
	_, err := (&VirtualClusterReconciler{Client: client, HostReaderFactory: fakeVirtualNodeHostReaderFactory{reader: &fakeVirtualNodeHostReader{nodes: readyHostNodeList()}}}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}})
	if err == nil {
		t.Fatal("reconcile succeeded while a foreign same-name Work existed")
	}
	if !strings.Contains(err.Error(), "foreign") {
		t.Fatalf("error = %v, want foreign ownership error", err)
	}
	current := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.FoundationWork, Namespace: "cell-a"}, current); err != nil {
		t.Fatal(err)
	}
	if current.Labels[helpers.LabelManagedBy] != "another-controller" {
		t.Fatal("foreign Work was modified")
	}
}

func TestVirtualClusterReconcilerAcceptsOwnedWorkRecreatedByOCM(t *testing.T) {
	cluster, _, _, _, scheme := virtualClusterTestObjects(t)
	labels := helpers.OwnedHostLabels("cell-a", cluster.Namespace, cluster.Name, string(cluster.UID), "profile")
	current := &workv1.ManifestWork{TypeMeta: metav1.TypeMeta{APIVersion: workv1.GroupVersion.String(), Kind: "ManifestWork"}, ObjectMeta: metav1.ObjectMeta{
		Name:      "foundation",
		Namespace: "cell-a",
		UID:       "new-work-uid",
		Labels:    labels,
	}}
	desired := current.DeepCopy()
	desired.UID = ""
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(current).Build()

	reconciler := &VirtualClusterReconciler{Client: client}
	if err := reconciler.createOrUpdateWork(context.Background(), desired); err != nil {
		t.Fatalf("owned Work recreated by OCM was rejected: %v", err)
	}
}

func hasCondition(conditions []metav1.Condition, conditionType string, statuses ...metav1.ConditionStatus) bool {
	for _, condition := range conditions {
		if condition.Type != conditionType {
			continue
		}
		if len(statuses) == 0 && condition.Status == metav1.ConditionTrue {
			return true
		}
		for _, status := range statuses {
			if condition.Status == status {
				return true
			}
		}
	}
	return false
}

type fakeChildObserver struct{ ready bool }

func (f fakeChildObserver) Observe(_ context.Context, _ *api.VirtualCluster, _ []string) (ChildObservation, error) {
	return ChildObservation{APIReady: f.ready, LogicalNodeName: "kubelet", LogicalNodeReady: f.ready, StorageClassReady: f.ready}, nil
}

type fakeChildObserverWithCredential struct{}

func (fakeChildObserverWithCredential) Observe(_ context.Context, _ *api.VirtualCluster, _ []string) (ChildObservation, error) {
	return ChildObservation{
		APIReady:          true,
		LogicalNodeName:   "kubelet",
		LogicalNodeReady:  true,
		StorageClassReady: true,
		NodePort:          30443,
		Endpoint:          "https://203.0.113.10:30443",
		SourceSecretName:  "k3k-kc-team-a-training-cluster-kubeconfig",
		Kubeconfig:        []byte("apiVersion: v1\nclusters:\n- cluster:\n    server: https://203.0.113.10:30443\n"),
	}, nil
}

type fakeVirtualNodeHostReader struct {
	nodes    *corev1.NodeList
	pods     *corev1.PodList
	quota    *corev1.ResourceQuota
	quotaErr error
	pvcs     map[string]*corev1.PersistentVolumeClaim
	pvs      map[string]*corev1.PersistentVolume
}

func (f *fakeVirtualNodeHostReader) ListNodes(context.Context) (*corev1.NodeList, error) {
	if f.nodes != nil {
		return f.nodes, nil
	}
	return &corev1.NodeList{}, nil
}

func (f *fakeVirtualNodeHostReader) ListPods(_ context.Context, _ string) (*corev1.PodList, error) {
	if f.pods != nil {
		return f.pods, nil
	}
	return &corev1.PodList{}, nil
}

func (f *fakeVirtualNodeHostReader) GetResourceQuota(context.Context, string, string) (*corev1.ResourceQuota, error) {
	return f.quota, f.quotaErr
}

func (f *fakeVirtualNodeHostReader) GetPersistentVolumeClaim(_ context.Context, namespace, name string) (*corev1.PersistentVolumeClaim, error) {
	if claim, found := f.pvcs[namespace+"/"+name]; found {
		return claim, nil
	}
	return nil, apierrors.NewNotFound(corev1.Resource("persistentvolumeclaims"), name)
}

func (f *fakeVirtualNodeHostReader) GetPersistentVolume(_ context.Context, name string) (*corev1.PersistentVolume, error) {
	if pv, found := f.pvs[name]; found {
		return pv, nil
	}
	return nil, apierrors.NewNotFound(corev1.Resource("persistentvolumes"), name)
}

type fakeVirtualNodeHostReaderFactory struct {
	reader VirtualNodeHostReader
	err    error
}

func (f fakeVirtualNodeHostReaderFactory) ForCell(context.Context, *api.Cell) (VirtualNodeHostReader, error) {
	return f.reader, f.err
}

func containsBytes(value, fragment []byte) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if string(value[i:i+len(fragment)]) == string(fragment) {
			return true
		}
	}
	return false
}

type fakeMirrorChildClient struct {
	ingresses *networkingv1.IngressList
	services  map[string]*corev1.Service
	endpoints map[string]*corev1.Endpoints
}

func (f fakeMirrorChildClient) Ready(context.Context) error { return nil }
func (f fakeMirrorChildClient) ListNodes(context.Context) (*corev1.NodeList, error) {
	return &corev1.NodeList{}, nil
}
func (f fakeMirrorChildClient) EnsureStorageClass(context.Context, *storagev1.StorageClass) error {
	return nil
}
func (f fakeMirrorChildClient) ListIngresses(context.Context) (*networkingv1.IngressList, error) {
	return f.ingresses, nil
}
func (f fakeMirrorChildClient) GetService(_ context.Context, namespace, name string) (*corev1.Service, error) {
	service, ok := f.services[namespace+"/"+name]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return service, nil
}
func (f fakeMirrorChildClient) GetEndpoints(_ context.Context, namespace, name string) (*corev1.Endpoints, error) {
	return f.endpoints[namespace+"/"+name], nil
}

func mirrorTestIngress(name, class, service string) networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	return networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &class,
			Rules: []networkingv1.IngressRule{
				{
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: service,
											Port: networkingv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestMirrorChildIngressesRendersOnlyKubecellClass(t *testing.T) {
	ingresses := &networkingv1.IngressList{Items: []networkingv1.Ingress{
		mirrorTestIngress("web", "kubecell", "web-svc"),
		mirrorTestIngress("other", "nginx", "other-svc"),
	}}
	client := fakeMirrorChildClient{
		ingresses: ingresses,
		services: map[string]*corev1.Service{
			"default/web-svc": {ObjectMeta: metav1.ObjectMeta{Name: "web-svc", Namespace: "default"}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}}},
		},
		endpoints: map[string]*corev1.Endpoints{},
	}
	cluster := &api.VirtualCluster{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "team-a", UID: "uid-1234"}, Spec: api.VirtualClusterSpec{CellRef: api.ObjectReference{Name: "cell-ci"}}}
	resolved := api.ResolvedSnapshot{HostNamespace: "kc-host", K3kClusterName: "kc-1234", ProfileHash: "abc"}
	objects, err := mirrorChildIngresses(context.Background(), cluster, resolved, "apps.example.com", client)
	if err != nil {
		t.Fatalf("mirrorChildIngresses() error = %v", err)
	}
	if len(objects) != 3 {
		t.Fatalf("objects = %d, want Service+Endpoints+Ingress for the kubecell ingress only", len(objects))
	}
}

type fakeChildObserverFailing struct{}

func (fakeChildObserverFailing) Observe(_ context.Context, _ *api.VirtualCluster, _ []string) (ChildObservation, error) {
	return ChildObservation{}, fmt.Errorf("child API is down")
}

func TestVirtualClusterReconcilerClearsStaleChildOnObserveFailure(t *testing.T) {
	cluster, cell, class, managedCluster, scheme := virtualClusterTestObjects(t)
	cluster.Status.Phase = api.VirtualClusterPhaseReady
	cluster.Status.Child = api.ChildClusterObservation{APIReady: true, LogicalNodeName: "kubelet", LogicalNodeReady: true, StorageClassReady: true}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster, cell, class, &workv1.ManifestWork{}).WithObjects(cluster, cell, class, managedCluster).Build()
	reconciler := &VirtualClusterReconciler{
		Client:            client,
		HostReaderFactory: fakeVirtualNodeHostReaderFactory{reader: &fakeVirtualNodeHostReader{nodes: readyHostNodeList()}},
		ChildObserver:     fakeChildObserverFailing{},
	}
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	names := helpers.NamesForVirtualCluster(cluster.Namespace, cluster.Name, cluster.UID)
	foundation := &workv1.ManifestWork{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.FoundationWork, Namespace: "cell-a"}, foundation); err != nil {
		t.Fatal(err)
	}
	foundation.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue, Reason: "Applied", LastTransitionTime: metav1.Now()}}
	if err := client.Status().Update(context.Background(), foundation); err != nil {
		t.Fatal(err)
	}
	instance := &workv1.ManifestWork{}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: names.InstanceWork, Namespace: "cell-a"}, instance); err != nil {
		t.Fatal(err)
	}
	instance.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue, Reason: "Applied", LastTransitionTime: metav1.Now()}}
	if err := client.Status().Update(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("observe-failure Reconcile() error = %v", err)
	}
	updated := &api.VirtualCluster{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Child.APIReady || updated.Status.Child.LogicalNodeReady || updated.Status.Child.LogicalNodeName != "" {
		t.Fatalf("stale Child observation retained: %#v (KI-16)", updated.Status.Child)
	}
	if updated.Status.Phase == api.VirtualClusterPhaseReady {
		t.Fatalf("phase = Ready with failed observation (KI-16)")
	}
}

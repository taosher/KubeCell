package controller

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/platform"
)

func TestEvaluateVirtualClusterPlanAcceptsCompleteSnapshotAndSortsChecks(t *testing.T) {
	cluster, cell, class := planEvaluationObjects()
	now := time.Date(2026, 8, 19, 1, 2, 3, 0, time.UTC)

	evaluation := evaluateVirtualClusterPlan(cluster, cell, class, now)
	if evaluation.Decision != api.VirtualClusterPlanDecisionAccepted {
		t.Fatalf("decision = %q, want Accepted; checks=%#v", evaluation.Decision, evaluation.Checks)
	}
	if len(evaluation.Checks) == 0 {
		t.Fatal("evaluation returned no checks")
	}
	for index := 1; index < len(evaluation.Checks); index++ {
		if evaluation.Checks[index-1].Name > evaluation.Checks[index].Name {
			t.Fatalf("checks are not sorted: %#v", evaluation.Checks)
		}
	}
	if evaluation.Summary.CellUID != string(cell.UID) || evaluation.Summary.ChildK3sVersion != platform.ChildK3sVersion {
		t.Fatalf("summary = %#v", evaluation.Summary)
	}
	if evaluation.Summary.K3kVersion != platform.K3kVersion {
		t.Fatalf("summary k3kVersion = %q, want platform constant %q", evaluation.Summary.K3kVersion, platform.K3kVersion)
	}
	if evaluation.Summary.StorageClassName != class.Spec.StorageClassName {
		t.Fatalf("summary storage = %#v", evaluation.Summary)
	}
	if serialized := strings.ToLower(string(mustJSON(t, evaluation.Summary))); strings.Contains(serialized, "kubeconfig") || strings.Contains(serialized, "token") {
		t.Fatalf("summary contains forbidden credential or runtime fields: %s", serialized)
	}
}

func TestEvaluateVirtualClusterPlanRejectsInsufficientDeviceCapacity(t *testing.T) {
	cluster, cell, class := planEvaluationObjects()
	class.Spec.Entitlement.WorkloadHard["requests.huawei.com/Ascend910"] = resource.MustParse("2")
	class.Spec.Entitlement.WorkloadHard["limits.huawei.com/Ascend910"] = resource.MustParse("2")

	evaluation := evaluateVirtualClusterPlan(cluster, cell, class, time.Now())
	if evaluation.Decision != api.VirtualClusterPlanDecisionRejected {
		t.Fatalf("decision = %q, want Rejected; checks=%#v", evaluation.Decision, evaluation.Checks)
	}
	if check := findPlanCheck(evaluation.Checks, "Accelerators"); check == nil || check.Status != api.VirtualClusterPlanCheckFailed {
		t.Fatalf("Accelerators check = %#v, want Failed", check)
	}
}

func TestEvaluateVirtualClusterPlanReturnsUnknownForStaleInventory(t *testing.T) {
	cluster, cell, class := planEvaluationObjects()
	cell.Status.Conditions = []metav1.Condition{{Type: "InventoryFresh", Status: metav1.ConditionFalse, Reason: "HostReadFailed"}}

	evaluation := evaluateVirtualClusterPlan(cluster, cell, class, time.Now())
	if evaluation.Decision != api.VirtualClusterPlanDecisionUnknown {
		t.Fatalf("decision = %q, want Unknown; checks=%#v", evaluation.Decision, evaluation.Checks)
	}
	if check := findPlanCheck(evaluation.Checks, "InventoryFresh"); check == nil || check.Status != api.VirtualClusterPlanCheckUnknown {
		t.Fatalf("InventoryFresh check = %#v, want Unknown", check)
	}
}

func TestEvaluateVirtualClusterPlanRejectsExhaustedNodePortRange(t *testing.T) {
	cluster, cell, class := planEvaluationObjects()
	allocated := make([]int32, 0, platform.NodePortMax-platform.NodePortMin+1)
	for port := platform.NodePortMin; port <= platform.NodePortMax; port++ {
		allocated = append(allocated, port)
	}
	cell.Status.NodePorts.Allocated = allocated
	cell.Status.NodePorts.Fresh = true

	evaluation := evaluateVirtualClusterPlan(cluster, cell, class, time.Now())
	if evaluation.Decision != api.VirtualClusterPlanDecisionRejected {
		t.Fatalf("decision = %q, want Rejected; checks=%#v", evaluation.Decision, evaluation.Checks)
	}
	if check := findPlanCheck(evaluation.Checks, "NodePort"); check == nil || check.Reason != "NodePortRangeExhausted" {
		t.Fatalf("NodePort check = %#v, want NodePortRangeExhausted", check)
	}
}

func TestEvaluateVirtualClusterPlanReturnsUnknownForStaleNodePortInventory(t *testing.T) {
	cluster, cell, class := planEvaluationObjects()
	cell.Status.NodePorts.Fresh = false

	evaluation := evaluateVirtualClusterPlan(cluster, cell, class, time.Now())
	if evaluation.Decision != api.VirtualClusterPlanDecisionUnknown {
		t.Fatalf("decision = %q, want Unknown; checks=%#v", evaluation.Decision, evaluation.Checks)
	}
	if check := findPlanCheck(evaluation.Checks, "NodePort"); check == nil || check.Status != api.VirtualClusterPlanCheckUnknown {
		t.Fatalf("NodePort check = %#v, want Unknown", check)
	}
}

func TestEvaluateVirtualClusterPlanRejectsUnlockedVersionProfile(t *testing.T) {
	cluster, cell, class := planEvaluationObjects()
	cell.Status.Provider.K3kVersion = "v0.0.0"

	evaluation := evaluateVirtualClusterPlan(cluster, cell, class, time.Now())
	if evaluation.Decision != api.VirtualClusterPlanDecisionRejected {
		t.Fatalf("decision = %q, want Rejected; checks=%#v", evaluation.Decision, evaluation.Checks)
	}
	if check := findPlanCheck(evaluation.Checks, "LockedVersions"); check == nil || check.Status != api.VirtualClusterPlanCheckFailed {
		t.Fatalf("LockedVersions check = %#v, want Failed", check)
	}
}

func TestVirtualClusterPlanReconcilerIsSideEffectFree(t *testing.T) {
	cluster, cell, class := planEvaluationObjects()
	plan := &api.VirtualClusterPlan{
		ObjectMeta: metav1.ObjectMeta{Name: "training-plan", Namespace: "team-a", UID: "plan-uid"},
		Spec:       api.VirtualClusterPlanSpec{VirtualCluster: *cluster.Spec.DeepCopy()},
	}
	scheme := planTestScheme(t)
	fakeClient := newPlanFakeClient(scheme, plan, cell, class)
	reconciler := &VirtualClusterPlanReconciler{Client: fakeClient}
	if _, err := reconciler.Reconcile(t.Context(), reconcileRequest(plan)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	updated := &api.VirtualClusterPlan{}
	if err := fakeClient.Get(t.Context(), objectKey(plan), updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Decision != api.VirtualClusterPlanDecisionAccepted {
		t.Fatalf("plan status = %#v", updated.Status)
	}
	objects := []client.ObjectList{&api.VirtualClusterList{}, &corev1.SecretList{}, &corev1.NamespaceList{}}
	for _, list := range objects {
		if err := fakeClient.List(t.Context(), list); err != nil {
			t.Fatal(err)
		}
		if count := listLen(list); count != 0 {
			t.Fatalf("plan reconciliation created %T objects: %d", list, count)
		}
	}
}

func TestVirtualClusterPlanReconcilerIsOneShotWithoutRequeue(t *testing.T) {
	cluster, cell, class := planEvaluationObjects()
	plan := &api.VirtualClusterPlan{
		ObjectMeta: metav1.ObjectMeta{Name: "training-plan", Namespace: "team-a", UID: "plan-uid"},
		Spec:       api.VirtualClusterPlanSpec{VirtualCluster: *cluster.Spec.DeepCopy()},
	}
	scheme := planTestScheme(t)
	fakeClient := newPlanFakeClient(scheme, plan, cell, class)
	result, err := (&VirtualClusterPlanReconciler{Client: fakeClient}).Reconcile(t.Context(), reconcileRequest(plan))
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Fatalf("plan reconcile result = %#v, want one-shot without requeue", result)
	}
}

func TestVirtualClusterPlanDefaultAndValidationReuseVirtualClusterContract(t *testing.T) {
	plan := &api.VirtualClusterPlan{
		ObjectMeta: metav1.ObjectMeta{Name: "training", Namespace: "kubecell-system"},
		Spec:       api.VirtualClusterPlanSpec{VirtualCluster: api.VirtualClusterSpec{CellRef: api.ObjectReference{Name: "cell-a", Namespace: "kubecell-system"}, ClassRef: api.ObjectReference{Name: "small"}}},
	}
	plan.Default()
	if err := plan.ValidateCreate(); err != nil {
		t.Fatalf("ValidateCreate() error = %v", err)
	}
	badRefs := plan.DeepCopy()
	badRefs.Spec.VirtualCluster.ClassRef = api.ObjectReference{}
	if err := badRefs.ValidateCreate(); err == nil {
		t.Fatal("ValidateCreate() accepted missing classRef")
	}
	badNamespace := plan.DeepCopy()
	badNamespace.Namespace = "team-a"
	if err := badNamespace.ValidateCreate(); err == nil {
		t.Fatal("ValidateCreate() accepted plan outside kubecell-system")
	}
}

type planEvaluation struct {
	Decision api.VirtualClusterPlanDecision
	Checks   []api.VirtualClusterPlanCheck
	Summary  api.VirtualClusterPlanResolvedSummary
}

func findPlanCheck(checks []api.VirtualClusterPlanCheck, name string) *api.VirtualClusterPlanCheck {
	for index := range checks {
		if checks[index].Name == name {
			return &checks[index]
		}
	}
	return nil
}

func quantityPtr(value resource.Quantity) *resource.Quantity { return &value }

func planTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func newPlanFakeClient(scheme *runtime.Scheme, objects ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&api.VirtualClusterPlan{}).WithObjects(objects...).Build()
}

func reconcileRequest(plan *api.VirtualClusterPlan) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Name: plan.Name, Namespace: plan.Namespace}}
}

func objectKey(object client.Object) types.NamespacedName {
	return types.NamespacedName{Name: object.GetName(), Namespace: object.GetNamespace()}
}

func mustJSON(t *testing.T, object any) []byte {
	t.Helper()
	data, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func listLen(list client.ObjectList) int {
	switch value := list.(type) {
	case *api.VirtualClusterList:
		return len(value.Items)
	case *corev1.SecretList:
		return len(value.Items)
	case *corev1.NamespaceList:
		return len(value.Items)
	default:
		return -1
	}
}

func TestVirtualClusterPlanStatusDoesNotExposeSensitiveStrings(t *testing.T) {
	cluster, cell, class := planEvaluationObjects()
	evaluation := evaluateVirtualClusterPlan(cluster, cell, class, time.Now())
	serialized := strings.ToLower(string(mustJSON(t, evaluation.Summary)))
	if strings.Contains(serialized, "token") || strings.Contains(serialized, "kubeconfig") {
		t.Fatalf("summary exposes sensitive field: %s", serialized)
	}
	if !strings.Contains(serialized, strings.ToLower(string(cell.UID))) {
		t.Fatalf("summary misses non-sensitive identity: %s", serialized)
	}
}

func planEvaluationObjects() (*api.VirtualCluster, *api.Cell, *api.VirtualNodeClass) {
	cluster := &api.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "training", Namespace: "team-a", UID: "vc-uid"},
		Spec: api.VirtualClusterSpec{
			CellRef:  api.ObjectReference{Name: "cell-a", Namespace: "kubecell-system"},
			ClassRef: api.ObjectReference{Name: "small"},
		},
	}
	cell := &api.Cell{
		ObjectMeta: metav1.ObjectMeta{Name: "cell-a", Namespace: "kubecell-system", UID: "cell-uid"},
		Spec: api.CellSpec{
			ManagedClusterRef: api.ObjectReference{Name: "cell-a"},
			MachineProfile: api.MachineProfile{
				Name:    "ascend-910b",
				Devices: []api.DeviceContract{{Name: "ascend", ResourceName: "huawei.com/Ascend910"}},
			},
		},
		Status: api.CellStatus{
			Phase:          api.CellPhaseReady,
			ManagedCluster: api.ManagedClusterStatus{Joined: true, Available: true, LeaseFresh: true},
			Provider:       api.ProviderStatus{K3kVersion: platform.K3kVersion, NamespaceReady: true, K3kCRDsReady: true, ControllerReady: true, CNIReady: true, TopoLVMReady: true},
			NodePorts:      api.NodePortInventoryStatus{Fresh: true},
			Nodes: []api.CellNodeStatus{{
				Name: "node-a", Profile: "ascend-910b", Ready: true,
				Allocatable: api.ResourceList{"huawei.com/Ascend910": resource.MustParse("1")},
			}},
			Conditions: []metav1.Condition{{Type: "InventoryFresh", Status: metav1.ConditionTrue}},
			Inventory: map[string]api.InventoryStatus{
				"cpu":                  {Allocatable: resource.MustParse("16"), AvailableEstimate: resource.MustParse("12")},
				"memory":               {Allocatable: resource.MustParse("32Gi"), AvailableEstimate: resource.MustParse("24Gi")},
				"ephemeral-storage":    {Allocatable: resource.MustParse("100Gi"), AvailableEstimate: resource.MustParse("80Gi")},
				"huawei.com/Ascend910": {Allocatable: resource.MustParse("1"), AvailableEstimate: resource.MustParse("1")},
			},
			StorageInventory: map[string]api.StorageInventoryStatus{
				"topolvm-provisioner": {
					Provisioner:       "topolvm.io",
					FreeCapacity:      quantityPtr(resource.MustParse("100Gi")),
					FreeCapacityKnown: true,
					Fresh:             true,
					Nodes: map[string]api.StorageNodeInventoryStatus{
						"node-a": {FreeCapacity: quantityPtr(resource.MustParse("100Gi")), FreeCapacityKnown: true, Fresh: true},
					},
				},
			},
		},
	}
	class := &api.VirtualNodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "small", UID: "class-uid"},
		Spec: api.VirtualNodeClassSpec{
			Entitlement: api.EntitlementSpec{WorkloadHard: api.ResourceList{
				"requests.cpu": resource.MustParse("2"), "limits.cpu": resource.MustParse("2"),
				"requests.memory": resource.MustParse("4Gi"), "limits.memory": resource.MustParse("4Gi"),
				"requests.ephemeral-storage": resource.MustParse("10Gi"), "limits.ephemeral-storage": resource.MustParse("10Gi"),
				"requests.huawei.com/Ascend910": resource.MustParse("1"), "limits.huawei.com/Ascend910": resource.MustParse("1"),
			}},
			StorageClassName: "topolvm-provisioner",
		},
	}
	return cluster, cell, class
}

package host

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/kubecell/kubecell/api/v1alpha1"
)

func TestObserveNodeUsesOnlyGenericExtendedResourceFacts(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "ascend-a", Labels: map[string]string{"hardware.kubecell.io/profile": "ascend-910b"}},
		Status: corev1.NodeStatus{
			Capacity:    corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("8")},
			Allocatable: corev1.ResourceList{"huawei.com/Ascend910": resource.MustParse("7")},
			Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
		},
	}
	status, ready := ObserveNode(node, "ascend-910b", []api.DeviceContract{{Name: "ascend", ResourceName: "huawei.com/Ascend910"}})
	if !ready || !status.Ready {
		t.Fatalf("node ready = %v status=%#v", ready, status)
	}
	if got := status.Capacity["huawei.com/Ascend910"]; !got.Equal(resource.MustParse("8")) {
		t.Fatalf("capacity = %s", got.String())
	}
	if got := status.Allocatable["huawei.com/Ascend910"]; !got.Equal(resource.MustParse("7")) {
		t.Fatalf("allocatable = %s", got.String())
	}
}

func TestObserveNodeRejectsMissingResourceWithoutVendorProbe(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu-a", Labels: map[string]string{"hardware.kubecell.io/profile": "nvidia"}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}
	_, ready := ObserveNode(node, "nvidia", []api.DeviceContract{{Name: "gpu", ResourceName: "nvidia.com/gpu"}})
	if ready {
		t.Fatal("node without generic resource should not be ready")
	}
}

func TestAggregateResourceInventorySeparatesActiveAndPendingUIDPods(t *testing.T) {
	nodes := []*corev1.Node{
		{Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("4")}, Allocatable: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("4")}}},
		{Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("4")}, Allocatable: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("3")}}},
	}
	pods := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"k3k.io/clusterName": "kc-vc-1"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")}}}}}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"k3k.io/clusterName": "kc-vc-1"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("2")}}}}}, Status: corev1.PodStatus{Phase: corev1.PodPending}},
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"k3k.io/clusterName": "kc-other"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("9")}}}}}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	}
	result := AggregateResourceInventory(nodes, pods, "kc-vc-1", "nvidia.com/gpu")
	if !result.Capacity.Equal(resource.MustParse("8")) || !result.Allocatable.Equal(resource.MustParse("7")) {
		t.Fatalf("node totals = capacity %s allocatable %s", result.Capacity.String(), result.Allocatable.String())
	}
	if !result.Requested.Equal(resource.MustParse("1")) || !result.Pending.Equal(resource.MustParse("2")) {
		t.Fatalf("pod totals = requested %s pending %s", result.Requested.String(), result.Pending.String())
	}
	if !result.AvailableEstimate.Equal(resource.MustParse("6")) {
		t.Fatalf("available = %s", result.AvailableEstimate.String())
	}
}

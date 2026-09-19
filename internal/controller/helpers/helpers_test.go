package helpers

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestVirtualClusterNamesAreStableAndUIDScoped(t *testing.T) {
	names := NamesForVirtualCluster("Team_A", "Training.VC", types.UID("0123456789abcdef"))
	if names.HostNamespace != "kc-team-a-training-vc-01234567" {
		t.Fatalf("host namespace = %q", names.HostNamespace)
	}
	if names.K3kCluster != "kc-01234567" {
		t.Fatalf("K3k cluster = %q", names.K3kCluster)
	}
	if len("k3k-"+names.K3kCluster+"-server-0123456789") > 63 {
		t.Fatalf("K3k-derived StatefulSet labels may exceed 63 bytes: %q", names.K3kCluster)
	}
	if names.FoundationWork != names.HostNamespace+"-foundation" {
		t.Fatalf("foundation work = %q", names.FoundationWork)
	}
	if names.AdminKubeconfig != "kubecell-training-vc-01234567-admin-kubeconfig" {
		t.Fatalf("admin kubeconfig = %q", names.AdminKubeconfig)
	}
}

func TestOwnedHostLabelsRequireFullIdentity(t *testing.T) {
	labels := OwnedHostLabels("cell-a", "team-a", "training", "uid-1", "hash-1")
	if !MatchesOwnedHostLabels(labels, "cell-a", "team-a", "training", "uid-1") {
		t.Fatal("expected complete labels to match")
	}
	labels[LabelVirtualClusterUID] = "uid-2"
	if MatchesOwnedHostLabels(labels, "cell-a", "team-a", "training", "uid-1") {
		t.Fatal("expected mismatched UID not to match")
	}
}

func TestEffectivePodRequestsUsesContainerSumAndInitMaximum(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{
		Containers: []corev1.Container{
			{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			}}},
			{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			}}},
		},
		InitContainers: []corev1.Container{
			{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			}}},
		},
		Overhead: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m")},
	}}

	requests := EffectivePodRequests(pod)
	if got := requests[corev1.ResourceCPU]; !got.Equal(resource.MustParse("550m")) {
		t.Fatalf("cpu request = %s, want 550m", got.String())
	}
	if got := requests[corev1.ResourceMemory]; !got.Equal(resource.MustParse("1536Mi")) {
		t.Fatalf("memory request = %s, want 1536Mi", got.String())
	}
}

func TestSetConditionDoesNotChangeTransitionTimeForSameState(t *testing.T) {
	first := metav1.Now()
	conditions := []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready", LastTransitionTime: first}}
	updated := SetCondition(conditions, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "StillReady"}, metav1.Now())
	if !updated[0].LastTransitionTime.Equal(&first) {
		t.Fatalf("transition time changed from %v to %v", first, updated[0].LastTransitionTime)
	}
	if updated[0].Reason != "StillReady" {
		t.Fatalf("reason = %q, want StillReady", updated[0].Reason)
	}
}

func TestVirtualClusterNamesPreserveUIDSuffixForLongNames(t *testing.T) {
	long := "extremely-long-team-name-that-keeps-going-and-going-for-a-while"
	first := NamesForVirtualCluster(long, "workload-one-with-a-very-long-name", types.UID("aaaaaaaa00000000"))
	second := NamesForVirtualCluster(long, "workload-two-with-a-very-long-name", types.UID("bbbbbbbb11111111"))
	if len(first.HostNamespace) > 63 {
		t.Fatalf("host namespace too long: %q", first.HostNamespace)
	}
	if first.HostNamespace == second.HostNamespace {
		t.Fatalf("long names collide: %q", first.HostNamespace)
	}
	for _, names := range []VirtualClusterNames{first, second} {
		_ = names
	}
	if !strings.HasSuffix(first.HostNamespace, "-aaaaaaaa") || !strings.HasSuffix(second.HostNamespace, "-bbbbbbbb") {
		t.Fatalf("UID suffix lost: %q vs %q", first.HostNamespace, second.HostNamespace)
	}
}

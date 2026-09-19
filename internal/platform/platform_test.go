package platform

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestCombinedReservationSumsBothParts(t *testing.T) {
	combined := CombinedReservation()
	if got := combined[corev1.ResourceCPU]; !got.Equal(resource.MustParse("600m")) {
		t.Fatalf("cpu = %s, want 600m", got.String())
	}
	if got := combined[corev1.ResourceMemory]; !got.Equal(resource.MustParse("1280Mi")) {
		t.Fatalf("memory = %s, want 1280Mi", got.String())
	}
}

func TestCombinedReservationResultsShareNoState(t *testing.T) {
	first := CombinedReservation()
	mutated := first[corev1.ResourceCPU]
	mutated.Add(resource.MustParse("10"))
	first[corev1.ResourceCPU] = mutated

	second := CombinedReservation()
	if got := second[corev1.ResourceCPU]; !got.Equal(resource.MustParse("600m")) {
		t.Fatalf("second call cpu = %s, want unaffected 600m", got.String())
	}
}

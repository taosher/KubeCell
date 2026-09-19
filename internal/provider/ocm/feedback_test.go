package ocm

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

func TestSummarizeWorkProjectsAppliedProgressingAndDegraded(t *testing.T) {
	work := &workv1.ManifestWork{Status: workv1.ManifestWorkStatus{Conditions: []metav1.Condition{
		{Type: workv1.WorkApplied, Status: metav1.ConditionTrue},
		{Type: workv1.WorkProgressing, Status: metav1.ConditionFalse},
		{Type: workv1.WorkDegraded, Status: metav1.ConditionTrue, Message: "apply failed"},
	}}}
	summary := SummarizeWork(work)
	if !summary.Applied || summary.Progressing || !summary.Failed || summary.Message != "apply failed" {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestFeedbackResourceListRoundTrips(t *testing.T) {
	values := ResourceListFeedbackValue("hard", mapResourceListForFeedback())
	decoded, err := FeedbackResourceList([]workv1.FeedbackValue{values}, "hard")
	if err != nil {
		t.Fatalf("FeedbackResourceList() error = %v", err)
	}
	gotQuantity := decoded["cpu"]
	if got := gotQuantity.String(); got != "2" {
		t.Fatalf("decoded cpu = %q", got)
	}
}

func mapResourceListForFeedback() corev1.ResourceList {
	return corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")}
}

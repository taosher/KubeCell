package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

func TestDeletionPlanDeletesInstanceBeforeFoundation(t *testing.T) {
	labels := map[string]string{"kubecell.io/virtual-cluster-uid": "cluster-uid"}
	plan := BuildDeletionPlan(&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: "instance", UID: "instance-uid", Labels: labels}}, &workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: "foundation", UID: "foundation-uid", Labels: labels}}, "cluster-uid")
	if plan.Blocked {
		t.Fatalf("plan blocked: %v", plan.Reason)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Name != "instance" || plan.Steps[1].Name != "foundation" {
		t.Fatalf("steps = %#v", plan.Steps)
	}
}

func TestDeletionPlanBlocksForeignWork(t *testing.T) {
	foreign := &workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: "instance", UID: "foreign"}}
	plan := BuildDeletionPlan(foreign, nil, "cluster-uid")
	if !plan.Blocked {
		t.Fatal("expected foreign work to block cleanup")
	}
}

func TestValidateWorkReferenceRejectsUIDReplacement(t *testing.T) {
	work := &workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: "instance", UID: "new-uid", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "cluster-uid"}}}
	if err := ValidateWorkReference(work, "cluster-uid", "old-uid"); err == nil {
		t.Fatal("replaced Work UID was accepted")
	}
}

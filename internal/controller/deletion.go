package controller

import "fmt"

import workv1 "open-cluster-management.io/api/work/v1"

type DeletionStep struct{ Name string }
type DeletionPlan struct {
	Steps   []DeletionStep
	Blocked bool
	Reason  string
}

func BuildDeletionPlan(instance, foundation *workv1.ManifestWork, expectedUID string) DeletionPlan {
	plan := DeletionPlan{}
	for _, work := range []*workv1.ManifestWork{instance, foundation} {
		if work == nil {
			continue
		}
		if expectedUID == "" || work.Labels["kubecell.io/virtual-cluster-uid"] != expectedUID {
			plan.Blocked = true
			plan.Reason = "foreign or UID-mismatched Work"
			return plan
		}
		plan.Steps = append(plan.Steps, DeletionStep{Name: work.Name})
	}
	return plan
}

func ValidateWorkReference(work *workv1.ManifestWork, expectedClusterUID, expectedWorkUID string) error {
	if work == nil {
		return fmt.Errorf("Work is nil")
	}
	if work.Labels["kubecell.io/virtual-cluster-uid"] != expectedClusterUID {
		return fmt.Errorf("Work %s/%s has foreign VirtualCluster UID", work.Namespace, work.Name)
	}
	if expectedWorkUID != "" && string(work.UID) != expectedWorkUID {
		return fmt.Errorf("Work %s/%s UID changed from %q to %q", work.Namespace, work.Name, expectedWorkUID, work.UID)
	}
	return nil
}

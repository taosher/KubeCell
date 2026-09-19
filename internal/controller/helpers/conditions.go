package helpers

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func SetCondition(conditions []metav1.Condition, desired metav1.Condition, now metav1.Time) []metav1.Condition {
	if desired.LastTransitionTime.IsZero() {
		desired.LastTransitionTime = now
	}
	for index := range conditions {
		if conditions[index].Type != desired.Type {
			continue
		}
		if conditions[index].Status == desired.Status {
			desired.LastTransitionTime = conditions[index].LastTransitionTime
		}
		conditions[index] = desired
		return conditions
	}
	return append(conditions, desired)
}

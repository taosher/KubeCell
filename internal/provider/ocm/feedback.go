package ocm

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

type WorkSummary struct {
	Applied     bool
	Progressing bool
	Failed      bool
	Message     string
}

func SummarizeWork(work *workv1.ManifestWork) WorkSummary {
	if work == nil {
		return WorkSummary{}
	}
	summary := WorkSummary{}
	for _, condition := range work.Status.Conditions {
		switch condition.Type {
		case workv1.WorkApplied:
			summary.Applied = condition.Status == metav1.ConditionTrue
		case workv1.WorkProgressing:
			summary.Progressing = condition.Status == metav1.ConditionTrue
		case workv1.WorkDegraded:
			summary.Failed = condition.Status == metav1.ConditionTrue
			if condition.Message != "" {
				summary.Message = condition.Message
			}
		}
	}
	return summary
}

func FeedbackValues(work *workv1.ManifestWork, group, resourceName, name, namespace string) []workv1.FeedbackValue {
	if work == nil {
		return nil
	}
	for _, manifest := range work.Status.ResourceStatus.Manifests {
		meta := manifest.ResourceMeta
		if meta.Group == group && meta.Resource == resourceName && meta.Name == name && meta.Namespace == namespace {
			return manifest.StatusFeedbacks.Values
		}
	}
	return nil
}

func FeedbackValue(values []workv1.FeedbackValue, name string) (workv1.FieldValue, bool) {
	for _, value := range values {
		if value.Name == name {
			return value.Value, true
		}
	}
	return workv1.FieldValue{}, false
}

func FeedbackString(values []workv1.FeedbackValue, name string) (string, bool) {
	value, found := FeedbackValue(values, name)
	if !found || value.Type != workv1.String || value.String == nil {
		return "", false
	}
	return *value.String, true
}

func FeedbackInt64(values []workv1.FeedbackValue, name string) (int64, bool) {
	value, found := FeedbackValue(values, name)
	if !found || value.Type != workv1.Integer || value.Integer == nil {
		return 0, false
	}
	return *value.Integer, true
}

func FeedbackResourceList(values []workv1.FeedbackValue, name string) (corev1.ResourceList, error) {
	value, found := FeedbackValue(values, name)
	if !found {
		return nil, nil
	}
	if value.Type != workv1.JsonRaw || value.JsonRaw == nil {
		return nil, fmt.Errorf("feedback %q is not JsonRaw", name)
	}
	result := corev1.ResourceList{}
	if err := json.Unmarshal([]byte(*value.JsonRaw), &result); err != nil {
		return nil, fmt.Errorf("decode feedback %q: %w", name, err)
	}
	return result, nil
}

func ResourceListFeedbackValue(name string, values corev1.ResourceList) workv1.FeedbackValue {
	raw, _ := json.Marshal(values)
	rawString := string(raw)
	return workv1.FeedbackValue{Name: name, Value: workv1.FieldValue{Type: workv1.JsonRaw, JsonRaw: &rawString}}
}

func QuantityFeedbackValue(name string, value resource.Quantity) workv1.FeedbackValue {
	integer, ok := value.AsInt64()
	if ok {
		return workv1.FeedbackValue{Name: name, Value: workv1.FieldValue{Type: workv1.Integer, Integer: &integer}}
	}
	stringValue := value.String()
	return workv1.FeedbackValue{Name: name, Value: workv1.FieldValue{Type: workv1.String, String: &stringValue}}
}

package ocm

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	workv1 "open-cluster-management.io/api/work/v1"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/controller/helpers"
	"github.com/kubecell/kubecell/internal/platform"
	"github.com/kubecell/kubecell/internal/render"
)

func BuildFoundationWork(cluster api.VirtualCluster, resolved api.ResolvedSnapshot, names render.Names, managedClusterName string) (*workv1.ManifestWork, error) {
	objects, err := render.RenderFoundation(cluster, resolved, names)
	if err != nil {
		return nil, fmt.Errorf("render foundation: %w", err)
	}
	manifests, err := manifestsFor(objects)
	if err != nil {
		return nil, err
	}
	return &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{APIVersion: workv1.GroupVersion.String(), Kind: "ManifestWork"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.FoundationWork,
			Namespace: managedClusterName,
			Labels:    workLabels(cluster, resolved),
		},
		Spec: workv1.ManifestWorkSpec{Workload: workv1.ManifestsTemplate{Manifests: manifests}},
	}, nil
}

func BuildInstanceWork(cluster api.VirtualCluster, resolved api.ResolvedSnapshot, names render.Names, managedClusterName string, mirrored []*unstructured.Unstructured) (*workv1.ManifestWork, error) {
	k3k, err := render.RenderK3kCluster(cluster, resolved, names)
	if err != nil {
		return nil, fmt.Errorf("render K3k Cluster: %w", err)
	}
	policy := renderVirtualClusterPolicy(resolved, names, workLabels(cluster, resolved))
	manifests, err := manifestsFor(append([]*unstructured.Unstructured{k3k, policy}, mirrored...))
	if err != nil {
		return nil, err
	}
	return &workv1.ManifestWork{
		TypeMeta:   metav1.TypeMeta{APIVersion: workv1.GroupVersion.String(), Kind: "ManifestWork"},
		ObjectMeta: metav1.ObjectMeta{Name: names.InstanceWork, Namespace: managedClusterName, Labels: workLabels(cluster, resolved)},
		Spec:       workv1.ManifestWorkSpec{Workload: workv1.ManifestsTemplate{Manifests: manifests}, ManifestConfigs: instanceFeedback(names)},
	}, nil
}

func renderVirtualClusterPolicy(resolved api.ResolvedSnapshot, names render.Names, labels map[string]string) *unstructured.Unstructured {
	spec := map[string]any{
		"allowedMode": "shared",
		"quota": map[string]any{
			"hard": quantityMap(resolved.Quota),
		},
		"podSecurityAdmissionLevel": platform.PodSecurityEnforceLevel,
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "k3k.io/v1beta1",
		"kind":       "VirtualClusterPolicy",
		"metadata":   map[string]any{"name": names.K3kCluster, "labels": labels},
		"spec":       spec,
	}}
}

func quantityMap(values api.ResourceList) map[string]any {
	result := map[string]any{}
	for key, value := range values {
		result[string(key)] = value.String()
	}
	return result
}

func foundationFeedback(names render.Names) []workv1.ManifestConfigOption {
	return []workv1.ManifestConfigOption{{
		ResourceIdentifier: workv1.ResourceIdentifier{Resource: "resourcequotas", Name: "kubecell-workload", Namespace: names.HostNamespace},
		FeedbackRules:      []workv1.FeedbackRule{{Type: workv1.JSONPathsType, JsonPaths: []workv1.JsonPath{{Name: "hard", Path: ".status.hard"}, {Name: "used", Path: ".status.used"}}}},
	}}
}

func instanceFeedback(names render.Names) []workv1.ManifestConfigOption {
	return []workv1.ManifestConfigOption{
		{ResourceIdentifier: workv1.ResourceIdentifier{Group: "k3k.io", Resource: "clusters", Name: names.K3kCluster, Namespace: names.HostNamespace}, FeedbackRules: []workv1.FeedbackRule{{Type: workv1.JSONPathsType, JsonPaths: []workv1.JsonPath{{Name: "phase", Path: ".status.phase"}}}}},
		{ResourceIdentifier: workv1.ResourceIdentifier{Resource: "services", Name: "k3k-" + names.K3kCluster + "-service", Namespace: names.HostNamespace}, FeedbackRules: []workv1.FeedbackRule{{Type: workv1.JSONPathsType, JsonPaths: []workv1.JsonPath{{Name: "nodePort", Path: ".spec.ports[0].nodePort"}}}}},
	}
}

func IsApplied(work *workv1.ManifestWork) bool {
	return hasCondition(work.Status.Conditions, workv1.WorkApplied, metav1.ConditionTrue)
}

func IsDegraded(work *workv1.ManifestWork) bool {
	return hasCondition(work.Status.Conditions, workv1.WorkDegraded, metav1.ConditionTrue)
}

func manifestFor(object *unstructured.Unstructured) (workv1.Manifest, error) {
	raw, err := json.Marshal(object.Object)
	if err != nil {
		return workv1.Manifest{}, fmt.Errorf("marshal manifest %s/%s: %w", object.GetKind(), object.GetName(), err)
	}
	return workv1.Manifest{RawExtension: runtime.RawExtension{Raw: raw}}, nil
}

func manifestsFor(objects []*unstructured.Unstructured) ([]workv1.Manifest, error) {
	result := make([]workv1.Manifest, 0, len(objects))
	for _, object := range objects {
		manifest, err := manifestFor(object)
		if err != nil {
			return nil, err
		}
		result = append(result, manifest)
	}
	return result, nil
}

func workLabels(cluster api.VirtualCluster, resolved api.ResolvedSnapshot) map[string]string {
	return helpers.OwnedHostLabels(cluster.Spec.CellRef.Name, cluster.Namespace, cluster.Name, string(cluster.UID), resolved.ProfileHash)
}

func hasCondition(conditions []metav1.Condition, conditionType string, status metav1.ConditionStatus) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType && condition.Status == status {
			return true
		}
	}
	return false
}

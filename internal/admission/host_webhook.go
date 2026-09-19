package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	labelVirtualClusterUID     = "kubecell.io/virtual-cluster-uid"
	labelProfileLabelKey       = "kubecell.io/profile-label-key"
	labelProfileName           = "kubecell.io/profile-name"
	annotationStorageClass     = "kubecell.io/storage-class-names"
	annotationAllowedResources = "kubecell.io/allowed-resource-names"
)

type HostWorkloadWebhook struct {
	Client  client.Client
	Decoder admission.Decoder
}

func RegisterHostWorkloadWebhook(server webhook.Server, kubeClient client.Client, scheme *runtime.Scheme) error {
	if server == nil || kubeClient == nil || scheme == nil {
		return fmt.Errorf("Host workload webhook requires server, client, and scheme")
	}
	server.Register("/mutate-v1-pod", &webhook.Admission{Handler: &HostWorkloadWebhook{
		Client:  kubeClient,
		Decoder: admission.NewDecoder(scheme),
	}})
	return nil
}

func (h *HostWorkloadWebhook) Handle(ctx context.Context, request admission.Request) admission.Response {
	if h.Client == nil || h.Decoder == nil {
		return admission.Errored(500, fmt.Errorf("Host workload webhook is not configured"))
	}
	pod := &corev1.Pod{}
	if err := h.Decoder.Decode(request, pod); err != nil {
		return admission.Errored(400, err)
	}
	namespace := &corev1.Namespace{}
	if err := h.Client.Get(ctx, client.ObjectKey{Name: request.Namespace}, namespace); err != nil {
		return admission.Errored(500, fmt.Errorf("read workload namespace: %w", err))
	}
	uid := namespace.Labels[labelVirtualClusterUID]
	if uid == "" {
		return admission.Allowed("namespace is outside KubeCell policy")
	}
	reflectedSystem, resourceErr := ParseReflectedSystemResources(namespace.Annotations)
	if resourceErr != nil {
		return admission.Denied(resourceErr.Error())
	}
	policy := Policy{
		Namespace:            request.Namespace,
		VirtualClusterUID:    uid,
		ProfileLabelKey:      namespace.Annotations[labelProfileLabelKey],
		ProfileName:          namespace.Labels[labelProfileName],
		StorageClassNames:    splitCSV(namespace.Annotations[annotationStorageClass]),
		AllowedResourceNames: resourceNames(splitCSV(namespace.Annotations[annotationAllowedResources])),
		K3kClusterName:       namespace.Labels[k3kPolicyNameLabel],
		ReflectedSystem:      reflectedSystem,
	}
	if policy.ProfileLabelKey == "" || policy.ProfileName == "" {
		return admission.Denied("KubeCell workload policy is incomplete")
	}
	if isK3kInfrastructurePod(pod, policy) {
		inherited := MutateK3kInfraPodResources(pod)
		if ctrllog.Log.Enabled() {
			seen, _ := json.Marshal(pod.Spec.Resources)
			ctrllog.Log.Info("K3k infrastructure Pod admission", "pod", request.Namespace+"/"+request.Name, "inherited", inherited, "seenPodResources", string(seen))
		}
		if inherited {
			return hostPodPatchResponse(request, pod, "K3k infrastructure Pod inherited pod-level resources")
		}
		return admission.Allowed("K3k infrastructure Pod is outside reflected workload policy")
	}
	if isReflectedSystemPod(pod, policy) {
		if _, err := MutateReflectedSystemPodForPolicy(pod, policy); err != nil {
			return admission.Denied(err.Error())
		}
		return hostPodPatchResponse(request, pod, "reserved reflected system resources and resolved profile affinity applied")
	}
	storageNodes, storageErr := h.resolveStorageNodes(ctx, request.Namespace, pod, policy)
	if storageErr != nil {
		return admission.Denied(storageErr.Error())
	}
	policy.AllowedStorageNodes = storageNodes
	if violations := ValidateReflectedPodWithoutPlacement(pod, policy); len(violations) != 0 {
		return admission.Denied(fmt.Sprintf("Host workload rejected: %v", violations))
	}
	if _, err := MutateReflectedPod(pod, policy); err != nil {
		return admission.Denied(err.Error())
	}
	return hostPodPatchResponse(request, pod, "resolved profile and storage affinity applied")
}

func hostPodPatchResponse(request admission.Request, pod *corev1.Pod, message string) admission.Response {
	updated, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(500, err)
	}
	response := admission.PatchResponseFromRaw(request.Object.Raw, updated)
	if response.Result != nil {
		response.Result.Message = message
	}
	return response
}

func resourceNames(values []string) []corev1.ResourceName {
	result := make([]corev1.ResourceName, 0, len(values))
	for _, value := range values {
		result = append(result, corev1.ResourceName(value))
	}
	return result
}

func profileAffinity(policy Policy) *corev1.Affinity {
	return &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
		NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{
			Key: policy.ProfileLabelKey, Operator: corev1.NodeSelectorOpIn, Values: []string{policy.ProfileName},
		}}}},
	}}}
}

func (h *HostWorkloadWebhook) resolveStorageNodes(ctx context.Context, namespace string, pod *corev1.Pod, policy Policy) ([]string, error) {
	if len(policy.StorageClassNames) == 0 {
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil {
				return nil, fmt.Errorf("PVC %s has no resolved StorageClass mapping", volume.PersistentVolumeClaim.ClaimName)
			}
		}
		return nil, nil
	}
	nodes := sets.New[string]()
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue
		}
		claim := &corev1.PersistentVolumeClaim{}
		if err := h.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: volume.PersistentVolumeClaim.ClaimName}, claim); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("PVC %s/%s is not visible to Host admission", namespace, volume.PersistentVolumeClaim.ClaimName)
			}
			return nil, fmt.Errorf("read PVC %s/%s: %w", namespace, volume.PersistentVolumeClaim.ClaimName, err)
		}
		if claim.Spec.StorageClassName == nil || !containsString(policy.StorageClassNames, *claim.Spec.StorageClassName) {
			return nil, fmt.Errorf("PVC %s/%s does not use an approved StorageClass", namespace, claim.Name)
		}
		if claim.Spec.VolumeName == "" {
			continue
		}
		pv := &corev1.PersistentVolume{}
		if err := h.Client.Get(ctx, types.NamespacedName{Name: claim.Spec.VolumeName}, pv); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("PV %s for PVC %s/%s is not visible to Host admission", claim.Spec.VolumeName, namespace, claim.Name)
			}
			return nil, fmt.Errorf("read PV %s: %w", claim.Spec.VolumeName, err)
		}
		if node, found := storageNode(pv); found {
			nodes.Insert(node)
		}
	}
	return sets.List(nodes), nil
}

func storageNode(pv *corev1.PersistentVolume) (string, bool) {
	if pv == nil || pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		return "", false
	}
	for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
		for _, expression := range term.MatchExpressions {
			if expression.Key == corev1.LabelHostname && expression.Operator == corev1.NodeSelectorOpIn && len(expression.Values) == 1 {
				return expression.Values[0], true
			}
		}
	}
	return "", false
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	values := strings.Split(value, ",")
	result := make([]string, 0, len(values))
	for _, item := range values {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

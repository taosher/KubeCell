package admission

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kubecell/kubecell/internal/controller/helpers"
	"github.com/kubecell/kubecell/internal/platform"
)

type Policy struct {
	Namespace            string
	VirtualClusterUID    string
	ProfileLabelKey      string
	ProfileName          string
	AllowedStorageNodes  []string
	StorageClassNames    []string
	AllowedResourceNames []corev1.ResourceName
	K3kClusterName       string
	ReflectedSystem      corev1.ResourceList
}

const (
	k3kClusterNameLabel  = "k3k.io/clusterName"
	k3kAgentNameLabel    = "k3k.io/agentName"
	k3kResourceNamespace = "k3k.io/namespace"
	k3kPolicyNameLabel   = "policy.k3k.io/policy-name"
	childSystemNamespace = "kube-system"
)

func ValidateReflectedPod(pod *corev1.Pod, policy Policy) []string {
	violations := make([]string, 0)
	if pod == nil {
		return []string{"pod is nil"}
	}
	if pod.Namespace != policy.Namespace {
		violations = append(violations, "pod namespace is outside resolved policy")
	}
	if pod.Labels["kubecell.io/virtual-cluster-uid"] != policy.VirtualClusterUID && !isReflectedWorkloadPod(pod, policy) {
		violations = append(violations, "virtual cluster UID does not match resolved policy")
	}
	if isReflectedWorkloadPod(pod, policy) && pod.Labels[k3kClusterNameLabel] != policy.K3kClusterName {
		violations = append(violations, "reflected Pod K3k cluster does not match resolved policy")
	}
	if pod.Spec.HostNetwork {
		violations = append(violations, "hostNetwork is forbidden")
	}
	if pod.Spec.HostPID {
		violations = append(violations, "hostPID is forbidden")
	}
	if pod.Spec.HostIPC {
		violations = append(violations, "hostIPC is forbidden")
	}
	if pod.Spec.HostUsers != nil && !*pod.Spec.HostUsers {
		violations = append(violations, "host user namespace is forbidden")
	}
	if pod.Spec.NodeName != "" {
		violations = append(violations, "physical nodeName is forbidden")
	}
	if pod.Spec.NodeSelector != nil {
		violations = append(violations, "physical nodeSelector is forbidden")
	}
	violations = append(violations, validateAffinity(pod.Spec.Affinity, pod, policy)...)
	for _, toleration := range pod.Spec.Tolerations {
		if !isK3kGeneratedToleration(toleration) {
			violations = append(violations, "arbitrary tolerations are forbidden")
		}
	}
	for _, volume := range pod.Spec.Volumes {
		if volume.HostPath != nil {
			// hostPath is always rejected; there is no passthrough or rewrite (§6.7).
			violations = append(violations, platform.HostPathDenyMessage)
		}
	}
	for _, container := range append(append([]corev1.Container{}, pod.Spec.InitContainers...), pod.Spec.Containers...) {
		if container.SecurityContext != nil {
			if container.SecurityContext.Privileged != nil && *container.SecurityContext.Privileged {
				violations = append(violations, "privileged containers are forbidden")
			}
			if container.SecurityContext.Capabilities != nil && len(container.SecurityContext.Capabilities.Add) > 0 {
				violations = append(violations, "added capabilities are forbidden")
			}
		}
		for _, port := range container.Ports {
			if port.HostPort != 0 {
				violations = append(violations, "hostPort is forbidden")
			}
		}
		violations = append(violations, validateDeviceResources(container.Resources, policy)...)
	}
	return violations
}

func isReflectedWorkloadPod(pod *corev1.Pod, policy Policy) bool {
	return pod != nil && policy.K3kClusterName != "" && pod.Labels[k3kClusterNameLabel] != "" && pod.Annotations[k3kResourceNamespace] != ""
}

func isK3kInfrastructurePod(pod *corev1.Pod, policy Policy) bool {
	if pod == nil || policy.K3kClusterName == "" {
		return false
	}
	return pod.Labels["cluster"] == policy.K3kClusterName && (pod.Labels["role"] == "server" || pod.Labels["type"] == "agent")
}

// MutateK3kInfraPodResources propagates pod-level reservations of native K3k Pods down to containers (§5):
// K3k server/agent use pod-level requests/limits plus empty containers, while LimitRange already filled
// container defaults before the webhook runs. Rules: empty containers inherit directly; values exactly equal to the platform
// defaults (LimitRange fingerprint) are overwritten with the pod-level values; other declared values are untouched. Translated/reflected
// Pods are excluded and still defaulted by LimitRange.
func MutateK3kInfraPodResources(pod *corev1.Pod) bool {
	if pod == nil || pod.Spec.Resources == nil || len(pod.Spec.Resources.Requests) == 0 || len(pod.Spec.Resources.Limits) == 0 {
		return false
	}
	defaults := platform.DefaultContainerResources()
	changed := false
	inherit := func(container *corev1.Container) {
		if len(container.Resources.Requests) == 0 && len(container.Resources.Limits) == 0 {
			container.Resources.Requests = pod.Spec.Resources.Requests.DeepCopy()
			container.Resources.Limits = pod.Spec.Resources.Limits.DeepCopy()
			changed = true
			return
		}
		if resourcesEqual(container.Resources.Requests, defaults) && resourcesEqual(container.Resources.Limits, defaults) {
			container.Resources.Requests = pod.Spec.Resources.Requests.DeepCopy()
			container.Resources.Limits = pod.Spec.Resources.Limits.DeepCopy()
			changed = true
		}
	}
	for i := range pod.Spec.InitContainers {
		inherit(&pod.Spec.InitContainers[i])
	}
	for i := range pod.Spec.Containers {
		inherit(&pod.Spec.Containers[i])
	}
	return changed
}

func resourcesEqual(left corev1.ResourceList, right corev1.ResourceList) bool {
	if len(left) != len(right) {
		return false
	}
	for name, want := range right {
		got, found := left[name]
		if !found || !got.Equal(want) {
			return false
		}
	}
	return true
}

func isReflectedSystemPod(pod *corev1.Pod, policy Policy) bool {
	return pod != nil && policy.K3kClusterName != "" && pod.Labels[k3kClusterNameLabel] == policy.K3kClusterName && pod.Annotations[k3kResourceNamespace] == childSystemNamespace
}

func MutateReflectedSystemPod(pod *corev1.Pod, reservation corev1.ResourceList) bool {
	if pod == nil || pod.Annotations[k3kResourceNamespace] != childSystemNamespace || pod.Labels[k3kClusterNameLabel] == "" || len(reservation) == 0 {
		return false
	}
	changed := false
	mutate := func(container *corev1.Container) {
		if container.Resources.Requests == nil {
			container.Resources.Requests = corev1.ResourceList{}
		}
		if container.Resources.Limits == nil {
			container.Resources.Limits = corev1.ResourceList{}
		}
		for _, name := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
			reservationQuantity, found := reservation[name]
			if !found {
				continue
			}
			request, hasRequest := container.Resources.Requests[name]
			limit, hasLimit := container.Resources.Limits[name]
			if !hasRequest && !hasLimit {
				container.Resources.Requests[name] = reservationQuantity.DeepCopy()
				container.Resources.Limits[name] = reservationQuantity.DeepCopy()
				changed = true
				continue
			}
			if !hasRequest {
				request = reservationQuantity.DeepCopy()
				if request.Cmp(limit) > 0 {
					request = limit.DeepCopy()
				}
				container.Resources.Requests[name] = request
				changed = true
			}
			if !hasLimit {
				limit = reservationQuantity.DeepCopy()
				if limit.Cmp(request) < 0 {
					limit = request.DeepCopy()
				}
				container.Resources.Limits[name] = limit
				changed = true
			}
		}
	}
	for index := range pod.Spec.InitContainers {
		mutate(&pod.Spec.InitContainers[index])
	}
	for index := range pod.Spec.Containers {
		mutate(&pod.Spec.Containers[index])
	}
	return changed
}

func MutateReflectedSystemPodForPolicy(pod *corev1.Pod, policy Policy) (bool, error) {
	if pod == nil {
		return false, fmt.Errorf("pod is nil")
	}
	if !isReflectedSystemPod(pod, policy) {
		return false, fmt.Errorf("reflected system Pod does not match resolved K3k cluster")
	}
	changed := MutateReflectedSystemPod(pod, policy.ReflectedSystem)
	affinityChanged := addProfileAffinity(pod, policy)
	return changed || affinityChanged, nil
}

func addProfileAffinity(pod *corev1.Pod, policy Policy) bool {
	desired := profileAffinity(policy).NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if pod.Spec.Affinity == nil {
		pod.Spec.Affinity = &corev1.Affinity{}
	}
	if pod.Spec.Affinity.NodeAffinity == nil {
		pod.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}
	if pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = desired
		return true
	}
	required := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if len(required.NodeSelectorTerms) == 0 {
		required.NodeSelectorTerms = desired.NodeSelectorTerms
		return true
	}
	changed := false
	for index := range required.NodeSelectorTerms {
		if !hasRequirement(required.NodeSelectorTerms[index], policy.ProfileLabelKey, policy.ProfileName) {
			required.NodeSelectorTerms[index].MatchExpressions = append(required.NodeSelectorTerms[index].MatchExpressions, desired.NodeSelectorTerms[0].MatchExpressions[0])
			changed = true
		}
	}
	return changed
}

func ParseReflectedSystemResources(annotations map[string]string) (corev1.ResourceList, error) {
	result := corev1.ResourceList{}
	for name, annotation := range map[corev1.ResourceName]string{
		corev1.ResourceCPU:    helpers.AnnotationReflectedSystemCPU,
		corev1.ResourceMemory: helpers.AnnotationReflectedSystemMemory,
	} {
		value := strings.TrimSpace(annotations[annotation])
		if value == "" {
			continue
		}
		quantity, err := resource.ParseQuantity(value)
		if err != nil || quantity.Sign() <= 0 {
			if err == nil {
				err = fmt.Errorf("quantity must be positive")
			}
			return nil, fmt.Errorf("invalid %s annotation %q: %w", annotation, value, err)
		}
		result[name] = quantity
	}
	return result, nil
}

func validateDeviceResources(resources corev1.ResourceRequirements, policy Policy) []string {
	violations := make([]string, 0)
	seen := map[corev1.ResourceName]struct{}{}
	for name := range resources.Requests {
		seen[name] = struct{}{}
	}
	for name := range resources.Limits {
		seen[name] = struct{}{}
	}
	for name := range seen {
		if !isExtendedDevice(name) {
			continue
		}
		if len(policy.AllowedResourceNames) > 0 && !containsResourceName(policy.AllowedResourceNames, name) {
			violations = append(violations, fmt.Sprintf("device %s is not allowed by the resolved profile", name))
			continue
		}
		request, requested := resources.Requests[name]
		limit, limited := resources.Limits[name]
		if !requested || !limited {
			violations = append(violations, fmt.Sprintf("device %s must define both request and limit", name))
			continue
		}
		if !request.Equal(limit) {
			violations = append(violations, fmt.Sprintf("device request and limit differ for %s", name))
		}
		if _, ok := request.AsInt64(); !ok || request.Sign() <= 0 {
			violations = append(violations, fmt.Sprintf("device %s must use a positive integer quantity", name))
		}
	}
	return violations
}

func containsResourceName(values []corev1.ResourceName, target corev1.ResourceName) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func isExtendedDevice(name corev1.ResourceName) bool {
	return strings.Contains(string(name), "/")
}

func MutateReflectedPod(pod *corev1.Pod, policy Policy) (bool, error) {
	if pod == nil {
		return false, fmt.Errorf("pod is nil")
	}
	if violations := ValidateReflectedPodWithoutPlacement(pod, policy); len(violations) != 0 {
		return false, fmt.Errorf("pod rejected: %v", violations)
	}
	return addProfileAffinity(pod, policy), nil
}

func ValidateReflectedPodWithoutPlacement(pod *corev1.Pod, policy Policy) []string {
	// Note: never clear Spec.NodeName before validation here (KI-2). Reflected Pods arrive at admission
	// without a nodeName (filled in by the scheduler); any preset nodeName is rejected,
	// otherwise tenants could bypass profile affinity and bind directly to physical nodes.
	return ValidateReflectedPod(pod, policy)
}

func validateAffinity(affinity *corev1.Affinity, pod *corev1.Pod, policy Policy) []string {
	if affinity == nil {
		return nil
	}
	violations := make([]string, 0)
	if affinity.PodAffinity != nil || affinity.PodAntiAffinity != nil {
		violations = append(violations, "pod affinity and anti-affinity are forbidden")
	}
	if affinity.NodeAffinity == nil {
		return violations
	}
	for _, preferred := range affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
		if !isK3kGeneratedPreferredAffinity(preferred, pod) {
			violations = append(violations, "preferred node affinity is not K3k-generated")
		}
	}
	required := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if required == nil {
		return violations
	}
	for _, term := range required.NodeSelectorTerms {
		for _, expression := range term.MatchExpressions {
			switch expression.Key {
			case policy.ProfileLabelKey:
				if expression.Operator != corev1.NodeSelectorOpIn || len(expression.Values) != 1 || expression.Values[0] != policy.ProfileName {
					violations = append(violations, "profile node affinity conflicts with resolved profile")
				}
			case corev1.LabelHostname:
				if !allowedStorageNodeValues(expression, policy.AllowedStorageNodes) {
					violations = append(violations, "node affinity references an unapproved storage locality")
				}
			default:
				violations = append(violations, "physical node affinity override is forbidden")
			}
		}
	}
	return violations
}

func isK3kGeneratedPreferredAffinity(preferred corev1.PreferredSchedulingTerm, pod *corev1.Pod) bool {
	if pod == nil || pod.Labels[k3kAgentNameLabel] == "" || preferred.Weight != 100 {
		return false
	}
	preference := preferred.Preference
	if len(preference.MatchFields) != 0 || len(preference.MatchExpressions) != 1 {
		return false
	}
	expression := preference.MatchExpressions[0]
	return expression.Key == corev1.LabelHostname &&
		expression.Operator == corev1.NodeSelectorOpIn &&
		len(expression.Values) == 1 &&
		expression.Values[0] == pod.Labels[k3kAgentNameLabel]
}

func isK3kGeneratedToleration(toleration corev1.Toleration) bool {
	if toleration.Operator != corev1.TolerationOpExists ||
		toleration.Effect != corev1.TaintEffectNoExecute ||
		toleration.TolerationSeconds == nil ||
		*toleration.TolerationSeconds != 300 {
		return false
	}
	return toleration.Key == "node.kubernetes.io/not-ready" || toleration.Key == "node.kubernetes.io/unreachable"
}

func allowedStorageNodeValues(expression corev1.NodeSelectorRequirement, allowed []string) bool {
	if expression.Operator != corev1.NodeSelectorOpIn || len(expression.Values) == 0 || len(allowed) == 0 {
		return false
	}
	approved := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		approved[value] = struct{}{}
	}
	for _, value := range expression.Values {
		if _, ok := approved[value]; !ok {
			return false
		}
	}
	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func hasRequirement(term corev1.NodeSelectorTerm, key, value string) bool {
	for _, expression := range term.MatchExpressions {
		if expression.Key == key && expression.Operator == corev1.NodeSelectorOpIn && len(expression.Values) == 1 && expression.Values[0] == value {
			return true
		}
	}
	return false
}

func nodeSelectorEqual(left, right *corev1.NodeSelector) bool {
	if left == nil || right == nil {
		return left == right
	}
	if len(left.NodeSelectorTerms) != len(right.NodeSelectorTerms) {
		return false
	}
	for index := range left.NodeSelectorTerms {
		if len(left.NodeSelectorTerms[index].MatchExpressions) != len(right.NodeSelectorTerms[index].MatchExpressions) {
			return false
		}
		for expressionIndex := range left.NodeSelectorTerms[index].MatchExpressions {
			if left.NodeSelectorTerms[index].MatchExpressions[expressionIndex].Key != right.NodeSelectorTerms[index].MatchExpressions[expressionIndex].Key || left.NodeSelectorTerms[index].MatchExpressions[expressionIndex].Operator != right.NodeSelectorTerms[index].MatchExpressions[expressionIndex].Operator || len(left.NodeSelectorTerms[index].MatchExpressions[expressionIndex].Values) != len(right.NodeSelectorTerms[index].MatchExpressions[expressionIndex].Values) {
				return false
			}
		}
	}
	return true
}

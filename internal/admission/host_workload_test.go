package admission

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateReflectedPodRejectsHostFeaturesAndPhysicalPlacement(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kc-vc", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}}, Spec: corev1.PodSpec{HostNetwork: true, HostPID: true, NodeName: "physical-a", Containers: []corev1.Container{{Name: "work", Image: "example/work", SecurityContext: &corev1.SecurityContext{Privileged: ptr(true)}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{"nvidia.com/gpu": quantity("1")}, Limits: corev1.ResourceList{"nvidia.com/gpu": quantity("2")}}}}}}
	violations := ValidateReflectedPod(pod, Policy{Namespace: "kc-vc", VirtualClusterUID: "vc-1", ProfileLabelKey: "hardware.kubecell.io/profile", ProfileName: "nvidia"})
	if len(violations) < 4 {
		t.Fatalf("violations = %v, want host feature, placement, privilege, device mismatch", violations)
	}
}

func TestMutateReflectedPodInjectsOnlyProfileAffinity(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kc-vc", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "work", Image: "example/work"}}}}
	changed, err := MutateReflectedPod(pod, Policy{Namespace: "kc-vc", VirtualClusterUID: "vc-1", ProfileLabelKey: "hardware.kubecell.io/profile", ProfileName: "nvidia"})
	if err != nil {
		t.Fatalf("MutateReflectedPod() error = %v", err)
	}
	if !changed || pod.Spec.Affinity == nil {
		t.Fatal("expected profile affinity injection")
	}
	if pod.Spec.RuntimeClassName != nil {
		t.Fatal("RuntimeClass must not be injected")
	}
	if got := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]; got != "nvidia" {
		t.Fatalf("profile = %q", got)
	}
}

func TestMutateReflectedSystemPodInjectsMissingCPUAndMemoryWithoutOverwritingExplicitValues(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "kc-vc",
			Labels:      map[string]string{"k3k.io/clusterName": "kc-vc"},
			Annotations: map[string]string{"k3k.io/namespace": "kube-system"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "coredns",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: quantity("100m"), corev1.ResourceMemory: quantity("70Mi")},
				Limits:   corev1.ResourceList{corev1.ResourceMemory: quantity("170Mi")},
			},
		}}},
	}

	changed := MutateReflectedSystemPod(pod, corev1.ResourceList{
		corev1.ResourceCPU:    quantity("100m"),
		corev1.ResourceMemory: quantity("256Mi"),
	})
	if !changed {
		t.Fatal("expected missing CPU limit to be injected")
	}
	container := pod.Spec.Containers[0]
	memoryRequest := container.Resources.Requests[corev1.ResourceMemory]
	if got := memoryRequest.String(); got != "70Mi" {
		t.Fatalf("explicit memory request = %q, want 70Mi", got)
	}
	memoryLimit := container.Resources.Limits[corev1.ResourceMemory]
	if got := memoryLimit.String(); got != "170Mi" {
		t.Fatalf("explicit memory limit = %q, want 170Mi", got)
	}
	cpuLimit := container.Resources.Limits[corev1.ResourceCPU]
	if got := cpuLimit.String(); got != "100m" {
		t.Fatalf("injected CPU limit = %q, want 100m", got)
	}
}

func TestMutateReflectedSystemPodIgnoresOrdinaryReflectedWorkload(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "kc-vc",
			Labels:      map[string]string{"k3k.io/clusterName": "kc-vc"},
			Annotations: map[string]string{"k3k.io/namespace": "default"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "work"}}},
	}

	if changed := MutateReflectedSystemPod(pod, corev1.ResourceList{corev1.ResourceCPU: quantity("100m")}); changed {
		t.Fatal("ordinary reflected workload was mutated as a system Pod")
	}
}

func TestValidateReflectedPodRejectsUnapprovedExtendedResource(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kc-vc", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "work", Image: "example/work", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{"vendor.example/unknown": quantity("1")}, Limits: corev1.ResourceList{"vendor.example/unknown": quantity("1")}}}}}}
	policy := testPolicy()
	policy.AllowedResourceNames = []corev1.ResourceName{"huawei.com/Ascend910"}
	violations := ValidateReflectedPod(pod, policy)
	if !containsViolation(violations, "not allowed") {
		t.Fatalf("violations = %v, want unapproved-resource violation", violations)
	}
}

func TestMutateReflectedPodRepairsEmptyRequiredNodeAffinity(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kc-vc", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}}, Spec: corev1.PodSpec{Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{}}}, Containers: []corev1.Container{{Name: "work", Image: "example/work"}}}}
	changed, err := MutateReflectedPod(pod, testPolicy())
	if err != nil {
		t.Fatalf("MutateReflectedPod() error = %v", err)
	}
	required := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if !changed || required == nil || len(required.NodeSelectorTerms) != 1 {
		t.Fatalf("MutateReflectedPod() did not repair empty required affinity: changed=%v affinity=%#v", changed, pod.Spec.Affinity)
	}
}

func TestValidateReflectedPodRejectsHostPathAndHostPortWithoutPhysicalPlacement(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kc-vc", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "work", Image: "example/work", Ports: []corev1.ContainerPort{{HostPort: 8080}}}},
			Volumes:    []corev1.Volume{{Name: "host", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/"}}}},
		},
	}
	violations := ValidateReflectedPod(pod, testPolicy())
	if !containsViolation(violations, "hostPath") || !containsViolation(violations, "hostPort") {
		t.Fatalf("violations = %v, want HostPath and hostPort violations", violations)
	}
}

func TestValidateReflectedPodRejectsAddedCapabilitiesAndNonIntegerDevice(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kc-vc", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "work", Image: "example/work", SecurityContext: &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"SYS_ADMIN"}},
		}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{"nvidia.com/gpu": quantity("1.5")}, Limits: corev1.ResourceList{"nvidia.com/gpu": quantity("1.5")}}}}},
	}
	violations := ValidateReflectedPod(pod, testPolicy())
	if !containsViolation(violations, "capabilities") || !containsViolation(violations, "integer") {
		t.Fatalf("violations = %v, want capabilities and integer-device violations", violations)
	}
}

func TestValidateReflectedPodRejectsMissingDevicePair(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kc-vc", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "work", Image: "example/work", Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{"huawei.com/Ascend910": quantity("1")},
		}}}},
	}
	violations := ValidateReflectedPod(pod, testPolicy())
	if !containsViolation(violations, "must define both request and limit") {
		t.Fatalf("violations = %v, want missing device pair violation", violations)
	}
}

func TestMutateReflectedPodMergesApprovedStorageLocality(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kc-vc", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}}, Spec: corev1.PodSpec{Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{"node-a"}}}}}}}}, Containers: []corev1.Container{{Name: "work", Image: "example/work"}}}}
	changed, err := MutateReflectedPod(pod, Policy{Namespace: "kc-vc", VirtualClusterUID: "vc-1", ProfileLabelKey: "hardware.kubecell.io/profile", ProfileName: "nvidia", AllowedStorageNodes: []string{"node-a"}})
	if err != nil {
		t.Fatalf("MutateReflectedPod() error = %v", err)
	}
	if !changed {
		t.Fatal("expected profile affinity merge")
	}
	term := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0]
	if !hasNodeSelectorRequirement(term, "hardware.kubecell.io/profile", "nvidia") || !hasNodeSelectorRequirement(term, corev1.LabelHostname, "node-a") {
		t.Fatalf("merged term = %#v", term)
	}
}

func TestValidateReflectedPodRejectsArbitraryAffinityAndToleration(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kc-vc", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}}, Spec: corev1.PodSpec{Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "topology.kubernetes.io/zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"zone-a"}}}}}}}}, Tolerations: []corev1.Toleration{{Operator: corev1.TolerationOpExists}}, Containers: []corev1.Container{{Name: "work", Image: "example/work"}}}}
	violations := ValidateReflectedPod(pod, testPolicy())
	if !containsViolation(violations, "node affinity") || !containsViolation(violations, "toleration") {
		t.Fatalf("violations = %v, want affinity and toleration violations", violations)
	}
}

func TestValidateReflectedPodAllowsK3kGeneratedPlacementDefaults(t *testing.T) {
	tolerationSeconds := int64(300)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "kc-vc",
			Labels: map[string]string{
				"kubecell.io/virtual-cluster-uid": "vc-1",
				"k3k.io/agentName":                "host-01",
			},
		},
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{{
					Weight: 100,
					Preference: corev1.NodeSelectorTerm{MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{"host-01"},
					}}},
				}},
			}},
			Tolerations: []corev1.Toleration{
				{Key: "node.kubernetes.io/not-ready", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute, TolerationSeconds: &tolerationSeconds},
				{Key: "node.kubernetes.io/unreachable", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute, TolerationSeconds: &tolerationSeconds},
			},
			Containers: []corev1.Container{{Name: "work", Image: "example/work"}},
		},
	}
	if violations := ValidateReflectedPod(pod, testPolicy()); len(violations) != 0 {
		t.Fatalf("violations = %v, want no violations", violations)
	}
}

func TestValidateReflectedPodRejectsK3kPlacementOverrides(t *testing.T) {
	tolerationSeconds := int64(300)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "kc-vc",
			Labels: map[string]string{
				"kubecell.io/virtual-cluster-uid": "vc-1",
				"k3k.io/agentName":                "host-01",
			},
		},
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{{
					Weight: 100,
					Preference: corev1.NodeSelectorTerm{MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{"other-node"},
					}}},
				}},
			}},
			Tolerations: []corev1.Toleration{
				{Key: "dedicated", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule, TolerationSeconds: &tolerationSeconds},
			},
			Containers: []corev1.Container{{Name: "work", Image: "example/work"}},
		},
	}
	violations := ValidateReflectedPod(pod, testPolicy())
	if !containsViolation(violations, "preferred node affinity") || !containsViolation(violations, "toleration") {
		t.Fatalf("violations = %v, want placement override violations", violations)
	}
}

func TestValidateReflectedPodWithoutPlacementStillRejectsNodeSelector(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kc-vc", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}}, Spec: corev1.PodSpec{NodeSelector: map[string]string{"kubernetes.io/hostname": "node-a"}, Containers: []corev1.Container{{Name: "work", Image: "example/work"}}}}
	violations := ValidateReflectedPodWithoutPlacement(pod, testPolicy())
	if !containsViolation(violations, "nodeSelector") {
		t.Fatalf("violations = %v, want nodeSelector violation", violations)
	}
}

func hasNodeSelectorRequirement(term corev1.NodeSelectorTerm, key, value string) bool {
	for _, expression := range term.MatchExpressions {
		if expression.Key == key && expression.Operator == corev1.NodeSelectorOpIn {
			for _, candidate := range expression.Values {
				if candidate == value {
					return true
				}
			}
		}
	}
	return false
}

func testPolicy() Policy {
	return Policy{Namespace: "kc-vc", VirtualClusterUID: "vc-1", ProfileLabelKey: "hardware.kubecell.io/profile", ProfileName: "nvidia"}
}

func containsViolation(violations []string, fragment string) bool {
	for _, violation := range violations {
		if strings.Contains(violation, fragment) {
			return true
		}
	}
	return false
}

func ptr[T any](value T) *T                   { return &value }
func quantity(value string) resource.Quantity { return resource.MustParse(value) }

func TestValidateReflectedPodWithoutPlacementRejectsPresetNodeName(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kc-vc", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}}, Spec: corev1.PodSpec{NodeName: "physical-a", Containers: []corev1.Container{{Name: "work", Image: "example/work"}}}}
	violations := ValidateReflectedPodWithoutPlacement(pod, testPolicy())
	if !containsViolation(violations, "nodeName") {
		t.Fatalf("violations = %v, want preset nodeName rejected (KI-2)", violations)
	}
}

func TestMutateK3kInfraPodResourcesInheritsPodLevel(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: quantity("100m"), corev1.ResourceMemory: quantity("256Mi")},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: quantity("100m"), corev1.ResourceMemory: quantity("256Mi")},
		},
		Containers: []corev1.Container{{Name: "agent", Image: "example/agent"}},
	}}
	if !MutateK3kInfraPodResources(pod) {
		t.Fatal("MutateK3kInfraPodResources() = false, want true")
	}
	got := pod.Spec.Containers[0].Resources
	if got.Requests.Cpu().String() != "100m" || got.Limits.Memory().String() != "256Mi" {
		t.Fatalf("container resources = %#v, want inherited pod-level", got)
	}
}

func TestMutateK3kInfraPodResourcesLeavesDeclaredContainersAlone(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: quantity("500m")},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: quantity("500m")},
		},
		Containers: []corev1.Container{{Name: "server", Image: "example/server", Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: quantity("1")},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: quantity("1")},
		}}},
	}}
	if MutateK3kInfraPodResources(pod) {
		t.Fatal("MutateK3kInfraPodResources() = true, want false for declared containers")
	}
	if pod.Spec.Containers[0].Resources.Requests.Cpu().String() != "1" {
		t.Fatalf("declared container resources were overwritten: %#v", pod.Spec.Containers[0].Resources)
	}
}

func TestMutateK3kInfraPodResourcesSkipsPartialPodLevel(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{
		Resources:  &corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: quantity("100m")}},
		Containers: []corev1.Container{{Name: "agent", Image: "example/agent"}},
	}}
	if MutateK3kInfraPodResources(pod) {
		t.Fatal("MutateK3kInfraPodResources() = true, want false for partial pod-level resources")
	}
}

func TestMutateK3kInfraPodResourcesReplacesLimitRangeDefaults(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: quantity("500m"), corev1.ResourceMemory: quantity("1Gi")},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: quantity("500m"), corev1.ResourceMemory: quantity("1Gi")},
		},
		Containers: []corev1.Container{{Name: "server", Image: "example/server", Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: quantity("100m"), corev1.ResourceMemory: quantity("256Mi")},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: quantity("100m"), corev1.ResourceMemory: quantity("256Mi")},
		}}},
	}}
	if !MutateK3kInfraPodResources(pod) {
		t.Fatal("MutateK3kInfraPodResources() = false, want true for LimitRange-filled containers")
	}
	got := pod.Spec.Containers[0].Resources
	if got.Requests.Cpu().String() != "500m" || got.Limits.Memory().String() != "1Gi" {
		t.Fatalf("container resources = %#v, want pod-level values", got)
	}
}

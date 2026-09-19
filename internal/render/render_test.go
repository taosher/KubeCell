package render

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/platform"
)

func testNames() Names {
	return Names{
		HostNamespace:    "kc-team-a-training-01234567",
		K3kCluster:       "kc-01234567",
		HostAPIAddresses: []string{"203.0.113.10"},
		FoundationWork:   "kc-team-a-training-01234567-foundation",
		InstanceWork:     "kc-team-a-training-01234567-instance",
		AdminKubeconfig:  "kubecell-training-01234567-admin-kubeconfig",
	}
}

func testCluster() api.VirtualCluster {
	return api.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "training", Namespace: "team-a", UID: "cluster-uid"},
		Spec: api.VirtualClusterSpec{
			CellRef:  api.ObjectReference{Name: "cell-a"},
			ClassRef: api.ObjectReference{Name: "ascend-small"},
		},
	}
}

func testResolved() api.ResolvedSnapshot {
	return api.ResolvedSnapshot{
		ProfileHash:        "profile-hash-abc",
		MachineProfileName: "ascend-910b",
		K3kChildVersion:    "v1.34.2-k3s1",
		Quota: corev1.ResourceList{
			"requests.cpu":                  resource.MustParse("8"),
			"limits.cpu":                    resource.MustParse("8"),
			"requests.huawei.com/Ascend910": resource.MustParse("1"),
			"limits.huawei.com/Ascend910":   resource.MustParse("1"),
		},
		AcceleratorKeys: []string{"huawei.com/Ascend910"},
		Storage: api.StorageMappingSpec{Class: api.StorageClassMapping{
			ChildName:            "topolvm-provisioner",
			HostName:             "topolvm-provisioner",
			VolumeBindingMode:    storagev1.VolumeBindingWaitForFirstConsumer,
			ReclaimPolicy:        corev1.PersistentVolumeReclaimRetain,
			AllowVolumeExpansion: true,
		}},
	}
}

func TestRenderFoundationCreatesHardHostBoundary(t *testing.T) {
	objects, err := RenderFoundation(testCluster(), testResolved(), testNames())
	if err != nil {
		t.Fatalf("RenderFoundation() error = %v", err)
	}
	if len(objects) != 9 {
		t.Fatalf("foundation object count = %d, want 9", len(objects))
	}

	quota, ok := findObject(objects, "ResourceQuota", "kubecell-workload")
	if !ok {
		t.Fatal("ResourceQuota/kubecell-workload not rendered")
	}
	hard, found, err := unstructured.NestedStringMap(quota.Object, "spec", "hard")
	if err != nil || !found {
		t.Fatalf("quota hard missing: found=%v err=%v", found, err)
	}
	if hard["requests.huawei.com/Ascend910"] != "1" {
		t.Fatalf("device hard quota = %q", hard["requests.huawei.com/Ascend910"])
	}
	if hard["requests.cpu"] != "8" || hard["limits.cpu"] != "8" {
		t.Fatalf("cpu hard quota = %#v", hard)
	}
	if quota.GetNamespace() != testNames().HostNamespace {
		t.Fatalf("quota namespace = %q", quota.GetNamespace())
	}

	defaults, ok := findObject(objects, "LimitRange", "kubecell-defaults")
	if !ok {
		t.Fatal("Host LimitRange kubecell-defaults not rendered")
	}
	if defaults.GetNamespace() != testNames().HostNamespace {
		t.Fatalf("LimitRange namespace = %q", defaults.GetNamespace())
	}
	limits, found, err := unstructured.NestedSlice(defaults.Object, "spec", "limits")
	if err != nil || !found || len(limits) != 1 {
		t.Fatalf("LimitRange limits = %#v, found=%v, err=%v", limits, found, err)
	}
	limit, _ := limits[0].(map[string]any)
	if limit["type"] != "Container" {
		t.Fatalf("LimitRange type = %#v, want Container", limit["type"])
	}
	for _, key := range []string{"default", "defaultRequest"} {
		got, found, err := unstructured.NestedStringMap(limit, key)
		if err != nil || !found || got["cpu"] != "100m" || got["memory"] != "256Mi" {
			t.Fatalf("LimitRange %s = %#v, found=%v, err=%v", key, got, found, err)
		}
	}
	if _, ok := findObject(objects, "NetworkPolicy", "kubecell-default-deny"); !ok {
		t.Fatal("default deny NetworkPolicy not rendered")
	}
	dnsAllow, ok := findObject(objects, "NetworkPolicy", "kubecell-dns-allow")
	if !ok {
		t.Fatal("DNS allow NetworkPolicy not rendered")
	}
	dnsEgress, found, err := unstructured.NestedSlice(dnsAllow.Object, "spec", "egress")
	if err != nil || !found || !containsNetworkPolicyPort(flattenPorts(dnsEgress), int64(53)) {
		t.Fatalf("DNS egress does not allow port 53: %#v", dnsEgress)
	}
	apiAllow, ok := findObject(objects, "NetworkPolicy", "kubecell-child-api-allow")
	if !ok {
		t.Fatal("Child API allow NetworkPolicy not rendered")
	}
	selector, found, err := unstructured.NestedStringMap(apiAllow.Object, "spec", "podSelector", "matchLabels")
	if err != nil || !found || selector["role"] != "server" {
		t.Fatalf("Child API selector = %#v, found=%v, err=%v", selector, found, err)
	}
	serverEgress, found, err := unstructured.NestedSlice(apiAllow.Object, "spec", "egress")
	if err != nil || !found || !containsNetworkPolicyPeerLabelsPortDirection(serverEgress, "to", map[string]string{"cluster": testNames().K3kCluster, "type": "agent"}, int64(10250)) {
		t.Fatalf("Child API egress does not allow shared-agent kubelet proxy traffic: %#v", serverEgress)
	}
	agentAllow, ok := findObject(objects, "NetworkPolicy", "kubecell-shared-agent-egress")
	if !ok {
		t.Fatal("shared agent egress NetworkPolicy not rendered")
	}
	agentSelector, found, err := unstructured.NestedStringMap(agentAllow.Object, "spec", "podSelector", "matchLabels")
	if err != nil || !found || agentSelector["cluster"] != testNames().K3kCluster || agentSelector["type"] != "agent" {
		t.Fatalf("shared agent selector = %#v, found=%v, err=%v", agentSelector, found, err)
	}
	ingress, found, err := unstructured.NestedSlice(agentAllow.Object, "spec", "ingress")
	if err != nil || !found || !containsNetworkPolicyPeerPort(ingress, "role", "server", int64(10250)) {
		t.Fatalf("shared agent ingress does not allow Child API Server kubelet proxy traffic: %#v", ingress)
	}
	egress, found, err := unstructured.NestedSlice(agentAllow.Object, "spec", "egress")
	if err != nil || !found || len(egress) != 3 {
		t.Fatalf("shared agent egress = %#v, found=%v, err=%v", egress, found, err)
	}
	if !containsNetworkPolicyIP(egress, "10.43.0.1/32", int64(443)) {
		t.Fatalf("shared agent egress does not allow child API clusterIP: %#v", egress)
	}
	if !containsNetworkPolicyIP(egress, "203.0.113.10/32", int64(6443)) {
		t.Fatalf("shared agent egress does not allow Host API address: %#v", egress)
	}
	if !containsNetworkPolicyPeerLabelsPortDirection(egress, "to", map[string]string{"role": "server"}, int64(6443)) {
		t.Fatalf("shared agent egress does not allow child API server: %#v", egress)
	}
}

func TestRenderFoundationAddsNarrowPortForwardCompatibilityRBAC(t *testing.T) {
	objects, err := RenderFoundation(testCluster(), testResolved(), testNames())
	if err != nil {
		t.Fatalf("RenderFoundation() error = %v", err)
	}
	role, ok := findObject(objects, "Role", "kubecell-portforward")
	if !ok {
		t.Fatal("KubeCell port-forward Role not rendered")
	}
	rules, found, err := unstructured.NestedSlice(role.Object, "rules")
	if err != nil || !found || !hasRBACRule(rules, "", "pods/portforward", "create") {
		t.Fatalf("port-forward Role rules = %#v, want create on pods/portforward", rules)
	}
	binding, ok := findObject(objects, "RoleBinding", "kubecell-portforward")
	if !ok {
		t.Fatal("KubeCell port-forward RoleBinding not rendered")
	}
	subjects, found, err := unstructured.NestedSlice(binding.Object, "subjects")
	if err != nil || !found || !hasServiceAccountSubject(subjects, "k3k-"+testNames().K3kCluster+"-kubelet", testNames().HostNamespace) {
		t.Fatalf("port-forward subjects = %#v", subjects)
	}
}

func hasRBACRule(rules []interface{}, group, resource, verb string) bool {
	for _, raw := range rules {
		rule, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		groups, _, _ := unstructured.NestedStringSlice(rule, "apiGroups")
		resources, _, _ := unstructured.NestedStringSlice(rule, "resources")
		verbs, _, _ := unstructured.NestedStringSlice(rule, "verbs")
		if containsString(groups, group) && containsString(resources, resource) && containsString(verbs, verb) {
			return true
		}
	}
	return false
}

func hasServiceAccountSubject(subjects []interface{}, name, namespace string) bool {
	for _, raw := range subjects {
		subject, ok := raw.(map[string]interface{})
		if ok && subject["kind"] == "ServiceAccount" && subject["name"] == name && subject["namespace"] == namespace {
			return true
		}
	}
	return false
}

func flattenPorts(rules []interface{}) []interface{} {
	ports := []interface{}{}
	for _, raw := range rules {
		rule, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		items, ok := rule["ports"].([]interface{})
		if !ok {
			continue
		}
		ports = append(ports, items...)
	}
	return ports
}

func containsNetworkPolicyIP(egress []interface{}, cidr string, port int64) bool {
	for _, item := range egress {
		rule, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		ports, ok := rule["ports"].([]interface{})
		if !ok {
			continue
		}
		portMatches := false
		for _, item := range ports {
			portSpec, ok := item.(map[string]interface{})
			if ok && networkPort(portSpec["port"]) == port {
				portMatches = true
			}
		}
		if !portMatches {
			continue
		}
		to, ok := rule["to"].([]interface{})
		if !ok {
			continue
		}
		for _, item := range to {
			peer, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			block, ok := peer["ipBlock"].(map[string]interface{})
			if ok && block["cidr"] == cidr {
				return true
			}
		}
	}
	return false
}

func networkPort(value interface{}) int64 {
	switch port := value.(type) {
	case int64:
		return port
	case int32:
		return int64(port)
	case int:
		return int64(port)
	case float64:
		return int64(port)
	default:
		return -1
	}
}

func containsNetworkPolicyPeerPort(rules []interface{}, labelKey, labelValue string, port int64) bool {
	return containsNetworkPolicyPeerLabelsPort(rules, map[string]string{labelKey: labelValue}, port)
}

func containsNetworkPolicyPeerLabelsPort(rules []interface{}, expectedLabels map[string]string, port int64) bool {
	return containsNetworkPolicyPeerLabelsPortDirection(rules, "from", expectedLabels, port)
}

func containsNetworkPolicyPeerLabelsPortDirection(rules []interface{}, direction string, expectedLabels map[string]string, port int64) bool {
	for _, item := range rules {
		rule, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		ports, ok := rule["ports"].([]interface{})
		if !ok || !containsNetworkPolicyPort(ports, port) {
			continue
		}
		from, ok := rule[direction].([]interface{})
		if !ok {
			continue
		}
		for _, item := range from {
			peer, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			selector, ok := peer["podSelector"].(map[string]interface{})
			if !ok {
				continue
			}
			labels, ok := selector["matchLabels"].(map[string]interface{})
			if !ok {
				continue
			}
			matches := true
			for labelKey, labelValue := range expectedLabels {
				if labels[labelKey] != labelValue {
					matches = false
					break
				}
			}
			if matches {
				return true
			}
		}
	}
	return false
}

func containsNetworkPolicyPort(ports []interface{}, wanted int64) bool {
	for _, item := range ports {
		portSpec, ok := item.(map[string]interface{})
		if ok && networkPort(portSpec["port"]) == wanted {
			return true
		}
	}
	return false
}

func TestRenderFoundationPublishesHostAdmissionPolicyLabels(t *testing.T) {
	objects, err := RenderFoundation(testCluster(), testResolved(), testNames())
	if err != nil {
		t.Fatalf("RenderFoundation() error = %v", err)
	}
	namespace, ok := findObject(objects, "Namespace", testNames().HostNamespace)
	if !ok {
		t.Fatal("Host namespace not rendered")
	}
	labels, found := metadataStrings(namespace, "labels")
	if !found {
		t.Fatal("namespace labels missing")
	}
	if labels["kubecell.io/virtual-cluster-uid"] != "cluster-uid" {
		t.Fatalf("namespace UID label = %q", labels["kubecell.io/virtual-cluster-uid"])
	}
	if labels["kubecell.io/profile-name"] != "ascend-910b" {
		t.Fatalf("profile name = %q", labels["kubecell.io/profile-name"])
	}
	if labels["pod-security.kubernetes.io/enforce"] != platform.PodSecurityEnforceLevel {
		t.Fatalf("PSA enforce label = %q, want %q", labels["pod-security.kubernetes.io/enforce"], platform.PodSecurityEnforceLevel)
	}
	annotations, found := metadataStrings(namespace, "annotations")
	if !found {
		t.Fatal("namespace annotations missing")
	}
	if annotations["kubecell.io/profile-label-key"] != platform.ProfileLabelKey {
		t.Fatalf("profile label key annotation = %q", annotations["kubecell.io/profile-label-key"])
	}
	if annotations["kubecell.io/storage-class-names"] != "topolvm-provisioner" {
		t.Fatalf("storage class annotation = %q", annotations["kubecell.io/storage-class-names"])
	}
	if annotations["kubecell.io/allowed-resource-names"] != "huawei.com/Ascend910" {
		t.Fatalf("allowed resource names annotation = %q", annotations["kubecell.io/allowed-resource-names"])
	}
	if labels["policy.k3k.io/policy-name"] != testNames().K3kCluster {
		t.Fatalf("K3k policy binding label = %q, want %q", labels["policy.k3k.io/policy-name"], testNames().K3kCluster)
	}
}

func TestRenderInstanceUsesK3kSharedModeAndDoesNotRenderReflectedPods(t *testing.T) {
	object, err := RenderK3kCluster(testCluster(), testResolved(), testNames())
	if err != nil {
		t.Fatalf("RenderK3kCluster() error = %v", err)
	}
	if object.GetKind() != "Cluster" || object.GetAPIVersion() != "k3k.io/v1beta1" {
		t.Fatalf("K3k identity = %s/%s", object.GetAPIVersion(), object.GetKind())
	}
	mode, _, _ := unstructured.NestedString(object.Object, "spec", "mode")
	if mode != "shared" {
		t.Fatalf("K3k mode = %q, want shared", mode)
	}
	version, _, _ := unstructured.NestedString(object.Object, "spec", "version")
	if version != "v1.34.2-k3s1" {
		t.Fatalf("K3k version = %q", version)
	}
	if _, found, _ := unstructured.NestedMap(object.Object, "spec", "expose", "nodePort"); !found {
		t.Fatal("NodePort exposure missing")
	}
	serverArgs, found, err := unstructured.NestedStringSlice(object.Object, "spec", "serverArgs")
	if err != nil || !found {
		t.Fatalf("serverArgs missing: found=%v err=%v", found, err)
	}
	for _, wanted := range platform.ServerArgs {
		if !containsString(serverArgs, wanted) {
			t.Fatalf("serverArgs = %#v, want %q", serverArgs, wanted)
		}
	}
	if !containsString(serverArgs, "--tls-san=203.0.113.10") {
		t.Fatalf("serverArgs = %#v, want Cell API address TLS SAN", serverArgs)
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestRenderK3kClusterAppliesControlPlaneReservation(t *testing.T) {
	object, err := RenderK3kCluster(testCluster(), testResolved(), testNames())
	if err != nil {
		t.Fatalf("RenderK3kCluster() error = %v", err)
	}
	requests, found, err := unstructured.NestedStringMap(object.Object, "spec", "serverResources", "requests")
	if err != nil || !found {
		t.Fatalf("server requests = %v, found=%v, err=%v", requests, found, err)
	}
	limits, found, err := unstructured.NestedStringMap(object.Object, "spec", "serverResources", "limits")
	if err != nil || !found {
		t.Fatalf("server limits = %v, found=%v, err=%v", limits, found, err)
	}
	want := platform.ControlPlaneReservation()
	wantCPU := want[corev1.ResourceCPU]
	wantMemory := want[corev1.ResourceMemory]
	if requests[string(corev1.ResourceCPU)] != wantCPU.String() || requests[string(corev1.ResourceMemory)] != wantMemory.String() {
		t.Fatalf("serverResources requests = %v, want %v", requests, want)
	}
	wantLimitCPU := want[corev1.ResourceCPU]
	wantLimitMemory := want[corev1.ResourceMemory]
	if limits[string(corev1.ResourceCPU)] != wantLimitCPU.String() || limits[string(corev1.ResourceMemory)] != wantLimitMemory.String() {
		t.Fatalf("serverResources limits = %v, want %v", limits, want)
	}
}

func TestRenderK3kClusterAppliesReflectedSystemReservationToSharedAgents(t *testing.T) {
	object, err := RenderK3kCluster(testCluster(), testResolved(), testNames())
	if err != nil {
		t.Fatalf("RenderK3kCluster() error = %v", err)
	}
	requests, found, err := unstructured.NestedStringMap(object.Object, "spec", "workerResources", "requests")
	if err != nil || !found {
		t.Fatalf("worker requests = %v, found=%v, err=%v", requests, found, err)
	}
	limits, found, err := unstructured.NestedStringMap(object.Object, "spec", "workerResources", "limits")
	if err != nil || !found {
		t.Fatalf("worker limits = %v, found=%v, err=%v", limits, found, err)
	}
	want := platform.ReflectedSystemReservation()
	wantCPU := want[corev1.ResourceCPU]
	wantMemory := want[corev1.ResourceMemory]
	if requests[string(corev1.ResourceCPU)] != wantCPU.String() || requests[string(corev1.ResourceMemory)] != wantMemory.String() {
		t.Fatalf("worker requests = %v, want %v", requests, want)
	}
	wantLimitCPU := want[corev1.ResourceCPU]
	wantLimitMemory := want[corev1.ResourceMemory]
	if limits[string(corev1.ResourceCPU)] != wantLimitCPU.String() || limits[string(corev1.ResourceMemory)] != wantLimitMemory.String() {
		t.Fatalf("worker limits = %v, want %v", limits, want)
	}
}

func TestRenderK3kClusterPinsPersistentServerStorage(t *testing.T) {
	object, err := RenderK3kCluster(testCluster(), testResolved(), testNames())
	if err != nil {
		t.Fatalf("RenderK3kCluster() error = %v", err)
	}
	persistence, found, err := unstructured.NestedMap(object.Object, "spec", "persistence")
	if err != nil || !found {
		t.Fatalf("persistence = %v, found=%v, err=%v", persistence, found, err)
	}
	if persistence["type"] != "dynamic" {
		t.Fatalf("persistence type = %v, want dynamic", persistence["type"])
	}
	if persistence["storageClassName"] != "topolvm-provisioner" {
		t.Fatalf("persistence storageClassName = %v", persistence["storageClassName"])
	}
	if persistence["storageRequestSize"] != "2Gi" {
		t.Fatalf("persistence storageRequestSize = %v, want 2Gi", persistence["storageRequestSize"])
	}
}

func TestRenderChildStorageClassMapsOnlyApprovedHostClass(t *testing.T) {
	storageClass, err := RenderChildStorageClass(testResolved().Storage.Class.ChildName)
	if err != nil {
		t.Fatalf("RenderChildStorageClass() error = %v", err)
	}
	if storageClass.Name != "topolvm-provisioner" {
		t.Fatalf("child StorageClass name = %q", storageClass.Name)
	}
	if storageClass.Provisioner != "topolvm.io" {
		t.Fatalf("provisioner = %q", storageClass.Provisioner)
	}
	if len(storageClass.Parameters) != 0 {
		t.Fatalf("child storage class parameters = %#v, want no name translation parameters", storageClass.Parameters)
	}
	if storageClass.VolumeBindingMode == nil || *storageClass.VolumeBindingMode != storagev1.VolumeBindingWaitForFirstConsumer {
		t.Fatalf("volume binding mode = %v", storageClass.VolumeBindingMode)
	}
	if storageClass.ReclaimPolicy == nil || *storageClass.ReclaimPolicy != corev1.PersistentVolumeReclaimRetain {
		t.Fatalf("reclaim policy = %v", storageClass.ReclaimPolicy)
	}
	if storageClass.AllowVolumeExpansion == nil || *storageClass.AllowVolumeExpansion != true {
		t.Fatalf("allow volume expansion = %v", storageClass.AllowVolumeExpansion)
	}
}

func TestRenderChildStorageClassRejectsEmptyName(t *testing.T) {
	if _, err := RenderChildStorageClass(""); err == nil {
		t.Fatal("RenderChildStorageClass(\"\") error = nil, want error")
	}
}

func metadataStrings(object *unstructured.Unstructured, key string) (map[string]string, bool) {
	metadata, ok := object.Object["metadata"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	raw, ok := metadata[key]
	if !ok {
		return nil, false
	}
	if values, ok := raw.(map[string]string); ok {
		return values, true
	}
	if values, ok := raw.(map[string]interface{}); ok {
		result := make(map[string]string, len(values))
		for k, v := range values {
			s, ok := v.(string)
			if !ok {
				return nil, false
			}
			result[k] = s
		}
		return result, true
	}
	return nil, false
}

func findObject(objects []*unstructured.Unstructured, kind, name string) (*unstructured.Unstructured, bool) {
	for _, object := range objects {
		if object.GetKind() == kind && object.GetName() == name {
			return object, true
		}
	}
	return nil, false
}

func testMirrorResolved() api.ResolvedSnapshot {
	resolved := testResolved()
	resolved.HostNamespace = "kc-team-a-training-01234567"
	return resolved
}

func testMirrorBackend() ChildIngressBackend {
	class := platform.IngressClassName
	pathType := networkingv1.PathTypePrefix
	return ChildIngressBackend{
		Ingress: networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &class,
				Rules: []networkingv1.IngressRule{{
					Host: "ignored.example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{
						Path:     "/",
						PathType: &pathType,
						Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{
							Name: "web-svc",
							Port: networkingv1.ServiceBackendPort{Number: 80},
						}},
					}}}},
				}},
			},
		},
		Service: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "web-svc", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 80}}},
		},
		Endpoints: &corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "web-svc", Namespace: "default"},
			Subsets: []corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "10.42.0.7"}},
				Ports:     []corev1.EndpointPort{{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP}},
			}},
		},
	}
}

func findMirrorObject(t *testing.T, objects []*unstructured.Unstructured, kind, name string) *unstructured.Unstructured {
	t.Helper()
	for _, object := range objects {
		if object.GetKind() == kind && object.GetName() == name {
			return object
		}
	}
	t.Fatalf("%s %q not rendered", kind, name)
	return nil
}

func TestRenderIngressMirrorComposesHostnameAndMirrorsBackend(t *testing.T) {
	objects, err := RenderIngressMirror(testCluster(), testMirrorResolved(), "apps.example.com", []ChildIngressBackend{testMirrorBackend()})
	if err != nil {
		t.Fatalf("RenderIngressMirror() error = %v", err)
	}
	if len(objects) != 3 {
		t.Fatalf("objects = %d, want Service+Endpoints+Ingress", len(objects))
	}
	service := findMirrorObject(t, objects, "Service", "ing-web-web-svc-80")
	ports, _, _ := unstructured.NestedSlice(service.Object, "spec", "ports")
	if len(ports) != 1 {
		t.Fatalf("service ports = %v", ports)
	}
	endpoints := findMirrorObject(t, objects, "Endpoints", "ing-web-web-svc-80")
	subsets, _, _ := unstructured.NestedSlice(endpoints.Object, "subsets")
	if len(subsets) != 1 {
		t.Fatalf("endpoint subsets = %v", subsets)
	}
	subset, _ := subsets[0].(map[string]any)
	epPorts, _, _ := unstructured.NestedSlice(subset, "ports")
	if len(epPorts) != 1 {
		t.Fatalf("endpoint ports = %v", epPorts)
	}
	if name, _, _ := unstructured.NestedString(epPorts[0].(map[string]any), "name"); name != "" {
		t.Fatalf("mirrored endpoint port name = %q, want empty (§11.3)", name)
	}
	ingress := findMirrorObject(t, objects, "Ingress", "default-web")
	host, _, _ := unstructured.NestedString(ingress.Object, "spec", "rules")
	_ = host
	rules, _, _ := unstructured.NestedSlice(ingress.Object, "spec", "rules")
	rule, _ := rules[0].(map[string]any)
	hostname, _, _ := unstructured.NestedString(rule, "host")
	if hostname != "web.training.apps.example.com" {
		t.Fatalf("hostname = %q, want composed name", hostname)
	}
	class, _, _ := unstructured.NestedString(ingress.Object, "spec", "ingressClassName")
	if class != "kubecell" {
		t.Fatalf("ingressClassName = %q, want kubecell", class)
	}
}

func TestRenderIngressMirrorSkipsForeignClassAndMissingBackend(t *testing.T) {
	other := "nginx"
	foreign := testMirrorBackend()
	foreign.Ingress.Spec.IngressClassName = &other
	missing := testMirrorBackend()
	missing.Ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Port = networkingv1.ServiceBackendPort{Name: "http"}
	missing.Service = nil
	objects, err := RenderIngressMirror(testCluster(), testMirrorResolved(), "apps.example.com", []ChildIngressBackend{foreign, missing})
	if err != nil {
		t.Fatalf("RenderIngressMirror() error = %v", err)
	}
	if len(objects) != 0 {
		t.Fatalf("objects = %d, want none", len(objects))
	}
}

func TestRenderIngressMirrorResolvesNamedPorts(t *testing.T) {
	backend := testMirrorBackend()
	backend.Ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Port = networkingv1.ServiceBackendPort{Name: "http"}
	objects, err := RenderIngressMirror(testCluster(), testMirrorResolved(), "apps.example.com", []ChildIngressBackend{backend})
	if err != nil {
		t.Fatalf("RenderIngressMirror() error = %v", err)
	}
	findMirrorObject(t, objects, "Service", "ing-web-web-svc-80")
}

func TestRenderIngressMirrorRejectsEmptySuffix(t *testing.T) {
	if _, err := RenderIngressMirror(testCluster(), testMirrorResolved(), "", []ChildIngressBackend{testMirrorBackend()}); err == nil {
		t.Fatal("empty apps suffix was accepted")
	}
}

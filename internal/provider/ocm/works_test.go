package ocm

import (
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	workv1 "open-cluster-management.io/api/work/v1"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/controller/helpers"
	"github.com/kubecell/kubecell/internal/platform"
	"github.com/kubecell/kubecell/internal/render"
)

func testWorkCluster() api.VirtualCluster {
	return api.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "training", Namespace: "team-a", UID: "cluster-uid"},
		Spec:       api.VirtualClusterSpec{CellRef: api.ObjectReference{Name: "cell-a"}, ClassRef: api.ObjectReference{Name: "small"}},
	}
}

func testResolvedSnapshot() api.ResolvedSnapshot {
	return api.ResolvedSnapshot{
		ProfileHash:        "profile-hash-01",
		MachineProfileName: "ascend-910b",
		K3kChildVersion:    "v1.34.2-k3s1",
		Quota: api.ResourceList{
			"requests.cpu": resource.MustParse("1"),
			"limits.cpu":   resource.MustParse("1"),
		},
		Storage: api.StorageMappingSpec{Class: api.StorageClassMapping{
			ChildName:            "topolvm-provisioner",
			HostName:             "topolvm-provisioner",
			VolumeBindingMode:    "WaitForFirstConsumer",
			ReclaimPolicy:        corev1.PersistentVolumeReclaimRetain,
			AllowVolumeExpansion: true,
		}},
		AcceleratorKeys: []string{"huawei.com/Ascend910"},
	}
}

func testWorkNames() render.Names {
	return render.Names{
		HostNamespace:    "kc-team-a-training-01234567",
		K3kCluster:       "kc-01234567",
		HostAPIAddresses: []string{"203.0.113.10"},
		FoundationWork:   "kc-team-a-training-01234567-foundation",
		InstanceWork:     "kc-team-a-training-01234567-instance",
		AdminKubeconfig:  "kubecell-training-01234567-admin-kubeconfig",
	}
}

func TestBuildFoundationWorkTargetsManagedClusterNamespace(t *testing.T) {
	cluster := testWorkCluster()
	resolved := testResolvedSnapshot()
	names := testWorkNames()
	work, err := BuildFoundationWork(cluster, resolved, names, "cell-a")
	if err != nil {
		t.Fatalf("BuildFoundationWork() error = %v", err)
	}
	if work.APIVersion != workv1.GroupVersion.String() || work.Kind != "ManifestWork" {
		t.Fatalf("work identity = %s/%s", work.APIVersion, work.Kind)
	}
	if work.Namespace != "cell-a" {
		t.Fatalf("work namespace = %q, want cell-a", work.Namespace)
	}
	if work.Name != names.FoundationWork {
		t.Fatalf("work name = %q", work.Name)
	}
	if len(work.Spec.Workload.Manifests) != 9 {
		t.Fatalf("foundation manifests = %d, want 9", len(work.Spec.Workload.Manifests))
	}
	if len(work.Spec.ManifestConfigs) != 0 {
		t.Fatalf("foundation Work must not encode structured ResourceQuota feedback: %#v", work.Spec.ManifestConfigs)
	}
	if work.Labels[helpers.LabelVirtualClusterUID] != "cluster-uid" {
		t.Fatalf("UID label = %q", work.Labels[helpers.LabelVirtualClusterUID])
	}
	if work.Labels[helpers.LabelCellName] != "cell-a" {
		t.Fatalf("cell label = %q", work.Labels[helpers.LabelCellName])
	}
	if work.Labels[helpers.LabelProfileHash] != resolved.ProfileHash {
		t.Fatalf("profile hash label = %q", work.Labels[helpers.LabelProfileHash])
	}
}

func TestBuildFoundationWorkRendersQuotaAndNetworkPolicy(t *testing.T) {
	cluster := testWorkCluster()
	resolved := testResolvedSnapshot()
	names := testWorkNames()
	work, err := BuildFoundationWork(cluster, resolved, names, "cell-a")
	if err != nil {
		t.Fatalf("BuildFoundationWork() error = %v", err)
	}
	manifests := decodeManifests(t, work.Spec.Workload.Manifests)
	quota, ok := findManifest(manifests, "ResourceQuota", "kubecell-workload")
	if !ok {
		t.Fatal("ResourceQuota/kubecell-workload not rendered")
	}
	if quota.GetNamespace() != names.HostNamespace {
		t.Fatalf("quota namespace = %q, want %q", quota.GetNamespace(), names.HostNamespace)
	}
	hard, found, err := unstructured.NestedStringMap(quota.Object, "spec", "hard")
	if err != nil || !found {
		t.Fatalf("quota hard missing: found=%v err=%v", found, err)
	}
	if hard["requests.cpu"] != "1" || hard["limits.cpu"] != "1" {
		t.Fatalf("quota hard = %#v", hard)
	}
	if _, ok := findManifest(manifests, "NetworkPolicy", "kubecell-default-deny"); !ok {
		t.Fatal("default deny NetworkPolicy not rendered")
	}
	if _, ok := findManifest(manifests, "NetworkPolicy", "kubecell-child-api-allow"); !ok {
		t.Fatal("child API allow NetworkPolicy not rendered")
	}
}

func TestBuildInstanceWorkContainsOnlyK3kClusterAndPolicy(t *testing.T) {
	cluster := testWorkCluster()
	resolved := testResolvedSnapshot()
	names := testWorkNames()
	work, err := BuildInstanceWork(cluster, resolved, names, "cell-a", nil)
	if err != nil {
		t.Fatalf("BuildInstanceWork() error = %v", err)
	}
	if work.Namespace != "cell-a" {
		t.Fatalf("work namespace = %q, want cell-a", work.Namespace)
	}
	if work.Name != names.InstanceWork {
		t.Fatalf("work name = %q", work.Name)
	}
	if len(work.Spec.Workload.Manifests) != 2 {
		t.Fatalf("instance manifests = %d, want 2", len(work.Spec.Workload.Manifests))
	}
	if len(work.Spec.ManifestConfigs) != 2 {
		t.Fatalf("instance feedback configs = %d, want 2", len(work.Spec.ManifestConfigs))
	}
	if work.Spec.ManifestConfigs[1].ResourceIdentifier.Name != "k3k-"+names.K3kCluster+"-service" {
		t.Fatalf("NodePort feedback Service = %q", work.Spec.ManifestConfigs[1].ResourceIdentifier.Name)
	}
	for _, manifest := range work.Spec.Workload.Manifests {
		if string(manifest.Raw) == "" {
			t.Fatal("instance manifest has empty raw data")
		}
	}
	manifests := decodeManifests(t, work.Spec.Workload.Manifests)
	k3k, ok := findManifest(manifests, "Cluster", names.K3kCluster)
	if !ok {
		t.Fatal("K3k Cluster not rendered")
	}
	if k3k.GetNamespace() != names.HostNamespace {
		t.Fatalf("K3k namespace = %q, want %q", k3k.GetNamespace(), names.HostNamespace)
	}
	if work.Labels[helpers.LabelVirtualClusterUID] != "cluster-uid" {
		t.Fatalf("UID label = %q", work.Labels[helpers.LabelVirtualClusterUID])
	}
	if work.Labels[helpers.LabelProfileHash] != resolved.ProfileHash {
		t.Fatalf("profile hash label = %q", work.Labels[helpers.LabelProfileHash])
	}
}

func TestBuildInstanceWorkRendersK3kClusterSpec(t *testing.T) {
	cluster := testWorkCluster()
	resolved := testResolvedSnapshot()
	names := testWorkNames()
	work, err := BuildInstanceWork(cluster, resolved, names, "cell-a", nil)
	if err != nil {
		t.Fatalf("BuildInstanceWork() error = %v", err)
	}
	manifests := decodeManifests(t, work.Spec.Workload.Manifests)
	k3k, ok := findManifest(manifests, "Cluster", names.K3kCluster)
	if !ok {
		t.Fatal("K3k Cluster not rendered")
	}
	mode, _, _ := unstructured.NestedString(k3k.Object, "spec", "mode")
	if mode != "shared" {
		t.Fatalf("K3k mode = %q, want shared", mode)
	}
	version, _, _ := unstructured.NestedString(k3k.Object, "spec", "version")
	if version != resolved.K3kChildVersion {
		t.Fatalf("K3k version = %q, want %q", version, resolved.K3kChildVersion)
	}
	serverArgs, found, err := unstructured.NestedStringSlice(k3k.Object, "spec", "serverArgs")
	if err != nil || !found || !containsString(serverArgs, "--tls-san=203.0.113.10") {
		t.Fatalf("serverArgs = %#v, want Host API address TLS SAN", serverArgs)
	}
	persistence, found, err := unstructured.NestedMap(k3k.Object, "spec", "persistence")
	if err != nil || !found {
		t.Fatalf("persistence = %v, found=%v, err=%v", persistence, found, err)
	}
	if persistence["storageClassName"] != resolved.Storage.Class.HostName {
		t.Fatalf("persistence storageClassName = %v, want %q", persistence["storageClassName"], resolved.Storage.Class.HostName)
	}
}

func TestBuildInstanceWorkProjectsChildPolicyFromResolvedQuota(t *testing.T) {
	cluster := testWorkCluster()
	resolved := testResolvedSnapshot()
	names := testWorkNames()

	work, err := BuildInstanceWork(cluster, resolved, names, "cell-a", nil)
	if err != nil {
		t.Fatalf("BuildInstanceWork() error = %v", err)
	}
	var policy map[string]any
	if err := json.Unmarshal(work.Spec.Workload.Manifests[1].Raw, &policy); err != nil {
		t.Fatalf("decode VirtualClusterPolicy: %v", err)
	}
	if policy["kind"] != "VirtualClusterPolicy" {
		t.Fatalf("second manifest kind = %v", policy["kind"])
	}
	spec, ok := policy["spec"].(map[string]any)
	if !ok {
		t.Fatalf("policy spec = %#v", policy["spec"])
	}
	if spec["allowedMode"] != "shared" {
		t.Fatalf("allowedMode = %v, want shared", spec["allowedMode"])
	}
	quota := spec["quota"].(map[string]any)["hard"].(map[string]any)
	if quota["requests.cpu"] != "1" || quota["limits.cpu"] != "1" {
		t.Fatalf("quota hard = %#v", quota)
	}
	if spec["podSecurityAdmissionLevel"] != platform.PodSecurityEnforceLevel {
		t.Fatalf("PSA level = %v, want %q", spec["podSecurityAdmissionLevel"], platform.PodSecurityEnforceLevel)
	}
	if _, found := spec["limit"]; found {
		t.Fatalf("VirtualClusterPolicy must not render a LimitRange: %#v", spec["limit"])
	}
	metadata, ok := policy["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("policy metadata = %#v", policy["metadata"])
	}
	if _, found := metadata["namespace"]; found {
		t.Fatalf("VirtualClusterPolicy must be cluster-scoped: metadata=%#v", metadata)
	}
	if metadata["name"] != names.K3kCluster {
		t.Fatalf("policy name = %v, want %q", metadata["name"], names.K3kCluster)
	}
}

func TestManifestWorkStatusClassifiesAppliedAndDegraded(t *testing.T) {
	work := &workv1.ManifestWork{Status: workv1.ManifestWorkStatus{Conditions: []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue}}}}
	if !IsApplied(work) || IsDegraded(work) {
		t.Fatal("expected Applied work classification")
	}
	work.Status.Conditions = []metav1.Condition{{Type: workv1.WorkDegraded, Status: metav1.ConditionTrue}}
	if IsApplied(work) || !IsDegraded(work) {
		t.Fatal("expected Degraded work classification")
	}
}

func TestManifestToRawPreservesKindAndNamespace(t *testing.T) {
	object := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": "example"}}}
	manifest, err := manifestFor(object)
	if err != nil {
		t.Fatalf("manifestFor() error = %v", err)
	}
	if string(manifest.Raw) == "" || !contains(string(manifest.Raw), `"kind":"Namespace"`) {
		t.Fatalf("raw manifest = %s", manifest.Raw)
	}
}

func decodeManifests(t *testing.T, manifests []workv1.Manifest) []*unstructured.Unstructured {
	t.Helper()
	result := make([]*unstructured.Unstructured, 0, len(manifests))
	for _, manifest := range manifests {
		var object map[string]any
		if err := json.Unmarshal(manifest.Raw, &object); err != nil {
			t.Fatalf("decode manifest: %v", err)
		}
		result = append(result, &unstructured.Unstructured{Object: object})
	}
	return result
}

func findManifest(objects []*unstructured.Unstructured, kind, name string) (*unstructured.Unstructured, bool) {
	for _, object := range objects {
		if object.GetKind() == kind && object.GetName() == name {
			return object, true
		}
	}
	return nil, false
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func contains(value, fragment string) bool {
	return len(value) >= len(fragment) && (value == fragment || indexOf(value, fragment) >= 0)
}

func indexOf(value, fragment string) int {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return i
		}
	}
	return -1
}

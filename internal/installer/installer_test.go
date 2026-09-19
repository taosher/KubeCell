package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/yaml"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/platform"
)

func TestValidateCompatibilityRequiresPinnedOCMAndSharedModeVersions(t *testing.T) {
	manifest := CompatibilityManifest{
		KubecellVersion: "0.1.0",
		K3kVersion:      platform.K3kVersion,
		ChildK3sVersion: platform.ChildK3sVersion,
		OCMVersion:      "v1.3.0",
		OCMBundleDigest: "sha256:ocm",
		OCMJoinCommand:  []string{"clusteradm", "join"},
		HostProfiles:    []string{"k3s-v1.35.7+k3s1"},
		AppsSuffix:      "apps.example.com",
	}
	if err := ValidateCompatibility(manifest); err != nil {
		t.Fatalf("ValidateCompatibility() error = %v", err)
	}
}

func TestValidateCompatibilityRejectsUnpinnedOrVirtualModeProfile(t *testing.T) {
	manifest := CompatibilityManifest{K3kVersion: "v1.1.0", ChildK3sVersion: "v1.34.2+k3s1", OCMVersion: "v1.3.0"}
	if err := ValidateCompatibility(manifest); err == nil {
		t.Fatal("ValidateCompatibility() error = nil for unsupported profile")
	}
}

func TestManagementInstallOrderKeepsOCMOutsideHelmLifecycle(t *testing.T) {
	steps := InstallPlan(InstallManagement)
	expected := []string{"preflight", "ocm-hub", "ocm-addons", "kubecell-management", "readiness"}
	if len(steps) != len(expected) {
		t.Fatalf("steps = %v, want %v", steps, expected)
	}
	for index, step := range expected {
		if steps[index] != step {
			t.Fatalf("step %d = %q, want %q", index, steps[index], step)
		}
	}
}

func TestManagementInstallCommandsApplyKubeCellCRDsBeforeController(t *testing.T) {
	commands := ManagementInstallCommands("ocm/bundle.yaml", "charts/kubecell-management", "config/crd", "ghcr.io/kubecell/kubecell@sha256:"+strings.Repeat("c", 64))
	if len(commands) < 4 {
		t.Fatalf("commands = %v, want CRD, OCM, and Controller commands", commands)
	}
	if commands[0].Program != "kubectl" || !containsArg(commands[0].Args, "config/crd") {
		t.Fatalf("first command = %#v, want explicit KubeCell CRD apply", commands[0])
	}
	if commands[1].Program != "kubectl" || !containsArg(commands[1].Args, "--for=condition=Established") || !containsArg(commands[1].Args, "config/crd") {
		t.Fatalf("second command = %#v, want CRD Established wait", commands[1])
	}
	if commands[2].Program != "kubectl" || !containsArg(commands[2].Args, "ocm/bundle.yaml") {
		t.Fatalf("third command = %#v, want OCM apply", commands[2])
	}
}

func TestManagementInstallCommandsPassImmutableImageAsRepositoryAndDigest(t *testing.T) {
	commands := ManagementInstallCommands("ocm/bundle.yaml", "charts/kubecell-management", "config/crd", "registry.example/kubecell@sha256:"+strings.Repeat("a", 64))
	last := commands[len(commands)-1]
	if !containsArg(last.Args, "--set") {
		t.Fatalf("last command = %#v, want Helm image settings", last)
	}
	joined := strings.Join(last.Args, " ")
	if !strings.Contains(joined, "image.repository=registry.example/kubecell") || !strings.Contains(joined, "image.digest=sha256:"+strings.Repeat("a", 64)) {
		t.Fatalf("last command = %#v, want repository and digest settings", last)
	}
	if strings.Contains(joined, "image.repository=registry.example/kubecell@sha256:") {
		t.Fatalf("digest was incorrectly placed in repository: %#v", last)
	}
}

func TestHostInstallCommandsUseManifestJoinCommand(t *testing.T) {
	bundle := ReleaseBundle{OCMJoinBundlePath: "/release/ocm/join.yaml", Compatibility: CompatibilityManifest{OCMJoinCommand: []string{"clusteradm", "join", "--bundle", "{{OCMJoinBundlePath}}"}}, HostChart: "/release/host", ControllerImage: "registry.example/kubecell@sha256:" + strings.Repeat("b", 64)}
	commands := HostInstallCommands(bundle)
	if len(commands) != 2 || commands[0].Program != "clusteradm" || !containsArg(commands[0].Args, "/release/ocm/join.yaml") {
		t.Fatalf("commands = %#v, want rendered OCM join command", commands)
	}
}

func TestLifecycleUpgradeUsesImmutableImageValues(t *testing.T) {
	bundle := ReleaseBundle{KubecellCRDPath: "/release/crds", ManagementChart: "/release/management", ControllerImage: "registry.example/kubecell@sha256:" + strings.Repeat("d", 64)}
	commands := LifecycleCommands(LifecycleUpgrade, InstallManagement, bundle, 1)
	joined := strings.Join(commands[1].Args, " ")
	if !strings.Contains(joined, "image.repository=registry.example/kubecell") || !strings.Contains(joined, "image.digest=sha256:"+strings.Repeat("d", 64)) {
		t.Fatalf("upgrade command = %#v", commands[1])
	}
}

func TestHostInstallOrderJoinsOCMBeforeHostChart(t *testing.T) {
	steps := InstallPlan(InstallHost)
	expected := []string{"preflight", "ocm-join", "managedcluster-approval", "kubecell-host", "readiness"}
	for index, step := range expected {
		if index >= len(steps) || steps[index] != step {
			t.Fatalf("step %d = %q, want %q; all=%v", index, steps[index], step, steps)
		}
	}
}

func TestCheckOwnershipRejectsForeignRelease(t *testing.T) {
	err := CheckOwnership(OwnershipRecord{Release: "other", ManagedBy: "other"}, "kubecell")
	if err == nil {
		t.Fatal("CheckOwnership() error = nil for foreign resource")
	}
}

func TestCheckOwnershipAcceptsUnownedOrKubeCellOwnedResource(t *testing.T) {
	for _, record := range []OwnershipRecord{{}, {Release: "kubecell", ManagedBy: "kubecell"}} {
		if err := CheckOwnership(record, "kubecell"); err != nil {
			t.Fatalf("CheckOwnership(%#v) error = %v", record, err)
		}
	}
}

func TestLoadReleaseBundleRequiresVerifiedArtifacts(t *testing.T) {
	root := t.TempDir()
	writeBundleFixture(t, root)
	bundle, err := LoadReleaseBundle(root)
	if err != nil {
		t.Fatalf("LoadReleaseBundle() error = %v", err)
	}
	if bundle.Compatibility.K3kVersion != platform.K3kVersion || bundle.OCMBundlePath == "" {
		t.Fatalf("bundle = %#v", bundle)
	}
}

func TestLoadReleaseBundleRejectsArtifactDigestMismatch(t *testing.T) {
	root := t.TempDir()
	writeBundleFixture(t, root)
	if err := os.WriteFile(filepath.Join(root, "ocm", "bundle.yaml"), []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReleaseBundle(root); err == nil {
		t.Fatal("tampered OCM bundle was accepted")
	}
}

func TestLoadReleaseBundleRejectsSymlinkOutsideBundle(t *testing.T) {
	root := t.TempDir()
	writeBundleFixture(t, root)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chart := filepath.Join(root, "charts", "kubecell-host")
	if err := os.RemoveAll(chart); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), chart); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReleaseBundle(root); err == nil {
		t.Fatal("bundle with an external symlink was accepted")
	}
}

func TestDoctorCommandsCoverManagementPreflight(t *testing.T) {
	root := t.TempDir()
	writeBundleFixture(t, root)
	bundle, err := LoadReleaseBundle(root)
	if err != nil {
		t.Fatal(err)
	}
	joined := doctorCheckText(DoctorCommands(InstallManagement, bundle))
	for _, expected := range []string{"kubectl version", "managedclusters", "manifestworks", "helm template"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("doctor commands do not contain %q: %s", expected, joined)
		}
	}
}

func TestDoctorCommandsCoverHostBaseline(t *testing.T) {
	root := t.TempDir()
	writeBundleFixture(t, root)
	bundle, err := LoadReleaseBundle(root)
	if err != nil {
		t.Fatal(err)
	}
	joined := doctorCheckText(DoctorCommands(InstallHost, bundle))
	for _, expected := range []string{"k3k.io", "k3k-system", "get sc", "helm template"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("host doctor commands do not contain %q: %s", expected, joined)
		}
	}
}

func TestLifecycleCommandsNeverRemoveOCM(t *testing.T) {
	root := t.TempDir()
	writeBundleFixture(t, root)
	bundle, err := LoadReleaseBundle(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []LifecycleOperation{LifecycleUpgrade, LifecycleRollback, LifecycleUninstall} {
		joined := commandText(LifecycleCommands(operation, InstallManagement, bundle, 3))
		if strings.Contains(joined, "open-cluster-management") || strings.Contains(joined, "managedcluster") || strings.Contains(joined, "manifestwork") {
			t.Fatalf("%s commands include OCM lifecycle ownership: %s", operation, joined)
		}
	}
}

func commandText(commands []Command) string {
	var lines []string
	for _, command := range commands {
		lines = append(lines, command.Program+" "+strings.Join(command.Args, " "))
	}
	return strings.Join(lines, "\n")
}

func doctorCheckText(checks []DoctorCheck) string {
	var lines []string
	for _, check := range checks {
		lines = append(lines, "["+check.ID+"] "+check.Command.Program+" "+strings.Join(check.Command.Args, " "))
	}
	return strings.Join(lines, "\n")
}

func TestDoctorCommandsCoverFullSection81Matrix(t *testing.T) {
	root := t.TempDir()
	writeBundleFixture(t, root)
	bundle, err := LoadReleaseBundle(root)
	if err != nil {
		t.Fatal(err)
	}
	management := doctorCheckText(DoctorCommands(InstallManagement, bundle))
	for _, expected := range []string{"[M1]", "[M2]", "[M3]", "[M4]", "[M5]", "cluster-manager"} {
		if !strings.Contains(management, expected) {
			t.Fatalf("management doctor misses %q:\n%s", expected, management)
		}
	}
	host := doctorCheckText(DoctorCommandsWithDevices(InstallHost, bundle, []string{"huawei.com/Ascend910"}))
	for _, expected := range []string{"[H1]", "[H2]", "[H3]", "[H4]", "[H5]", "[H6]", "[H7]", "[H8]", "topolvm.io", "local-path", "traefik", "kubecell-host-mutating-webhook", "probe.apps.example.com", "huawei.com/Ascend910"} {
		if !strings.Contains(host, expected) {
			t.Fatalf("host doctor misses %q:\n%s", expected, host)
		}
	}
	withoutDevices := doctorCheckText(DoctorCommands(InstallHost, bundle))
	if strings.Contains(withoutDevices, "huawei.com/Ascend910") {
		t.Fatalf("device check should be skipped without --device:\n%s", withoutDevices)
	}
	report := strings.Join(BundleCheckReport(bundle), "\n")
	for _, expected := range []string{"B1", "B2", "B3", "apps.example.com"} {
		if !strings.Contains(report, expected) {
			t.Fatalf("bundle report misses %q:\n%s", expected, report)
		}
	}
}

func writeBundleFixture(t *testing.T, root string) {
	t.Helper()
	for _, directory := range []string{"ocm", "crds", "charts/kubecell-management", "charts/kubecell-host"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "crds", "kubecell.io_cells.yaml"), []byte("apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ocm", "bundle.yaml"), []byte("apiVersion: v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ocm", "join.yaml"), []byte("apiVersion: v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "charts/kubecell-management", "Chart.yaml"), []byte("name: management\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "charts/kubecell-host", "Chart.yaml"), []byte("name: host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `kubecellVersion: 0.1.0
k3kVersion: v1.2.0
childK3sVersion: v1.34.2+k3s1
ocmVersion: v1.3.0
ocmBundleDigest: sha256:REPLACE
ocmBundlePath: ocm/bundle.yaml
kubecellCRDPath: crds
ocmJoinBundlePath: ocm/join.yaml
ocmJoinCommand:
- clusteradm
- join
- --bundle
- '{{OCMJoinBundlePath}}'
managementChartPath: charts/kubecell-management
hostChartPath: charts/kubecell-host
controllerImage: ghcr.io/kubecell/kubecell@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
hostProfiles:
- k3s-v1.35.7+k3s1
appsSuffix: apps.example.com
`
	manifest += "artifactDigests:\n"
	for path, digest := range artifactDigestsForTest(t, root) {
		manifest += "  " + path + ": sha256:" + digest + "\n"
	}
	data, err := os.ReadFile(filepath.Join(root, "ocm", "bundle.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	manifest = strings.Replace(manifest, "sha256:REPLACE", "sha256:"+sha256Hex(data), 1)
	if err := os.WriteFile(filepath.Join(root, "compatibility.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func artifactDigestsForTest(t *testing.T, root string) map[string]string {
	t.Helper()
	paths := []string{"ocm/bundle.yaml", "ocm/join.yaml", "crds", "charts/kubecell-management", "charts/kubecell-host"}
	result := map[string]string{}
	for _, path := range paths {
		digest, err := digestPath(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		result[path] = digest
	}
	return result
}

func containsArg(args []string, expected string) bool {
	for _, arg := range args {
		if arg == expected {
			return true
		}
	}
	return false
}

func validManifestForTest() CompatibilityManifest {
	return CompatibilityManifest{
		KubecellVersion: "0.1.0",
		K3kVersion:      platform.K3kVersion,
		ChildK3sVersion: platform.ChildK3sVersion,
		OCMVersion:      "v1.3.0",
		OCMBundleDigest: "sha256:ocm",
		OCMJoinCommand:  []string{"clusteradm", "join"},
		HostProfiles:    []string{"k3s-v1.36.3+k3s1-arm64-ascend"},
		AppsSuffix:      "apps.example.com",
	}
}

func TestValidateCompatibilityEnforcesBareAppsSuffix(t *testing.T) {
	manifest := validManifestForTest()
	if err := ValidateCompatibility(manifest); err != nil {
		t.Fatalf("ValidateCompatibility() error = %v", err)
	}
	for _, bad := range []string{"", "HTTPS://X.NIP.IO", "https://x.nip.io", "x.nip.io:443", "x.nip.io/path", "-bad.nip.io", "bad-.nip.io", "x..nip.io", strings.Repeat("a", 64) + ".nip.io"} {
		manifest.AppsSuffix = bad
		if err := ValidateCompatibility(manifest); err == nil {
			t.Fatalf("appsSuffix %q was accepted", bad)
		}
	}
}

func TestValidateCompatibilityRejectsUnbalancedBaselineTemplate(t *testing.T) {
	manifest := validManifestForTest()
	manifest.HostBaselineCommands = []string{"kubectl label node {{HOST_NODE_NAME}}", "broken {{UNclosed"}
	if err := ValidateCompatibility(manifest); err == nil {
		t.Fatal("unbalanced baseline template was accepted")
	}
}

func TestRenderHostBaselineCommandsRequiresAllPlaceholders(t *testing.T) {
	rendered, err := RenderHostBaselineCommands(
		[]string{"kubectl label node {{HOST_NODE_NAME}} hardware.kubecell.io/profile={{HOST_PROFILE}} --overwrite"},
		map[string]string{"HOST_NODE_NAME": "host-01", "HOST_PROFILE": "ascend-910b"},
	)
	if err != nil {
		t.Fatalf("RenderHostBaselineCommands() error = %v", err)
	}
	if len(rendered) != 1 || strings.Contains(rendered[0], "{{") || !strings.Contains(rendered[0], "host-01") {
		t.Fatalf("rendered = %q", rendered)
	}
	if _, err := RenderHostBaselineCommands([]string{"echo {{MISSING}}"}, map[string]string{}); err == nil {
		t.Fatal("unresolved placeholder was accepted")
	}
}

func TestHostDNSCheckComparesEntryIP(t *testing.T) {
	check := hostDNSCheck("203.0.113.10.nip.io")
	joined := check.Command.Program + " " + strings.Join(check.Command.Args, " ")
	if check.ID != "H8" || !strings.Contains(joined, "Address: 203.0.113.10") {
		t.Fatalf("H8 check = %v, want anchored Address comparison", check)
	}
	if got := hostedEntryIP("example.com"); got != "" {
		t.Fatalf("hostedEntryIP(example.com) = %q, want empty", got)
	}
	if got := hostedEntryIP("999.1.1.1.nip.io"); got != "" {
		t.Fatalf("hostedEntryIP(999...) = %q, want empty", got)
	}
}

func TestHostBaselineVarsPinPlatformVersions(t *testing.T) {
	manifest := validManifestForTest()
	vars := HostBaselineVars(manifest, "host-01")
	if vars["HOST_K3S_VERSION"] != platform.HostK3sVersion {
		t.Fatalf("HOST_K3S_VERSION = %q, want %q", vars["HOST_K3S_VERSION"], platform.HostK3sVersion)
	}
	if vars["HOST_PROFILE"] != "k3s-v1.36.3+k3s1-arm64-ascend" || vars["HOST_NODE_NAME"] != "host-01" || vars["APPS_SUFFIX"] != "apps.example.com" {
		t.Fatalf("vars = %v", vars)
	}
}

func TestLoadReleaseBundleVerifiesOptionalBaselineArtifacts(t *testing.T) {
	root := t.TempDir()
	writeBundleFixture(t, root)
	if err := os.WriteFile(filepath.Join(root, "k3k-chart.tgz"), []byte("chart\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := digestPath(filepath.Join(root, "k3k-chart.tgz"))
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "compatibility.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	withBaseline := func(extra string) string {
		return strings.Replace(string(data), "artifactDigests:\n", "artifactDigests:\n  k3k-chart.tgz: "+extra+"\n", 1) + "k3kChartPath: k3k-chart.tgz\n"
	}
	// Matching digest: loading succeeds.
	if err := os.WriteFile(manifestPath, []byte(withBaseline("sha256:"+digest)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReleaseBundle(root); err != nil {
		t.Fatalf("LoadReleaseBundle() error = %v", err)
	}
	// Wrong digest: B2 must reject it.
	if err := os.WriteFile(manifestPath, []byte(withBaseline("sha256:"+strings.Repeat("0", 64))), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReleaseBundle(root); err == nil {
		t.Fatal("baseline artifact digest mismatch was accepted")
	}
}

func TestRenderCellYAMLProducesApplyReadyCell(t *testing.T) {
	rendered, err := RenderCellYAML(CellDraft{Name: "cell1", ProfileName: "ascend-910b", Devices: []api.DeviceContract{{Name: "Ascend910", ResourceName: "huawei.com/Ascend910"}}})
	if err != nil {
		t.Fatalf("RenderCellYAML() error = %v", err)
	}
	cell := &api.Cell{}
	if err := yaml.Unmarshal([]byte(rendered), cell); err != nil {
		t.Fatalf("unmarshal rendered Cell: %v", err)
	}
	if cell.Kind != "Cell" || cell.Name != "cell1" {
		t.Fatalf("rendered identity = %s/%s, want Cell/cell1", cell.Kind, cell.Name)
	}
	if cell.Spec.ManagedClusterRef.Name != "cell1" {
		t.Fatalf("managedClusterRef = %q, want cell1", cell.Spec.ManagedClusterRef.Name)
	}
	if cell.Spec.MachineProfile.Name != "ascend-910b" || len(cell.Spec.MachineProfile.Devices) != 1 {
		t.Fatalf("machineProfile = %+v, want single ascend-910b profile with one device", cell.Spec.MachineProfile)
	}
}

func TestRenderCellYAMLRejectsEmptyNameOrProfile(t *testing.T) {
	if _, err := RenderCellYAML(CellDraft{ProfileName: "ascend-910b"}); err == nil {
		t.Fatal("empty cell name was accepted")
	}
	if _, err := RenderCellYAML(CellDraft{Name: "cell1"}); err == nil {
		t.Fatal("empty profile name was accepted")
	}
}

func TestDiscoverCellDraftEnforcesSingleProfile(t *testing.T) {
	nodes := `{"items": [
		{"metadata": {"name": "a", "labels": {"hardware.kubecell.io/profile": "ascend-910b"}}, "status": {"allocatable": {"cpu": "8", "huawei.com/Ascend910": "4"}}},
		{"metadata": {"name": "b", "labels": {"hardware.kubecell.io/profile": "ascend-910b"}}, "status": {"allocatable": {"cpu": "8", "huawei.com/Ascend910": "2"}}}
	]}`
	profile, devices, err := DiscoverCellDraft([]byte(nodes))
	if err != nil {
		t.Fatalf("DiscoverCellDraft() error = %v", err)
	}
	if profile != "ascend-910b" {
		t.Fatalf("profile = %q, want ascend-910b", profile)
	}
	if len(devices) != 1 || devices[0] != (api.DeviceContract{Name: "Ascend910", ResourceName: "huawei.com/Ascend910"}) {
		t.Fatalf("devices = %+v, want single Ascend910 contract", devices)
	}
	mixed := `{"items": [
		{"metadata": {"name": "a", "labels": {"hardware.kubecell.io/profile": "ascend-910b"}}, "status": {"allocatable": {}}},
		{"metadata": {"name": "b", "labels": {"hardware.kubecell.io/profile": "other"}}, "status": {"allocatable": {}}}
	]}`
	if _, _, err := DiscoverCellDraft([]byte(mixed)); err == nil {
		t.Fatal("mixed profile labels were accepted")
	}
	unlabeled := `{"items": [{"metadata": {"name": "a", "labels": {}}, "status": {"allocatable": {}}}]}`
	if _, _, err := DiscoverCellDraft([]byte(unlabeled)); err == nil {
		t.Fatal("unlabeled node was accepted")
	}
}

func TestSuggestClassYAMLDerivesPairedDeviceQuota(t *testing.T) {
	cell := &api.Cell{}
	cell.Name = "cell1"
	cell.Status.Inventory = map[string]api.InventoryStatus{
		"cpu":                  {Allocatable: resource.MustParse("16")},
		"memory":               {Allocatable: resource.MustParse("32Gi")},
		"huawei.com/Ascend910": {Allocatable: resource.MustParse("8")},
	}
	rendered, err := SuggestClassYAML(cell, "ascend-910b")
	if err != nil {
		t.Fatalf("SuggestClassYAML() error = %v", err)
	}
	class := &api.VirtualNodeClass{}
	if err := yaml.Unmarshal([]byte(rendered), class); err != nil {
		t.Fatalf("unmarshal rendered class: %v", err)
	}
	hard := class.Spec.Entitlement.WorkloadHard
	cpuRequest := hard[corev1.ResourceName("requests.cpu")]
	cpuLimit := hard[corev1.ResourceName("limits.cpu")]
	memoryRequest := hard[corev1.ResourceName("requests.memory")]
	if cpuRequest.String() != "16" || !cpuRequest.Equal(cpuLimit) {
		t.Fatalf("cpu pair = %s/%s, want equal 16", cpuRequest.String(), cpuLimit.String())
	}
	if memoryRequest.String() != "32Gi" {
		t.Fatalf("memory request = %s, want 32Gi", memoryRequest.String())
	}
	request := hard[corev1.ResourceName("requests.huawei.com/Ascend910")]
	limit := hard[corev1.ResourceName("limits.huawei.com/Ascend910")]
	if request.String() != "8" || !request.Equal(limit) {
		t.Fatalf("device pair = %s/%s, want equal 8", request.String(), limit.String())
	}
	if _, err := SuggestClassYAML(&api.Cell{}, "ascend-910b"); err == nil {
		t.Fatal("empty inventory was accepted")
	}
}

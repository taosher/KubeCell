package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/platform"
)

type CompatibilityManifest struct {
	KubecellVersion     string            `json:"kubecellVersion"`
	K3kVersion          string            `json:"k3kVersion"`
	ChildK3sVersion     string            `json:"childK3sVersion"`
	OCMVersion          string            `json:"ocmVersion"`
	OCMBundleDigest     string            `json:"ocmBundleDigest"`
	ArtifactDigests     map[string]string `json:"artifactDigests"`
	OCMJoinCommand      []string          `json:"ocmJoinCommand"`
	OCMBundlePath       string            `json:"ocmBundlePath"`
	OCMJoinBundlePath   string            `json:"ocmJoinBundlePath"`
	KubecellCRDPath     string            `json:"kubecellCRDPath"`
	ManagementChartPath string            `json:"managementChartPath"`
	HostChartPath       string            `json:"hostChartPath"`
	ControllerImage     string            `json:"controllerImage"`
	HostProfiles        []string          `json:"hostProfiles"`
	// AppsSuffix is the Ingress/entry DNS suffix (bare domain, e.g. apps.example.com); doctor H8 uses it to build the probe.
	AppsSuffix string `json:"appsSuffix"`
	// HostBaselineCommands are host baseline command templates ({{PLACEHOLDER}} style, see RenderHostBaselineCommands),
	// covering version-sensitive commands such as K3s install and node labeling: operators copy the rendered output and run it without transcribing versions.
	HostBaselineCommands []string `json:"hostBaselineCommands,omitempty"`
	// The following are in-bundle relative paths of the K8s-side baseline manifests (vendored charts/manifests, optional);
	// once set, each must exist and have a matching digest in artifactDigests (covered equally by B2).
	K3kChartPath     string `json:"k3kChartPath,omitempty"`
	TopoLVMPath      string `json:"topolvmPath,omitempty"`
	TraefikPath      string `json:"traefikPath,omitempty"`
	DevicePluginPath string `json:"devicePluginPath,omitempty"`
}

type ReleaseBundle struct {
	Root              string
	Compatibility     CompatibilityManifest
	OCMBundlePath     string
	OCMJoinBundlePath string
	KubecellCRDPath   string
	ManagementChart   string
	HostChart         string
	ControllerImage   string
}

func LoadReleaseBundle(root string) (ReleaseBundle, error) {
	if strings.TrimSpace(root) == "" {
		return ReleaseBundle{}, fmt.Errorf("release bundle path is required")
	}
	manifestPath := filepath.Join(root, "compatibility.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return ReleaseBundle{}, fmt.Errorf("read compatibility manifest: %w", err)
	}
	manifest := CompatibilityManifest{}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return ReleaseBundle{}, fmt.Errorf("parse compatibility manifest: %w", err)
	}
	if err := ValidateCompatibility(manifest); err != nil {
		return ReleaseBundle{}, err
	}
	ocmPath, err := bundlePath(root, manifest.OCMBundlePath, "OCM bundle")
	if err != nil {
		return ReleaseBundle{}, err
	}
	managementPath, err := bundlePath(root, manifest.ManagementChartPath, "Management Chart")
	if err != nil {
		return ReleaseBundle{}, err
	}
	hostPath, err := bundlePath(root, manifest.HostChartPath, "Host Chart")
	if err != nil {
		return ReleaseBundle{}, err
	}
	if err := validateImmutableImage(manifest.ControllerImage); err != nil {
		return ReleaseBundle{}, err
	}
	joinPath, err := bundlePath(root, manifest.OCMJoinBundlePath, "OCM join bundle")
	if err != nil {
		return ReleaseBundle{}, err
	}
	crdPath, err := bundlePath(root, manifest.KubecellCRDPath, "KubeCell CRD bundle")
	if err != nil {
		return ReleaseBundle{}, err
	}
	artifacts := map[string]string{
		manifest.OCMBundlePath:       ocmPath,
		manifest.OCMJoinBundlePath:   joinPath,
		manifest.KubecellCRDPath:     crdPath,
		manifest.ManagementChartPath: managementPath,
		manifest.HostChartPath:       hostPath,
	}
	for label, relative := range map[string]string{
		"K3k chart":              manifest.K3kChartPath,
		"TopoLVM baseline":       manifest.TopoLVMPath,
		"Traefik baseline":       manifest.TraefikPath,
		"Device Plugin baseline": manifest.DevicePluginPath,
	} {
		if strings.TrimSpace(relative) == "" {
			continue
		}
		resolved, err := bundlePath(root, relative, label)
		if err != nil {
			return ReleaseBundle{}, err
		}
		artifacts[relative] = resolved
	}
	for relative, path := range artifacts {
		if err := verifyArtifactDigest(relative, path, manifest.ArtifactDigests); err != nil {
			return ReleaseBundle{}, err
		}
	}
	if err := verifyLegacyOCMBundleDigest(ocmPath, manifest.OCMBundleDigest); err != nil {
		return ReleaseBundle{}, err
	}
	return ReleaseBundle{Root: root, Compatibility: manifest, OCMBundlePath: ocmPath, OCMJoinBundlePath: joinPath, KubecellCRDPath: crdPath, ManagementChart: managementPath, HostChart: hostPath, ControllerImage: manifest.ControllerImage}, nil
}

func bundlePath(root, relative, label string) (string, error) {
	if strings.TrimSpace(relative) == "" {
		return "", fmt.Errorf("%s path is required", label)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Join(root, relative))
	if err != nil {
		return "", err
	}
	if pathAbs != rootAbs && !strings.HasPrefix(pathAbs, rootAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("%s path escapes release bundle: %q", label, relative)
	}
	if _, err := os.Stat(pathAbs); err != nil {
		return "", fmt.Errorf("%s %q is unavailable: %w", label, relative, err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("resolve release bundle root: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(pathAbs)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", label, err)
	}
	if resolvedPath != resolvedRoot && !strings.HasPrefix(resolvedPath, resolvedRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("%s resolves outside release bundle: %q", label, relative)
	}
	return resolvedPath, nil
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func ValidateCompatibility(manifest CompatibilityManifest) error {
	if manifest.KubecellVersion == "" {
		return fmt.Errorf("kubecell version is required")
	}
	if manifest.K3kVersion != platform.K3kVersion {
		return fmt.Errorf("K3k version %q is not the certified Shared Mode baseline", manifest.K3kVersion)
	}
	if manifest.ChildK3sVersion != platform.ChildK3sVersion {
		return fmt.Errorf("Child K3s version %q is not the certified target", manifest.ChildK3sVersion)
	}
	if manifest.OCMVersion == "" || manifest.OCMBundleDigest == "" {
		return fmt.Errorf("OCM version and bundle digest are required")
	}
	if len(manifest.HostProfiles) == 0 {
		return fmt.Errorf("at least one Host compatibility profile is required")
	}
	if len(manifest.OCMJoinCommand) == 0 {
		return fmt.Errorf("OCM join command is required")
	}
	if err := ValidateAppsSuffix(manifest.AppsSuffix); err != nil {
		return err
	}
	for _, template := range manifest.HostBaselineCommands {
		if strings.TrimSpace(template) == "" {
			return fmt.Errorf("host baseline command template must not be blank")
		}
		if strings.Count(template, "{{") != strings.Count(template, "}}") {
			return fmt.Errorf("host baseline command template has unbalanced placeholders: %q", template)
		}
	}
	return nil
}

// ValidateAppsSuffix validates doctor B1: a bare domain (no scheme, no port, no path, lowercase, valid length).
// The controller also uses it to validate KUBECELL_APPS_SUFFIX at startup and refuses to start on invalid input (§11.2).
func ValidateAppsSuffix(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("appsSuffix is required")
	}
	if value != strings.ToLower(value) {
		return fmt.Errorf("appsSuffix %q must be lowercase", value)
	}
	if strings.Contains(value, "://") || strings.Contains(value, "/") {
		return fmt.Errorf("appsSuffix %q must be a bare domain without scheme or path", value)
	}
	if strings.Contains(value, ":") {
		return fmt.Errorf("appsSuffix %q must not contain a port", value)
	}
	if len(value) > 253 {
		return fmt.Errorf("appsSuffix %q exceeds 253 characters", value)
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 {
			return fmt.Errorf("appsSuffix %q has an empty or overlong label", value)
		}
		for _, r := range label {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
				continue
			}
			return fmt.Errorf("appsSuffix %q contains illegal character %q", value, r)
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return fmt.Errorf("appsSuffix %q has a label with leading or trailing hyphen", value)
		}
	}
	return nil
}

// RenderHostBaselineCommands renders host baseline command templates with strict {{NAME}} substitution;
// missing variables or leftover placeholders are always errors (operators never transcribe versions, but must supply every parameter explicitly).
func RenderHostBaselineCommands(templates []string, vars map[string]string) ([]string, error) {
	result := make([]string, 0, len(templates))
	for _, template := range templates {
		rendered := template
		for name, value := range vars {
			rendered = strings.ReplaceAll(rendered, "{{"+name+"}}", value)
		}
		if strings.Contains(rendered, "{{") || strings.Contains(rendered, "}}") {
			return nil, fmt.Errorf("unresolved placeholder in baseline command: %q", template)
		}
		result = append(result, rendered)
	}
	return result, nil
}

// HostBaselineVars returns the fixed variable table for rendering baseline templates: versions come from platform constants,
// the profile is the first HostProfile in the manifest, and node name plus install parameters are supplied by the operator.
func HostBaselineVars(manifest CompatibilityManifest, nodeName string) map[string]string {
	profile := ""
	if len(manifest.HostProfiles) > 0 {
		profile = manifest.HostProfiles[0]
	}
	return map[string]string{
		"HOST_K3S_VERSION": platform.HostK3sVersion,
		"HOST_PROFILE":     profile,
		"HOST_NODE_NAME":   nodeName,
		"APPS_SUFFIX":      manifest.AppsSuffix,
	}
}

func validateImmutableImage(image string) error {
	parts := strings.Split(image, "@sha256:")
	if len(parts) != 2 || parts[0] == "" || len(parts[1]) != 64 {
		return fmt.Errorf("controllerImage must be an immutable sha256 image reference")
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return fmt.Errorf("controllerImage must contain a hexadecimal sha256 digest: %w", err)
	}
	return nil
}

func verifyLegacyOCMBundleDigest(path, expected string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read OCM bundle: %w", err)
	}
	if actual := sha256Hex(data); actual != strings.TrimPrefix(expected, "sha256:") {
		return fmt.Errorf("OCM bundle digest mismatch: got sha256:%s, want %s", actual, expected)
	}
	return nil
}

func verifyArtifactDigest(relative, path string, expectedDigests map[string]string) error {
	expected, ok := expectedDigests[relative]
	if !ok || strings.TrimSpace(expected) == "" {
		return fmt.Errorf("artifact digest is required for %q", relative)
	}
	actual, err := digestPath(path)
	if err != nil {
		return fmt.Errorf("digest %q: %w", relative, err)
	}
	if actual != strings.TrimPrefix(expected, "sha256:") {
		return fmt.Errorf("artifact digest mismatch for %q: got sha256:%s, want %s", relative, actual, expected)
	}
	return nil
}

func digestPath(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("symlink is not a supported release artifact")
	}
	if !info.IsDir() {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return sha256Hex(data), nil
	}
	var files []string
	if err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink %q is not a supported release artifact", current)
		}
		if entry.Type().IsRegular() {
			files = append(files, current)
		}
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(files)
	hash := sha256.New()
	for _, file := range files {
		relative, err := filepath.Rel(path, file)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		if _, err := hash.Write([]byte(relative + "\x00")); err != nil {
			return "", err
		}
		if _, err := hash.Write(data); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func DigestPath(path string) (string, error) {
	return digestPath(path)
}

type InstallTarget string

const (
	InstallManagement InstallTarget = "management"
	InstallHost       InstallTarget = "host"
)

func InstallPlan(target InstallTarget) []string {
	switch target {
	case InstallManagement:
		return []string{"preflight", "ocm-hub", "ocm-addons", "kubecell-management", "readiness"}
	case InstallHost:
		return []string{"preflight", "ocm-join", "managedcluster-approval", "kubecell-host", "readiness"}
	default:
		return nil
	}
}

type OwnershipRecord struct {
	Release   string
	ManagedBy string
}

type Command struct {
	Program string
	Args    []string
}

type LifecycleOperation string

const (
	LifecycleUpgrade   LifecycleOperation = "upgrade"
	LifecycleRollback  LifecycleOperation = "rollback"
	LifecycleUninstall LifecycleOperation = "uninstall"
)

func ParseLifecycleOperation(value string) (LifecycleOperation, error) {
	operation := LifecycleOperation(strings.ToLower(value))
	if operation != LifecycleUpgrade && operation != LifecycleRollback && operation != LifecycleUninstall {
		return "", fmt.Errorf("unsupported lifecycle operation %q", value)
	}
	return operation, nil
}

// DoctorCheck is one read-only doctor check from §8.1: ID is the table number, and failure exits non-zero.
type DoctorCheck struct {
	ID      string
	Command Command
}

func shCheck(id, script string) DoctorCheck {
	return DoctorCheck{ID: id, Command: Command{Program: "sh", Args: []string{"-c", script}}}
}

// hostDNSCheck implements H8: it resolves probe.<appsSuffix> and compares it against the host ingress IP.
// Checking only the nslookup exit code is not enough — some lab DNS servers return wildcard addresses for any domain;
// IP-literal suffixes (e.g. 203.0.113.10.nip.io) compare the expected IP extracted directly from the suffix.
func hostDNSCheck(appsSuffix string) DoctorCheck {
	probe := "probe." + appsSuffix
	if ip := hostedEntryIP(appsSuffix); ip != "" {
		// Anchor on the "Address:" line: the queried name itself contains the IP text, so a plain grep would match spuriously.
		return shCheck("H8", fmt.Sprintf(`nslookup %s | grep -q "Address: %s"`, probe, ip))
	}
	return DoctorCheck{ID: "H8", Command: Command{Program: "nslookup", Args: []string{probe}}}
}

func hostedEntryIP(suffix string) string {
	parts := strings.Split(suffix, ".")
	if len(parts) < 5 {
		return ""
	}
	octets := parts[:4]
	for _, octet := range octets {
		value := 0
		for _, r := range octet {
			if r < '0' || r > '9' {
				return ""
			}
			value = value*10 + int(r-'0')
		}
		if value > 255 || (len(octet) > 1 && octet[0] == '0') {
			return ""
		}
	}
	return strings.Join(octets, ".")
}

// BundleCheckReport reports the release-bundle layer verdicts (B1/B2/B3): LoadReleaseBundle already enforces them,
// this only restates them in human-readable form with a leading PASS/FAIL for `--dry-run` review.
func BundleCheckReport(bundle ReleaseBundle) []string {
	manifest := bundle.Compatibility
	return []string{
		fmt.Sprintf("B1 manifest parse: PASS (appsSuffix=%s)", manifest.AppsSuffix),
		fmt.Sprintf("B2 artifact digests: PASS (%d artifacts verified)", len(manifest.ArtifactDigests)),
		fmt.Sprintf("B3 locked versions: PASS (k3k=%s child=%s image-digest-pinned)", manifest.K3kVersion, manifest.ChildK3sVersion),
	}
}

func DoctorCommands(target InstallTarget, bundle ReleaseBundle) []DoctorCheck {
	return DoctorCommandsWithDevices(target, bundle, nil)
}

// DoctorCommandsWithDevices extends DoctorCommands with an H3 device-key check
// (allocatable[device] must be positive on every Ready node); skipped when devices is empty.
func DoctorCommandsWithDevices(target InstallTarget, bundle ReleaseBundle, devices []string) []DoctorCheck {
	chart := bundle.ManagementChart
	if target == InstallHost {
		chart = bundle.HostChart
	}
	versionID, renderID := "M1", "M2"
	if target == InstallHost {
		versionID, renderID = "H1", "H1"
	}
	checks := []DoctorCheck{
		{ID: versionID, Command: Command{Program: "kubectl", Args: []string{"version", "--request-timeout=10s"}}},
		{ID: renderID, Command: Command{Program: "helm", Args: []string{"template", "kubecell-preflight", chart, "--namespace", "kubecell-system"}}},
	}
	if target == InstallManagement {
		return append(checks,
			DoctorCheck{ID: "M3", Command: Command{Program: "kubectl", Args: []string{"get", "crd", "managedclusters.cluster.open-cluster-management.io"}}},
			DoctorCheck{ID: "M3", Command: Command{Program: "kubectl", Args: []string{"get", "crd", "manifestworks.work.open-cluster-management.io"}}},
			DoctorCheck{ID: "M4", Command: Command{Program: "kubectl", Args: []string{"auth", "can-i", "create", "manifestworks.work.open-cluster-management.io", "--all-namespaces"}}},
			DoctorCheck{ID: "M5", Command: Command{Program: "kubectl", Args: []string{"-n", "open-cluster-management", "wait", "--for=condition=Available", "--timeout=60s", "deployment/cluster-manager"}}},
		)
	}
	checks = append(checks,
		DoctorCheck{ID: "H2", Command: Command{Program: "kubectl", Args: []string{"get", "crd", "clusters.k3k.io"}}},
		DoctorCheck{ID: "H2", Command: Command{Program: "kubectl", Args: []string{"-n", "k3k-system", "wait", "--for=condition=Available", "--timeout=60s", "deployment", "--all"}}},
		DoctorCheck{ID: "H3", Command: Command{Program: "kubectl", Args: []string{"wait", "--for=condition=Ready", "--timeout=60s", "nodes", "--all"}}},
		shCheck("H3", `test -n "$(kubectl get nodes -o jsonpath="{.items[*].metadata.labels['hardware.kubecell.io/profile']}")" && test "$(kubectl get nodes -o jsonpath="{.items[*].metadata.labels['hardware.kubecell.io/profile']}" | tr ' ' '\n' | sort -u | wc -l | tr -d ' ')" = 1`),
		shCheck("H4", `test -n "$(kubectl get sc -o jsonpath="{.items[?(@.provisioner=='topolvm.io' && @.volumeBindingMode=='WaitForFirstConsumer')].metadata.name}")"`),
		shCheck("H4", `test -z "$(kubectl get sc -o jsonpath="{.items[?(@.provisioner=='rancher.io/local-path')].metadata.name}")"`),
		// The TopoLVM release name varies with the install method (legacy experiment leftovers such as an s4-cell2- prefix); match by substring plus Available.
		shCheck("H4", `found=""; for nn in $(kubectl get deployment -A -o jsonpath='{range .items[*]}{.metadata.namespace}/{.metadata.name}{"\n"}{end}' | grep 'topolvm-controller$'); do found="$nn"; kubectl -n "${nn%%/*}" wait --for=condition=Available --timeout=60s "deployment/${nn##*/}" || exit 1; done; test -n "$found"`),
		shCheck("H4", `kubectl get nodes -o jsonpath="{.items[*].metadata.annotations}" | grep -q "capacity.topolvm.io"`),
		shCheck("H5", `test "$(kubectl get ds -A -o jsonpath="{.items[?(@.metadata.name=='traefik')].status.numberAvailable}")" -gt 0`),
		shCheck("H5", `kubectl get pods -A -o jsonpath="{.items[*].spec.containers[*].ports[*].hostPort}" | tr ' ' '\n' | grep -q "^80$"`),
		shCheck("H5", `kubectl get pods -A -o jsonpath="{.items[*].spec.containers[*].ports[*].hostPort}" | tr ' ' '\n' | grep -v "^$" | grep -v "^80$" | grep -v "^443$" | grep -q . && exit 1 || exit 0`),
		DoctorCheck{ID: "H6", Command: Command{Program: "kubectl", Args: []string{"get", "mutatingwebhookconfigurations", "kubecell-host-mutating-webhook"}}},
		shCheck("H6", `test "$(kubectl get deployment -A -o jsonpath="{.items[?(@.metadata.name=='kubecell-host-webhook')].status.conditions[?(@.type=='Available')].status}")" = True`),
		// kubectl version does not support -o jsonpath: query the /version endpoint and compare after stripping quotes.
		shCheck("H7", fmt.Sprintf(`kubectl get --raw /version | tr -d ' "' | grep -q "gitVersion:%s"`, platform.HostK3sVersion)),
		shCheck("H7", fmt.Sprintf(`kubectl -n k3k-system get deployment -o jsonpath="{.items[*].spec.template.spec.containers[*].image}" | grep -q "%s"`, platform.K3kVersion)),
		hostDNSCheck(bundle.Compatibility.AppsSuffix),
	)
	for _, device := range devices {
		if strings.TrimSpace(device) == "" {
			continue
		}
		checks = append(checks, shCheck("H3", fmt.Sprintf(`values="$(kubectl get nodes -o jsonpath="{.items[*].status.allocatable['%s']}")"; test -n "$values" && for v in $values; do [ "$v" -gt 0 ] || exit 1; done`, device)))
	}
	return checks
}

func LifecycleCommands(operation LifecycleOperation, target InstallTarget, bundle ReleaseBundle, revision int) []Command {
	release, chart := "kubecell-management", bundle.ManagementChart
	if target == InstallHost {
		release, chart = "kubecell-host", bundle.HostChart
	}
	switch operation {
	case LifecycleUpgrade:
		imageArgs, err := imageSetArgs(bundle.ControllerImage)
		if err != nil {
			return nil
		}
		return []Command{
			{Program: "kubectl", Args: []string{"apply", "--server-side", "--filename", bundle.KubecellCRDPath}},
			{Program: "helm", Args: append([]string{"upgrade", "--install", release, chart, "--namespace", "kubecell-system", "--create-namespace"}, imageArgs...)},
		}
	case LifecycleRollback:
		return []Command{{Program: "helm", Args: []string{"rollback", release, fmt.Sprintf("%d", revision), "--namespace", "kubecell-system", "--wait", "--timeout", "10m"}}}
	case LifecycleUninstall:
		return []Command{{Program: "helm", Args: []string{"uninstall", release, "--namespace", "kubecell-system", "--wait", "--timeout", "10m"}}}
	default:
		return nil
	}
}

func imageSetArgs(image string) ([]string, error) {
	parts := strings.Split(image, "@sha256:")
	if len(parts) != 2 {
		return nil, fmt.Errorf("controllerImage must be an immutable sha256 image reference")
	}
	return []string{"--set", "image.repository=" + parts[0], "--set", "image.digest=sha256:" + parts[1]}, nil
}

func ManagementInstallCommands(ocmBundle, managementChart, kubecellCRDs, image string) []Command {
	imageArgs, err := imageSetArgs(image)
	if err != nil {
		return nil
	}
	commands := []Command{
		{Program: "kubectl", Args: []string{"apply", "--server-side", "--filename", kubecellCRDs}},
		{Program: "kubectl", Args: []string{"wait", "--for=condition=Established", "--timeout=5m", "--filename", kubecellCRDs}},
		{Program: "kubectl", Args: []string{"apply", "--server-side", "--filename", ocmBundle}},
		{Program: "kubectl", Args: []string{"wait", "--for=condition=Available", "--timeout=10m", "-n", "open-cluster-management", "deployment/cluster-manager"}},
	}
	commands = append(commands, Command{Program: "helm", Args: append([]string{"upgrade", "--install", "kubecell-management", managementChart, "--namespace", "kubecell-system", "--create-namespace"}, imageArgs...)})
	return commands
}

func HostInstallCommands(bundle ReleaseBundle) []Command {
	imageArgs, err := imageSetArgs(bundle.ControllerImage)
	if err != nil || len(bundle.Compatibility.OCMJoinCommand) == 0 {
		return nil
	}
	joinArgs := make([]string, len(bundle.Compatibility.OCMJoinCommand))
	for index, value := range bundle.Compatibility.OCMJoinCommand {
		joinArgs[index] = strings.ReplaceAll(value, "{{OCMJoinBundlePath}}", bundle.OCMJoinBundlePath)
	}
	return []Command{
		{Program: joinArgs[0], Args: joinArgs[1:]},
		{Program: "helm", Args: append([]string{"upgrade", "--install", "kubecell-host", bundle.HostChart, "--namespace", "kubecell-system", "--create-namespace"}, imageArgs...)},
	}
}

func CheckOwnership(record OwnershipRecord, expectedManager string) error {
	if record.Release != "" && record.Release != expectedManager {
		return fmt.Errorf("resource belongs to Helm release %q", record.Release)
	}
	if record.ManagedBy != "" && record.ManagedBy != expectedManager {
		return fmt.Errorf("resource is managed by %q", record.ManagedBy)
	}
	return nil
}

func ParseTarget(value string) (InstallTarget, error) {
	target := InstallTarget(strings.ToLower(value))
	if target != InstallManagement && target != InstallHost {
		return "", fmt.Errorf("unsupported install target %q", value)
	}
	return target, nil
}

// CellDraft is the input for host --step=cell: the name must equal the approved
// ManagedCluster name (enforced by the generator, §9.2); profile and device keys come from live discovery.
type CellDraft struct {
	Name        string
	ProfileName string
	Devices     []api.DeviceContract
}

// RenderCellYAML renders a Cell draft into Cell YAML ready for direct apply (zero hand-written lines).
func RenderCellYAML(draft CellDraft) (string, error) {
	if strings.TrimSpace(draft.Name) == "" {
		return "", fmt.Errorf("cell name is required and must equal the approved ManagedCluster name")
	}
	if strings.TrimSpace(draft.ProfileName) == "" {
		return "", fmt.Errorf("machine profile name is required")
	}
	for _, device := range draft.Devices {
		if strings.TrimSpace(device.Name) == "" || strings.TrimSpace(device.ResourceName) == "" {
			return "", fmt.Errorf("device contract requires name and resourceName")
		}
	}
	cell := api.Cell{
		Spec: api.CellSpec{
			ManagedClusterRef: api.ObjectReference{Name: draft.Name},
			MachineProfile:    api.MachineProfile{Name: draft.ProfileName, Devices: draft.Devices},
		},
	}
	cell.TypeMeta.Kind = "Cell"
	cell.TypeMeta.APIVersion = api.GroupVersion.String()
	cell.ObjectMeta.Name = draft.Name
	data, err := yaml.Marshal(cell)
	if err != nil {
		return "", fmt.Errorf("marshal Cell: %w", err)
	}
	return string(data), nil
}

// SuggestClassYAML derives workloadHard from the allocatable observed in Cell.status.inventory,
// emitting a VirtualNodeClass draft (§9.4): operators only adjust numbers and fill storageClassName.
// The quota contract requires every key to appear as a pair (requests.X == limits.X, see the §3.2 example).
func SuggestClassYAML(cell *api.Cell, className string) (string, error) {
	if cell == nil {
		return "", fmt.Errorf("cell is required")
	}
	if strings.TrimSpace(className) == "" {
		return "", fmt.Errorf("class name is required")
	}
	if len(cell.Status.Inventory) == 0 {
		return "", fmt.Errorf("cell %q has no observed inventory", cell.Name)
	}
	hard := api.ResourceList{}
	keys := make([]string, 0, len(cell.Status.Inventory))
	for key := range cell.Status.Inventory {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		quantity := cell.Status.Inventory[key].Allocatable.DeepCopy()
		hard[corev1.ResourceName("requests."+key)] = quantity
		hard[corev1.ResourceName("limits."+key)] = quantity.DeepCopy()
	}
	class := api.VirtualNodeClass{
		Spec: api.VirtualNodeClassSpec{
			Entitlement:      api.EntitlementSpec{WorkloadHard: hard},
			StorageClassName: "",
		},
	}
	class.TypeMeta.Kind = "VirtualNodeClass"
	class.TypeMeta.APIVersion = api.GroupVersion.String()
	class.ObjectMeta.Name = className
	data, err := yaml.Marshal(class)
	if err != nil {
		return "", fmt.Errorf("marshal VirtualNodeClass: %w", err)
	}
	return string(data), nil
}

// DiscoverCellDraft parses `kubectl get nodes -o json` output and enforces the single-profile constraint:
// all nodes must carry exactly one shared profile label value; device keys are the full set of
// extended resource keys containing "/" in allocatable (covering kernel drivers plus Device Plugin readiness in one pass).
func DiscoverCellDraft(nodesJSON []byte) (profile string, devices []api.DeviceContract, err error) {
	list := &corev1.NodeList{}
	if err := json.Unmarshal(nodesJSON, list); err != nil {
		return "", nil, fmt.Errorf("parse nodes: %w", err)
	}
	if len(list.Items) == 0 {
		return "", nil, fmt.Errorf("no nodes observed")
	}
	profiles := map[string]struct{}{}
	deviceKeys := map[string]struct{}{}
	for index := range list.Items {
		node := &list.Items[index]
		value := strings.TrimSpace(node.Labels[platform.ProfileLabelKey])
		if value == "" {
			return "", nil, fmt.Errorf("node %q misses profile label %q", node.Name, platform.ProfileLabelKey)
		}
		profiles[value] = struct{}{}
		for name, quantity := range node.Status.Allocatable {
			if strings.Contains(string(name), "/") && quantity.Sign() > 0 {
				deviceKeys[string(name)] = struct{}{}
			}
		}
	}
	if len(profiles) != 1 {
		names := make([]string, 0, len(profiles))
		for value := range profiles {
			names = append(names, value)
		}
		sort.Strings(names)
		return "", nil, fmt.Errorf("nodes carry %d profile values %q, want exactly one (single-profile constraint)", len(names), strings.Join(names, ","))
	}
	for value := range profiles {
		profile = value
	}
	keys := make([]string, 0, len(deviceKeys))
	for key := range deviceKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		short := key
		if parts := strings.Split(key, "/"); len(parts) == 2 {
			short = parts[1]
		}
		devices = append(devices, api.DeviceContract{Name: short, ResourceName: key})
	}
	return profile, devices, nil
}

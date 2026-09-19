package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"version"}, &output, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) != version {
		t.Fatalf("version output = %q", output.String())
	}
}

func TestRunDigestReportsFileDigest(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "artifact")
	if err := os.WriteFile(path, []byte("artifact\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"digest", path}, &output, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(output.String()), "sha256:") {
		t.Fatalf("digest output = %q", output.String())
	}
}

func TestRunPlanManagementShowsOCMBeforeKubecell(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"plan", "management"}, &output, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	result := output.String()
	if strings.Index(result, "ocm-hub") > strings.Index(result, "kubecell-management") {
		t.Fatalf("management plan installs Kubecell before OCM: %s", result)
	}
}

func TestRunRejectsImplicitInstallation(t *testing.T) {
	if err := run([]string{"management"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("implicit management installation was accepted")
	}
}

func TestRunManagementRequiresBundleAndPrintsVerifiedCommands(t *testing.T) {
	root := installerFixture(t)
	var output bytes.Buffer
	if err := run([]string{"management", "--bundle", root}, &output, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	result := output.String()
	if !strings.Contains(result, "ocm/bundle.yaml") || !strings.Contains(result, "kubecell-management") {
		t.Fatalf("output = %q", result)
	}
}

func TestRunDoctorRequiresAndUsesReleaseBundle(t *testing.T) {
	root := installerFixture(t)
	var output bytes.Buffer
	if err := run([]string{"doctor", "management", "--bundle", root, "--dry-run"}, &output, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	result := output.String()
	if !strings.Contains(result, "kubectl version") || !strings.Contains(result, "helm template") {
		t.Fatalf("doctor output = %q", result)
	}
}

func TestRunDoctorExecutesEveryReadOnlyCheckByDefault(t *testing.T) {
	root := installerFixture(t)
	executor := &recordingExecutor{}
	if err := runDoctor([]string{"management", "--bundle", root}, &bytes.Buffer{}, &bytes.Buffer{}, executor); err != nil {
		t.Fatalf("runDoctor() error = %v", err)
	}
	if len(executor.commands) < 4 {
		t.Fatalf("executed commands = %#v, want the management checks", executor.commands)
	}
}

func TestRunDoctorDryRunPrintsButDoesNotExecuteChecks(t *testing.T) {
	root := installerFixture(t)
	executor := &recordingExecutor{}
	var output bytes.Buffer
	if err := runDoctor([]string{"host", "--bundle", root, "--dry-run"}, &output, &bytes.Buffer{}, executor); err != nil {
		t.Fatalf("runDoctor() error = %v", err)
	}
	if len(executor.commands) != 0 {
		t.Fatalf("dry-run executed commands = %#v", executor.commands)
	}
	if !strings.Contains(output.String(), "kubectl version") {
		t.Fatalf("dry-run output = %q", output.String())
	}
}

func TestRunDoctorRejectsMissingBundle(t *testing.T) {
	if err := run([]string{"doctor", "management"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("doctor without bundle was accepted")
	}
}

func TestRunLifecycleRequiresExplicitConfirmation(t *testing.T) {
	root := installerFixture(t)
	if err := run([]string{"upgrade", "management", "--bundle", root}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("upgrade without confirmation was accepted")
	}
}

func TestRunRejectsApplyWithoutExplicitConfirmation(t *testing.T) {
	root := installerFixture(t)
	if err := run([]string{"management", "--bundle", root, "--apply"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("--apply without confirmation was accepted")
	}
}

func TestRunHostApplyRequiresExplicitEnrollmentStep(t *testing.T) {
	root := installerFixture(t)
	if err := run([]string{"host", "--bundle", root, "--apply", "--confirm"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("host apply without an enrollment step was accepted")
	}
}

func TestRunHostEnrollmentStopsBeforeApproval(t *testing.T) {
	root := installerFixture(t)
	executor := &recordingExecutor{}
	var output bytes.Buffer
	if err := runInstallWithExecutor("host", []string{"--bundle", root, "--step=join", "--apply", "--confirm"}, &output, &bytes.Buffer{}, executor); err != nil {
		t.Fatalf("runInstallWithExecutor() error = %v", err)
	}
	if len(executor.commands) != 1 || !strings.HasPrefix(executor.commands[0], "clusteradm ") {
		t.Fatalf("executed commands = %#v, want only OCM join", executor.commands)
	}
}

func TestRunHostDryRunAcceptsExplicitStep(t *testing.T) {
	root := installerFixture(t)
	var output bytes.Buffer
	if err := run([]string{"host", "--bundle", root, "--step=join", "--dry-run"}, &output, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(output.String(), "clusteradm") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRunHostChartStepExecutesOnlyAfterApproval(t *testing.T) {
	root := installerFixture(t)
	executor := &recordingExecutor{}
	if err := runInstallWithExecutor("host", []string{"--bundle", root, "--step=chart", "--apply", "--confirm"}, &bytes.Buffer{}, &bytes.Buffer{}, executor); err != nil {
		t.Fatalf("runInstallWithExecutor() error = %v", err)
	}
	if len(executor.commands) != 1 || !strings.HasPrefix(executor.commands[0], "helm upgrade") {
		t.Fatalf("executed commands = %#v, want only Host Chart install", executor.commands)
	}
}

func installerFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, directory := range []string{"ocm", "crds", "charts/kubecell-management", "charts/kubecell-host"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "ocm/bundle.yaml"), []byte("apiVersion: v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ocm/join.yaml"), []byte("apiVersion: v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "crds/kubecell.io_cells.yaml"), []byte("apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "charts/kubecell-management/Chart.yaml"), []byte("name: management\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "charts/kubecell-host/Chart.yaml"), []byte("name: host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest := sha256HexForTest([]byte("apiVersion: v1\n"))
	manifest := "kubecellVersion: 0.1.0\nk3kVersion: v1.2.0\nchildK3sVersion: v1.34.2+k3s1\nocmVersion: v1.3.0\nocmBundleDigest: sha256:" + digest + "\nocmBundlePath: ocm/bundle.yaml\nkubecellCRDPath: crds\nocmJoinBundlePath: ocm/join.yaml\nocmJoinCommand:\n- clusteradm\n- join\n- --bundle\n- '{{OCMJoinBundlePath}}'\nmanagementChartPath: charts/kubecell-management\nhostChartPath: charts/kubecell-host\ncontrollerImage: ghcr.io/kubecell/kubecell@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\nhostProfiles:\n- k3s-v1.35.7+k3s1\nappsSuffix: apps.example.com\n"
	manifest += "artifactDigests:\n"
	for path, digest := range artifactDigestsForTest(t, root) {
		manifest += "  " + path + ": sha256:" + digest + "\n"
	}
	if err := os.WriteFile(filepath.Join(root, "compatibility.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func sha256HexForTest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func artifactDigestsForTest(t *testing.T, root string) map[string]string {
	t.Helper()
	paths := []string{"ocm/bundle.yaml", "ocm/join.yaml", "crds", "charts/kubecell-management", "charts/kubecell-host"}
	result := map[string]string{}
	for _, path := range paths {
		digest, err := digestPathForTest(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		result[path] = digest
	}
	return result
}

func digestPathForTest(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return sha256HexForTest(data), nil
	}
	var files []string
	err = filepath.Walk(path, func(current string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() {
			files = append(files, current)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	hashValue := sha256.New()
	for _, file := range files {
		relative, err := filepath.Rel(path, file)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		if _, err := hashValue.Write([]byte(relative + "\x00")); err != nil {
			return "", err
		}
		if _, err := hashValue.Write(data); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hashValue.Sum(nil)), nil
}

type recordingExecutor struct {
	commands []string
}

func (executor *recordingExecutor) Run(program string, args []string, stdout, stderr io.Writer) error {
	executor.commands = append(executor.commands, program+" "+strings.Join(args, " "))
	return nil
}

func writeNodesFile(t *testing.T, items string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nodes.json")
	if err := os.WriteFile(path, []byte(`{"items": [`+items+`]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunHostCellStepRendersDiscoveredCell(t *testing.T) {
	nodes := writeNodesFile(t, `{"metadata": {"name": "host-01", "labels": {"hardware.kubecell.io/profile": "ascend-910b"}}, "status": {"allocatable": {"cpu": "8", "huawei.com/Ascend910": "4"}}}`)
	var output bytes.Buffer
	if err := run([]string{"host", "--step=cell", "--cell", "cell1", "--nodes-file", nodes}, &output, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	result := output.String()
	for _, want := range []string{"kind: Cell", "name: cell1", "ascend-910b", "huawei.com/Ascend910", "managedClusterRef"} {
		if !strings.Contains(result, want) {
			t.Fatalf("cell output misses %q:\n%s", want, result)
		}
	}
}

func TestRunHostCellStepAcceptsExplicitOverrides(t *testing.T) {
	var output bytes.Buffer
	err := run([]string{"host", "--step=cell", "--cell", "cell1", "--profile", "ascend-910b", "--device", "Ascend910=huawei.com/Ascend910"}, &output, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(output.String(), "ascend-910b") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRunHostCellStepRequiresCellNameAndRejectsApply(t *testing.T) {
	nodes := writeNodesFile(t, `{}`)
	if err := run([]string{"host", "--step=cell", "--nodes-file", nodes}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("cell step without --cell was accepted")
	}
	if err := run([]string{"host", "--step=cell", "--cell", "cell1", "--nodes-file", nodes, "--apply", "--confirm"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("cell step with --apply was accepted")
	}
	if err := run([]string{"host", "--step=cell", "--cell", "cell1", "--profile", "p", "--device", "bad-flag"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("malformed --device was accepted")
	}
}

func TestRunSuggestClassRendersDraftFromCellFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cell.yaml")
	cell := "apiVersion: kubecell.io/v1alpha1\nkind: Cell\nmetadata:\n  name: cell1\nstatus:\n  inventory:\n    cpu: {allocatable: 16}\n    huawei.com/Ascend910: {allocatable: \"8\"}\n"
	if err := os.WriteFile(path, []byte(cell), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"suggest-class", "--cell-file", path, "--name", "ascend-910b"}, &output, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	result := output.String()
	for _, want := range []string{"kind: VirtualNodeClass", "name: ascend-910b", "requests.huawei.com/Ascend910", "limits.huawei.com/Ascend910", "storageClassName"} {
		if !strings.Contains(result, want) {
			t.Fatalf("class draft misses %q:\n%s", want, result)
		}
	}
}

func TestRunSuggestClassRequiresCellFileAndName(t *testing.T) {
	if err := run([]string{"suggest-class", "--name", "x"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("suggest-class without --cell-file was accepted")
	}
	if err := run([]string{"suggest-class", "--cell-file", filepath.Join(t.TempDir(), "missing.yaml"), "--name", "x"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("suggest-class with missing file was accepted")
	}
}

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"sigs.k8s.io/yaml"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/installer"
)

var version = "dev"

type commandExecutor interface {
	Run(string, []string, io.Writer, io.Writer) error
}

type osCommandExecutor struct{}

func (osCommandExecutor) Run(program string, args []string, stdout, stderr io.Writer) error {
	command := exec.Command(program, args...)
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "kubecell-installer:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr interface{ Write([]byte) (int, error) }) error {
	if len(args) == 0 {
		return fmt.Errorf("command is required: management, host, doctor, version, plan, suggest-class, or digest")
	}
	switch args[0] {
	case "version":
		_, err := fmt.Fprintf(stdout, "%s\n", version)
		return err
	case "suggest-class":
		return runSuggestClass(args[1:], stdout, stderr)
	case "digest":
		if len(args) != 2 {
			return fmt.Errorf("digest requires an artifact path")
		}
		digest, err := installer.DigestPath(args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "sha256:%s\n", digest)
		return err
	case "doctor":
		return runDoctor(args[1:], stdout, stderr, osCommandExecutor{})
	case "plan":
		if len(args) != 2 {
			return fmt.Errorf("plan requires management or host")
		}
		target, err := installer.ParseTarget(args[1])
		if err != nil {
			return err
		}
		for _, step := range installer.InstallPlan(target) {
			if _, err := fmt.Fprintln(stdout, step); err != nil {
				return err
			}
		}
		return nil
	case "management", "host":
		return runInstall(args[0], args[1:], stdout, stderr, osCommandExecutor{})
	case "upgrade", "rollback", "uninstall":
		if len(args) < 2 {
			return fmt.Errorf("%s requires management or host", args[0])
		}
		return runLifecycle(args[0], args[1:], stdout, stderr, osCommandExecutor{})
	default:
		return fmt.Errorf("unknown command %q", strings.Join(args, " "))
	}
}

func runDoctor(args []string, stdout, stderr interface{ Write([]byte) (int, error) }, executor commandExecutor) error {
	if len(args) < 1 {
		return fmt.Errorf("doctor requires management or host, --bundle, optional --dry-run, and repeatable --device for host")
	}
	target, err := installer.ParseTarget(args[0])
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	bundlePath := flags.String("bundle", "", "verified release bundle directory")
	dryRun := flags.Bool("dry-run", false, "print checks without executing them")
	var devices stringSliceFlag
	flags.Var(&devices, "device", "host device key for the H3 check, e.g. huawei.com/Ascend910 (repeatable)")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*bundlePath) == "" {
		return fmt.Errorf("doctor requires --bundle <release-directory>")
	}
	bundle, err := installer.LoadReleaseBundle(*bundlePath)
	if err != nil {
		return err
	}
	for _, line := range installer.BundleCheckReport(bundle) {
		if _, err := fmt.Fprintf(stdout, "%s\n", line); err != nil {
			return err
		}
	}
	checks := installer.DoctorCommandsWithDevices(target, bundle, devices)
	if target == installer.InstallHost && len(devices) == 0 {
		if _, err := fmt.Fprintln(stdout, "H3 devices: skipped (pass --device KEY to enforce per-node device capacity)"); err != nil {
			return err
		}
	}
	for _, check := range checks {
		if _, err := fmt.Fprintf(stdout, "[%s] $ %s %s\n", check.ID, check.Command.Program, strings.Join(check.Command.Args, " ")); err != nil {
			return err
		}
		if !*dryRun {
			if err := executor.Run(check.Command.Program, check.Command.Args, stdout, stderr); err != nil {
				return fmt.Errorf("doctor check %s (%s): %w", check.ID, check.Command.Program, err)
			}
		}
	}
	return nil
}

func bundleFlag(args []string) (string, error) {
	if len(args) != 2 || args[0] != "--bundle" || strings.TrimSpace(args[1]) == "" {
		return "", fmt.Errorf("--bundle <release-directory> is required")
	}
	return args[1], nil
}

func runInstall(targetName string, args []string, stdout, stderr interface{ Write([]byte) (int, error) }, executor commandExecutor) error {
	return runInstallWithExecutor(targetName, args, stdout, stderr, executor)
}

func runInstallWithExecutor(targetName string, args []string, stdout, stderr interface{ Write([]byte) (int, error) }, executor commandExecutor) error {
	flags := flag.NewFlagSet(targetName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	bundlePath := flags.String("bundle", "", "verified release bundle directory")
	apply := flags.Bool("apply", false, "execute the generated commands")
	confirm := flags.Bool("confirm", false, "confirm external installation")
	step := flags.String("step", "all", "Host enrollment step: join, chart, or cell")
	dryRun := flags.Bool("dry-run", false, "print commands without executing them")
	cellName := flags.String("cell", "", "cell name for --step=cell (must equal the approved ManagedCluster name)")
	profileName := flags.String("profile", "", "override discovered machine profile for --step=cell")
	nodesFile := flags.String("nodes-file", "", "read `kubectl get nodes -o json` output from file instead of live cluster")
	kubeconfig := flags.String("kubeconfig", "", "kubeconfig for live host reads in --step=cell")
	var deviceFlags stringSliceFlag
	flags.Var(&deviceFlags, "device", "override device contract as name=resourceName (repeatable)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if targetName == string(installer.InstallHost) && *step == "cell" {
		return runCellStep(cellStepOptions{cell: *cellName, profile: *profileName, devices: deviceFlags, nodesFile: *nodesFile, kubeconfig: *kubeconfig, apply: *apply}, stdout, stderr)
	}
	if *bundlePath == "" {
		return fmt.Errorf("%s requires --bundle <release-directory>", targetName)
	}
	if *apply && !*confirm {
		return fmt.Errorf("--apply requires --confirm")
	}
	if *apply && *dryRun {
		return fmt.Errorf("--apply cannot be combined with --dry-run")
	}
	if targetName == string(installer.InstallHost) && *apply && *step == "all" {
		return fmt.Errorf("host apply requires --step=join or --step=chart; approve the ManagedCluster between steps")
	}
	if targetName == string(installer.InstallHost) && *step != "all" && *step != "join" && *step != "chart" {
		return fmt.Errorf("unsupported Host installation step %q", *step)
	}
	bundle, err := installer.LoadReleaseBundle(*bundlePath)
	if err != nil {
		return err
	}
	target, err := installer.ParseTarget(targetName)
	if err != nil {
		return err
	}
	var commands []installer.Command
	if target == installer.InstallManagement {
		commands = installer.ManagementInstallCommands(bundle.OCMBundlePath, bundle.ManagementChart, bundle.KubecellCRDPath, bundle.ControllerImage)
	} else {
		commands = installer.HostInstallCommands(bundle)
		if *step == "join" {
			commands = commands[:1]
		} else if *step == "chart" {
			commands = commands[1:]
		}
	}
	// Command builders return nil on internal error (KI-13): silently exiting 0 is worse than failing, so fail explicitly.
	if len(commands) == 0 {
		return fmt.Errorf("no commands generated for %s (check bundle image and join command)", targetName)
	}
	for _, command := range commands {
		if _, err := fmt.Fprintf(stdout, "$ %s %s\n", command.Program, strings.Join(command.Args, " ")); err != nil {
			return err
		}
		if *apply && !*dryRun {
			if err := executor.Run(command.Program, command.Args, stdout, stderr); err != nil {
				return fmt.Errorf("run %s: %w", command.Program, err)
			}
		}
	}
	return nil
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ",") }

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type cellStepOptions struct {
	cell       string
	profile    string
	devices    []string
	nodesFile  string
	kubeconfig string
	apply      bool
}

// runCellStep implements host --step=cell: it only renders Cell YAML and prints it for the operator to apply.
// Live discovery goes through kubectl (host context from environment/--kubeconfig); --nodes-file allows offline reproduction.
func runCellStep(options cellStepOptions, stdout, stderr interface{ Write([]byte) (int, error) }) error {
	if strings.TrimSpace(options.cell) == "" {
		return fmt.Errorf("--step=cell requires --cell <name> equal to the approved ManagedCluster name")
	}
	if options.apply {
		return fmt.Errorf("--step=cell only renders YAML; apply it with kubectl after review")
	}
	profile := strings.TrimSpace(options.profile)
	deviceOverrides, err := parseDeviceFlags(options.devices)
	if err != nil {
		return err
	}
	if profile == "" || len(deviceOverrides) == 0 {
		nodesJSON, err := readNodesJSON(options.nodesFile, options.kubeconfig)
		if err != nil {
			return err
		}
		discoveredProfile, discoveredDevices, err := installer.DiscoverCellDraft(nodesJSON)
		if err != nil {
			return err
		}
		if profile == "" {
			profile = discoveredProfile
		}
		if len(deviceOverrides) == 0 {
			deviceOverrides = discoveredDevices
		}
	}
	rendered, err := installer.RenderCellYAML(installer.CellDraft{Name: options.cell, ProfileName: profile, Devices: deviceOverrides})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "%s# Cell name must equal the approved ManagedCluster name.\n# Review and apply with: kubectl apply -f <this file>\n", rendered); err != nil {
		return err
	}
	return nil
}

func parseDeviceFlags(values []string) ([]api.DeviceContract, error) {
	result := make([]api.DeviceContract, 0, len(values))
	for _, value := range values {
		name, resource, found := strings.Cut(value, "=")
		if !found || strings.TrimSpace(name) == "" || strings.TrimSpace(resource) == "" {
			return nil, fmt.Errorf("invalid --device %q, want name=resourceName", value)
		}
		result = append(result, api.DeviceContract{Name: strings.TrimSpace(name), ResourceName: strings.TrimSpace(resource)})
	}
	return result, nil
}

func readNodesJSON(nodesFile, kubeconfig string) ([]byte, error) {
	if strings.TrimSpace(nodesFile) != "" {
		data, err := os.ReadFile(nodesFile)
		if err != nil {
			return nil, fmt.Errorf("read nodes file: %w", err)
		}
		return data, nil
	}
	kubectlArgs := []string{"get", "nodes", "-o", "json"}
	if strings.TrimSpace(kubeconfig) != "" {
		kubectlArgs = append([]string{"--kubeconfig", kubeconfig}, kubectlArgs...)
	}
	data, err := exec.Command("kubectl", kubectlArgs...).Output()
	if err != nil {
		return nil, fmt.Errorf("discover host nodes: %w (hint: use --nodes-file or point the kubeconfig context at the host)", err)
	}
	return data, nil
}

func runSuggestClass(args []string, stdout, stderr interface{ Write([]byte) (int, error) }) error {
	flags := flag.NewFlagSet("suggest-class", flag.ContinueOnError)
	flags.SetOutput(stderr)
	cellFile := flags.String("cell-file", "", "Cell YAML file, e.g. from `kubectl get cell NAME -o yaml`")
	className := flags.String("name", "", "VirtualNodeClass name for the draft")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*cellFile) == "" || strings.TrimSpace(*className) == "" {
		return fmt.Errorf("suggest-class requires --cell-file <path> and --name <class>")
	}
	data, err := os.ReadFile(*cellFile)
	if err != nil {
		return fmt.Errorf("read cell file: %w", err)
	}
	cell := &api.Cell{}
	if err := yaml.Unmarshal(data, cell); err != nil {
		return fmt.Errorf("parse cell file: %w", err)
	}
	rendered, err := installer.SuggestClassYAML(cell, *className)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "%s# workloadHard mirrors observed allocatable: trim the numbers to the\n# tenant envelope, then fill storageClassName with the certified host class.\n", rendered); err != nil {
		return err
	}
	return nil
}

func runLifecycle(operationName string, args []string, stdout, stderr interface{ Write([]byte) (int, error) }, executor commandExecutor) error {
	if len(args) < 1 || len(args) > 6 {
		return fmt.Errorf("%s requires target, --bundle, and --confirm", operationName)
	}
	target, err := installer.ParseTarget(args[0])
	if err != nil {
		return err
	}
	operation, err := installer.ParseLifecycleOperation(operationName)
	if err != nil {
		return err
	}
	bundlePath, err := bundleFlag(args[1:3])
	if err != nil {
		return err
	}
	confirmed := false
	revision := 1
	for _, arg := range args[3:] {
		switch arg {
		case "--confirm":
			confirmed = true
		default:
			if strings.HasPrefix(arg, "--revision=") {
				if _, scanErr := fmt.Sscanf(strings.TrimPrefix(arg, "--revision="), "%d", &revision); scanErr != nil || revision < 1 {
					return fmt.Errorf("invalid --revision")
				}
			} else {
				return fmt.Errorf("unknown flag %q", arg)
			}
		}
	}
	if !confirmed {
		return fmt.Errorf("--confirm is required for %s", operation)
	}
	bundle, err := installer.LoadReleaseBundle(bundlePath)
	if err != nil {
		return err
	}
	lifecycleCommands := installer.LifecycleCommands(operation, target, bundle, revision)
	if len(lifecycleCommands) == 0 {
		return fmt.Errorf("no commands generated for %s %s (check bundle image)", operation, target)
	}
	for _, command := range lifecycleCommands {
		if _, err := fmt.Fprintf(stdout, "$ %s %s\n", command.Program, strings.Join(command.Args, " ")); err != nil {
			return err
		}
		if err := executor.Run(command.Program, command.Args, stdout, stderr); err != nil {
			return fmt.Errorf("run %s: %w", command.Program, err)
		}
	}
	return nil
}

package charts_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestManagementChartRendersCompleteManagementContract(t *testing.T) {
	objects := renderChart(t, "charts/kubecell-management")
	required := map[string]bool{
		"CustomResourceDefinition/cells.kubecell.io":                 false,
		"CustomResourceDefinition/virtualclusters.kubecell.io":       false,
		"CustomResourceDefinition/virtualnodeclasses.kubecell.io":    false,
		"Service/kubecell-system/kubecell-webhook":                   false,
		"MutatingWebhookConfiguration/kubecell-mutating-webhook":     false,
		"ValidatingWebhookConfiguration/kubecell-validating-webhook": false,
		"Certificate/kubecell-system/kubecell-webhook-cert":          false,
	}
	for _, object := range objects {
		key := objectKey(object)
		if _, found := required[key]; found {
			required[key] = true
		}
	}
	for key, found := range required {
		if !found {
			t.Errorf("management chart did not render %s", key)
		}
	}
}

func TestManagementChartAllowsSecretCacheObservation(t *testing.T) {
	objects := renderChart(t, "charts/kubecell-management")
	for _, verb := range []string{"list", "watch"} {
		if !containsRuleForRole(objects, "kubecell-controller", "", "secrets", verb) {
			t.Fatalf("management chart controller RBAC does not allow Secret %s for cache observation", verb)
		}
	}
}

func TestHostChartRendersOnlyHostWebhookContract(t *testing.T) {
	objects := renderChart(t, "charts/kubecell-host")
	seenDeployment := false
	seenWebhook := false
	for _, object := range objects {
		if object.GetKind() == "Deployment" && object.GetName() == "kubecell-host-webhook" {
			seenDeployment = true
		}
		if object.GetKind() == "MutatingWebhookConfiguration" && object.GetName() == "kubecell-host-mutating-webhook" {
			seenWebhook = true
		}
		if strings.Contains(object.GetAPIVersion(), "k3k.io") || object.GetKind() == "DaemonSet" {
			t.Errorf("host chart rendered an upstream baseline object: %s/%s", object.GetKind(), object.GetName())
		}
	}
	if !seenDeployment || !seenWebhook {
		t.Fatalf("host chart must render host webhook Deployment and MutatingWebhookConfiguration")
	}
	if !containsRule(objects, "apiextensions.k8s.io", "customresourcedefinitions", "get") {
		t.Fatal("host chart observer RBAC does not allow reading CRDs for baseline readiness")
	}
	if !containsRule(objects, "storage.k8s.io", "csidrivers", "get") {
		t.Fatal("host chart observer RBAC does not allow reading CSIDrivers for baseline readiness")
	}
	if !containsRule(objects, "storage.k8s.io", "storageclasses", "get") {
		t.Fatal("host chart observer RBAC does not allow reading StorageClasses for baseline readiness")
	}
}

func TestHostChartSupportsOCMManagedNamespaceAndRBAC(t *testing.T) {
	objects := renderChartWithArgs(t, "charts/kubecell-host",
		"--set", "rbac.observerCreate=false",
		"--set", "rbac.managedServiceAccountBindingCreate=false",
		"--set", "rbac.foundationApplierCreate=false")
	for _, object := range objects {
		if object.GetKind() == "Namespace" {
			t.Fatalf("OCM-managed host chart must not render Namespace %s", object.GetName())
		}
		if object.GetKind() == "ClusterRole" && object.GetName() != "kubecell-host-webhook" {
			t.Fatalf("OCM-managed host chart must not render external ClusterRole %s", object.GetName())
		}
		if object.GetKind() == "ClusterRoleBinding" && object.GetName() != "kubecell-host-webhook" {
			t.Fatalf("OCM-managed host chart must not render cluster RBAC %s/%s", object.GetKind(), object.GetName())
		}
	}
}

func TestHostChartGrantsWebhookReadAccessWhenOCMRBACIsExternal(t *testing.T) {
	objects := renderChartWithArgs(t, "charts/kubecell-host",
		"--set", "rbac.observerCreate=false",
		"--set", "rbac.managedServiceAccountBindingCreate=false",
		"--set", "rbac.foundationApplierCreate=false")
	for _, resource := range []string{"namespaces", "persistentvolumes", "persistentvolumeclaims"} {
		if !containsRuleForRole(objects, "kubecell-host-webhook", "", resource, "list") ||
			!containsRuleForRole(objects, "kubecell-host-webhook", "", resource, "watch") {
			t.Errorf("host webhook RBAC does not allow list/watch on %s", resource)
		}
	}
}

func TestHostChartBindsOCMManagedServiceAccount(t *testing.T) {
	objects := renderChart(t, "charts/kubecell-host")
	for _, object := range objects {
		if object.GetKind() != "ClusterRoleBinding" || object.GetName() != "kubecell-managed-service-account-observer" {
			continue
		}
		subjects, found, err := unstructured.NestedSlice(object.Object, "subjects")
		if err != nil || !found || len(subjects) != 1 {
			t.Fatalf("OCM observer subjects = %#v, error = %v", subjects, err)
		}
		subject := subjects[0].(map[string]interface{})
		if subject["kind"] != "ServiceAccount" || subject["name"] != "kubecell-host-reader" || subject["namespace"] != "open-cluster-management-agent-addon" {
			t.Fatalf("OCM observer subject = %#v", subject)
		}
		return
	}
	t.Fatal("host chart did not bind the OCM ManagedServiceAccount")
}

func TestHostChartBindsOCMWorkAgentForFoundationResources(t *testing.T) {
	objects := renderChart(t, "charts/kubecell-host")
	for _, object := range objects {
		if object.GetKind() != "ClusterRoleBinding" || object.GetName() != "kubecell-foundation-applier" {
			continue
		}
		subjects, found, err := unstructured.NestedSlice(object.Object, "subjects")
		if err != nil || !found || len(subjects) != 1 {
			t.Fatalf("foundation applier subjects = %#v, error = %v", subjects, err)
		}
		subject := subjects[0].(map[string]interface{})
		if subject["kind"] != "ServiceAccount" || subject["name"] != "klusterlet-work-sa" || subject["namespace"] != "open-cluster-management-agent" {
			t.Fatalf("foundation applier subject = %#v", subject)
		}
		return
	}
	t.Fatal("host chart did not bind the OCM work agent foundation applier")
}

func TestHostChartGrantsOCMWorkAgentK3kInstanceAccess(t *testing.T) {
	objects := renderChart(t, "charts/kubecell-host")
	for _, resource := range []string{"clusters", "virtualclusterpolicies"} {
		for _, verb := range []string{"get", "list", "watch", "create", "update", "patch", "delete"} {
			if !containsRuleForRole(objects, "kubecell-foundation-applier", "k3k.io", resource, verb) {
				t.Errorf("host chart Work Agent RBAC does not allow %s on k3k.io/%s", verb, resource)
			}
		}
	}
}

func TestHostChartGrantsOCMWorkAgentFoundationAccess(t *testing.T) {
	objects := renderChart(t, "charts/kubecell-host")
	checks := []struct {
		group    string
		resource string
		verb     string
	}{
		{group: "", resource: "namespaces", verb: "create"},
		{group: "", resource: "resourcequotas", verb: "create"},
		{group: "", resource: "limitranges", verb: "create"},
		{group: "", resource: "secrets", verb: "get"},
		{group: "networking.k8s.io", resource: "networkpolicies", verb: "create"},
		{group: "rbac.authorization.k8s.io", resource: "roles", verb: "create"},
		{group: "rbac.authorization.k8s.io", resource: "rolebindings", verb: "create"},
	}
	for _, check := range checks {
		if !containsRuleForRole(objects, "kubecell-foundation-applier", check.group, check.resource, check.verb) {
			t.Errorf("foundation applier RBAC does not allow %s on %s/%s", check.verb, check.group, check.resource)
		}
	}
}

func TestManagementChartInjectsOCMProxyConfiguration(t *testing.T) {
	objects := renderChart(t, "charts/kubecell-management")
	for _, object := range objects {
		if object.GetKind() != "Deployment" || object.GetName() != "kubecell-controller" {
			continue
		}
		containers, found, err := unstructured.NestedSlice(object.Object, "spec", "template", "spec", "containers")
		if err != nil || !found || len(containers) != 1 {
			t.Fatalf("controller containers = %#v, error = %v", containers, err)
		}
		container := containers[0].(map[string]interface{})
		env, found, err := unstructured.NestedSlice(container, "env")
		if err != nil || !found {
			t.Fatalf("controller env = %#v, error = %v", env, err)
		}
		for _, name := range []string{"KUBECELL_OCM_CLUSTER_PROXY_ADDRESS", "KUBECELL_OCM_CLUSTER_PROXY_CA_FILE", "KUBECELL_OCM_CLUSTER_PROXY_CLIENT_CERT_FILE", "KUBECELL_OCM_CLUSTER_PROXY_CLIENT_KEY_FILE", "KUBECELL_OCM_HOST_SERVER_NAME"} {
			if !envHasName(env, name) {
				t.Errorf("controller env does not contain %s", name)
			}
		}
		volumes, found, err := unstructured.NestedSlice(object.Object, "spec", "template", "spec", "volumes")
		if err != nil || !found {
			t.Fatalf("controller volumes = %#v, error = %v", volumes, err)
		}
		if !volumeHasName(volumes, "ocm-proxy-tls") {
			t.Fatal("controller does not mount OCM proxy TLS secrets")
		}
		return
	}
	t.Fatal("kubecell-controller Deployment was not rendered")
}

func TestManagementChartPassesWebhookToggleToController(t *testing.T) {
	objects := renderChartWithArgs(t, "charts/kubecell-management", "--set", "webhook.enabled=false")
	for _, object := range objects {
		if object.GetKind() != "Deployment" || object.GetName() != "kubecell-controller" {
			continue
		}
		containers, found, err := unstructured.NestedSlice(object.Object, "spec", "template", "spec", "containers")
		if err != nil || !found || len(containers) != 1 {
			t.Fatalf("controller containers = %#v, error = %v", containers, err)
		}
		container := containers[0].(map[string]interface{})
		args, found, err := unstructured.NestedStringSlice(container, "args")
		if err != nil || !found {
			t.Fatalf("controller args = %#v, error = %v", args, err)
		}
		if !contains(args, "--webhook-enabled=false") {
			t.Fatalf("controller args = %v, want explicit disabled webhook flag", args)
		}
		return
	}
	t.Fatal("kubecell-controller Deployment was not rendered")
}

func TestManagementChartSeparatesLeaderElectionLeaseAccess(t *testing.T) {
	objects := renderChart(t, "charts/kubecell-management")
	for _, verb := range []string{"get", "list", "watch"} {
		if !containsRuleForRole(objects, "kubecell-controller", "coordination.k8s.io", "leases", verb) {
			t.Fatalf("management controller ClusterRole must allow %s on OCM leases", verb)
		}
	}
	if containsRuleForRole(objects, "kubecell-controller", "coordination.k8s.io", "leases", "update") ||
		containsRuleForRole(objects, "kubecell-controller", "coordination.k8s.io", "leases", "patch") {
		t.Fatal("management controller ClusterRole must not mutate OCM leases")
	}
	for _, verb := range []string{"get", "list", "watch", "create", "update", "patch", "delete"} {
		if !containsNamespacedRuleForRole(objects, "kubecell-controller-leader-election", "coordination.k8s.io", "leases", verb) {
			t.Errorf("leader-election Role does not allow %s on coordination.k8s.io/leases", verb)
		}
	}
	for _, object := range objects {
		if object.GetKind() == "RoleBinding" && object.GetName() == "kubecell-controller-leader-election" && object.GetNamespace() == "kubecell-system" {
			return
		}
	}
	t.Fatal("management chart did not bind the namespaced leader-election Role")
}

func TestManagementChartGrantsEventRecordingAccess(t *testing.T) {
	objects := renderChart(t, "charts/kubecell-management")
	if !containsRuleForRole(objects, "kubecell-controller", "", "events", "create") ||
		!containsRuleForRole(objects, "kubecell-controller", "", "events", "patch") {
		t.Fatal("management controller RBAC must allow create and patch on Events")
	}
}

func envHasName(values []interface{}, expected string) bool {
	for _, raw := range values {
		value, ok := raw.(map[string]interface{})
		if ok && value["name"] == expected {
			return true
		}
	}
	return false
}

func volumeHasName(values []interface{}, expected string) bool {
	for _, raw := range values {
		value, ok := raw.(map[string]interface{})
		if ok && value["name"] == expected {
			return true
		}
	}
	return false
}

func containsRule(objects []*unstructured.Unstructured, apiGroup, resource, verb string) bool {
	return containsRuleForRole(objects, "kubecell-host-observer", apiGroup, resource, verb)
}

func containsRuleForRole(objects []*unstructured.Unstructured, roleName, apiGroup, resource, verb string) bool {
	for _, object := range objects {
		if object.GetKind() != "ClusterRole" || object.GetName() != roleName {
			continue
		}
		rules, found, err := unstructured.NestedSlice(object.Object, "rules")
		if err != nil || !found {
			return false
		}
		for _, rawRule := range rules {
			rule, ok := rawRule.(map[string]interface{})
			if !ok {
				continue
			}
			groups, _, _ := unstructured.NestedStringSlice(rule, "apiGroups")
			resources, _, _ := unstructured.NestedStringSlice(rule, "resources")
			verbs, _, _ := unstructured.NestedStringSlice(rule, "verbs")
			if contains(groups, apiGroup) && contains(resources, resource) && contains(verbs, verb) {
				return true
			}
		}
	}
	return false
}

func containsNamespacedRuleForRole(objects []*unstructured.Unstructured, roleName, apiGroup, resource, verb string) bool {
	for _, object := range objects {
		if object.GetKind() != "Role" || object.GetName() != roleName {
			continue
		}
		rules, found, err := unstructured.NestedSlice(object.Object, "rules")
		if err != nil || !found {
			return false
		}
		for _, rawRule := range rules {
			rule, ok := rawRule.(map[string]interface{})
			if !ok {
				continue
			}
			groups, _, _ := unstructured.NestedStringSlice(rule, "apiGroups")
			resources, _, _ := unstructured.NestedStringSlice(rule, "resources")
			verbs, _, _ := unstructured.NestedStringSlice(rule, "verbs")
			if contains(groups, apiGroup) && contains(resources, resource) && contains(verbs, verb) {
				return true
			}
		}
	}
	return false
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func renderChart(t *testing.T, chart string) []*unstructured.Unstructured {
	return renderChartWithArgs(t, chart)
}

func renderChartWithArgs(t *testing.T, chart string, args ...string) []*unstructured.Unstructured {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	commandArgs := []string{"template", "test", filepath.Join(root, chart), "--include-crds"}
	commandArgs = append(commandArgs, args...)
	command := exec.Command("helm", commandArgs...)
	output, err := command.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			t.Fatalf("helm template failed: %v\n%s", err, exitError.Stderr)
		}
		t.Fatalf("helm template failed: %v", err)
	}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(output), 4096)
	objects := make([]*unstructured.Unstructured, 0)
	for {
		object := &unstructured.Unstructured{}
		if err := decoder.Decode(object); err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("decode rendered object: %v", err)
		}
		if len(object.Object) == 0 {
			continue
		}
		objects = append(objects, object)
	}
	return objects
}

func objectKey(object *unstructured.Unstructured) string {
	if object.GetNamespace() == "" {
		return object.GetKind() + "/" + object.GetName()
	}
	return object.GetKind() + "/" + object.GetNamespace() + "/" + object.GetName()
}

func TestManagementChartHardensControllerPod(t *testing.T) {
	objects := renderChart(t, "charts/kubecell-management")
	for _, object := range objects {
		if object.GetKind() != "Deployment" || object.GetName() != "kubecell-controller" {
			continue
		}
		containers, found, err := unstructured.NestedSlice(object.Object, "spec", "template", "spec", "containers")
		if err != nil || !found || len(containers) != 1 {
			t.Fatalf("controller containers = %#v, error = %v", containers, err)
		}
		container := containers[0].(map[string]interface{})
		if _, found, _ := unstructured.NestedMap(container, "resources", "requests"); !found {
			t.Error("controller container requests missing (KI-15)")
		}
		if _, found, _ := unstructured.NestedMap(container, "livenessProbe"); !found {
			t.Error("controller livenessProbe missing (KI-15)")
		}
		if _, found, _ := unstructured.NestedMap(container, "readinessProbe"); !found {
			t.Error("controller readinessProbe missing (KI-15)")
		}
		runAsNonRoot, found, _ := unstructured.NestedBool(container, "securityContext", "runAsNonRoot")
		if !found || !runAsNonRoot {
			t.Error("controller securityContext.runAsNonRoot missing (KI-15)")
		}
		return
	}
	t.Fatal("kubecell-controller Deployment not rendered")
}

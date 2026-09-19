package manifests_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

func TestGeneratedCRDsExposeRequiredTopLevelFieldsAndEnums(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	checks := map[string][]string{
		"config/crd/bases/kubecell.io_cells.yaml":              {"managedClusterRef", "machineProfile"},
		"config/crd/bases/kubecell.io_virtualclusters.yaml":    {"cellRef", "classRef"},
		"config/crd/bases/kubecell.io_virtualnodeclasses.yaml": {"entitlement", "storageClassName"},
	}
	for relativePath, fields := range checks {
		data, err := os.ReadFile(filepath.Join(root, relativePath))
		if err != nil {
			t.Fatal(err)
		}
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := yaml.Unmarshal(data, crd); err != nil {
			t.Fatalf("decode %s: %v", relativePath, err)
		}
		schema := crd.Spec.Versions[0].Schema.OpenAPIV3Schema
		spec := schema.Properties["spec"]
		for _, field := range fields {
			if _, found := spec.Properties[field]; !found {
				t.Fatalf("%s spec.%s is absent from OpenAPI schema", relativePath, field)
			}
		}
	}
}

func TestManagementChartCRDsMatchGeneratedCRDs(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	for _, name := range []string{"cells", "virtualclusters", "virtualnodeclasses"} {
		generatedPath := filepath.Join(root, "config", "crd", "bases", "kubecell.io_"+name+".yaml")
		chartPath := filepath.Join(root, "charts", "kubecell-management", "crds", "kubecell.io_"+name+".yaml")
		generated, err := os.ReadFile(generatedPath)
		if err != nil {
			t.Fatalf("read generated CRD %s: %v", name, err)
		}
		chart, err := os.ReadFile(chartPath)
		if err != nil {
			t.Fatalf("read chart CRD %s: %v", name, err)
		}
		if !bytes.Equal(generated, chart) {
			t.Errorf("Management Chart CRD %s differs from generated CRD; run make manifests and synchronize chart CRDs", name)
		}
	}
}

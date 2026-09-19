package manifests_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGeneratedWebhookManifestsExist(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	for _, relativePath := range []string{
		"config/webhook/manifests.yaml",
		"config/webhook/service.yaml",
	} {
		path := filepath.Join(root, relativePath)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("generated manifest %q is unavailable: %v", relativePath, err)
		}
	}
}

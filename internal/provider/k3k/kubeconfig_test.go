package k3k

import "testing"

func TestRewriteKubeconfigEndpointPreservesCredentials(t *testing.T) {
	input := []byte(`apiVersion: v1
kind: Config
clusters:
- name: child
  cluster:
    server: https://10.0.0.1:6443
    certificate-authority-data: Q0E=
contexts:
- name: child
  context:
    cluster: child
    user: admin
current-context: child
users:
- name: admin
  user:
    client-certificate-data: Q0xJRU5U
    client-key-data: S0VZ
`)
	output, err := RewriteKubeconfigEndpoint(input, "https://203.0.113.10:30443")
	if err != nil {
		t.Fatalf("RewriteKubeconfigEndpoint() error = %v", err)
	}
	if !contains(string(output), "https://203.0.113.10:30443") {
		t.Fatalf("endpoint missing: %s", output)
	}
	if !contains(string(output), "Q0xJRU5U") || !contains(string(output), "S0VZ") || !contains(string(output), "Q0E=") {
		t.Fatalf("credential data changed: %s", output)
	}
}

func TestRewriteKubeconfigRejectsMissingCluster(t *testing.T) {
	if _, err := RewriteKubeconfigEndpoint([]byte("apiVersion: v1\nkind: Config\nclusters: []\n"), "https://example:443"); err == nil {
		t.Fatal("expected missing cluster error")
	}
}

func contains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}

package profile

import "testing"

func TestResolveChildVersionUsesCertifiedK3kSyntax(t *testing.T) {
	resolved, err := ResolveChildVersion("v1.34.2+k3s1")
	if err != nil {
		t.Fatalf("ResolveChildVersion() error = %v", err)
	}
	if resolved.Canonical != "v1.34.2+k3s1" {
		t.Fatalf("canonical version = %q", resolved.Canonical)
	}
	if resolved.K3kSpec != "v1.34.2-k3s1" {
		t.Fatalf("K3k spec version = %q, want v1.34.2-k3s1", resolved.K3kSpec)
	}
	if resolved.ImageTag != "v1.34.2-k3s1" {
		t.Fatalf("image tag = %q, want v1.34.2-k3s1", resolved.ImageTag)
	}
}

func TestResolveChildVersionRejectsUncertifiedVersion(t *testing.T) {
	if _, err := ResolveChildVersion("v1.35.0+k3s1"); err == nil {
		t.Fatal("ResolveChildVersion() accepted an uncertified Child version")
	}
}

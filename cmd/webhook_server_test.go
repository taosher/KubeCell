package main

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func TestNewWebhookServerDisabledDoesNotRequireCertificates(t *testing.T) {
	server := newWebhookServer(false, webhook.Options{CertDir: t.TempDir()})

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("disabled webhook server Start() error = %v", err)
	}
}

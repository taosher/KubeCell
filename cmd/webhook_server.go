package main

import (
	"context"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type disabledWebhookServer struct {
	mux *http.ServeMux
}

func newWebhookServer(enabled bool, options webhook.Options) webhook.Server {
	if !enabled {
		return &disabledWebhookServer{mux: http.NewServeMux()}
	}
	return webhook.NewServer(options)
}

func (s *disabledWebhookServer) NeedLeaderElection() bool {
	return false
}

func (s *disabledWebhookServer) Register(path string, handler http.Handler) {
	s.mux.Handle(path, handler)
}

func (s *disabledWebhookServer) Start(context.Context) error {
	return nil
}

func (s *disabledWebhookServer) StartedChecker() healthz.Checker {
	return healthz.Ping
}

func (s *disabledWebhookServer) WebhookMux() *http.ServeMux {
	return s.mux
}

package ocm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func TestRESTHostReaderReadsOnlyExpectedHostResources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer msa-token" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/api/v1/namespaces/host-ns/secrets/k3k-demo-kubeconfig":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"k3k-demo-kubeconfig","namespace":"host-ns"},"data":{"kubeconfig.yaml":"YXBpVmVyc2lvbjogdjE="}}`))
		case "/api/v1/namespaces/host-ns/services/demo-service":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"apiVersion":"v1","kind":"Service","metadata":{"name":"demo-service","namespace":"host-ns"},"spec":{"ports":[{"port":443,"nodePort":30443}]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	reader, err := NewRESTHostReader(&rest.Config{Host: server.URL, BearerToken: "msa-token"})
	if err != nil {
		t.Fatalf("NewRESTHostReader() error = %v", err)
	}
	secret, err := reader.GetSecret(context.Background(), "host-ns", "k3k-demo-kubeconfig")
	if err != nil || secret.Name != "k3k-demo-kubeconfig" {
		t.Fatalf("GetSecret() = %#v, error = %v", secret, err)
	}
	service, err := reader.GetService(context.Background(), "host-ns", "demo-service")
	if err != nil || service.Spec.Ports[0].NodePort != 30443 {
		t.Fatalf("GetService() = %#v, error = %v", service, err)
	}
}

func TestRESTHostReaderRejectsNilConfig(t *testing.T) {
	if _, err := NewRESTHostReader(nil); err == nil {
		t.Fatal("NewRESTHostReader(nil) error = nil")
	}
}

func TestSplitTopoLVMDaemonSetsAcceptsUpstreamMetadataShape(t *testing.T) {
	list := &apps.DaemonSetList{Items: []apps.DaemonSet{
		{ObjectMeta: metav1.ObjectMeta{Name: "topolvm-node", Labels: map[string]string{"app.kubernetes.io/name": "topolvm"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "topolvm-lvmd-0", Labels: map[string]string{"app.kubernetes.io/name": "topolvm"}}},
	}}
	node, lvmd := splitTopoLVMDaemonSets(list)
	if len(node.Items) != 1 || node.Items[0].Name != "topolvm-node" {
		t.Fatalf("node DaemonSets = %#v", node.Items)
	}
	if len(lvmd.Items) != 1 || lvmd.Items[0].Name != "topolvm-lvmd-0" {
		t.Fatalf("lvmd DaemonSets = %#v", lvmd.Items)
	}
}

var _ = metav1.GetOptions{}

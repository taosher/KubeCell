package ocm

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/platform"
)

func testProxyCell() *api.Cell {
	return &api.Cell{Spec: api.CellSpec{ManagedClusterRef: api.ObjectReference{Name: "cell-a"}}}
}

func TestKonnectivityConfigProviderBuildsManagedClusterConfig(t *testing.T) {
	cell := testProxyCell()
	provider := KonnectivityConfigProvider{
		BaseConfig:       &rest.Config{Host: "https://hub.example"},
		ProxyAddress:     "proxy.example:8090",
		CredentialReader: fakeCredentialReader{token: "msa-token", caData: []byte("host-ca")},
		HostServerName:   "kubernetes.default.svc",
	}
	config, err := provider.RESTConfigForCell(context.Background(), cell)
	if err != nil {
		t.Fatalf("RESTConfigForCell() error = %v", err)
	}
	if config.Host != "https://cell-a" || config.BearerToken != "msa-token" || config.Dial == nil {
		t.Fatalf("config = %#v", config)
	}
	if config.TLSClientConfig.ServerName != "kubernetes.default.svc" {
		t.Fatalf("TLS server name = %q", config.TLSClientConfig.ServerName)
	}
	if string(config.TLSClientConfig.CAData) != "host-ca" {
		t.Fatalf("TLS CA data = %q", config.TLSClientConfig.CAData)
	}
}

func TestHubSecretTokenReaderRequiresTokenData(t *testing.T) {
	cell := testProxyCell()
	reader := HubSecretReader{GetSecret: func(_ context.Context, namespace, name string) (*corev1.Secret, error) {
		if namespace != "cell-a" {
			t.Fatalf("secret namespace = %q, want cell-a", namespace)
		}
		if name != platform.ManagedServiceAccountName {
			t.Fatalf("secret name = %q, want %q", name, platform.ManagedServiceAccountName)
		}
		return &corev1.Secret{}, nil
	}}
	if _, err := reader.CredentialsForCell(context.Background(), cell); err == nil {
		t.Fatal("CredentialsForCell() error = nil")
	}
}

func TestKubernetesHubTokenReaderUsesManagedClusterNamespace(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: platform.ManagedServiceAccountName, Namespace: "cell-a"},
		Data:       map[string][]byte{"token": []byte("msa-token")},
	})
	reader := KubernetesHubCredentialReader{Client: client}
	credentials, err := reader.CredentialsForCell(context.Background(), testProxyCell())
	if err != nil || credentials.Token != "msa-token" || string(credentials.CAData) != "" {
		t.Fatalf("CredentialsForCell() = %#v, error = %v", credentials, err)
	}
}

func TestLoadProxyTLSConfigLoadsCAAndClientCertificate(t *testing.T) {
	directory := t.TempDir()
	caPEM, certPEM, keyPEM := testCertificateMaterial(t)
	caPath := filepath.Join(directory, "ca.crt")
	certPath := filepath.Join(directory, "tls.crt")
	keyPath := filepath.Join(directory, "tls.key")
	for path, data := range map[string][]byte{caPath: caPEM, certPath: certPEM, keyPath: keyPEM} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	config, err := LoadProxyTLSConfig(caPath, certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadProxyTLSConfig() error = %v", err)
	}
	if config == nil || config.RootCAs == nil || len(config.Certificates) != 1 {
		t.Fatalf("config = %#v", config)
	}
}

func TestLoadProxyTLSConfigRejectsPartialConfiguration(t *testing.T) {
	if _, err := LoadProxyTLSConfig("/tmp/ca.crt", "", ""); err == nil {
		t.Fatal("LoadProxyTLSConfig() error = nil")
	}
}

func testCertificateMaterial(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "proxy"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, certPEM, keyPEM
}

type fakeCredentialReader struct {
	token  string
	caData []byte
}

func (f fakeCredentialReader) CredentialsForCell(context.Context, *api.Cell) (ManagedServiceAccountCredentials, error) {
	return ManagedServiceAccountCredentials{Token: f.token, CAData: f.caData}, nil
}

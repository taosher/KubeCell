package ocm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	konnectivity "sigs.k8s.io/apiserver-network-proxy/konnectivity-client/pkg/client"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/platform"
)

type ManagedServiceAccountCredentials struct {
	Token  string
	CAData []byte
}

type ManagedServiceAccountCredentialReader interface {
	CredentialsForCell(context.Context, *api.Cell) (ManagedServiceAccountCredentials, error)
}

type KonnectivityConfigProvider struct {
	BaseConfig         *rest.Config
	ProxyAddress       string
	ProxyTLSConfig     *tls.Config
	HostServerName     string
	CredentialReader   ManagedServiceAccountCredentialReader
	ManagedClusterName func(*api.Cell) string
}

func LoadProxyTLSConfig(caPath, certPath, keyPath string) (*tls.Config, error) {
	if strings.TrimSpace(caPath) == "" || strings.TrimSpace(certPath) == "" || strings.TrimSpace(keyPath) == "" {
		return nil, fmt.Errorf("OCM Cluster Proxy CA, client certificate, and client key paths are all required")
	}
	caPEM, err := os.ReadFile(filepath.Clean(caPath))
	if err != nil {
		return nil, fmt.Errorf("read OCM Cluster Proxy CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("parse OCM Cluster Proxy CA: no certificates found")
	}
	certificate, err := tls.LoadX509KeyPair(filepath.Clean(certPath), filepath.Clean(keyPath))
	if err != nil {
		return nil, fmt.Errorf("load OCM Cluster Proxy client certificate: %w", err)
	}
	return &tls.Config{RootCAs: roots, Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}, nil
}

func (p KonnectivityConfigProvider) RESTConfigForCell(ctx context.Context, cell *api.Cell) (*rest.Config, error) {
	if p.BaseConfig == nil {
		return nil, fmt.Errorf("Hub REST config is not configured")
	}
	if cell == nil {
		return nil, fmt.Errorf("Cell is nil")
	}
	if strings.TrimSpace(p.ProxyAddress) == "" {
		return nil, fmt.Errorf("Cluster Proxy gRPC address is not configured")
	}
	if p.CredentialReader == nil {
		return nil, fmt.Errorf("ManagedServiceAccount credential reader is not configured")
	}
	msaCredentials, err := p.CredentialReader.CredentialsForCell(ctx, cell)
	if err != nil {
		return nil, fmt.Errorf("read ManagedServiceAccount credentials: %w", err)
	}
	if strings.TrimSpace(msaCredentials.Token) == "" {
		return nil, fmt.Errorf("ManagedServiceAccount token is empty")
	}
	clusterName := cell.Spec.ManagedClusterRef.Name
	if p.ManagedClusterName != nil {
		clusterName = p.ManagedClusterName(cell)
	}
	if clusterName == "" {
		return nil, fmt.Errorf("ManagedCluster name is empty")
	}
	config := rest.CopyConfig(p.BaseConfig)
	config.Host = "https://" + clusterName
	config.BearerToken = msaCredentials.Token
	config.BearerTokenFile = ""
	if len(msaCredentials.CAData) > 0 {
		config.TLSClientConfig.CAData = append([]byte(nil), msaCredentials.CAData...)
		config.TLSClientConfig.CAFile = ""
	}
	if strings.TrimSpace(p.HostServerName) != "" {
		config.TLSClientConfig.ServerName = p.HostServerName
	}
	config.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		options := []grpc.DialOption{}
		if p.ProxyTLSConfig != nil {
			options = append(options, grpc.WithTransportCredentials(credentials.NewTLS(p.ProxyTLSConfig)))
		} else {
			options = append(options, grpc.WithInsecure())
		}
		tunnel, err := konnectivity.CreateSingleUseGrpcTunnelWithContext(ctx, ctx, p.ProxyAddress, options...)
		if err != nil {
			return nil, err
		}
		return tunnel.DialContext(ctx, network, address)
	}
	return config, nil
}

type HubSecretReader struct {
	GetSecret func(context.Context, string, string) (*corev1.Secret, error)
}

type KubernetesHubCredentialReader struct {
	Client kubernetes.Interface
}

func (r KubernetesHubCredentialReader) CredentialsForCell(ctx context.Context, cell *api.Cell) (ManagedServiceAccountCredentials, error) {
	if r.Client == nil {
		return ManagedServiceAccountCredentials{}, fmt.Errorf("Hub Kubernetes client is not configured")
	}
	if cell == nil {
		return ManagedServiceAccountCredentials{}, fmt.Errorf("Cell is nil")
	}
	name := platform.ManagedServiceAccountName
	secret, err := r.Client.CoreV1().Secrets(cell.Spec.ManagedClusterRef.Name).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ManagedServiceAccountCredentials{}, err
	}
	if token := string(secret.Data["token"]); strings.TrimSpace(token) != "" {
		return ManagedServiceAccountCredentials{Token: token, CAData: append([]byte(nil), secret.Data["ca.crt"]...)}, nil
	}
	return ManagedServiceAccountCredentials{}, fmt.Errorf("ManagedServiceAccount Secret %s/%s has no token", secret.Namespace, secret.Name)
}

func (r HubSecretReader) CredentialsForCell(ctx context.Context, cell *api.Cell) (ManagedServiceAccountCredentials, error) {
	if r.GetSecret == nil {
		return ManagedServiceAccountCredentials{}, fmt.Errorf("Hub Secret reader is not configured")
	}
	if cell == nil {
		return ManagedServiceAccountCredentials{}, fmt.Errorf("Cell is nil")
	}
	name := platform.ManagedServiceAccountName
	secret, err := r.GetSecret(ctx, cell.Spec.ManagedClusterRef.Name, name)
	if err != nil {
		return ManagedServiceAccountCredentials{}, err
	}
	if token := string(secret.Data["token"]); strings.TrimSpace(token) != "" {
		return ManagedServiceAccountCredentials{Token: token, CAData: append([]byte(nil), secret.Data["ca.crt"]...)}, nil
	}
	return ManagedServiceAccountCredentials{}, fmt.Errorf("ManagedServiceAccount Secret %s/%s has no token", secret.Namespace, secret.Name)
}

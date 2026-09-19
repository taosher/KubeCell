package controller

import (
	"context"
	"fmt"

	"k8s.io/client-go/rest"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/provider/host"
	"github.com/kubecell/kubecell/internal/provider/k3k"
	"github.com/kubecell/kubecell/internal/provider/ocm"
)

type CellProxyConfigProvider interface {
	RESTConfigForCell(context.Context, *api.Cell) (*rest.Config, error)
}

type OCMChildObserverFactory struct {
	ProxyConfigProvider CellProxyConfigProvider
	ChildClientFactory  k3k.ChildClientFactory
}

type OCMInventoryReaderFactory struct {
	ProxyConfigProvider CellProxyConfigProvider
}

type OCMBaselineReaderFactory struct {
	ProxyConfigProvider CellProxyConfigProvider
}

type OCMVirtualNodeHostReaderFactory struct {
	ProxyConfigProvider CellProxyConfigProvider
}

func (f OCMInventoryReaderFactory) ForCell(ctx context.Context, cell *api.Cell) (InventoryReader, error) {
	if f.ProxyConfigProvider == nil {
		return nil, fmt.Errorf("OCM Cluster Proxy config provider is not configured")
	}
	config, err := f.ProxyConfigProvider.RESTConfigForCell(ctx, cell)
	if err != nil {
		return nil, err
	}
	return ocm.NewRESTHostReader(config)
}

func (f OCMBaselineReaderFactory) ForCell(ctx context.Context, cell *api.Cell) (host.BaselineReader, error) {
	if f.ProxyConfigProvider == nil {
		return nil, fmt.Errorf("OCM Cluster Proxy config provider is not configured")
	}
	config, err := f.ProxyConfigProvider.RESTConfigForCell(ctx, cell)
	if err != nil {
		return nil, err
	}
	return ocm.NewRESTBaselineReader(config)
}

func (f OCMVirtualNodeHostReaderFactory) ForCell(ctx context.Context, cell *api.Cell) (VirtualNodeHostReader, error) {
	if f.ProxyConfigProvider == nil {
		return nil, fmt.Errorf("OCM Cluster Proxy config provider is not configured")
	}
	config, err := f.ProxyConfigProvider.RESTConfigForCell(ctx, cell)
	if err != nil {
		return nil, err
	}
	return ocm.NewRESTHostReader(config)
}

func (f OCMChildObserverFactory) ForCell(ctx context.Context, cell *api.Cell) (ChildObserver, error) {
	if f.ProxyConfigProvider == nil {
		return nil, fmt.Errorf("OCM Cluster Proxy config provider is not configured")
	}
	config, err := f.ProxyConfigProvider.RESTConfigForCell(ctx, cell)
	if err != nil {
		return nil, fmt.Errorf("resolve OCM Cluster Proxy config: %w", err)
	}
	reader, err := ocm.NewRESTHostReader(config)
	if err != nil {
		return nil, err
	}
	childFactory := f.ChildClientFactory
	if childFactory == nil {
		childFactory = k3k.KubernetesChildClientFactory{}
	}
	return childObserverAdapter{observer: k3k.ResourceObserver{Reader: reader, ClientFactory: childFactory}}, nil
}

type childObserverAdapter struct{ observer k3k.ResourceObserver }

func (a childObserverAdapter) Observe(ctx context.Context, cluster *api.VirtualCluster, addresses []string) (ChildObservation, error) {
	result, err := a.observer.Observe(ctx, cluster, addresses)
	if err != nil {
		return ChildObservation{}, err
	}
	return ChildObservation{APIReady: result.APIReady, LogicalNodeName: result.LogicalNodeName, LogicalNodeReady: result.LogicalNodeReady, NodePort: result.NodePort, Endpoint: result.Endpoint, SourceSecretName: result.SourceSecretName, Kubeconfig: result.Kubeconfig, StorageClassReady: result.StorageClassReady, Client: result.Client}, nil
}

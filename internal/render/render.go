package render

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/controller/helpers"
	"github.com/kubecell/kubecell/internal/platform"
)

type Names struct {
	HostNamespace    string
	K3kCluster       string
	HostAPIAddresses []string
	FoundationWork   string
	InstanceWork     string
	AdminKubeconfig  string
}

func RenderFoundation(cluster api.VirtualCluster, resolved api.ResolvedSnapshot, names Names) ([]*unstructured.Unstructured, error) {
	labels := helpers.OwnedHostLabels(cluster.Spec.CellRef.Name, cluster.Namespace, cluster.Name, string(cluster.UID), resolved.ProfileHash)
	quota := map[string]any{"apiVersion": "v1", "kind": "ResourceQuota", "metadata": map[string]any{"name": "kubecell-workload", "namespace": names.HostNamespace, "labels": labels}, "spec": map[string]any{"hard": quantityMap(resolved.Quota)}}
	containerDefaults := quantityMap(platform.DefaultContainerResources())
	defaults := map[string]any{"apiVersion": "v1", "kind": "LimitRange", "metadata": map[string]any{"name": "kubecell-defaults", "namespace": names.HostNamespace, "labels": labels}, "spec": map[string]any{"limits": []any{map[string]any{"type": "Container", "default": containerDefaults, "defaultRequest": containerDefaults}}}}
	defaultDeny := networkPolicy("kubecell-default-deny", names.HostNamespace, labels, map[string]any{"podSelector": map[string]any{}})
	dnsAllow := networkPolicy("kubecell-dns-allow", names.HostNamespace, labels, map[string]any{"podSelector": map[string]any{}, "egress": []any{map[string]any{"ports": []any{map[string]any{"protocol": "UDP", "port": int64(53)}, map[string]any{"protocol": "TCP", "port": int64(53)}}}}})
	childAPIAllow := networkPolicy("kubecell-child-api-allow", names.HostNamespace, labels, map[string]any{
		"podSelector": map[string]any{"matchLabels": map[string]any{"role": "server"}},
		"ingress":     []any{map[string]any{}},
		"egress": []any{map[string]any{
			"to":    []any{map[string]any{"podSelector": map[string]any{"matchLabels": map[string]any{"cluster": names.K3kCluster, "type": "agent"}}}},
			"ports": []any{map[string]any{"protocol": "TCP", "port": int64(10250)}},
		}},
	})
	sharedAgentEgress := networkPolicy("kubecell-shared-agent-egress", names.HostNamespace, labels, map[string]any{
		"podSelector": map[string]any{"matchLabels": map[string]any{"cluster": names.K3kCluster, "type": "agent"}},
		"ingress": []any{map[string]any{
			"from":  []any{map[string]any{"podSelector": map[string]any{"matchLabels": map[string]any{"role": "server"}}}},
			"ports": []any{map[string]any{"protocol": "TCP", "port": int64(10250)}},
		}},
		"egress": []any{
			map[string]any{
				"to":    []any{map[string]any{"ipBlock": map[string]any{"cidr": "10.43.0.1/32"}}},
				"ports": []any{map[string]any{"protocol": "TCP", "port": int64(443)}},
			},
			map[string]any{
				"to":    ipPeers(names.HostAPIAddresses),
				"ports": []any{map[string]any{"protocol": "TCP", "port": int64(6443)}},
			},
			map[string]any{
				"to":    []any{map[string]any{"podSelector": map[string]any{"matchLabels": map[string]any{"role": "server"}}}},
				"ports": []any{map[string]any{"protocol": "TCP", "port": int64(6443)}},
			},
		},
	})
	portForwardRole := map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "Role",
		"metadata":   map[string]any{"name": "kubecell-portforward", "namespace": names.HostNamespace, "labels": labels},
		"rules": []any{map[string]any{
			"apiGroups": []any{""},
			"resources": []any{"pods/portforward"},
			"verbs":     []any{"create"},
		}},
	}
	portForwardBinding := map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "RoleBinding",
		"metadata":   map[string]any{"name": "kubecell-portforward", "namespace": names.HostNamespace, "labels": labels},
		"roleRef": map[string]any{
			"apiGroup": "rbac.authorization.k8s.io",
			"kind":     "Role",
			"name":     "kubecell-portforward",
		},
		"subjects": []any{map[string]any{
			"kind":      "ServiceAccount",
			"name":      "k3k-" + names.K3kCluster + "-kubelet",
			"namespace": names.HostNamespace,
		}},
	}
	policyLabels := map[string]string{
		"kubecell.io/virtual-cluster-uid": string(cluster.UID),
		"kubecell.io/profile-name":        resolved.MachineProfileName,
		"policy.k3k.io/policy-name":       names.K3kCluster,
	}
	policyAnnotations := map[string]string{
		"kubecell.io/profile-label-key":      platform.ProfileLabelKey,
		"kubecell.io/storage-class-names":    resolved.Storage.Class.ChildName,
		"kubecell.io/allowed-resource-names": acceleratorKeyList(resolved.AcceleratorKeys),
	}
	psa := map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": names.HostNamespace, "labels": merge(merge(labels, policyLabels), map[string]string{"pod-security.kubernetes.io/enforce": platform.PodSecurityEnforceLevel}), "annotations": policyAnnotations}}
	objects := []map[string]any{psa, quota, defaults, defaultDeny, dnsAllow, childAPIAllow, sharedAgentEgress, portForwardRole, portForwardBinding}
	result := make([]*unstructured.Unstructured, 0, len(objects))
	for _, object := range objects {
		unstructuredObject, err := toUnstructured(object)
		if err != nil {
			return nil, err
		}
		result = append(result, unstructuredObject)
	}
	return result, nil
}

func ipPeers(addresses []string) []any {
	peers := make([]any, 0, len(addresses))
	for _, address := range addresses {
		if strings.TrimSpace(address) == "" {
			continue
		}
		peers = append(peers, map[string]any{"ipBlock": map[string]any{"cidr": address + "/32"}})
	}
	return peers
}

func acceleratorKeyList(keys []string) string {
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

func RenderK3kCluster(cluster api.VirtualCluster, resolved api.ResolvedSnapshot, names Names) (*unstructured.Unstructured, error) {
	labels := helpers.OwnedHostLabels(cluster.Spec.CellRef.Name, cluster.Namespace, cluster.Name, string(cluster.UID), resolved.ProfileHash)
	serverArgs := append([]string(nil), platform.ServerArgs...)
	for _, address := range names.HostAPIAddresses {
		argument := "--tls-san=" + address
		if !containsArgument(serverArgs, argument) {
			serverArgs = append(serverArgs, argument)
		}
	}
	controlPlane := quantityMap(platform.ControlPlaneReservation())
	reflectedSystem := quantityMap(platform.ReflectedSystemReservation())
	spec := map[string]any{
		"mode":       "shared",
		"version":    resolved.K3kChildVersion,
		"expose":     map[string]any{"nodePort": map[string]any{}},
		"serverArgs": stringSliceAny(serverArgs),
		"persistence": map[string]any{
			"type":               "dynamic",
			"storageClassName":   resolved.Storage.Class.HostName,
			"storageRequestSize": "2Gi",
		},
		"serverResources": map[string]any{"requests": controlPlane, "limits": controlPlane},
		"workerResources": map[string]any{"requests": reflectedSystem, "limits": reflectedSystem},
	}
	object := map[string]any{"apiVersion": "k3k.io/v1beta1", "kind": "Cluster", "metadata": map[string]any{"name": names.K3kCluster, "namespace": names.HostNamespace, "labels": labels}, "spec": spec}
	return toUnstructured(object)
}

func containsArgument(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func RenderChildStorageClass(name string) (*storagev1.StorageClass, error) {
	if name == "" {
		return nil, errors.New("storage class name is required")
	}
	binding := platform.VolumeBindingMode
	reclaim := platform.ReclaimPolicy
	expand := platform.AllowVolumeExpansion
	return &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"kubecell.io/managed": "true"}}, Provisioner: "topolvm.io", VolumeBindingMode: &binding, ReclaimPolicy: &reclaim, AllowVolumeExpansion: &expand}, nil
}

func quantityMap(values corev1.ResourceList) map[string]any {
	result := map[string]any{}
	for key, value := range values {
		result[string(key)] = value.String()
	}
	return result
}
func stringSliceAny(values []string) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}
func merge(left, right map[string]string) map[string]string {
	result := map[string]string{}
	for key, value := range left {
		result[string(key)] = value
	}
	for key, value := range right {
		result[string(key)] = value
	}
	return result
}
func networkPolicy(name, namespace string, labels map[string]string, spec map[string]any) map[string]any {
	spec["podSelector"] = spec["podSelector"]
	return map[string]any{"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy", "metadata": map[string]any{"name": name, "namespace": namespace, "labels": labels}, "spec": spec}
}
func toUnstructured(value map[string]any) (*unstructured.Unstructured, error) {
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&unstructured.Unstructured{Object: value})
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: raw}, nil
}

// ChildIngressBackend is a snapshot of a child-cluster Ingress plus its backend Service/Endpoints.
// A nil Service/Endpoints means it could not be read, so the backend rule is skipped (eventual consistency on the child side).
type ChildIngressBackend struct {
	Ingress   networkingv1.Ingress
	Service   *corev1.Service
	Endpoints *corev1.Endpoints
}

// RenderIngressMirror mirrors child-cluster Ingresses with class=kubecell into host objects (§11.3):
// one host Service per backend (no selector, unnamed port) plus a same-name host Endpoints (address subset copy with port names stripped),
// plus one host Ingress (class kubecell). The child host fields are always ignored;
// the host hostname is always built as <ingress name>.<vc name>.<appsSuffix>.
// Ingresses of other classes are silently skipped; when same-named Ingresses appear in different child namespaces,
// host object names carry the child-namespace prefix while hostnames stay identical (later render wins, known v1 limitation).
func RenderIngressMirror(cluster api.VirtualCluster, resolved api.ResolvedSnapshot, appsSuffix string, backends []ChildIngressBackend) ([]*unstructured.Unstructured, error) {
	if strings.TrimSpace(appsSuffix) == "" {
		return nil, errors.New("apps suffix is required")
	}
	labels := helpers.OwnedHostLabels(cluster.Spec.CellRef.Name, cluster.Namespace, cluster.Name, string(cluster.UID), resolved.ProfileHash)
	namespace := resolved.HostNamespace
	if namespace == "" {
		return nil, errors.New("resolved Host namespace is required")
	}
	result := make([]*unstructured.Unstructured, 0)
	seen := map[string]struct{}{}
	type pendingIngress struct {
		name  string
		rules []any
	}
	pending := map[string]*pendingIngress{}
	order := make([]string, 0)
	for _, backend := range backends {
		ingress := backend.Ingress
		if ingress.Spec.IngressClassName == nil || *ingress.Spec.IngressClassName != platform.IngressClassName {
			continue
		}
		if ingress.Spec.Rules == nil {
			continue
		}
		hostname := ingress.Name + "." + cluster.Name + "." + appsSuffix
		for _, rule := range ingress.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service == nil {
					continue
				}
				port, ok := mirrorServicePort(backend.Service, path.Backend.Service.Port)
				if !ok {
					continue
				}
				serviceName := truncateName("ing-" + ingress.Name + "-" + path.Backend.Service.Name + "-" + portName(port))
				if _, exists := seen[serviceName]; !exists {
					seen[serviceName] = struct{}{}
					service, err := toUnstructured(mirrorService(serviceName, namespace, labels, port))
					if err != nil {
						return nil, err
					}
					result = append(result, service)
					endpoints, err := toUnstructured(mirrorEndpoints(serviceName, namespace, labels, backend.Endpoints))
					if err != nil {
						return nil, err
					}
					result = append(result, endpoints)
				}
				hostIngressName := truncateName(ingress.Namespace + "-" + ingress.Name)
				entry, ok := pending[hostIngressName]
				if !ok {
					entry = &pendingIngress{name: hostIngressName}
					pending[hostIngressName] = entry
					order = append(order, hostIngressName)
				}
				entry.rules = append(entry.rules, mirrorRule(hostname, path, serviceName, port))
			}
		}
	}
	for _, key := range order {
		entry := pending[key]
		ingress, err := toUnstructured(mirrorIngress(entry.name, namespace, labels, entry.rules))
		if err != nil {
			return nil, err
		}
		result = append(result, ingress)
	}
	return result, nil
}

func mirrorServicePort(service *corev1.Service, port networkingv1.ServiceBackendPort) (int32, bool) {
	if port.Number > 0 {
		return port.Number, true
	}
	if service == nil || port.Name == "" {
		return 0, false
	}
	for _, servicePort := range service.Spec.Ports {
		if servicePort.Name == port.Name && servicePort.Port > 0 {
			return servicePort.Port, true
		}
	}
	return 0, false
}

func portName(port int32) string {
	return fmt.Sprintf("%d", port)
}

func truncateName(value string) string {
	if len(value) <= 63 {
		return strings.Trim(value, "-")
	}
	return strings.Trim(value[:63], "-")
}

func mirrorService(name, namespace string, labels map[string]string, port int32) map[string]any {
	// Ports are intentionally unnamed: Traefik matches EndpointSlice ports by name, while child
	// Endpoints ports are often unnamed; unnamed ports always match by number.
	return map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": name, "namespace": namespace, "labels": labels}, "spec": map[string]any{"ports": []any{map[string]any{"protocol": "TCP", "port": int64(port), "targetPort": int64(port)}}}}
}

func mirrorEndpoints(name, namespace string, labels map[string]string, source *corev1.Endpoints) map[string]any {
	subsets := []any{}
	if source != nil {
		for _, subset := range source.Subsets {
			addresses := []any{}
			for _, address := range subset.Addresses {
				entry := map[string]any{"ip": address.IP}
				if address.Hostname != "" {
					entry["hostname"] = address.Hostname
				}
				if address.NodeName != nil {
					entry["nodeName"] = *address.NodeName
				}
				addresses = append(addresses, entry)
			}
			ports := []any{}
			for _, port := range subset.Ports {
				// Strip all port names: when an Ingress backend specifies a numeric port, Traefik matching by name
				// misses (e.g. a chart's "http") and returns 503; unnamed ports always match by number (§11.3, M9).
				entry := map[string]any{"protocol": string(port.Protocol), "port": int64(port.Port)}
				ports = append(ports, entry)
			}
			subsets = append(subsets, map[string]any{"addresses": addresses, "ports": ports})
		}
	}
	return map[string]any{"apiVersion": "v1", "kind": "Endpoints", "metadata": map[string]any{"name": name, "namespace": namespace, "labels": labels}, "subsets": subsets}
}

func mirrorRule(hostname string, path networkingv1.HTTPIngressPath, serviceName string, port int32) map[string]any {
	pathType := string(networkingv1.PathTypeImplementationSpecific)
	if path.PathType != nil && *path.PathType != "" {
		pathType = string(*path.PathType)
	}
	backend := map[string]any{"service": map[string]any{"name": serviceName, "port": map[string]any{"number": int64(port)}}}
	return map[string]any{"host": hostname, "http": map[string]any{"paths": []any{map[string]any{"path": path.Path, "pathType": pathType, "backend": backend}}}}
}

func mirrorIngress(name, namespace string, labels map[string]string, rules []any) map[string]any {
	className := platform.IngressClassName
	// Traefik's providers.kubernetesIngress.ingressClass runs in annotation mode,
	// honoring only the kubernetes.io/ingress.class annotation, not the spec field: write both.
	annotations := map[string]string{"kubernetes.io/ingress.class": className}
	return map[string]any{"apiVersion": "networking.k8s.io/v1", "kind": "Ingress", "metadata": map[string]any{"name": name, "namespace": namespace, "labels": labels, "annotations": annotations}, "spec": map[string]any{"ingressClassName": className, "rules": rules}}
}

var _ = intstr.FromInt

package helpers

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/types"
)

const (
	LabelManagedBy               = "app.kubernetes.io/managed-by"
	LabelCellName                = "kubecell.io/cell-name"
	LabelVirtualClusterName      = "kubecell.io/virtual-cluster-name"
	LabelVirtualClusterNamespace = "kubecell.io/virtual-cluster-namespace"
	LabelVirtualClusterUID       = "kubecell.io/virtual-cluster-uid"
	LabelExecutionMode           = "kubecell.io/execution-mode"
	LabelProfileHash             = "kubecell.io/profile-hash"
	// K3kClusterNameLabel is stamped on reflected Pods by the K3k agent (value is the K3k cluster name with UID suffix,
	// unique within a VC). K3k sync overwrites labels, so self-applied labels do not survive; ownership matching uses it (§5).
	K3kClusterNameLabel = "k3k.io/clusterName"
	ManagedByKubecell   = "kubecell"
	ExecutionModeShared = "shared-virtual-kubelet"
)

type VirtualClusterNames struct {
	HostNamespace   string
	K3kCluster      string
	FoundationWork  string
	InstanceWork    string
	AdminKubeconfig string
}

func NamesForVirtualCluster(namespace, name string, uid types.UID) VirtualClusterNames {
	slug := dnsSlug(namespace + "-" + name)
	uidPart := strings.ToLower(string(uid))
	if len(uidPart) > 8 {
		uidPart = uidPart[:8]
	}
	// Truncation only eats the middle slug; the UID suffix is always preserved (KI-17: truncating the tail drops the UID and causes cross-VC collisions).
	maxSlug := 63 - len("kc-") - 1 - len(uidPart)
	if maxSlug < 1 {
		maxSlug = 1
	}
	if len(slug) > maxSlug {
		slug = strings.Trim(slug[:maxSlug], "-")
	}
	hostNamespace := "kc-" + slug + "-" + uidPart
	k3kCluster := "kc-" + uidPart
	return VirtualClusterNames{
		HostNamespace:   hostNamespace,
		K3kCluster:      k3kCluster,
		FoundationWork:  hostNamespace + "-foundation",
		InstanceWork:    hostNamespace + "-instance",
		AdminKubeconfig: "kubecell-" + truncateDNS(dnsSlug(name)) + "-" + uidPart + "-admin-kubeconfig",
	}
}

func dnsSlug(value string) string {
	value = strings.ToLower(value)
	value = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func truncateDNS(value string) string {
	if len(value) <= 63 {
		return strings.Trim(value, "-")
	}
	return strings.Trim(value[:63], "-")
}

func ProfileHash(value any) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%#v", value)))
	return fmt.Sprintf("%x", digest[:8])
}

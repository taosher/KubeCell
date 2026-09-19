package platform

import (
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	api "github.com/kubecell/kubecell/api/v1alpha1"
)

const (
	K3kVersion      = "v1.2.0"
	HostK3sVersion  = "v1.36.3+k3s1"
	ChildK3sVersion = "v1.34.2+k3s1"

	K3kControllerNamespace    = "k3k-system"
	ManagedServiceAccountName = "kubecell-host-reader"

	ProfileLabelKey = "hardware.kubecell.io/profile"

	IngressClassName = "kubecell"

	PodSecurityEnforceLevel = "baseline"

	NodePortMin int32 = 30000
	NodePortMax int32 = 32767

	HostPathDenyMessage = "hostPath volumes are not supported in KubeCell shared-mode namespaces; use a PVC with the mapped TopoLVM StorageClass"

	VolumeBindingMode    = storagev1.VolumeBindingWaitForFirstConsumer
	ReclaimPolicy        = corev1.PersistentVolumeReclaimRetain
	AllowVolumeExpansion = true
)

var ServerArgs = []string{"--disable=traefik", "--disable=local-storage"}

func ControlPlaneReservation() api.ResourceList {
	return api.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("500m"),
		corev1.ResourceMemory: resource.MustParse("1Gi"),
	}
}

func ReflectedSystemReservation() api.ResourceList {
	return api.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("100m"),
		corev1.ResourceMemory: resource.MustParse("256Mi"),
	}
}

// DefaultContainerResources is the container default for the host-namespace LimitRange (§3.2 platform constant):
// third-party Pods without declared resources are defaulted automatically. The value always equals the reflected system reservation: the K3k agent uses
// pod-level reservations plus empty containers, and a larger default would blow up K3k's own Pods (diagnosed online in M9),
// so both share this constant to prevent drift.
func DefaultContainerResources() api.ResourceList {
	return ReflectedSystemReservation()
}

func CombinedReservation() api.ResourceList {
	combined := ControlPlaneReservation()
	for name, quantity := range ReflectedSystemReservation() {
		if existing, found := combined[name]; found {
			merged := existing.DeepCopy()
			addend := quantity.DeepCopy()
			merged.Add(addend)
			combined[name] = merged
		} else {
			combined[name] = quantity.DeepCopy()
		}
	}
	return combined
}

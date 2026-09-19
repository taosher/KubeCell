package controller

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/controller/helpers"
	"github.com/kubecell/kubecell/internal/platform"
	"github.com/kubecell/kubecell/internal/provider/profile"
)

func resolveVirtualCluster(cluster *api.VirtualCluster, cell *api.Cell, class *api.VirtualNodeClass) (api.ResolvedSnapshot, error) {
	if cluster == nil || cell == nil || class == nil {
		return api.ResolvedSnapshot{}, fmt.Errorf("VirtualCluster, Cell, and VirtualNodeClass are required")
	}
	machineProfile := cell.Spec.MachineProfile
	if machineProfile.Name == "" {
		return api.ResolvedSnapshot{}, fmt.Errorf("Cell machine profile is not declared")
	}
	for resourceName, quantity := range class.Spec.Entitlement.WorkloadHard {
		if !strings.HasPrefix(string(resourceName), "requests.") && !strings.HasPrefix(string(resourceName), "limits.") {
			continue
		}
		baseName := strings.TrimPrefix(strings.TrimPrefix(string(resourceName), "requests."), "limits.")
		if !strings.Contains(baseName, "/") {
			continue
		}
		found := false
		for _, device := range machineProfile.Devices {
			if device.ResourceName == baseName {
				found = true
				if quantity.Sign() <= 0 {
					return api.ResolvedSnapshot{}, fmt.Errorf("device entitlement %q must be a positive integer", resourceName)
				}
				if _, ok := quantity.AsInt64(); !ok {
					return api.ResolvedSnapshot{}, fmt.Errorf("device entitlement %q must be a positive integer", resourceName)
				}
				break
			}
		}
		if !found {
			return api.ResolvedSnapshot{}, fmt.Errorf("device entitlement %q is not declared by machine profile %q", resourceName, machineProfile.Name)
		}
	}
	for _, device := range machineProfile.Devices {
		requestKey := corev1.ResourceName("requests." + device.ResourceName)
		limitKey := corev1.ResourceName("limits." + device.ResourceName)
		request, requestFound := class.Spec.Entitlement.WorkloadHard[requestKey]
		limit, limitFound := class.Spec.Entitlement.WorkloadHard[limitKey]
		if requestFound != limitFound || (requestFound && !request.Equal(limit)) {
			return api.ResolvedSnapshot{}, fmt.Errorf("device entitlement for %q must have equal request and limit", device.ResourceName)
		}
	}
	acceleratorKeys := make([]string, 0, len(machineProfile.Devices))
	for _, device := range machineProfile.Devices {
		acceleratorKeys = append(acceleratorKeys, device.ResourceName)
	}
	childProvider, err := profile.ResolveChildVersion(platform.ChildK3sVersion)
	if err != nil {
		return api.ResolvedSnapshot{}, err
	}
	return api.ResolvedSnapshot{
		CellUID:            string(cell.UID),
		ManagedClusterName: cell.Spec.ManagedClusterRef.Name,
		ClassUID:           string(class.UID),
		ProfileHash:        profileFingerprint(machineProfile, class.Spec.Entitlement.WorkloadHard, class.Spec.StorageClassName),
		MachineProfileName: machineProfile.Name,
		K3kVersion:         platform.K3kVersion,
		ChildK3sVersion:    childProvider.Canonical,
		K3kChildVersion:    childProvider.K3kSpec,
		ChildImageTag:      childProvider.ImageTag,
		Quota:              class.Spec.Entitlement.WorkloadHard,
		Reservation: api.ReservationSpec{
			ControlPlane:    platform.ControlPlaneReservation(),
			ReflectedSystem: platform.ReflectedSystemReservation(),
			Policy:          api.ReservationPolicyIncludedInHostQuota,
		},
		AcceleratorKeys: acceleratorKeys,
		Storage: api.StorageMappingSpec{Class: api.StorageClassMapping{
			ChildName:            class.Spec.StorageClassName,
			HostName:             class.Spec.StorageClassName,
			VolumeBindingMode:    platform.VolumeBindingMode,
			ReclaimPolicy:        platform.ReclaimPolicy,
			AllowVolumeExpansion: platform.AllowVolumeExpansion,
		}},
	}, nil
}

// profileFingerprint hashes only stable string content (names, quantities,
// storage). It never formats structs with pointers, so repeated evaluations
// of equal inputs produce equal hashes.
func profileFingerprint(profile api.MachineProfile, quota api.ResourceList, storageClass string) string {
	parts := []string{"profile=" + profile.Name, "storage=" + storageClass}
	for _, device := range profile.Devices {
		parts = append(parts, "device="+device.Name+"/"+device.ResourceName)
	}
	keys := make([]string, 0, len(quota))
	for key := range quota {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	for _, key := range keys {
		quantity := quota[corev1.ResourceName(key)]
		parts = append(parts, key+"="+quantity.String())
	}
	return helpers.ProfileHash(strings.Join(parts, "\n"))
}

package v1alpha1

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

func ValidateCellSpec(spec CellSpec) []error {
	var errors []error
	if spec.ManagedClusterRef.Name == "" {
		errors = append(errors, fmt.Errorf("managedClusterRef.name is required"))
	}
	if spec.MachineProfile.Name == "" {
		errors = append(errors, fmt.Errorf("machineProfile.name is required"))
	} else if nameErrors := validation.IsDNS1123Subdomain(spec.MachineProfile.Name); len(nameErrors) != 0 {
		errors = append(errors, fmt.Errorf("machineProfile %q: %s", spec.MachineProfile.Name, strings.Join(nameErrors, "; ")))
	}
	for _, device := range spec.MachineProfile.Devices {
		errors = append(errors, validateDeviceContract(device)...)
	}
	return errors
}

func validateDeviceContract(device DeviceContract) []error {
	var errors []error
	if device.Name == "" || device.ResourceName == "" {
		errors = append(errors, fmt.Errorf("device name and resourceName are required"))
	}
	if device.ResourceName != "" && !isExtendedResourceName(device.ResourceName) {
		errors = append(errors, fmt.Errorf("device %q resourceName %q must be a qualified extended resource", device.Name, device.ResourceName))
	}
	return errors
}

func ValidateVirtualNodeClassSpec(spec VirtualNodeClassSpec) []error {
	var errors []error
	if spec.StorageClassName == "" {
		errors = append(errors, fmt.Errorf("storageClassName is required"))
	} else if nameErrors := validation.IsDNS1123Subdomain(spec.StorageClassName); len(nameErrors) != 0 {
		errors = append(errors, fmt.Errorf("storageClassName %q: %s", spec.StorageClassName, strings.Join(nameErrors, "; ")))
	}
	for key, quantity := range spec.Entitlement.WorkloadHard {
		if quantity.Sign() <= 0 {
			errors = append(errors, fmt.Errorf("workloadHard[%q] must be positive", key))
		}
		if strings.HasPrefix(string(key), "requests.") {
			limitKey := resourceNameWithPrefix(key, "requests.", "limits.")
			limit, found := spec.Entitlement.WorkloadHard[limitKey]
			if !found || !quantity.Equal(limit) {
				errors = append(errors, fmt.Errorf("workloadHard[%q] must equal %q", key, limitKey))
			}
		}
		if strings.HasPrefix(string(key), "limits.") {
			requestKey := resourceNameWithPrefix(key, "limits.", "requests.")
			if request, found := spec.Entitlement.WorkloadHard[requestKey]; !found || !quantity.Equal(request) {
				errors = append(errors, fmt.Errorf("workloadHard[%q] must equal %q", key, requestKey))
			}
		}
		if !validQuotaResourceKey(string(key)) {
			errors = append(errors, fmt.Errorf("workloadHard[%q] is not an allowed quota resource", key))
		}
	}
	return errors
}

func isExtendedResourceName(value string) bool {
	parts := strings.Split(value, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && len(validation.IsDNS1123Subdomain(parts[0])) == 0 && len(validation.IsQualifiedName(value)) == 0
}

func validQuotaResourceKey(value string) bool {
	for _, prefix := range []string{"requests.", "limits."} {
		if strings.HasPrefix(value, prefix) {
			resourceName := strings.TrimPrefix(value, prefix)
			return resourceName == "cpu" || resourceName == "memory" || resourceName == "ephemeral-storage" || isExtendedResourceName(resourceName)
		}
	}
	return false
}

func resourceNameWithPrefix(value corev1.ResourceName, from, to string) corev1.ResourceName {
	return corev1.ResourceName(to + strings.TrimPrefix(string(value), from))
}

func ValidateVirtualClusterSpec(spec VirtualClusterSpec) []error {
	var errors []error
	if spec.CellRef.Name == "" {
		errors = append(errors, fmt.Errorf("cellRef.name is required"))
	}
	if spec.ClassRef.Name == "" {
		errors = append(errors, fmt.Errorf("classRef.name is required"))
	}
	return errors
}

func ValidateVirtualClusterUpdate(oldSpec, newSpec VirtualClusterSpec) []error {
	var errors []error
	if oldSpec.CellRef != newSpec.CellRef {
		errors = append(errors, fmt.Errorf("cellRef is immutable"))
	}
	if oldSpec.ClassRef != newSpec.ClassRef {
		errors = append(errors, fmt.Errorf("classRef is immutable"))
	}
	return errors
}

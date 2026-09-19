package v1alpha1

import (
	"fmt"
	"strings"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
)

// ManagementNamespace is the only namespace that may hold Cell,
// VirtualCluster, and VirtualClusterPlan objects.
const ManagementNamespace = "kubecell-system"

func (r *VirtualCluster) Default() {}

func (r *VirtualCluster) ValidateCreate() error {
	errors := ValidateVirtualClusterSpec(r.Spec)
	if r.Namespace != "" && r.Namespace != ManagementNamespace {
		errors = append(errors, fmt.Errorf("VirtualCluster must be created in namespace %q", ManagementNamespace))
	}
	return validationError("VirtualCluster", errors)
}

func (r *VirtualCluster) ValidateUpdate(old runtime.Object) error {
	previous, ok := old.(*VirtualCluster)
	if !ok {
		return fmt.Errorf("old object is %T, want *VirtualCluster", old)
	}
	errors := ValidateVirtualClusterSpec(r.Spec)
	errors = append(errors, ValidateVirtualClusterUpdate(previous.Spec, r.Spec)...)
	return validationError("VirtualCluster", errors)
}

func (r *VirtualCluster) ValidateDelete() error { return nil }

func (r *VirtualClusterPlan) Default() {}

func (r *VirtualClusterPlan) ValidateCreate() error {
	errors := ValidateVirtualClusterSpec(r.Spec.VirtualCluster)
	if r.Namespace != "" && r.Namespace != ManagementNamespace {
		errors = append(errors, fmt.Errorf("VirtualClusterPlan must be created in namespace %q", ManagementNamespace))
	}
	return validationError("VirtualClusterPlan", errors)
}

func (r *VirtualClusterPlan) ValidateUpdate(old runtime.Object) error {
	previous, ok := old.(*VirtualClusterPlan)
	if !ok {
		return fmt.Errorf("old object is %T, want *VirtualClusterPlan", old)
	}
	errors := ValidateVirtualClusterSpec(r.Spec.VirtualCluster)
	errors = append(errors, ValidateVirtualClusterUpdate(previous.Spec.VirtualCluster, r.Spec.VirtualCluster)...)
	return validationError("VirtualClusterPlan", errors)
}

func (r *VirtualClusterPlan) ValidateDelete() error { return nil }

func (r *Cell) Default() {}

func (r *Cell) ValidateCreate() error {
	errors := ValidateCellSpec(r.Spec)
	if r.Namespace != "" && r.Namespace != ManagementNamespace {
		errors = append(errors, fmt.Errorf("Cell must be created in namespace %q", ManagementNamespace))
	}
	return validationError("Cell", errors)
}

// ValidateUpdate enforces structural validity plus, when referenced is true,
// immutability of the binding declaration and the machine profile. The
// admission layer computes referenced by listing live VirtualClusters; unit
// tests pass it explicitly.
func (r *Cell) ValidateUpdate(old runtime.Object, referenced bool) error {
	previous, ok := old.(*Cell)
	if !ok {
		return fmt.Errorf("old object is %T, want *Cell", old)
	}
	errors := ValidateCellSpec(r.Spec)
	if !referenced {
		return validationError("Cell", errors)
	}
	if previous.Spec.ManagedClusterRef != r.Spec.ManagedClusterRef {
		errors = append(errors, fmt.Errorf("managedClusterRef is immutable while referenced by a VirtualCluster"))
	}
	if previous.Spec.MachineProfile.Name != r.Spec.MachineProfile.Name ||
		!sameDeviceContracts(previous.Spec.MachineProfile.Devices, r.Spec.MachineProfile.Devices) {
		errors = append(errors, fmt.Errorf("machineProfile name and device contracts are immutable while referenced by a VirtualCluster"))
	}
	return validationError("Cell", errors)
}

func (r *Cell) ValidateDelete() error { return nil }

func (r *VirtualNodeClass) Default() {}

func (r *VirtualNodeClass) ValidateCreate() error {
	return validationError("VirtualNodeClass", ValidateVirtualNodeClassSpec(r.Spec))
}

// ValidateUpdate enforces structural validity plus, when referenced is true,
// whole-spec immutability (change quota by creating a new class). See Cell
// for how referenced is computed.
func (r *VirtualNodeClass) ValidateUpdate(old runtime.Object, referenced bool) error {
	previous, ok := old.(*VirtualNodeClass)
	if !ok {
		return fmt.Errorf("old object is %T, want *VirtualNodeClass", old)
	}
	errors := ValidateVirtualNodeClassSpec(r.Spec)
	if !referenced {
		return validationError("VirtualNodeClass", errors)
	}
	if !apiequality.Semantic.DeepEqual(previous.Spec, r.Spec) {
		errors = append(errors, fmt.Errorf("VirtualNodeClass spec is immutable while referenced by a VirtualCluster"))
	}
	return validationError("VirtualNodeClass", errors)
}

func (r *VirtualNodeClass) ValidateDelete() error { return nil }

func validationError(kind string, errors []error) error {
	if len(errors) == 0 {
		return nil
	}
	messages := make([]string, 0, len(errors))
	for _, err := range errors {
		messages = append(messages, err.Error())
	}
	return fmt.Errorf("%s validation failed: %s", kind, strings.Join(messages, "; "))
}

func sameDeviceContracts(left, right []DeviceContract) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

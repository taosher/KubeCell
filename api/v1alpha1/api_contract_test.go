package v1alpha1

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func validCellSpec() CellSpec {
	return CellSpec{
		ManagedClusterRef: ObjectReference{Name: "cell-a"},
		MachineProfile: MachineProfile{
			Name: "ascend-910b",
			Devices: []DeviceContract{{
				Name:         "ascend",
				ResourceName: "huawei.com/Ascend910",
			}},
		},
	}
}

func TestValidateCellSpecAcceptsMinimalProfile(t *testing.T) {
	if validationErrors := ValidateCellSpec(validCellSpec()); len(validationErrors) != 0 {
		t.Fatalf("ValidateCellSpec() errors = %v, want none", validationErrors)
	}
}

func TestValidateCellSpecRejectsMissingRefAndBadDevice(t *testing.T) {
	spec := validCellSpec()
	spec.ManagedClusterRef = ObjectReference{}
	spec.MachineProfile.Devices = []DeviceContract{{Name: "gpu", ResourceName: "gpu"}}

	validationErrors := ValidateCellSpec(spec)
	if len(validationErrors) != 2 {
		t.Fatalf("ValidateCellSpec() errors = %v, want missing-ref and bad-resource errors", validationErrors)
	}
}

func TestValidateVirtualNodeClassRequiresEqualPositiveDeviceQuota(t *testing.T) {
	classSpec := VirtualNodeClassSpec{
		StorageClassName: "topolvm-provisioner",
		Entitlement: EntitlementSpec{WorkloadHard: ResourceList{
			"requests.huawei.com/Ascend910": resource.MustParse("1"),
			"limits.huawei.com/Ascend910":   resource.MustParse("2"),
		}},
	}

	validationErrors := ValidateVirtualNodeClassSpec(classSpec)
	if len(validationErrors) == 0 {
		t.Fatal("ValidateVirtualNodeClassSpec() errors = nil, want device request/limit mismatch")
	}
}

func TestValidateVirtualNodeClassRequiresStorageClassName(t *testing.T) {
	classSpec := VirtualNodeClassSpec{
		Entitlement: EntitlementSpec{WorkloadHard: ResourceList{
			"requests.cpu": resource.MustParse("4"),
			"limits.cpu":   resource.MustParse("4"),
		}},
	}

	validationErrors := ValidateVirtualNodeClassSpec(classSpec)
	if len(validationErrors) == 0 {
		t.Fatal("ValidateVirtualNodeClassSpec() errors = nil, want missing storageClassName")
	}
}

func TestValidateVirtualNodeClassAcceptsBalancedQuota(t *testing.T) {
	classSpec := VirtualNodeClassSpec{
		StorageClassName: "topolvm-provisioner",
		Entitlement: EntitlementSpec{WorkloadHard: ResourceList{
			"requests.cpu":                  resource.MustParse("4"),
			"limits.cpu":                    resource.MustParse("4"),
			"requests.huawei.com/Ascend910": resource.MustParse("1"),
			"limits.huawei.com/Ascend910":   resource.MustParse("1"),
		}},
	}

	if validationErrors := ValidateVirtualNodeClassSpec(classSpec); len(validationErrors) != 0 {
		t.Fatalf("ValidateVirtualNodeClassSpec() errors = %v, want none", validationErrors)
	}
}

func TestValidateVirtualClusterRequiresRefs(t *testing.T) {
	if validationErrors := ValidateVirtualClusterSpec(VirtualClusterSpec{}); len(validationErrors) != 2 {
		t.Fatalf("ValidateVirtualClusterSpec() errors = %v, want cellRef and classRef errors", validationErrors)
	}

	spec := VirtualClusterSpec{CellRef: ObjectReference{Name: "cell-a"}, ClassRef: ObjectReference{Name: "class-a"}}
	if validationErrors := ValidateVirtualClusterSpec(spec); len(validationErrors) != 0 {
		t.Fatalf("ValidateVirtualClusterSpec() errors = %v, want none", validationErrors)
	}
}

func TestValidateVirtualClusterUpdateProtectsRefs(t *testing.T) {
	oldSpec := VirtualClusterSpec{CellRef: ObjectReference{Name: "cell-a"}, ClassRef: ObjectReference{Name: "class-a"}}
	newSpec := oldSpec
	newSpec.CellRef.Name = "cell-b"
	if errors := ValidateVirtualClusterUpdate(oldSpec, newSpec); len(errors) == 0 {
		t.Fatal("ValidateVirtualClusterUpdate() allowed cellRef change")
	}
	newSpec = oldSpec
	newSpec.ClassRef.Name = "class-b"
	if errors := ValidateVirtualClusterUpdate(oldSpec, newSpec); len(errors) == 0 {
		t.Fatal("ValidateVirtualClusterUpdate() allowed classRef change")
	}
	if errors := ValidateVirtualClusterUpdate(oldSpec, oldSpec); len(errors) != 0 {
		t.Fatalf("ValidateVirtualClusterUpdate() errors = %v, want none", errors)
	}
}

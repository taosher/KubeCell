package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestVirtualClusterValidateCreateRequiresRefsAndNamespace(t *testing.T) {
	cluster := &VirtualCluster{ObjectMeta: metav1.ObjectMeta{Name: "demo"}}
	if err := cluster.ValidateCreate(); err == nil {
		t.Fatal("ValidateCreate() error = nil, want missing refs")
	}

	cluster.Spec = VirtualClusterSpec{CellRef: ObjectReference{Name: "cell-a"}, ClassRef: ObjectReference{Name: "class-a"}}
	if err := cluster.ValidateCreate(); err != nil {
		t.Fatalf("ValidateCreate() error = %v, want none", err)
	}

	cluster.Namespace = "team-a"
	if err := cluster.ValidateCreate(); err == nil {
		t.Fatal("ValidateCreate() error = nil, want namespace rejection")
	}
}

func TestVirtualClusterValidateUpdateRejectsRefChange(t *testing.T) {
	oldCluster := &VirtualCluster{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: ManagementNamespace}, Spec: VirtualClusterSpec{
		CellRef: ObjectReference{Name: "cell-a"}, ClassRef: ObjectReference{Name: "class-a"},
	}}
	newCluster := oldCluster.DeepCopy()
	newCluster.Spec.CellRef.Name = "cell-b"

	if err := newCluster.ValidateUpdate(oldCluster); err == nil {
		t.Fatal("ValidateUpdate() error = nil, want immutable cellRef rejection")
	}
}

func TestCellValidateUpdateHonorsReferencedFlag(t *testing.T) {
	oldCell := &Cell{
		ObjectMeta: metav1.ObjectMeta{Name: "cell-a", Namespace: ManagementNamespace},
		Spec: CellSpec{
			ManagedClusterRef: ObjectReference{Name: "cell-a"},
			MachineProfile: MachineProfile{Name: "ascend-910b", Devices: []DeviceContract{
				{Name: "ascend", ResourceName: "huawei.com/Ascend910"},
			}},
		},
	}

	changed := oldCell.DeepCopy()
	changed.Spec.MachineProfile.Devices = []DeviceContract{{Name: "ascend", ResourceName: "nvidia.com/gpu"}}

	if err := changed.ValidateUpdate(oldCell, false); err != nil {
		t.Fatalf("ValidateUpdate(unreferenced) error = %v, want none", err)
	}
	if err := changed.ValidateUpdate(oldCell, true); err == nil {
		t.Fatal("ValidateUpdate(referenced) error = nil, want device contract immutability")
	}
}

func TestVirtualNodeClassValidateUpdateHonorsReferencedFlag(t *testing.T) {
	oldClass := &VirtualNodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "class-a"},
		Spec: VirtualNodeClassSpec{
			StorageClassName: "topolvm-provisioner",
		},
	}

	changed := oldClass.DeepCopy()
	changed.Spec.StorageClassName = "other"

	if err := changed.ValidateUpdate(oldClass, false); err != nil {
		t.Fatalf("ValidateUpdate(unreferenced) error = %v, want none", err)
	}
	if err := changed.ValidateUpdate(oldClass, true); err == nil {
		t.Fatal("ValidateUpdate(referenced) error = nil, want spec immutability")
	}
}

func TestCellValidateCreateRejectsWrongNamespace(t *testing.T) {
	cell := &Cell{ObjectMeta: metav1.ObjectMeta{Namespace: "team-a"}, Spec: validCellSpec()}
	if err := cell.ValidateCreate(); err == nil {
		t.Fatal("ValidateCreate() error = nil, want namespace rejection")
	}

	cell.Namespace = ManagementNamespace
	if err := cell.ValidateCreate(); err != nil {
		t.Fatalf("ValidateCreate() error = %v, want none", err)
	}
}

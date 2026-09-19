package admission

import (
	"context"
	"testing"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestVirtualClusterWebhookDelegatesDefaultAndValidation(t *testing.T) {
	object := &api.VirtualCluster{}
	if err := (VirtualClusterDefaulter{}).Default(context.Background(), object); err != nil {
		t.Fatalf("Default() error = %v", err)
	}
	if _, err := (VirtualClusterValidator{}).ValidateCreate(context.Background(), object); err == nil {
		t.Fatal("ValidateCreate() error = nil, want invalid reference error")
	}
}

func TestVirtualClusterWebhookRejectsDuplicateActiveNameAcrossNamespaces(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	validator := VirtualClusterValidator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&api.VirtualCluster{ObjectMeta: metav1.ObjectMeta{Name: "training", Namespace: "team-a"}},
	).Build()}
	object := &api.VirtualCluster{ObjectMeta: metav1.ObjectMeta{Name: "training", Namespace: "team-b"}}
	if _, err := validator.ValidateCreate(context.Background(), object); err == nil {
		t.Fatal("ValidateCreate() error = nil, want duplicate-name rejection")
	}
}

func TestCellWebhookEnforcesImmutabilityOnlyWhenReferenced(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	withRef := CellValidator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&api.VirtualCluster{ObjectMeta: metav1.ObjectMeta{Name: "child", Namespace: api.ManagementNamespace}, Spec: api.VirtualClusterSpec{
			CellRef: api.ObjectReference{Name: "cell-a"}, ClassRef: api.ObjectReference{Name: "class-a"},
		}},
	).Build()}
	withoutRef := CellValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	oldCell := &api.Cell{ObjectMeta: metav1.ObjectMeta{Name: "cell-a", Namespace: api.ManagementNamespace}, Spec: api.CellSpec{
		ManagedClusterRef: api.ObjectReference{Name: "cell-a"},
		MachineProfile:    api.MachineProfile{Name: "ascend-910b"},
	}}
	newCell := oldCell.DeepCopy()
	newCell.Spec.MachineProfile.Name = "other"

	if _, err := withoutRef.ValidateUpdate(context.Background(), oldCell, newCell); err != nil {
		t.Fatalf("unreferenced update error = %v, want none", err)
	}
	if _, err := withRef.ValidateUpdate(context.Background(), oldCell, newCell); err == nil {
		t.Fatal("referenced update error = nil, want immutability rejection")
	}
}

func TestVirtualNodeClassWebhookEnforcesImmutabilityOnlyWhenReferenced(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	withRef := VirtualNodeClassValidator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&api.VirtualCluster{ObjectMeta: metav1.ObjectMeta{Name: "child", Namespace: api.ManagementNamespace}, Spec: api.VirtualClusterSpec{
			CellRef: api.ObjectReference{Name: "cell-a"}, ClassRef: api.ObjectReference{Name: "class-a"},
		}},
	).Build()}
	withoutRef := VirtualNodeClassValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	oldClass := &api.VirtualNodeClass{ObjectMeta: metav1.ObjectMeta{Name: "class-a"}, Spec: api.VirtualNodeClassSpec{
		StorageClassName: "topolvm-provisioner",
	}}
	newClass := oldClass.DeepCopy()
	newClass.Spec.StorageClassName = "other"

	if _, err := withoutRef.ValidateUpdate(context.Background(), oldClass, newClass); err != nil {
		t.Fatalf("unreferenced update error = %v, want none", err)
	}
	if _, err := withRef.ValidateUpdate(context.Background(), oldClass, newClass); err == nil {
		t.Fatal("referenced update error = nil, want immutability rejection")
	}
}

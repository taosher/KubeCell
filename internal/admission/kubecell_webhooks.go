package admission

import (
	"context"
	"fmt"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type CellDefaulter struct{}

func (CellDefaulter) Default(_ context.Context, object *api.Cell) error {
	object.Default()
	return nil
}

type CellValidator struct{ Client client.Client }

func (CellValidator) ValidateCreate(_ context.Context, object *api.Cell) (admission.Warnings, error) {
	return nil, object.ValidateCreate()
}

func (v CellValidator) ValidateUpdate(ctx context.Context, oldObject, newObject *api.Cell) (admission.Warnings, error) {
	referenced, err := v.isCellReferenced(ctx, newObject.Name)
	if err != nil {
		return nil, err
	}
	return nil, newObject.ValidateUpdate(oldObject, referenced)
}

func (v CellValidator) isCellReferenced(ctx context.Context, name string) (bool, error) {
	if v.Client == nil {
		return false, nil
	}
	var clusters api.VirtualClusterList
	if err := v.Client.List(ctx, &clusters); err != nil {
		return false, fmt.Errorf("check Cell references: %w", err)
	}
	for _, item := range clusters.Items {
		if item.DeletionTimestamp == nil && item.Spec.CellRef.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func (CellValidator) ValidateDelete(_ context.Context, object *api.Cell) (admission.Warnings, error) {
	return nil, object.ValidateDelete()
}

type VirtualClusterDefaulter struct{}

func (VirtualClusterDefaulter) Default(_ context.Context, object *api.VirtualCluster) error {
	object.Default()
	return nil
}

type VirtualClusterValidator struct{ Client client.Client }

func (v VirtualClusterValidator) ValidateCreate(ctx context.Context, object *api.VirtualCluster) (admission.Warnings, error) {
	if err := object.ValidateCreate(); err != nil {
		return nil, err
	}
	if v.Client != nil {
		var clusters api.VirtualClusterList
		if err := v.Client.List(ctx, &clusters); err != nil {
			return nil, fmt.Errorf("check VirtualCluster name uniqueness: %w", err)
		}
		for _, existing := range clusters.Items {
			if existing.Name == object.Name && existing.DeletionTimestamp == nil {
				return nil, fmt.Errorf("VirtualCluster name %q is already active in namespace %q; names must be globally unique", object.Name, existing.Namespace)
			}
		}
	}
	return nil, nil
}

func (VirtualClusterValidator) ValidateUpdate(_ context.Context, oldObject, newObject *api.VirtualCluster) (admission.Warnings, error) {
	return nil, newObject.ValidateUpdate(oldObject)
}

func (VirtualClusterValidator) ValidateDelete(_ context.Context, object *api.VirtualCluster) (admission.Warnings, error) {
	return nil, object.ValidateDelete()
}

type VirtualClusterPlanDefaulter struct{}

func (VirtualClusterPlanDefaulter) Default(_ context.Context, object *api.VirtualClusterPlan) error {
	object.Default()
	return nil
}

type VirtualClusterPlanValidator struct{}

func (VirtualClusterPlanValidator) ValidateCreate(_ context.Context, object *api.VirtualClusterPlan) (admission.Warnings, error) {
	return nil, object.ValidateCreate()
}

func (VirtualClusterPlanValidator) ValidateUpdate(_ context.Context, oldObject, newObject *api.VirtualClusterPlan) (admission.Warnings, error) {
	return nil, newObject.ValidateUpdate(oldObject)
}

func (VirtualClusterPlanValidator) ValidateDelete(_ context.Context, object *api.VirtualClusterPlan) (admission.Warnings, error) {
	return nil, object.ValidateDelete()
}

type VirtualNodeClassDefaulter struct{}

func (VirtualNodeClassDefaulter) Default(_ context.Context, object *api.VirtualNodeClass) error {
	object.Default()
	return nil
}

type VirtualNodeClassValidator struct{ Client client.Client }

func (VirtualNodeClassValidator) ValidateCreate(_ context.Context, object *api.VirtualNodeClass) (admission.Warnings, error) {
	return nil, object.ValidateCreate()
}

func (v VirtualNodeClassValidator) ValidateUpdate(ctx context.Context, oldObject, newObject *api.VirtualNodeClass) (admission.Warnings, error) {
	referenced, err := v.isClassReferenced(ctx, newObject.Name)
	if err != nil {
		return nil, err
	}
	return nil, newObject.ValidateUpdate(oldObject, referenced)
}

func (v VirtualNodeClassValidator) isClassReferenced(ctx context.Context, name string) (bool, error) {
	if v.Client == nil {
		return false, nil
	}
	var clusters api.VirtualClusterList
	if err := v.Client.List(ctx, &clusters); err != nil {
		return false, fmt.Errorf("check VirtualNodeClass references: %w", err)
	}
	for _, item := range clusters.Items {
		if item.DeletionTimestamp == nil && item.Spec.ClassRef.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func (VirtualNodeClassValidator) ValidateDelete(_ context.Context, object *api.VirtualNodeClass) (admission.Warnings, error) {
	return nil, object.ValidateDelete()
}

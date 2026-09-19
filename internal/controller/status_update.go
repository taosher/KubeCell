package controller

import (
	"context"

	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/kubecell/kubecell/api/v1alpha1"
)

func updateCellStatus(ctx context.Context, c client.Client, desired *api.Cell) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &api.Cell{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(desired), current); err != nil {
			return err
		}
		current.Status = *desired.Status.DeepCopy()
		return c.Status().Update(ctx, current)
	})
}

func updateVirtualClusterStatus(ctx context.Context, c client.Client, desired *api.VirtualCluster) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &api.VirtualCluster{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(desired), current); err != nil {
			return err
		}
		current.Status = *desired.Status.DeepCopy()
		return c.Status().Update(ctx, current)
	})
}

func updateVirtualClusterPlanStatus(ctx context.Context, c client.Client, desired *api.VirtualClusterPlan) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &api.VirtualClusterPlan{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(desired), current); err != nil {
			return err
		}
		current.Status = *desired.Status.DeepCopy()
		return c.Status().Update(ctx, current)
	})
}

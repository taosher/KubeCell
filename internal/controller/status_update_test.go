package controller

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	api "github.com/kubecell/kubecell/api/v1alpha1"
)

type conflictStatusClient struct {
	client.Client
	writer *conflictStatusWriter
}

func (c *conflictStatusClient) Status() client.SubResourceWriter {
	return c.writer
}

type conflictStatusWriter struct {
	delegate  client.SubResourceWriter
	remaining int
}

func (w *conflictStatusWriter) Update(ctx context.Context, object client.Object, options ...client.SubResourceUpdateOption) error {
	if w.remaining > 0 {
		w.remaining--
		return apierrors.NewConflict(schema.GroupResource{Group: api.GroupVersion.Group, Resource: "cells"}, object.GetName(), errors.New("deterministic test conflict"))
	}
	return w.delegate.Update(ctx, object, options...)
}

func (w *conflictStatusWriter) Create(ctx context.Context, object client.Object, subResource client.Object, options ...client.SubResourceCreateOption) error {
	return w.delegate.Create(ctx, object, subResource, options...)
}

func (w *conflictStatusWriter) Patch(ctx context.Context, object client.Object, patch client.Patch, options ...client.SubResourcePatchOption) error {
	return w.delegate.Patch(ctx, object, patch, options...)
}

func (w *conflictStatusWriter) Apply(ctx context.Context, object runtime.ApplyConfiguration, options ...client.SubResourceApplyOption) error {
	return w.delegate.Apply(ctx, object, options...)
}

func TestStatusUpdatesRetryAConflictForCell(t *testing.T) {
	cell := testCell()
	base := fake.NewClientBuilder().WithScheme(testControllerScheme(t)).WithStatusSubresource(cell).WithObjects(cell).Build()
	wrapped := &conflictStatusClient{Client: base}
	wrapped.writer = &conflictStatusWriter{delegate: base.Status(), remaining: 1}

	desired := cell.DeepCopy()
	desired.Status.Phase = api.CellPhaseReady
	desired.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}
	if err := updateCellStatus(context.Background(), wrapped, desired); err != nil {
		t.Fatalf("updateCellStatus() error = %v", err)
	}
	current := &api.Cell{}
	if err := base.Get(context.Background(), client.ObjectKeyFromObject(cell), current); err != nil {
		t.Fatal(err)
	}
	if current.Status.Phase != api.CellPhaseReady || len(current.Status.Conditions) != 1 {
		t.Fatalf("status = %#v, want retried desired status", current.Status)
	}
}

func TestStatusUpdatesRetryAConflictForVirtualCluster(t *testing.T) {
	cluster, _, _, _, scheme := virtualClusterTestObjects(t)
	base := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cluster).WithObjects(cluster).Build()
	wrapped := &conflictStatusClient{Client: base}
	wrapped.writer = &conflictStatusWriter{delegate: base.Status(), remaining: 1}

	desired := cluster.DeepCopy()
	desired.Status.Phase = api.VirtualClusterPhaseReady
	if err := updateVirtualClusterStatus(context.Background(), wrapped, desired); err != nil {
		t.Fatalf("updateVirtualClusterStatus() error = %v", err)
	}
	current := &api.VirtualCluster{}
	if err := base.Get(context.Background(), client.ObjectKeyFromObject(cluster), current); err != nil {
		t.Fatal(err)
	}
	if current.Status.Phase != api.VirtualClusterPhaseReady {
		t.Fatalf("phase = %q, want Ready after retry", current.Status.Phase)
	}
}

func TestStatusUpdatesRetryAConflictForVirtualClusterPlan(t *testing.T) {
	plan := &api.VirtualClusterPlan{ObjectMeta: metav1.ObjectMeta{Name: "training-plan", Namespace: "team-a"}}
	scheme := testControllerScheme(t)
	base := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(plan).WithObjects(plan).Build()
	wrapped := &conflictStatusClient{Client: base}
	wrapped.writer = &conflictStatusWriter{delegate: base.Status(), remaining: 1}

	desired := plan.DeepCopy()
	desired.Status.Decision = api.VirtualClusterPlanDecisionAccepted
	desired.Status.Phase = api.VirtualClusterPlanDecisionAccepted
	if err := updateVirtualClusterPlanStatus(context.Background(), wrapped, desired); err != nil {
		t.Fatalf("updateVirtualClusterPlanStatus() error = %v", err)
	}
	current := &api.VirtualClusterPlan{}
	if err := base.Get(context.Background(), client.ObjectKeyFromObject(plan), current); err != nil {
		t.Fatal(err)
	}
	if current.Status.Decision != api.VirtualClusterPlanDecisionAccepted {
		t.Fatalf("decision = %q, want Accepted after retry", current.Status.Decision)
	}
}

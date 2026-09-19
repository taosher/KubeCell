package controller

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/kubecell/kubecell/api/v1alpha1"
)

type VirtualClusterPlanReconciler struct{ client.Client }

// +kubebuilder:rbac:groups=kubecell.io,resources=virtualclusterplans,verbs=get;list;watch
// +kubebuilder:rbac:groups=kubecell.io,resources=virtualclusterplans/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubecell.io,resources=cells,verbs=get;list;watch
// +kubebuilder:rbac:groups=kubecell.io,resources=virtualnodeclasses,verbs=get;list;watch

func (r *VirtualClusterPlanReconciler) SetupWithManager(manager ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(manager).For(&api.VirtualClusterPlan{}).Complete(r)
}

func (r *VirtualClusterPlanReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	plan := &api.VirtualClusterPlan{}
	if err := r.Get(ctx, request.NamespacedName, plan); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if plan.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}
	cluster := &api.VirtualCluster{ObjectMeta: metav1.ObjectMeta{Name: plan.Name, Namespace: plan.Namespace}, Spec: *plan.Spec.VirtualCluster.DeepCopy()}
	cell := &api.Cell{}
	class := &api.VirtualNodeClass{}
	var lookupError error
	if err := r.Get(ctx, types.NamespacedName{Name: cluster.Spec.CellRef.Name, Namespace: cluster.Spec.CellRef.Namespace}, cell); err != nil {
		lookupError = err
	}
	if lookupError == nil {
		if err := r.Get(ctx, types.NamespacedName{Name: cluster.Spec.ClassRef.Name}, class); err != nil {
			lookupError = err
		}
	}
	evaluation := virtualClusterPlanEvaluation{}
	if lookupError != nil {
		evaluation = finalizePlanEvaluation(virtualClusterPlanEvaluation{Checks: []api.VirtualClusterPlanCheck{planCheck("References", api.VirtualClusterPlanCheckUnknown, "ReferenceUnavailable", lookupError.Error(), metav1.Now())}}, time.Now())
	} else {
		evaluation = evaluateVirtualClusterPlan(cluster, cell, class, time.Now())
	}
	plan.Status.ObservedGeneration = plan.Generation
	plan.Status.Phase = evaluation.Decision
	plan.Status.Decision = evaluation.Decision
	plan.Status.Checks = evaluation.Checks
	plan.Status.Resolved = evaluation.Summary
	plan.Status.InventoryObservedAt = evaluation.InventoryObservedAt
	plan.Status.InventoryFresh = evaluation.InventoryFresh
	plan.Status.ExpiresAt = evaluation.ExpiresAt
	if err := updateVirtualClusterPlanStatus(ctx, r.Client, plan); err != nil {
		return reconcile.Result{}, err
	}
	// One-shot evaluation: no periodic refresh; external systems rewriting the Plan trigger re-evaluation (§12).
	return reconcile.Result{}, nil
}

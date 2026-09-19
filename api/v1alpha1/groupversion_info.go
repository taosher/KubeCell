// Package v1alpha1 contains the Kubecell API.
//
// +kubebuilder:object:generate=true
// +groupName=kubecell.io
// +kubebuilder:webhook:path=/mutate-kubecell-io-v1alpha1-cell,mutating=true,failurePolicy=fail,groups=kubecell.io,resources=cells,verbs=create;update,versions=v1alpha1,name=mcell.kubecell.io,sideEffects=None,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-kubecell-io-v1alpha1-cell,mutating=false,failurePolicy=fail,groups=kubecell.io,resources=cells,verbs=create;update;delete,versions=v1alpha1,name=vcell.kubecell.io,sideEffects=None,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/mutate-kubecell-io-v1alpha1-virtualcluster,mutating=true,failurePolicy=fail,groups=kubecell.io,resources=virtualclusters,verbs=create;update,versions=v1alpha1,name=mvirtualcluster.kubecell.io,sideEffects=None,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-kubecell-io-v1alpha1-virtualcluster,mutating=false,failurePolicy=fail,groups=kubecell.io,resources=virtualclusters,verbs=create;update;delete,versions=v1alpha1,name=vvirtualcluster.kubecell.io,sideEffects=None,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/mutate-kubecell-io-v1alpha1-virtualclusterplan,mutating=true,failurePolicy=fail,groups=kubecell.io,resources=virtualclusterplans,verbs=create;update,versions=v1alpha1,name=mvirtualclusterplan.kubecell.io,sideEffects=None,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-kubecell-io-v1alpha1-virtualclusterplan,mutating=false,failurePolicy=fail,groups=kubecell.io,resources=virtualclusterplans,verbs=create;update;delete,versions=v1alpha1,name=vvirtualclusterplan.kubecell.io,sideEffects=None,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/mutate-kubecell-io-v1alpha1-virtualnodeclass,mutating=true,failurePolicy=fail,groups=kubecell.io,resources=virtualnodeclasses,verbs=create;update,versions=v1alpha1,name=mvirtualnodeclass.kubecell.io,sideEffects=None,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-kubecell-io-v1alpha1-virtualnodeclass,mutating=false,failurePolicy=fail,groups=kubecell.io,resources=virtualnodeclasses,verbs=create;update;delete,versions=v1alpha1,name=vvirtualnodeclass.kubecell.io,sideEffects=None,admissionReviewVersions=v1
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "kubecell.io", Version: "v1alpha1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(
		&Cell{}, &CellList{},
		&VirtualNodeClass{}, &VirtualNodeClassList{},
		&VirtualCluster{}, &VirtualClusterList{},
		&VirtualClusterPlan{}, &VirtualClusterPlanList{},
	)
}

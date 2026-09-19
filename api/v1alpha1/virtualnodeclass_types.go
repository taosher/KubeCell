package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ResourceList = corev1.ResourceList

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=vnclass
// +kubebuilder:subresource:status
type VirtualNodeClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VirtualNodeClassSpec   `json:"spec,omitempty"`
	Status            VirtualNodeClassStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type VirtualNodeClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualNodeClass `json:"items"`
}

type VirtualNodeClassSpec struct {
	Entitlement EntitlementSpec `json:"entitlement"`
	// StorageClassName names the certified host TopoLVM class.
	// Binding (WaitForFirstConsumer) and reclaim (Retain) are platform
	// constants enforced by the renderer, not user input.
	StorageClassName string `json:"storageClassName"`
}
type EntitlementSpec struct {
	WorkloadHard ResourceList `json:"workloadHard"`
}

// ReservationSpec survives only inside status snapshots (resolved quota
// accounting). No spec field uses it; reservation values are platform
// constants.
type ReservationSpec struct {
	ControlPlane    ResourceList      `json:"controlPlane,omitempty"`
	ReflectedSystem ResourceList      `json:"reflectedSystem,omitempty"`
	Policy          ReservationPolicy `json:"policy,omitempty"`
}
type ReservationPolicy string

const ReservationPolicyIncludedInHostQuota ReservationPolicy = "IncludedInHostQuota"

// StorageMappingSpec survives only inside status snapshots. The live spec
// carries a single StorageClassName instead.
type StorageMappingSpec struct {
	Class StorageClassMapping `json:"class"`
}
type StorageClassMapping struct {
	ChildName            string                               `json:"childName"`
	HostName             string                               `json:"hostName"`
	VolumeBindingMode    storagev1.VolumeBindingMode          `json:"volumeBindingMode"`
	ReclaimPolicy        corev1.PersistentVolumeReclaimPolicy `json:"reclaimPolicy"`
	AllowVolumeExpansion bool                                 `json:"allowVolumeExpansion,omitempty"`
}
type VirtualNodeClassStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

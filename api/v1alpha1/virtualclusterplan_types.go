package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=vcp
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Decision",type="string",JSONPath=".status.decision"
// +kubebuilder:printcolumn:name="Expires",type="date",JSONPath=".status.expiresAt"
type VirtualClusterPlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VirtualClusterPlanSpec   `json:"spec,omitempty"`
	Status            VirtualClusterPlanStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type VirtualClusterPlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualClusterPlan `json:"items"`
}

type VirtualClusterPlanSpec struct {
	VirtualCluster VirtualClusterSpec `json:"virtualCluster"`
}

type VirtualClusterPlanDecision string

const (
	VirtualClusterPlanDecisionAccepted VirtualClusterPlanDecision = "Accepted"
	VirtualClusterPlanDecisionRejected VirtualClusterPlanDecision = "Rejected"
	VirtualClusterPlanDecisionUnknown  VirtualClusterPlanDecision = "Unknown"
)

type VirtualClusterPlanCheckStatus string

const (
	VirtualClusterPlanCheckPassed  VirtualClusterPlanCheckStatus = "Passed"
	VirtualClusterPlanCheckFailed  VirtualClusterPlanCheckStatus = "Failed"
	VirtualClusterPlanCheckUnknown VirtualClusterPlanCheckStatus = "Unknown"
)

type VirtualClusterPlanCheck struct {
	// +kubebuilder:validation:MaxLength=64
	Name   string                        `json:"name"`
	Status VirtualClusterPlanCheckStatus `json:"status"`
	// +kubebuilder:validation:MaxLength=128
	Reason string `json:"reason"`
	// +kubebuilder:validation:MaxLength=1024
	Message    string      `json:"message,omitempty"`
	ObservedAt metav1.Time `json:"observedAt"`
}

type VirtualClusterPlanResolvedSummary struct {
	CellUID            string       `json:"cellUID,omitempty"`
	ManagedClusterName string       `json:"managedClusterName,omitempty"`
	ClassUID           string       `json:"classUID,omitempty"`
	ProfileHash        string       `json:"profileHash,omitempty"`
	MachineProfileName string       `json:"machineProfileName,omitempty"`
	K3kVersion         string       `json:"k3kVersion,omitempty"`
	ChildK3sVersion    string       `json:"childK3sVersion,omitempty"`
	WorkloadRequests   ResourceList `json:"workloadRequests,omitempty"`
	Reservation        ResourceList `json:"reservation,omitempty"`
	StorageClassName   string       `json:"storageClassName,omitempty"`
}

type VirtualClusterPlanStatus struct {
	ObservedGeneration int64                      `json:"observedGeneration,omitempty"`
	Phase              VirtualClusterPlanDecision `json:"phase,omitempty"`
	Decision           VirtualClusterPlanDecision `json:"decision,omitempty"`
	// +kubebuilder:validation:MaxItems=16
	Checks              []VirtualClusterPlanCheck         `json:"checks,omitempty"`
	Resolved            VirtualClusterPlanResolvedSummary `json:"resolved,omitempty"`
	InventoryObservedAt *metav1.Time                      `json:"inventoryObservedAt,omitempty"`
	InventoryFresh      bool                              `json:"inventoryFresh,omitempty"`
	ExpiresAt           *metav1.Time                      `json:"expiresAt,omitempty"`
}

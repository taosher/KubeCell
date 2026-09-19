package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=vc
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".status.endpoint.address"
type VirtualCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VirtualClusterSpec   `json:"spec,omitempty"`
	Status            VirtualClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type VirtualClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualCluster `json:"items"`
}
type VirtualClusterSpec struct {
	CellRef  ObjectReference `json:"cellRef"`
	ClassRef ObjectReference `json:"classRef"`
}
type VirtualClusterPhase string

const (
	VirtualClusterPhasePending      VirtualClusterPhase = "Pending"
	VirtualClusterPhaseProvisioning VirtualClusterPhase = "Provisioning"
	VirtualClusterPhaseReady        VirtualClusterPhase = "Ready"
	VirtualClusterPhaseDegraded     VirtualClusterPhase = "Degraded"
	VirtualClusterPhaseDeleting     VirtualClusterPhase = "Deleting"
	VirtualClusterPhaseFailed       VirtualClusterPhase = "Failed"
)

type VirtualClusterStatus struct {
	ObservedGeneration int64                     `json:"observedGeneration,omitempty"`
	Phase              VirtualClusterPhase       `json:"phase,omitempty"`
	Conditions         []metav1.Condition        `json:"conditions,omitempty"`
	Resolved           ResolvedSnapshot          `json:"resolved,omitempty"`
	WorkRefs           WorkReferences            `json:"workRefs,omitempty"`
	Endpoint           EndpointStatus            `json:"endpoint,omitempty"`
	Host               HostClusterObservation    `json:"host,omitempty"`
	Child              ChildClusterObservation   `json:"child,omitempty"`
	Credential         CredentialStatus          `json:"credential,omitempty"`
	FeasibilityChecks  []VirtualClusterPlanCheck `json:"feasibilityChecks,omitempty"`
	Placements         []WorkloadPlacement       `json:"placements,omitempty"`
}
type ResolvedSnapshot struct {
	CellUID            string             `json:"cellUID,omitempty"`
	ManagedClusterName string             `json:"managedClusterName,omitempty"`
	ClassUID           string             `json:"classUID,omitempty"`
	ProfileHash        string             `json:"profileHash,omitempty"`
	MachineProfileName string             `json:"machineProfileName,omitempty"`
	K3kVersion         string             `json:"k3kVersion,omitempty"`
	ChildK3sVersion    string             `json:"childK3sVersion,omitempty"`
	K3kChildVersion    string             `json:"k3kChildVersion,omitempty"`
	ChildImageTag      string             `json:"childImageTag,omitempty"`
	HostNamespace      string             `json:"hostNamespace,omitempty"`
	K3kClusterName     string             `json:"k3kClusterName,omitempty"`
	Quota              ResourceList       `json:"quota,omitempty"`
	Reservation        ReservationSpec    `json:"reservation,omitempty"`
	AcceleratorKeys    []string           `json:"acceleratorKeys,omitempty"`
	Storage            StorageMappingSpec `json:"storage,omitempty"`
	HostPathTenantName string             `json:"hostPathTenantName,omitempty"`
	HostPathTenantRoot string             `json:"hostPathTenantRoot,omitempty"`
}
type WorkReferences struct {
	Foundation WorkReference `json:"foundation,omitempty"`
	Instance   WorkReference `json:"instance,omitempty"`
}
type WorkReference struct {
	Namespace   string `json:"namespace,omitempty"`
	Name        string `json:"name,omitempty"`
	UID         string `json:"uid,omitempty"`
	Applied     bool   `json:"applied,omitempty"`
	Progressing bool   `json:"progressing,omitempty"`
	Failed      bool   `json:"failed,omitempty"`
}
type EndpointStatus struct {
	Address  string `json:"address,omitempty"`
	NodePort int32  `json:"nodePort,omitempty"`
	URLHash  string `json:"urlHash,omitempty"`
}
type HostClusterObservation struct {
	Namespace           string       `json:"namespace,omitempty"`
	QuotaReady          bool         `json:"quotaReady,omitempty"`
	StorageReady        bool         `json:"storageReady,omitempty"`
	NetworkReady        bool         `json:"networkReady,omitempty"`
	InventoryFresh      bool         `json:"inventoryFresh,omitempty"`
	QuotaHard           ResourceList `json:"quotaHard,omitempty"`
	QuotaUsed           ResourceList `json:"quotaUsed,omitempty"`
	TenantHeadroom      ResourceList `json:"tenantHeadroom,omitempty"`
	Reservation         ResourceList `json:"reservation,omitempty"`
	ReservationExceeded bool         `json:"reservationExceeded,omitempty"`
	K3kPhase            string       `json:"k3kPhase,omitempty"`
}
type ChildClusterObservation struct {
	APIReady          bool   `json:"apiReady,omitempty"`
	LogicalNodeName   string `json:"logicalNodeName,omitempty"`
	LogicalNodeReady  bool   `json:"logicalNodeReady,omitempty"`
	StorageClassReady bool   `json:"storageClassReady,omitempty"`
}
type CredentialStatus struct {
	SecretName              string `json:"secretName,omitempty"`
	SourceSecretName        string `json:"sourceSecretName,omitempty"`
	EndpointHash            string `json:"endpointHash,omitempty"`
	ObservedResourceVersion string `json:"observedResourceVersion,omitempty"`
}
type WorkloadPlacement struct {
	ChildUID     string       `json:"childUID,omitempty"`
	HostUID      string       `json:"hostUID,omitempty"`
	Phase        string       `json:"phase,omitempty"`
	HostNode     string       `json:"hostNode,omitempty"`
	Requests     ResourceList `json:"requests,omitempty"`
	PVNode       string       `json:"pvNode,omitempty"`
	FailureLayer string       `json:"failureLayer,omitempty"`
	ObservedAt   metav1.Time  `json:"observedAt,omitempty"`
}

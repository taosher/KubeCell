package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=cell
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="ManagedCluster",type="string",JSONPath=".status.managedCluster.name"
type Cell struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CellSpec   `json:"spec,omitempty"`
	Status            CellStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type CellList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cell `json:"items"`
}

type ObjectReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type CellSpec struct {
	ManagedClusterRef ObjectReference `json:"managedClusterRef"`
	MachineProfile    MachineProfile  `json:"machineProfile"`
}

type MachineProfile struct {
	Name    string           `json:"name"`
	Devices []DeviceContract `json:"devices,omitempty"`
}

type DeviceContract struct {
	Name         string `json:"name"`
	ResourceName string `json:"resourceName"`
}

type CellPhase string

const (
	CellPhasePending      CellPhase = "Pending"
	CellPhaseProvisioning CellPhase = "Provisioning"
	CellPhaseReady        CellPhase = "Ready"
	CellPhaseDegraded     CellPhase = "Degraded"
	CellPhaseFailed       CellPhase = "Failed"
	CellPhaseDeleting     CellPhase = "Deleting"
)

type CellStatus struct {
	ObservedGeneration int64                             `json:"observedGeneration,omitempty"`
	Phase              CellPhase                         `json:"phase,omitempty"`
	Conditions         []metav1.Condition                `json:"conditions,omitempty"`
	ManagedCluster     ManagedClusterStatus              `json:"managedCluster,omitempty"`
	Provider           ProviderStatus                    `json:"provider,omitempty"`
	Nodes              []CellNodeStatus                  `json:"nodes,omitempty"`
	Inventory          map[string]InventoryStatus        `json:"inventory,omitempty"`
	StorageInventory   map[string]StorageInventoryStatus `json:"storageInventory,omitempty"`
	NodePorts          NodePortInventoryStatus           `json:"nodePorts,omitempty"`
}

type NodePortInventoryStatus struct {
	Allocated  []int32     `json:"allocated,omitempty"`
	ObservedAt metav1.Time `json:"observedAt,omitempty"`
	Fresh      bool        `json:"fresh,omitempty"`
}
type ManagedClusterStatus struct {
	Name           string            `json:"name,omitempty"`
	UID            string            `json:"uid,omitempty"`
	Joined         bool              `json:"joined,omitempty"`
	Available      bool              `json:"available,omitempty"`
	LeaseFresh     bool              `json:"leaseFresh,omitempty"`
	LeaseRenewTime *metav1.MicroTime `json:"leaseRenewTime,omitempty"`
}
type ProviderStatus struct {
	K3kVersion      string `json:"k3kVersion,omitempty"`
	NamespaceReady  bool   `json:"namespaceReady,omitempty"`
	K3kCRDsReady    bool   `json:"k3kCRDsReady,omitempty"`
	ControllerReady bool   `json:"controllerReady,omitempty"`
	CNIReady        bool   `json:"cniReady,omitempty"`
	TopoLVMReady    bool   `json:"topolvmReady,omitempty"`
}
type CellNodeStatus struct {
	Name             string       `json:"name"`
	Profile          string       `json:"profile,omitempty"`
	Ready            bool         `json:"ready,omitempty"`
	LabelHash        string       `json:"labelHash,omitempty"`
	Capacity         ResourceList `json:"capacity,omitempty"`
	Allocatable      ResourceList `json:"allocatable,omitempty"`
	ActiveRequested  ResourceList `json:"activeRequested,omitempty"`
	PendingRequested ResourceList `json:"pendingRequested,omitempty"`
	ReadinessReasons []string     `json:"readinessReasons,omitempty"`
	ObservedAt       metav1.Time  `json:"observedAt,omitempty"`
}
type InventoryStatus struct {
	Capacity          resource.Quantity `json:"capacity"`
	Allocatable       resource.Quantity `json:"allocatable"`
	Requested         resource.Quantity `json:"requested"`
	Pending           resource.Quantity `json:"pending"`
	AvailableEstimate resource.Quantity `json:"availableEstimate"`
}

type StorageInventoryStatus struct {
	Provisioner        string                                `json:"provisioner,omitempty"`
	AllocatedPV        resource.Quantity                     `json:"allocatedPV,omitempty"`
	Nodes              map[string]StorageNodeInventoryStatus `json:"nodes,omitempty"`
	FreeCapacity       *resource.Quantity                    `json:"freeCapacity,omitempty"`
	FreeCapacityKnown  bool                                  `json:"freeCapacityKnown,omitempty"`
	FreeCapacitySource string                                `json:"freeCapacitySource,omitempty"`
	ObservedAt         metav1.Time                           `json:"observedAt,omitempty"`
	Fresh              bool                                  `json:"fresh,omitempty"`
}

type StorageNodeInventoryStatus struct {
	AllocatedPV        resource.Quantity  `json:"allocatedPV,omitempty"`
	FreeCapacity       *resource.Quantity `json:"freeCapacity,omitempty"`
	FreeCapacityKnown  bool               `json:"freeCapacityKnown,omitempty"`
	FreeCapacitySource string             `json:"freeCapacitySource,omitempty"`
	Fresh              bool               `json:"fresh,omitempty"`
}

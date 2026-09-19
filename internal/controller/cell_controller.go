package controller

import (
	"context"
	"sort"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/controller/helpers"
	"github.com/kubecell/kubecell/internal/platform"
	"github.com/kubecell/kubecell/internal/provider/host"
)

type InventoryReader interface {
	ListNodes(context.Context) (*corev1.NodeList, error)
}

type InventoryPodReader interface {
	ListAllPods(context.Context) (*corev1.PodList, error)
}

type InventoryReaderFactory interface {
	ForCell(context.Context, *api.Cell) (InventoryReader, error)
}

type BaselineReaderFactory interface {
	ForCell(context.Context, *api.Cell) (host.BaselineReader, error)
}

type CellReconciler struct {
	client.Client
	InventoryReader        InventoryReader
	InventoryReaderFactory InventoryReaderFactory
	BaselineReader         host.BaselineReader
	BaselineReaderFactory  BaselineReaderFactory
}

const (
	cellFinalizer           = "kubecell.io/cell-finalizer"
	conditionCleanupBlocked = "CleanupBlocked"
)

// +kubebuilder:rbac:groups=kubecell.io,resources=cells,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=kubecell.io,resources=cells/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubecell.io,resources=virtualclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *CellReconciler) SetupWithManager(manager ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(manager).For(&api.Cell{}).Complete(r)
}

func (r *CellReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	cell := &api.Cell{}
	if err := r.Get(ctx, request.NamespacedName, cell); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if cell.DeletionTimestamp != nil {
		return r.reconcileDeletion(ctx, cell)
	}
	if !containsString(cell.Finalizers, cellFinalizer) {
		cell.Finalizers = append(cell.Finalizers, cellFinalizer)
		if err := r.Update(ctx, cell); err != nil {
			return reconcile.Result{}, err
		}
	}
	managedCluster := &clusterv1.ManagedCluster{}
	if err := r.Get(ctx, types.NamespacedName{Name: cell.Spec.ManagedClusterRef.Name}, managedCluster); err != nil {
		if apierrors.IsNotFound(err) {
			cell.Status.Phase = api.CellPhaseDegraded
			cell.Status.ManagedCluster = api.ManagedClusterStatus{Name: cell.Spec.ManagedClusterRef.Name}
			setCellCondition(cell, "Joined", metav1.ConditionFalse, "ManagedClusterNotFound", "referenced OCM ManagedCluster does not exist")
			return reconcile.Result{RequeueAfter: 10 * time.Second}, updateCellStatus(ctx, r.Client, cell)
		}
		return reconcile.Result{}, err
	}
	joined := managedClusterCondition(managedCluster, clusterv1.ManagedClusterConditionJoined)
	availableCondition := managedClusterCondition(managedCluster, clusterv1.ManagedClusterConditionAvailable)
	leaseFresh, leaseRenewTime, leaseErr := r.managedClusterLeaseFresh(ctx, managedCluster)
	available := availableCondition && leaseFresh
	cell.Status.ObservedGeneration = cell.Generation
	cell.Status.ManagedCluster = api.ManagedClusterStatus{Name: managedCluster.Name, UID: string(managedCluster.UID), Joined: joined, Available: available, LeaseFresh: leaseFresh, LeaseRenewTime: leaseRenewTime}
	if leaseErr != nil {
		setCellCondition(cell, "Available", metav1.ConditionFalse, "ManagedClusterLeaseReadFailed", leaseErr.Error())
	} else if availableCondition && !leaseFresh {
		setCellCondition(cell, "Available", metav1.ConditionFalse, "ManagedClusterLeaseStale", "OCM ManagedCluster lease is missing or expired")
	}
	baseline := host.BaselineObservation{}
	baselineReadErr := error(nil)
	baselineReader := r.BaselineReader
	if baselineReader == nil && r.BaselineReaderFactory != nil {
		factoryReader, factoryErr := r.BaselineReaderFactory.ForCell(ctx, cell)
		if factoryErr != nil {
			baselineReadErr = factoryErr
			setCellCondition(cell, "ProviderReady", metav1.ConditionFalse, "HostReaderUnavailable", factoryErr.Error())
		} else {
			baselineReader = factoryReader
		}
	}
	if baselineReader != nil {
		observed, observeErr := baselineReader.ObserveBaseline(ctx, cell)
		if observeErr != nil {
			baselineReadErr = observeErr
			ctrl.LoggerFrom(ctx).Error(observeErr, "failed to observe Host baseline", "cell", cell.Name)
			setCellCondition(cell, "ProviderReady", metav1.ConditionFalse, "HostBaselineReadFailed", observeErr.Error())
		} else {
			baseline = observed
			ctrl.LoggerFrom(ctx).Info("observed Host baseline", "cell", cell.Name, "namespaceReady", baseline.NamespaceReady, "k3kCRDsReady", baseline.K3kCRDsReady, "controllerReady", baseline.ControllerReady, "cniReady", baseline.CNIReady, "topoLVMReady", baseline.TopoLVMReady)
		}
	} else if baselineReadErr == nil {
		setCellCondition(cell, "ProviderReady", metav1.ConditionFalse, "HostReaderUnavailable", "Host baseline reader is not configured")
	}
	cell.Status.Provider = api.ProviderStatus{
		K3kVersion:      platform.K3kVersion,
		NamespaceReady:  baseline.NamespaceReady,
		K3kCRDsReady:    baseline.K3kCRDsReady,
		ControllerReady: baseline.ControllerReady,
		CNIReady:        baseline.CNIReady,
		TopoLVMReady:    baseline.TopoLVMReady,
	}
	providerReady := baseline.NamespaceReady && baseline.K3kCRDsReady && baseline.ControllerReady && baseline.CNIReady && baseline.TopoLVMReady
	if providerReady {
		setCellCondition(cell, "ProviderReady", metav1.ConditionTrue, "HostBaselineReady", "K3k, Host CNI, and TopoLVM baseline is ready")
	} else if baselineReader != nil && baselineReadErr == nil {
		setCellCondition(cell, "ProviderReady", metav1.ConditionFalse, "HostBaselineNotReady", "one or more required Host baseline components are not ready")
	}
	// Single profile: MachineProfile is a required singleton, and inventory projection runs every round.
	// Workload ownership matches on k3k.io/clusterName (§5): first list the VCs referencing this Cell into a set.
	inventoryReady := false
	{
		clusterNames := map[string]struct{}{}
		clusters := &api.VirtualClusterList{}
		if err := r.List(ctx, clusters); err != nil {
			setCellCondition(cell, "InventoryFresh", metav1.ConditionFalse, "VirtualClusterListFailed", err.Error())
			return reconcile.Result{RequeueAfter: 10 * time.Second}, updateCellStatus(ctx, r.Client, cell)
		}
		for index := range clusters.Items {
			cluster := &clusters.Items[index]
			if cluster.Spec.CellRef.Name == cell.Name && cluster.Spec.CellRef.Namespace == cell.Namespace && cluster.Status.Resolved.K3kClusterName != "" {
				clusterNames[cluster.Status.Resolved.K3kClusterName] = struct{}{}
			}
		}
		reader := r.InventoryReader
		inventoryErr := error(nil)
		if reader == nil && r.InventoryReaderFactory != nil {
			factoryReader, factoryErr := r.InventoryReaderFactory.ForCell(ctx, cell)
			reader = factoryReader
			if factoryErr != nil {
				inventoryErr = factoryErr
				setCellCondition(cell, "InventoryFresh", metav1.ConditionFalse, "HostReaderUnavailable", factoryErr.Error())
			}
		}
		if reader != nil {
			if nodes, listErr := reader.ListNodes(ctx); listErr == nil {
				var pods *corev1.PodList
				if podReader, ok := reader.(InventoryPodReader); ok {
					pods, listErr = podReader.ListAllPods(ctx)
					if listErr != nil {
						setCellCondition(cell, "InventoryFresh", metav1.ConditionFalse, "HostReadFailed", listErr.Error())
						inventoryReady = false
					}
				}
				if listErr == nil {
					inventoryReady = r.projectInventory(ctx, cell, reader, nodes, pods, clusterNames)
				}
			} else {
				setCellCondition(cell, "InventoryFresh", metav1.ConditionFalse, "HostReadFailed", listErr.Error())
			}
		} else if inventoryErr == nil {
			setCellCondition(cell, "InventoryFresh", metav1.ConditionFalse, "HostReaderUnavailable", "Host inventory reader is not configured")
		}
	}
	if joined && available && providerReady && inventoryReady {
		cell.Status.Phase = api.CellPhaseReady
		setCellCondition(cell, "Joined", metav1.ConditionTrue, "ManagedClusterJoined", "OCM ManagedCluster is joined")
		setCellCondition(cell, "Available", metav1.ConditionTrue, "ManagedClusterAvailable", "OCM ManagedCluster is available")
	} else {
		cell.Status.Phase = api.CellPhaseDegraded
		setCellCondition(cell, "Joined", conditionStatus(joined), "ManagedClusterState", "OCM ManagedCluster joined state is not ready")
		if leaseErr == nil && !(availableCondition && !leaseFresh) {
			setCellCondition(cell, "Available", conditionStatus(available), "ManagedClusterState", "OCM ManagedCluster available state is not ready")
		}
	}
	if err := updateCellStatus(ctx, r.Client, cell); err != nil {
		return reconcile.Result{}, err
	}
	if joined && available {
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}
	return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
}

const managedClusterLeaseName = "managed-cluster-lease"

func (r *CellReconciler) managedClusterLeaseFresh(ctx context.Context, managedCluster *clusterv1.ManagedCluster) (bool, *metav1.MicroTime, error) {
	lease := &coordinationv1.Lease{}
	if err := r.Get(ctx, types.NamespacedName{Name: managedClusterLeaseName, Namespace: managedCluster.Name}, lease); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	if lease.Spec.RenewTime == nil {
		return false, nil, nil
	}
	duration := time.Duration(managedCluster.Spec.LeaseDurationSeconds) * time.Second
	if duration <= 0 {
		duration = 60 * time.Second
	}
	renewTime := lease.Spec.RenewTime.DeepCopy()
	return renewTime.Add(duration).After(time.Now()), renewTime, nil
}

func (r *CellReconciler) projectInventory(ctx context.Context, cell *api.Cell, reader InventoryReader, nodes *corev1.NodeList, pods *corev1.PodList, clusterNames map[string]struct{}) bool {
	cell.Status.Nodes = nil
	cell.Status.Inventory = map[string]api.InventoryStatus{}
	cell.Status.StorageInventory = map[string]api.StorageInventoryStatus{}
	activeByNode := map[string]corev1.ResourceList{}
	pendingByNode := map[string]corev1.ResourceList{}
	if pods != nil {
		for index := range pods.Items {
			pod := &pods.Items[index]
			// Ownership match: k3k.io/clusterName must belong to a VC referencing this Cell (§5).
			if _, ok := clusterNames[pod.Labels[helpers.K3kClusterNameLabel]]; !ok {
				continue
			}
			if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
				continue
			}
			requests := helpers.EffectivePodRequests(pod)
			if pod.Status.Phase == corev1.PodPending {
				if pod.Spec.NodeName != "" {
					addResourceList(pendingByNode, pod.Spec.NodeName, requests)
				}
				for name, quantity := range requests {
					entry := cell.Status.Inventory[string(name)]
					entry.Pending.Add(quantity)
					cell.Status.Inventory[string(name)] = entry
				}
				continue
			}
			if pod.Spec.NodeName != "" {
				addResourceList(activeByNode, pod.Spec.NodeName, requests)
			}
			for name, quantity := range requests {
				entry := cell.Status.Inventory[string(name)]
				entry.Requested.Add(quantity)
				cell.Status.Inventory[string(name)] = entry
			}
		}
	}
	profile := cell.Spec.MachineProfile
	readyProfile := false
	for index := range nodes.Items {
		node := &nodes.Items[index]
		profileName := node.Labels[platform.ProfileLabelKey]
		if profileName != profile.Name {
			cell.Status.Nodes = append(cell.Status.Nodes, api.CellNodeStatus{Name: node.Name, ReadinessReasons: []string{"Unclassified"}, ObservedAt: metav1.Now()})
			continue
		}
		status, ready := host.ObserveNode(node, profile.Name, profile.Devices)
		status.ActiveRequested = resourceListForNode(activeByNode[node.Name])
		status.PendingRequested = resourceListForNode(pendingByNode[node.Name])
		cell.Status.Nodes = append(cell.Status.Nodes, status)
		if ready {
			readyProfile = true
		}
		if !ready {
			continue
		}
		for key, quantity := range status.Capacity {
			entry := cell.Status.Inventory[string(key)]
			entry.Capacity.Add(quantity)
			entry.Allocatable.Add(status.Allocatable[key])
			entry.AvailableEstimate = entry.Allocatable.DeepCopy()
			entry.AvailableEstimate.Sub(entry.Requested)
			if entry.AvailableEstimate.Sign() < 0 {
				entry.AvailableEstimate = *resource.NewQuantity(0, entry.Allocatable.Format)
			}
			cell.Status.Inventory[string(key)] = entry
		}
	}
	sort.Slice(cell.Status.Nodes, func(i, j int) bool { return cell.Status.Nodes[i].Name < cell.Status.Nodes[j].Name })
	if !readyProfile {
		setCellCondition(cell, "AcceleratorReady", metav1.ConditionFalse, "NoReadyProfileNode", profile.Name)
		return false
	}
	storageReader, ok := reader.(storageInventoryReader)
	if !ok {
		setCellCondition(cell, "InventoryFresh", metav1.ConditionFalse, "HostStorageReaderUnavailable", "Host inventory reader does not implement PV, PVC, and StorageClass observation")
		return false
	}
	if !r.projectStorageInventory(ctx, cell, storageReader, nodes, cell.Status.Nodes) {
		return false
	}
	if serviceReader, ok := reader.(interface {
		ListServices(context.Context) (*corev1.ServiceList, error)
	}); ok {
		services, serviceErr := serviceReader.ListServices(ctx)
		if serviceErr != nil {
			cell.Status.NodePorts = api.NodePortInventoryStatus{ObservedAt: metav1.Now(), Fresh: false}
			setCellCondition(cell, "NodePortInventoryFresh", metav1.ConditionFalse, "ServiceReadFailed", serviceErr.Error())
			return false
		}
		cell.Status.NodePorts = projectNodePorts(services)
	} else {
		cell.Status.NodePorts = api.NodePortInventoryStatus{ObservedAt: metav1.Now(), Fresh: false}
		setCellCondition(cell, "NodePortInventoryFresh", metav1.ConditionFalse, "HostServiceReaderUnavailable", "Host inventory reader does not implement Service observation")
	}
	setCellCondition(cell, "InventoryFresh", metav1.ConditionTrue, "HostInventoryObserved", "profile-labelled Host inventory observed")
	setCellCondition(cell, "AcceleratorReady", metav1.ConditionTrue, "HostResourcesObserved", "declared extended resources are available")
	return true
}

func projectNodePorts(services *corev1.ServiceList) api.NodePortInventoryStatus {
	result := api.NodePortInventoryStatus{ObservedAt: metav1.Now(), Fresh: true}
	if services == nil {
		result.Fresh = false
		return result
	}
	for _, service := range services.Items {
		for _, port := range service.Spec.Ports {
			if port.NodePort > 0 {
				result.Allocated = append(result.Allocated, port.NodePort)
			}
		}
	}
	sort.Slice(result.Allocated, func(left, right int) bool { return result.Allocated[left] < result.Allocated[right] })
	return result
}

type storageInventoryReader interface {
	ListPersistentVolumes(context.Context) (*corev1.PersistentVolumeList, error)
	ListPersistentVolumeClaims(context.Context) (*corev1.PersistentVolumeClaimList, error)
	ListStorageClasses(context.Context) (*storagev1.StorageClassList, error)
}

func (r *CellReconciler) projectStorageInventory(ctx context.Context, cell *api.Cell, reader storageInventoryReader, nodes *corev1.NodeList, nodeStatuses []api.CellNodeStatus) bool {
	pvs, err := reader.ListPersistentVolumes(ctx)
	if err != nil {
		setCellCondition(cell, "InventoryFresh", metav1.ConditionFalse, "PersistentVolumeReadFailed", err.Error())
		return false
	}
	if _, err = reader.ListPersistentVolumeClaims(ctx); err != nil {
		setCellCondition(cell, "InventoryFresh", metav1.ConditionFalse, "PersistentVolumeClaimReadFailed", err.Error())
		return false
	}
	classes, err := reader.ListStorageClasses(ctx)
	if err != nil {
		setCellCondition(cell, "InventoryFresh", metav1.ConditionFalse, "StorageClassReadFailed", err.Error())
		return false
	}
	classByName := map[string]storagev1.StorageClass{}
	for _, storageClass := range classes.Items {
		if storageClass.Provisioner == "topolvm.io" {
			classByName[storageClass.Name] = storageClass
			cell.Status.StorageInventory[storageClass.Name] = api.StorageInventoryStatus{Provisioner: storageClass.Provisioner, Nodes: map[string]api.StorageNodeInventoryStatus{}, ObservedAt: metav1.Now()}
		}
	}
	for _, pv := range pvs.Items {
		if pv.Status.Phase != corev1.VolumeBound || pv.Spec.StorageClassName == "" {
			continue
		}
		if _, ok := classByName[pv.Spec.StorageClassName]; !ok {
			continue
		}
		entry := cell.Status.StorageInventory[pv.Spec.StorageClassName]
		entry.AllocatedPV.Add(pv.Spec.Capacity[corev1.ResourceStorage])
		if nodeName := persistentVolumeNodeName(&pv); nodeName != "" {
			nodeEntry := entry.Nodes[nodeName]
			nodeEntry.AllocatedPV.Add(pv.Spec.Capacity[corev1.ResourceStorage])
			entry.Nodes[nodeName] = nodeEntry
		}
		cell.Status.StorageInventory[pv.Spec.StorageClassName] = entry
	}
	eligibleNodes := map[string]struct{}{}
	for _, nodeStatus := range nodeStatuses {
		if nodeStatus.Ready {
			eligibleNodes[nodeStatus.Name] = struct{}{}
		}
	}
	eligibleNodeCount := len(eligibleNodes)
	capacityNodeCount := map[string]int{}
	for _, node := range nodes.Items {
		if _, ok := eligibleNodes[node.Name]; !ok {
			continue
		}
		for storageClassName, storageClass := range classByName {
			entry := cell.Status.StorageInventory[storageClassName]
			capacity, source, found := topolvmNodeCapacity(&node, storageClass.Parameters["topolvm.io/device-class"])
			if found {
				capacityNodeCount[storageClassName]++
				if entry.FreeCapacity == nil {
					entry.FreeCapacity = resource.NewQuantity(0, capacity.Format)
				}
				entry.FreeCapacity.Add(capacity)
				entry.FreeCapacityKnown = true
				entry.FreeCapacitySource = "TopoLVMNodeAnnotation:" + source
				entry.Fresh = true
				nodeEntry := entry.Nodes[node.Name]
				capacityCopy := capacity.DeepCopy()
				nodeEntry.FreeCapacity = &capacityCopy
				nodeEntry.FreeCapacityKnown = true
				nodeEntry.FreeCapacitySource = "TopoLVMNodeAnnotation:" + source
				nodeEntry.Fresh = true
				entry.Nodes[node.Name] = nodeEntry
			}
			cell.Status.StorageInventory[storageClassName] = entry
		}
	}
	for name, entry := range cell.Status.StorageInventory {
		if !entry.FreeCapacityKnown || capacityNodeCount[name] != eligibleNodeCount {
			entry.FreeCapacity = nil
			entry.FreeCapacityKnown = false
			entry.Fresh = false
			cell.Status.StorageInventory[name] = entry
			setCellCondition(cell, "InventoryFresh", metav1.ConditionFalse, "TopoLVMPhysicalCapacityUnknown", "TopoLVM physical free capacity has no certified observation source")
			return false
		}
		cell.Status.StorageInventory[name] = entry
	}
	return true
}

func topolvmNodeCapacity(node *corev1.Node, deviceClass string) (resource.Quantity, string, bool) {
	if node == nil {
		return resource.Quantity{}, "", false
	}
	if deviceClass != "" {
		value, ok := node.Annotations["capacity.topolvm.io/"+deviceClass]
		if !ok {
			return resource.Quantity{}, "", false
		}
		quantity, err := resource.ParseQuantity(value)
		if err == nil && quantity.Sign() >= 0 {
			return quantity, deviceClass, true
		}
		return resource.Quantity{}, "", false
	}
	if value, ok := node.Annotations["capacity.topolvm.io/00default"]; ok {
		quantity, err := resource.ParseQuantity(value)
		if err == nil && quantity.Sign() >= 0 {
			return quantity, "00default", true
		}
	}
	return resource.Quantity{}, "", false
}

func persistentVolumeNodeName(pv *corev1.PersistentVolume) string {
	if pv == nil || pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		return ""
	}
	for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
		for _, expression := range term.MatchExpressions {
			if expression.Key == "kubernetes.io/hostname" && expression.Operator == corev1.NodeSelectorOpIn && len(expression.Values) == 1 {
				return expression.Values[0]
			}
		}
	}
	return ""
}

func addResourceList(target map[string]corev1.ResourceList, nodeName string, values corev1.ResourceList) {
	if target[nodeName] == nil {
		target[nodeName] = corev1.ResourceList{}
	}
	for name, quantity := range values {
		current := target[nodeName][name]
		current.Add(quantity)
		target[nodeName][name] = current
	}
}

func resourceListForNode(values corev1.ResourceList) api.ResourceList {
	if values == nil {
		return nil
	}
	result := api.ResourceList{}
	for name, quantity := range values {
		result[name] = quantity.DeepCopy()
	}
	return result
}

func (r *CellReconciler) reconcileDeletion(ctx context.Context, cell *api.Cell) (reconcile.Result, error) {
	if !containsString(cell.Finalizers, cellFinalizer) {
		return reconcile.Result{}, nil
	}
	clusters := &api.VirtualClusterList{}
	if err := r.List(ctx, clusters); err != nil {
		return reconcile.Result{}, err
	}
	for index := range clusters.Items {
		cluster := &clusters.Items[index]
		if cluster.Spec.CellRef.Name == cell.Name && cluster.Spec.CellRef.Namespace == cell.Namespace {
			cell.Status.Phase = api.CellPhaseDeleting
			setCellCondition(cell, conditionCleanupBlocked, metav1.ConditionTrue, "VirtualClustersExist", "Cell deletion is blocked until all referenced VirtualClusters are deleted")
			return reconcile.Result{RequeueAfter: 15 * time.Second}, updateCellStatus(ctx, r.Client, cell)
		}
	}
	cell.Status.Phase = api.CellPhaseDeleting
	cell.Finalizers = removeString(cell.Finalizers, cellFinalizer)
	if err := r.Update(ctx, cell); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func managedClusterCondition(cluster *clusterv1.ManagedCluster, conditionType string) bool {
	for _, condition := range cluster.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Status == metav1.ConditionTrue
		}
	}
	return false
}
func conditionStatus(value bool) metav1.ConditionStatus {
	if value {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}
func setCellCondition(cell *api.Cell, conditionType string, status metav1.ConditionStatus, reason, message string) {
	cell.Status.Conditions = helpers.SetCondition(cell.Status.Conditions, metav1.Condition{Type: conditionType, Status: status, Reason: reason, Message: message, ObservedGeneration: cell.Generation}, metav1.Now())
}

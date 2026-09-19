package controller

import (
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/platform"
)

const planFreshnessWindow = 5 * time.Minute

type virtualClusterPlanEvaluation struct {
	Decision            api.VirtualClusterPlanDecision
	Checks              []api.VirtualClusterPlanCheck
	Summary             api.VirtualClusterPlanResolvedSummary
	InventoryObservedAt *metav1.Time
	InventoryFresh      bool
	ExpiresAt           *metav1.Time
}

func evaluateVirtualClusterPlan(cluster *api.VirtualCluster, cell *api.Cell, class *api.VirtualNodeClass, now time.Time) virtualClusterPlanEvaluation {
	evaluation := virtualClusterPlanEvaluation{}
	observedAt := metav1.NewTime(now)
	if cluster == nil || cell == nil || class == nil {
		evaluation.Checks = []api.VirtualClusterPlanCheck{planCheck("References", api.VirtualClusterPlanCheckUnknown, "ReferenceUnavailable", "VirtualCluster, Cell, and VirtualNodeClass must be available", observedAt)}
		return finalizePlanEvaluation(evaluation, now)
	}
	resolved, resolveErr := resolveVirtualCluster(cluster, cell, class)
	if resolveErr != nil {
		evaluation.Checks = append(evaluation.Checks, planCheck("ResolvedProfile", api.VirtualClusterPlanCheckFailed, "InvalidSpec", resolveErr.Error(), observedAt))
	} else {
		evaluation.Summary = planResolvedSummary(resolved, class)
		evaluation.Checks = append(evaluation.Checks, planCheck("ResolvedProfile", api.VirtualClusterPlanCheckPassed, "Resolved", "Cell, class, and Child K3s version resolved", observedAt))
	}

	evaluation.Checks = append(evaluation.Checks,
		cellAvailabilityCheck(cell, observedAt),
		leaseFreshnessCheck(cell, observedAt),
		hostBaselineCheck(cell, observedAt),
		inventoryFreshnessCheck(cell, observedAt),
		lockedVersionCheck(cell, observedAt),
	)
	if resolveErr == nil {
		evaluation.Checks = append(evaluation.Checks, computeChecks(cell, class, observedAt)...)
		evaluation.Checks = append(evaluation.Checks,
			hostQuotaCheck(class, observedAt),
			storageCheck(cell, class, observedAt),
			nodePortCheck(cell, observedAt),
		)
	}
	return finalizePlanEvaluation(evaluation, now)
}

func planResolvedSummary(resolved api.ResolvedSnapshot, class *api.VirtualNodeClass) api.VirtualClusterPlanResolvedSummary {
	return api.VirtualClusterPlanResolvedSummary{
		CellUID:            resolved.CellUID,
		ManagedClusterName: resolved.ManagedClusterName,
		ClassUID:           resolved.ClassUID,
		ProfileHash:        resolved.ProfileHash,
		MachineProfileName: resolved.MachineProfileName,
		K3kVersion:         resolved.K3kVersion,
		ChildK3sVersion:    resolved.ChildK3sVersion,
		WorkloadRequests:   entitlementRequests(class.Spec.Entitlement.WorkloadHard),
		Reservation:        platform.CombinedReservation(),
		StorageClassName:   class.Spec.StorageClassName,
	}
}

func entitlementRequests(hard api.ResourceList) api.ResourceList {
	result := api.ResourceList{}
	for key, quantity := range hard {
		name := strings.TrimPrefix(string(key), "requests.")
		if name == string(key) {
			continue
		}
		result[corev1.ResourceName(name)] = quantity.DeepCopy()
	}
	return result
}

func cellAvailabilityCheck(cell *api.Cell, observedAt metav1.Time) api.VirtualClusterPlanCheck {
	if cell.Status.Phase == api.CellPhaseReady && cell.Status.ManagedCluster.Joined && cell.Status.ManagedCluster.Available {
		return planCheck("CellAvailable", api.VirtualClusterPlanCheckPassed, "ManagedClusterAvailable", "referenced Cell is Ready", observedAt)
	}
	return planCheck("CellAvailable", api.VirtualClusterPlanCheckUnknown, "CellNotReady", "referenced Cell is not Ready", observedAt)
}

func leaseFreshnessCheck(cell *api.Cell, observedAt metav1.Time) api.VirtualClusterPlanCheck {
	if cell.Status.ManagedCluster.LeaseFresh {
		return planCheck("OCMLeaseFresh", api.VirtualClusterPlanCheckPassed, "LeaseFresh", "OCM ManagedCluster lease is current", observedAt)
	}
	return planCheck("OCMLeaseFresh", api.VirtualClusterPlanCheckUnknown, "LeaseStale", "OCM ManagedCluster lease is missing or stale", observedAt)
}

func hostBaselineCheck(cell *api.Cell, observedAt metav1.Time) api.VirtualClusterPlanCheck {
	provider := cell.Status.Provider
	if provider.NamespaceReady && provider.K3kCRDsReady && provider.ControllerReady && provider.CNIReady && provider.TopoLVMReady {
		return planCheck("HostBaseline", api.VirtualClusterPlanCheckPassed, "HostBaselineReady", "K3k, CNI, and TopoLVM baseline is ready", observedAt)
	}
	return planCheck("HostBaseline", api.VirtualClusterPlanCheckUnknown, "HostBaselineNotReady", "one or more required Host baseline components are not ready", observedAt)
}

func inventoryFreshnessCheck(cell *api.Cell, observedAt metav1.Time) api.VirtualClusterPlanCheck {
	if conditionTrue(cell.Status.Conditions, "InventoryFresh") {
		return planCheck("InventoryFresh", api.VirtualClusterPlanCheckPassed, "InventoryFresh", "Host inventory sources are current", observedAt)
	}
	return planCheck("InventoryFresh", api.VirtualClusterPlanCheckUnknown, "InventoryStale", "Host inventory is unavailable, stale, or incomplete", observedAt)
}

func lockedVersionCheck(cell *api.Cell, observedAt metav1.Time) api.VirtualClusterPlanCheck {
	observed := cell.Status.Provider.K3kVersion
	if observed == "" {
		return planCheck("LockedVersions", api.VirtualClusterPlanCheckUnknown, "VersionNotObserved", "Host K3k version has not been observed yet", observedAt)
	}
	if observed != platform.K3kVersion {
		return planCheck("LockedVersions", api.VirtualClusterPlanCheckFailed, "UnsupportedVersionProfile", fmt.Sprintf("requires K3k %s", platform.K3kVersion), observedAt)
	}
	return planCheck("LockedVersions", api.VirtualClusterPlanCheckPassed, "LockedVersionProfile", "observed Host K3k version matches the locked profile", observedAt)
}

func computeChecks(cell *api.Cell, class *api.VirtualNodeClass, observedAt metav1.Time) []api.VirtualClusterPlanCheck {
	requests := entitlementRequests(class.Spec.Entitlement.WorkloadHard)
	reservation := platform.CombinedReservation()
	result := make([]api.VirtualClusterPlanCheck, 0, 4)
	for _, resourceName := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory, corev1.ResourceEphemeralStorage} {
		name := resourceName.String()
		result = append(result, capacityCheck(nameToPlanCheck(resourceName), cell.Status.Inventory[name], addQuantities(requests[resourceName], reservation[resourceName]), observedAt))
	}
	acceleratorRequest := api.ResourceList{}
	for _, contract := range cell.Spec.MachineProfile.Devices {
		resourceName := corev1.ResourceName(contract.ResourceName)
		acceleratorRequest[resourceName] = requests[resourceName]
	}
	result = append(result, acceleratorCheck(cell, cell.Spec.MachineProfile.Name, acceleratorRequest, observedAt))
	return result
}

func capacityCheck(name string, inventory api.InventoryStatus, required resource.Quantity, observedAt metav1.Time) api.VirtualClusterPlanCheck {
	if inventory.Allocatable.IsZero() && inventory.AvailableEstimate.IsZero() {
		return planCheck(name, api.VirtualClusterPlanCheckUnknown, "InventoryMissing", "required resource is absent from the current Cell inventory", observedAt)
	}
	if inventory.AvailableEstimate.Cmp(required) < 0 {
		return planCheck(name, api.VirtualClusterPlanCheckFailed, "InsufficientCapacity", fmt.Sprintf("requires %s but only %s is currently estimated available", required.String(), inventory.AvailableEstimate.String()), observedAt)
	}
	return planCheck(name, api.VirtualClusterPlanCheckPassed, "CapacityAvailable", "current aggregate inventory can cover the requested envelope", observedAt)
}

func acceleratorCheck(cell *api.Cell, profileName string, requests api.ResourceList, observedAt metav1.Time) api.VirtualClusterPlanCheck {
	for resourceName, required := range requests {
		if required.IsZero() {
			continue
		}
		matched := false
		for _, node := range cell.Status.Nodes {
			if !node.Ready || node.Profile != profileName {
				continue
			}
			matched = true
			available := node.Allocatable[resourceName].DeepCopy()
			available.Sub(node.ActiveRequested[resourceName])
			if available.Cmp(required) >= 0 {
				goto nextResource
			}
		}
		if !matched {
			return planCheck("Accelerators", api.VirtualClusterPlanCheckUnknown, "ProfileLocalityUnknown", "no Ready per-Node inventory exists for the selected machine profile", observedAt)
		}
		return planCheck("Accelerators", api.VirtualClusterPlanCheckFailed, "InsufficientLocalCapacity", fmt.Sprintf("no selected-profile Host Node has %s available for %s", required.String(), resourceName), observedAt)
	nextResource:
	}
	return planCheck("Accelerators", api.VirtualClusterPlanCheckPassed, "LocalCapacityAvailable", "a selected-profile Host Node can satisfy each declared accelerator entitlement", observedAt)
}

func hostQuotaCheck(class *api.VirtualNodeClass, observedAt metav1.Time) api.VirtualClusterPlanCheck {
	for key, quantity := range class.Spec.Entitlement.WorkloadHard {
		if !strings.HasPrefix(string(key), "requests.") {
			continue
		}
		limitKey := corev1.ResourceName("limits." + strings.TrimPrefix(string(key), "requests."))
		limit, found := class.Spec.Entitlement.WorkloadHard[limitKey]
		if !found || !limit.Equal(quantity) {
			return planCheck("HostQuota", api.VirtualClusterPlanCheckFailed, "QuotaContractInvalid", fmt.Sprintf("%s must have an equal limit quota key", key), observedAt)
		}
	}
	return planCheck("HostQuota", api.VirtualClusterPlanCheckPassed, "QuotaContractValid", "Foundation Work can render matching Host request and limit quotas", observedAt)
}

func storageCheck(cell *api.Cell, class *api.VirtualNodeClass, observedAt metav1.Time) api.VirtualClusterPlanCheck {
	storage, found := cell.Status.StorageInventory[class.Spec.StorageClassName]
	if !found || !storage.FreeCapacityKnown || !storage.Fresh || storage.FreeCapacity == nil {
		return planCheck("TopoLVM", api.VirtualClusterPlanCheckUnknown, "PhysicalCapacityUnknown", "approved TopoLVM physical capacity is unavailable or stale", observedAt)
	}
	for _, node := range storage.Nodes {
		if node.FreeCapacityKnown && node.Fresh && node.FreeCapacity != nil && node.FreeCapacity.Sign() > 0 {
			return planCheck("TopoLVM", api.VirtualClusterPlanCheckPassed, "StorageLocalityAvailable", "at least one Host Node reports certified free capacity for the approved StorageClass", observedAt)
		}
	}
	return planCheck("TopoLVM", api.VirtualClusterPlanCheckUnknown, "StorageLocalityUnknown", "no per-Node certified TopoLVM free capacity is available", observedAt)
}

func nodePortCheck(cell *api.Cell, observedAt metav1.Time) api.VirtualClusterPlanCheck {
	ports := cell.Status.NodePorts
	if !ports.Fresh {
		return planCheck("NodePort", api.VirtualClusterPlanCheckUnknown, "NodePortInventoryStale", "Host Service NodePort inventory is unavailable or stale", observedAt)
	}
	minimum, maximum := platform.NodePortMin, platform.NodePortMax
	used := map[int32]struct{}{}
	for _, port := range ports.Allocated {
		if port >= minimum && port <= maximum {
			used[port] = struct{}{}
		}
	}
	if int64(len(used)) >= int64(maximum-minimum)+1 {
		return planCheck("NodePort", api.VirtualClusterPlanCheckFailed, "NodePortRangeExhausted", "no NodePort is currently available in the Cell allowed range", observedAt)
	}
	return planCheck("NodePort", api.VirtualClusterPlanCheckPassed, "NodePortAvailable", "the Cell allowed NodePort range has at least one unallocated candidate", observedAt)
}

func addQuantities(first, second resource.Quantity) resource.Quantity {
	result := first.DeepCopy()
	result.Add(second)
	return result
}

func nameToPlanCheck(resourceName corev1.ResourceName) string {
	switch resourceName {
	case corev1.ResourceCPU:
		return "CPU"
	case corev1.ResourceMemory:
		return "Memory"
	case corev1.ResourceEphemeralStorage:
		return "EphemeralStorage"
	default:
		return resourceName.String()
	}
}

func conditionTrue(conditions []metav1.Condition, conditionType string) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status == metav1.ConditionTrue
		}
	}
	return false
}

func planCheck(name string, status api.VirtualClusterPlanCheckStatus, reason, message string, observedAt metav1.Time) api.VirtualClusterPlanCheck {
	return api.VirtualClusterPlanCheck{Name: name, Status: status, Reason: reason, Message: boundedWorkMessage(message, ""), ObservedAt: observedAt}
}

func finalizePlanEvaluation(evaluation virtualClusterPlanEvaluation, now time.Time) virtualClusterPlanEvaluation {
	sort.Slice(evaluation.Checks, func(left, right int) bool { return evaluation.Checks[left].Name < evaluation.Checks[right].Name })
	evaluation.Decision = api.VirtualClusterPlanDecisionAccepted
	for _, check := range evaluation.Checks {
		if check.Status == api.VirtualClusterPlanCheckFailed {
			evaluation.Decision = api.VirtualClusterPlanDecisionRejected
			break
		}
		if check.Status == api.VirtualClusterPlanCheckUnknown {
			evaluation.Decision = api.VirtualClusterPlanDecisionUnknown
		}
	}
	if len(evaluation.Checks) == 0 {
		evaluation.Decision = api.VirtualClusterPlanDecisionUnknown
	}
	for _, check := range evaluation.Checks {
		if check.Name == "InventoryFresh" && check.Status == api.VirtualClusterPlanCheckPassed {
			evaluation.InventoryFresh = true
			evaluation.InventoryObservedAt = check.ObservedAt.DeepCopy()
			deadline := metav1.NewTime(now.Add(planFreshnessWindow))
			evaluation.ExpiresAt = &deadline
			break
		}
	}
	return evaluation
}

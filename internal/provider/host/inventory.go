package host

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/controller/helpers"
	"github.com/kubecell/kubecell/internal/platform"
)

type ResourceInventory struct {
	Capacity          resource.Quantity
	Allocatable       resource.Quantity
	Requested         resource.Quantity
	Pending           resource.Quantity
	AvailableEstimate resource.Quantity
}

func ObserveNode(node *corev1.Node, profileName string, devices []api.DeviceContract) (api.CellNodeStatus, bool) {
	status := api.CellNodeStatus{Name: node.Name, Profile: profileName, Capacity: corev1.ResourceList{}, Allocatable: corev1.ResourceList{}}
	ready := nodeReady(node) && node.Labels[platform.ProfileLabelKey] == profileName
	if !nodeReady(node) {
		status.ReadinessReasons = append(status.ReadinessReasons, "NodeNotReady")
	}
	if node.Labels[platform.ProfileLabelKey] != profileName {
		status.ReadinessReasons = append(status.ReadinessReasons, "ProfileLabelMismatch")
	}
	for _, resourceName := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory, corev1.ResourceEphemeralStorage} {
		if quantity, found := node.Status.Capacity[resourceName]; found {
			status.Capacity[resourceName] = quantity.DeepCopy()
		}
		if quantity, found := node.Status.Allocatable[resourceName]; found {
			status.Allocatable[resourceName] = quantity.DeepCopy()
		}
	}
	for _, device := range devices {
		capacity, capacityFound := node.Status.Capacity[corev1.ResourceName(device.ResourceName)]
		allocatable, allocatableFound := node.Status.Allocatable[corev1.ResourceName(device.ResourceName)]
		if capacityFound {
			status.Capacity[corev1.ResourceName(device.ResourceName)] = capacity.DeepCopy()
		}
		if allocatableFound {
			status.Allocatable[corev1.ResourceName(device.ResourceName)] = allocatable.DeepCopy()
		}
		if !capacityFound || capacity.Sign() <= 0 {
			ready = false
			status.ReadinessReasons = append(status.ReadinessReasons, "MissingCapacity:"+device.ResourceName)
		}
		if !allocatableFound || allocatable.Sign() <= 0 {
			ready = false
			status.ReadinessReasons = append(status.ReadinessReasons, "MissingAllocatable:"+device.ResourceName)
		}
	}
	status.Ready = ready
	status.ObservedAt = metav1.Now()
	return status, ready
}

func AggregateResourceInventory(nodes []*corev1.Node, pods []*corev1.Pod, k3kClusterName, resourceName string) ResourceInventory {
	result := ResourceInventory{Capacity: *resource.NewQuantity(0, resource.DecimalSI), Allocatable: *resource.NewQuantity(0, resource.DecimalSI), Requested: *resource.NewQuantity(0, resource.DecimalSI), Pending: *resource.NewQuantity(0, resource.DecimalSI)}
	for _, node := range nodes {
		if quantity, found := node.Status.Capacity[corev1.ResourceName(resourceName)]; found {
			result.Capacity.Add(quantity)
		}
		if quantity, found := node.Status.Allocatable[corev1.ResourceName(resourceName)]; found {
			result.Allocatable.Add(quantity)
		}
	}
	for _, pod := range pods {
		if pod.Labels[helpers.K3kClusterNameLabel] != k3kClusterName || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		quantity := helpers.EffectivePodRequests(pod)[corev1.ResourceName(resourceName)]
		if pod.Status.Phase == corev1.PodPending {
			result.Pending.Add(quantity)
		} else {
			result.Requested.Add(quantity)
		}
	}
	result.AvailableEstimate = result.Allocatable.DeepCopy()
	result.AvailableEstimate.Sub(result.Requested)
	if result.AvailableEstimate.Sign() < 0 {
		result.AvailableEstimate = *resource.NewQuantity(0, result.Allocatable.Format)
	}
	return result
}

func nodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

package helpers

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func EffectivePodRequests(pod *corev1.Pod) corev1.ResourceList {
	result := corev1.ResourceList{}
	for _, container := range pod.Spec.Containers {
		addResourceList(result, container.Resources.Requests)
	}
	initMaximum := corev1.ResourceList{}
	for _, container := range pod.Spec.InitContainers {
		for name, quantity := range container.Resources.Requests {
			if current, found := initMaximum[name]; !found || quantity.Cmp(current) > 0 {
				initMaximum[name] = quantity.DeepCopy()
			}
		}
	}
	for name, quantity := range initMaximum {
		regular := result[name]
		if quantity.Cmp(regular) > 0 {
			result[name] = quantity.DeepCopy()
		}
	}
	addResourceList(result, pod.Spec.Overhead)
	return result
}

func addResourceList(target corev1.ResourceList, source corev1.ResourceList) {
	for name, quantity := range source {
		current := target[name]
		current.Add(quantity)
		target[name] = current
	}
}

func SubtractResourceList(left, right corev1.ResourceList) corev1.ResourceList {
	result := corev1.ResourceList{}
	for name, quantity := range left {
		value := quantity.DeepCopy()
		if subtract, found := right[name]; found {
			value.Sub(subtract)
		}
		if value.Sign() < 0 {
			value = *resource.NewQuantity(0, quantity.Format)
		}
		result[name] = value
	}
	return result
}

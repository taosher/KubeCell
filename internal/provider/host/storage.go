package host

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
)

func ValidateStorageClass(class *storagev1.StorageClass, expectedName string) error {
	if class == nil {
		return fmt.Errorf("storage class is nil")
	}
	if class.Name != expectedName {
		return fmt.Errorf("storage class name %q does not match expected %q", class.Name, expectedName)
	}
	if class.Provisioner != "topolvm.io" {
		return fmt.Errorf("storage class provisioner %q is not certified TopoLVM", class.Provisioner)
	}
	if class.VolumeBindingMode == nil || *class.VolumeBindingMode != storagev1.VolumeBindingWaitForFirstConsumer {
		return fmt.Errorf("storage class must use WaitForFirstConsumer")
	}
	return nil
}

func PVLocalNode(pv *corev1.PersistentVolume) (string, bool) {
	if pv == nil || pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		return "", false
	}
	for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
		for _, expression := range term.MatchExpressions {
			if expression.Key != corev1.LabelHostname || expression.Operator != corev1.NodeSelectorOpIn || len(expression.Values) != 1 {
				continue
			}
			return expression.Values[0], true
		}
	}
	return "", false
}

func PodSatisfiesPVLocality(pod *corev1.Pod, node string) bool {
	return pod != nil && node != "" && pod.Spec.NodeName == node
}

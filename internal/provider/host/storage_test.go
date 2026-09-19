package host

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateStorageClassRequiresTopoLVMAndWaitForFirstConsumer(t *testing.T) {
	class := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "topolvm-provisioner"}, Provisioner: "topolvm.io", VolumeBindingMode: ptr(storagev1.VolumeBindingWaitForFirstConsumer), ReclaimPolicy: ptr(corev1.PersistentVolumeReclaimRetain)}
	if err := ValidateStorageClass(class, "topolvm-provisioner"); err != nil {
		t.Fatalf("ValidateStorageClass() error = %v", err)
	}
}

func TestValidateStorageClassRejectsHostPathFallback(t *testing.T) {
	class := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "local-path"}, Provisioner: "rancher.io/local-path", VolumeBindingMode: ptr(storagev1.VolumeBindingImmediate)}
	if err := ValidateStorageClass(class, "topolvm-provisioner"); err == nil {
		t.Fatal("ValidateStorageClass() error = nil, want HostPath fallback rejection")
	}
}

func TestPVNodeAffinityIsExtractedForPVCLocality(t *testing.T) {
	pv := &corev1.PersistentVolume{Spec: corev1.PersistentVolumeSpec{NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{"ascend-a"}}}}}}}}}
	if node, ok := PVLocalNode(pv); !ok || node != "ascend-a" {
		t.Fatalf("PVLocalNode() = %q, %v", node, ok)
	}
}

func TestHostPodMustSatisfyPVLocality(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{NodeName: "ascend-a"}}
	if !PodSatisfiesPVLocality(pod, "ascend-a") {
		t.Fatal("expected pod to satisfy local node")
	}
	if PodSatisfiesPVLocality(pod, "ascend-b") {
		t.Fatal("expected pod to violate local node")
	}
}

func ptr[T any](value T) *T { return &value }

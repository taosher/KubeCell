package admission

import (
	"strings"
	"testing"

	"github.com/kubecell/kubecell/internal/platform"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateReflectedPodRejectsAnyHostPathType(t *testing.T) {
	for _, volumeType := range []corev1.HostPathType{
		corev1.HostPathUnset,
		corev1.HostPathDirectory,
		corev1.HostPathDirectoryOrCreate,
		corev1.HostPathFile,
		corev1.HostPathFileOrCreate,
		corev1.HostPathSocket,
		corev1.HostPathCharDev,
		corev1.HostPathBlockDev,
	} {
		pod := hostPathPod("/workspace", volumeType)
		violations := ValidateReflectedPod(pod, testPolicy())
		if !containsExactViolation(violations, platform.HostPathDenyMessage) {
			t.Fatalf("type %q violations = %v, want %q", volumeType, violations, platform.HostPathDenyMessage)
		}
	}
}

func TestValidateReflectedPodRejectsHostPathRegardlessOfPath(t *testing.T) {
	volumeType := corev1.HostPathDirectoryOrCreate
	for _, path := range []string{"/", "/workspace", "/workspace/cache", "/data/vcluster-demo", "/data/vcluster-demo/cache"} {
		t.Run(strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			violations := ValidateReflectedPod(hostPathPod(path, volumeType), testPolicy())
			if !containsExactViolation(violations, platform.HostPathDenyMessage) {
				t.Fatalf("path %q violations = %v, want %q", path, violations, platform.HostPathDenyMessage)
			}
		})
	}
}

func TestValidateReflectedPodRejectsNilHostPathType(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kc-vc", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}},
		Spec: corev1.PodSpec{
			Volumes:    []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/workspace"}}}},
			Containers: []corev1.Container{{Name: "work", Image: "example/work"}},
		},
	}
	if violations := ValidateReflectedPod(pod, testPolicy()); !containsExactViolation(violations, platform.HostPathDenyMessage) {
		t.Fatalf("violations = %v, want %q", violations, platform.HostPathDenyMessage)
	}
}

func TestValidateReflectedPodRejectsEachHostPathVolume(t *testing.T) {
	volumeType := corev1.HostPathDirectory
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kc-vc", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{Name: "a", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/a", Type: &volumeType}}},
				{Name: "b", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/b", Type: &volumeType}}},
			},
			Containers: []corev1.Container{{Name: "work", Image: "example/work"}},
		},
	}
	violations := ValidateReflectedPod(pod, testPolicy())
	count := 0
	for _, violation := range violations {
		if violation == platform.HostPathDenyMessage {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("violations = %v, want 2 occurrences of %q", violations, platform.HostPathDenyMessage)
	}
}

func TestValidateReflectedPodWithoutPlacementRejectsHostPath(t *testing.T) {
	pod := hostPathPod("/workspace/cache", corev1.HostPathDirectoryOrCreate)
	if violations := ValidateReflectedPodWithoutPlacement(pod, testPolicy()); !containsExactViolation(violations, platform.HostPathDenyMessage) {
		t.Fatalf("violations = %v, want %q", violations, platform.HostPathDenyMessage)
	}
}

func TestMutateReflectedPodRejectsHostPathWithoutRewrite(t *testing.T) {
	pod := hostPathPod("/workspace/cache", corev1.HostPathDirectoryOrCreate)
	changed, err := MutateReflectedPod(pod, testPolicy())
	if err == nil {
		t.Fatal("MutateReflectedPod() error = nil, want hostPath rejection")
	}
	if changed {
		t.Fatal("MutateReflectedPod() changed = true, want false on rejection")
	}
	if !strings.Contains(err.Error(), platform.HostPathDenyMessage) {
		t.Fatalf("MutateReflectedPod() error = %v, want %q", err, platform.HostPathDenyMessage)
	}
	if got := pod.Spec.Volumes[0].HostPath.Path; got != "/workspace/cache" {
		t.Fatalf("HostPath was rewritten to %q, want untouched", got)
	}
}

func TestValidateReflectedPodAllowsPodWithoutHostPath(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kc-vc", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "work", Image: "example/work"}},
		},
	}
	if violations := ValidateReflectedPod(pod, testPolicy()); len(violations) != 0 {
		t.Fatalf("violations = %v, want none", violations)
	}
}

func containsExactViolation(violations []string, want string) bool {
	for _, violation := range violations {
		if violation == want {
			return true
		}
	}
	return false
}

func hostPathPod(path string, volumeType corev1.HostPathType) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kc-vc", Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}},
		Spec: corev1.PodSpec{
			Volumes:    []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: path, Type: &volumeType}}}},
			Containers: []corev1.Container{{Name: "work", Image: "example/work", VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/data"}}}},
		},
	}
}

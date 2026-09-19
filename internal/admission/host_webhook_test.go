package admission

import (
	"context"
	"strings"
	"testing"

	"github.com/kubecell/kubecell/internal/platform"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestHostWorkloadWebhookInjectsReflectedSystemResources(t *testing.T) {
	webhook := newHostWorkloadWebhook(t, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kc-work", Labels: map[string]string{
		"kubecell.io/virtual-cluster-uid": "vc-1",
		"kubecell.io/profile-name":        "ascend",
		"policy.k3k.io/policy-name":       "kc-vc",
	}, Annotations: map[string]string{
		"kubecell.io/profile-label-key":       "hardware.kubecell.io/profile",
		"kubecell.io/reflected-system-cpu":    "100m",
		"kubecell.io/reflected-system-memory": "256Mi",
	}}})
	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Labels:      map[string]string{"k3k.io/clusterName": "kc-vc"},
		Annotations: map[string]string{"k3k.io/namespace": "kube-system"},
	}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "coredns"}}}}
	response := webhook.Handle(context.Background(), podAdmissionRequest(t, "kc-work", "coredns-kc-vc", pod))
	if !response.Allowed {
		t.Fatalf("reflected system Pod denied: %#v", response)
	}
	for _, patch := range response.Patches {
		if strings.Contains(patch.Path, "/spec/containers/0/resources") {
			return
		}
	}
	t.Fatalf("resource patch missing from %#v", response.Patches)
}

func TestHostWorkloadWebhookAllowsNonKubecellNamespace(t *testing.T) {
	webhook := newHostWorkloadWebhook(t, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ordinary"}})
	response := webhook.Handle(context.Background(), podAdmissionRequest(t, "ordinary", "work", corev1.Pod{}))
	if !response.Allowed {
		t.Fatalf("ordinary namespace response denied: %#v", response)
	}
}

func TestHostWorkloadWebhookFailsClosedForIncompleteKubecellPolicy(t *testing.T) {
	webhook := newHostWorkloadWebhook(t, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kc-work", Labels: map[string]string{
		"kubecell.io/virtual-cluster-uid": "vc-1",
	}}})
	response := webhook.Handle(context.Background(), podAdmissionRequest(t, "kc-work", "work", corev1.Pod{}))
	if response.Allowed {
		t.Fatal("incomplete KubeCell namespace was allowed")
	}
}

func TestHostWorkloadWebhookInjectsOnlyProfileAffinity(t *testing.T) {
	class := "nvidia-runtime"
	webhook := newHostWorkloadWebhook(t, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kc-work", Labels: map[string]string{
		"kubecell.io/virtual-cluster-uid":  "vc-1",
		"kubecell.io/profile-name":         "nvidia",
		"kubecell.io/virtual-cluster-name": "demo",
	}, Annotations: map[string]string{"kubecell.io/profile-label-key": "hardware.kubecell.io/profile"}}})
	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		"kubecell.io/virtual-cluster-uid": "vc-1",
	}}, Spec: corev1.PodSpec{RuntimeClassName: &class}}
	response := webhook.Handle(context.Background(), podAdmissionRequest(t, "kc-work", "work", pod))
	if !response.Allowed || len(response.Patches) == 0 {
		t.Fatalf("expected allowed affinity patch, got %#v", response)
	}
	if response.Patches[0].Path != "/spec/affinity" {
		t.Fatalf("first patch path = %q, want /spec/affinity", response.Patches[0].Path)
	}
	if response.Patches[0].Value == nil {
		t.Fatal("affinity patch has no value")
	}
}

func TestHostWorkloadWebhookRejectsPhysicalPlacementAndHostPath(t *testing.T) {
	webhook := newHostWorkloadWebhook(t, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kc-work", Labels: map[string]string{
		"kubecell.io/virtual-cluster-uid": "vc-1",
		"kubecell.io/profile-name":        "nvidia",
	}, Annotations: map[string]string{"kubecell.io/profile-label-key": "hardware.kubecell.io/profile"}}})
	pod := corev1.Pod{Spec: corev1.PodSpec{
		NodeName:   "physical-a",
		Containers: []corev1.Container{{Name: "work", Image: "example/work", Ports: []corev1.ContainerPort{{HostPort: 3000}}, VolumeMounts: []corev1.VolumeMount{{Name: "host", MountPath: "/host"}}}},
		Volumes:    []corev1.Volume{{Name: "host", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/"}}}},
	}}
	response := webhook.Handle(context.Background(), podAdmissionRequest(t, "kc-work", "work", pod))
	if response.Allowed {
		t.Fatal("physical placement and HostPath workload was allowed")
	}
}

func TestHostWorkloadWebhookRejectsHostPathWithPlatformMessage(t *testing.T) {
	webhook := newHostWorkloadWebhook(t, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kc-work", Labels: map[string]string{
		"kubecell.io/virtual-cluster-uid":  "vc-1",
		"kubecell.io/profile-name":         "nvidia",
		"kubecell.io/virtual-cluster-name": "demo",
	}, Annotations: map[string]string{
		"kubecell.io/profile-label-key": "hardware.kubecell.io/profile",
	}}})
	pod := hostPathPod("/workspace/cache", corev1.HostPathDirectoryOrCreate)
	response := webhook.Handle(context.Background(), podAdmissionRequest(t, "kc-work", "work", *pod))
	if response.Allowed {
		t.Fatal("hostPath workload was allowed, want rejection")
	}
	if response.Result == nil || !strings.Contains(response.Result.Message, platform.HostPathDenyMessage) {
		t.Fatalf("denial = %#v, want message containing %q", response.Result, platform.HostPathDenyMessage)
	}
}

func TestHostWorkloadWebhookPatchesExistingApprovedStorageAffinity(t *testing.T) {
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kc-work", Labels: map[string]string{
		"kubecell.io/virtual-cluster-uid": "vc-1",
		"kubecell.io/profile-name":        "nvidia",
	}, Annotations: map[string]string{
		"kubecell.io/profile-label-key":   "hardware.kubecell.io/profile",
		"kubecell.io/storage-class-names": "topolvm",
	}}}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "claim", Namespace: "kc-work"}, Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: stringPtr("topolvm"), VolumeName: "pv-a"}}
	pv := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-a"}, Spec: corev1.PersistentVolumeSpec{NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{"node-a"}}}}}}}}}
	webhook := newHostWorkloadWebhook(t, namespace, pvc, pv)
	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"kubecell.io/virtual-cluster-uid": "vc-1"}}, Spec: corev1.PodSpec{Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "claim"}}}}, Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: corev1.LabelHostname, Operator: corev1.NodeSelectorOpIn, Values: []string{"node-a"}}}}}}}}, Containers: []corev1.Container{{Name: "work", Image: "example/work"}}}}
	response := webhook.Handle(context.Background(), podAdmissionRequest(t, "kc-work", "work", pod))
	if !response.Allowed || len(response.Patches) == 0 {
		t.Fatalf("expected profile patch, allowed=%v result=%v patches=%#v", response.Allowed, response.Result, response.Patches)
	}
}

func TestHostWorkloadWebhookRejectsPVCOutsideResolvedStorageMapping(t *testing.T) {
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kc-work", Labels: map[string]string{
		"kubecell.io/virtual-cluster-uid": "vc-1",
		"kubecell.io/profile-name":        "nvidia",
	}, Annotations: map[string]string{
		"kubecell.io/profile-label-key":   "hardware.kubecell.io/profile",
		"kubecell.io/storage-class-names": "topolvm",
	}}}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "claim", Namespace: "kc-work"}, Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: stringPtr("other")}}
	webhook := newHostWorkloadWebhook(t, namespace, pvc)
	pod := corev1.Pod{Spec: corev1.PodSpec{Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "claim"}}}}, Containers: []corev1.Container{{Name: "work", Image: "example/work"}}}}
	response := webhook.Handle(context.Background(), podAdmissionRequest(t, "kc-work", "work", pod))
	if response.Allowed {
		t.Fatal("PVC with an unapproved StorageClass was allowed")
	}
}

func newHostWorkloadWebhook(t *testing.T, objects ...client.Object) *HostWorkloadWebhook {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return &HostWorkloadWebhook{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(), Decoder: admission.NewDecoder(scheme)}
}

func podAdmissionRequest(t *testing.T, namespace, name string, pod corev1.Pod) admission.Request {
	t.Helper()
	pod.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
	pod.ObjectMeta.Namespace = namespace
	pod.ObjectMeta.Name = name
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pod)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID:       types.UID("request-1"),
		Operation: admissionv1.Create,
		Namespace: namespace,
		Name:      name,
		Resource:  metav1.GroupVersionResource{Version: "v1", Resource: "pods"},
		Object:    runtime.RawExtension{Raw: encoded},
	}}
}

func stringPtr(value string) *string { return &value }

func TestHostWorkloadWebhookInheritsPodLevelForK3kInfra(t *testing.T) {
	webhook := newHostWorkloadWebhook(t, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kc-vc", Labels: map[string]string{
		"kubecell.io/virtual-cluster-uid": "vc-1",
		"kubecell.io/profile-name":        "ascend",
		"policy.k3k.io/policy-name":       "kc-vc",
	}, Annotations: map[string]string{
		"kubecell.io/profile-label-key": "hardware.kubecell.io/profile",
	}}})
	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{"cluster": "kc-vc", "role": "server"},
	}, Spec: corev1.PodSpec{
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: quantity("500m"), corev1.ResourceMemory: quantity("1Gi")},
			Limits:   corev1.ResourceList{corev1.ResourceCPU: quantity("500m"), corev1.ResourceMemory: quantity("1Gi")},
		},
		Containers: []corev1.Container{{Name: "server", Image: "example/server"}},
	}}
	response := webhook.Handle(context.Background(), podAdmissionRequest(t, "kc-vc", "k3k-kc-vc-server-0", pod))
	if !response.Allowed {
		t.Fatalf("K3k infra Pod denied: %#v", response)
	}
	for _, patch := range response.Patches {
		if strings.Contains(patch.Path, "/spec/containers/0/resources") {
			return
		}
	}
	t.Fatalf("pod-level inheritance patch missing from %#v", response.Patches)
}

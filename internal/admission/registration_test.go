package admission

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func TestRegisterHostWorkloadWebhookExposesPodMutationPath(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "kc-work",
		Labels: map[string]string{
			labelVirtualClusterUID: "vc-1",
			labelProfileName:       "nvidia",
		},
		Annotations: map[string]string{labelProfileLabelKey: "hardware.kubecell.io/profile"},
	}}).Build()
	server := webhook.NewServer(webhook.Options{WebhookMux: http.NewServeMux()})
	if err := RegisterHostWorkloadWebhook(server, client, scheme); err != nil {
		t.Fatalf("RegisterHostWorkloadWebhook() error = %v", err)
	}
	pod := corev1.Pod{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}, ObjectMeta: metav1.ObjectMeta{
		Name: "work", Namespace: "kc-work", Labels: map[string]string{labelVirtualClusterUID: "vc-1"},
	}}
	podRaw, err := json.Marshal(pod)
	if err != nil {
		t.Fatal(err)
	}
	reviewRaw, err := json.Marshal(admissionv1.AdmissionReview{TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"}, Request: &admissionv1.AdmissionRequest{
		UID: types.UID("request-1"), Operation: admissionv1.Create, Namespace: "kc-work", Name: "work", Resource: metav1.GroupVersionResource{Version: "v1", Resource: "pods"}, Object: runtime.RawExtension{Raw: podRaw},
	}})
	if err != nil {
		t.Fatal(err)
	}
	recording := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/mutate-v1-pod", bytes.NewReader(reviewRaw))
	request.Header.Set("Content-Type", "application/json")
	server.WebhookMux().ServeHTTP(recording, request)
	if recording.Code != http.StatusOK {
		t.Fatalf("webhook HTTP status = %d, want 200; body=%s", recording.Code, recording.Body.String())
	}
	responseReview := &admissionv1.AdmissionReview{}
	if err := json.Unmarshal(recording.Body.Bytes(), responseReview); err != nil {
		t.Fatalf("decode webhook response: %v", err)
	}
	if responseReview.Response == nil || !responseReview.Response.Allowed {
		t.Fatalf("webhook response = %#v, result=%#v, want allowed", responseReview.Response, responseReview.Response.Result)
	}
}

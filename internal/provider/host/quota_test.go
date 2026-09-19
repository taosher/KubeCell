package host

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestBuildQuotaHardIncludesRequestAndLimitForms(t *testing.T) {
	hard := corev1.ResourceList{"requests.cpu": resource.MustParse("8"), "limits.cpu": resource.MustParse("8"), "requests.huawei.com/Ascend910": resource.MustParse("2"), "limits.huawei.com/Ascend910": resource.MustParse("2")}
	quota := BuildHostQuota(hard)
	if !quota["requests.cpu"].Equal(resource.MustParse("8")) || !quota["limits.cpu"].Equal(resource.MustParse("8")) {
		t.Fatalf("quota = %#v", quota)
	}
	if !quota["requests.huawei.com/Ascend910"].Equal(resource.MustParse("2")) {
		deviceQuota := quota["requests.huawei.com/Ascend910"]
		t.Fatalf("device quota = %s", deviceQuota.String())
	}
}

func TestTenantHeadroomSubtractsSystemReservationAndUsedQuota(t *testing.T) {
	hard := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("1Gi"), "huawei.com/Ascend910": resource.MustParse("2")}
	used := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("350m"), corev1.ResourceMemory: resource.MustParse("512Mi"), "huawei.com/Ascend910": resource.MustParse("1")}
	reservation := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi"), "huawei.com/Ascend910": resource.MustParse("0")}
	headroom := TenantHeadroom(hard, used, reservation)
	if !headroom[corev1.ResourceCPU].Equal(resource.MustParse("50m")) {
		cpu := headroom[corev1.ResourceCPU]
		t.Fatalf("cpu headroom = %s", cpu.String())
	}
	if !headroom[corev1.ResourceMemory].Equal(resource.MustParse("384Mi")) {
		memory := headroom[corev1.ResourceMemory]
		t.Fatalf("memory headroom = %s", memory.String())
	}
	if !headroom["huawei.com/Ascend910"].Equal(resource.MustParse("1")) {
		device := headroom["huawei.com/Ascend910"]
		t.Fatalf("device headroom = %s", device.String())
	}
}

func TestTenantHeadroomMapsPlainReservationToQuotaRequestKeys(t *testing.T) {
	hard := corev1.ResourceList{"requests.cpu": resource.MustParse("500m"), "limits.cpu": resource.MustParse("500m")}
	used := corev1.ResourceList{"requests.cpu": resource.MustParse("300m"), "limits.cpu": resource.MustParse("300m")}
	reservation := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")}
	headroom := TenantHeadroom(hard, used, reservation)
	if !headroom["requests.cpu"].Equal(resource.MustParse("100m")) || !headroom["limits.cpu"].Equal(resource.MustParse("100m")) {
		t.Fatalf("headroom = %#v, want 100m for both quota keys", headroom)
	}
	if !ReservationExceeded(hard, corev1.ResourceList{"requests.cpu": resource.MustParse("450m")}, reservation) {
		t.Fatal("ReservationExceeded() = false, want true")
	}
}

func TestClassifyQuotaDenialSeparatesHostAdmissionFromChildEntitlement(t *testing.T) {
	if got := ClassifyQuotaDenial("host ResourceQuota exceeded"); got != DenialHostQuota {
		t.Fatalf("classification = %q", got)
	}
	if got := ClassifyQuotaDenial("child logical capacity exceeded"); got != DenialChildEntitlement {
		t.Fatalf("classification = %q", got)
	}
}

func TestReservationExceededDoesNotMutateInputTables(t *testing.T) {
	hard := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("4")}
	used := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")}
	reservation := corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")}
	if ReservationExceeded(hard, used, reservation) {
		t.Fatal("ReservationExceeded = true, want false")
	}
	if !hard[corev1.ResourceCPU].Equal(resource.MustParse("4")) {
		cpu := hard[corev1.ResourceCPU]
		t.Fatalf("hard cpu mutated to %s (KI-9 aliasing)", cpu.String())
	}
	if ReservationExceeded(hard, used, reservation) {
		t.Fatal("second call differs: shared state was polluted")
	}
}

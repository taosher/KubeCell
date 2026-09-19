package host

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type DenialLayer string

const (
	DenialHostQuota        DenialLayer = "HostQuota"
	DenialChildEntitlement DenialLayer = "ChildEntitlement"
	DenialUnknown          DenialLayer = "Unknown"
)

func BuildHostQuota(hard corev1.ResourceList) corev1.ResourceList {
	result := corev1.ResourceList{}
	for name, quantity := range hard {
		result[name] = quantity.DeepCopy()
	}
	return result
}

func TenantHeadroom(hard, used, reservation corev1.ResourceList) corev1.ResourceList {
	result := corev1.ResourceList{}
	for name, limit := range hard {
		value := limit.DeepCopy()
		if current, found := used[name]; found {
			value.Sub(current)
		}
		if reserved, found := reservationForQuotaKey(reservation, name); found {
			value.Sub(reserved)
		}
		if value.Sign() < 0 {
			value = *resource.NewQuantity(0, limit.Format)
		}
		result[name] = value
	}
	return result
}

func ReservationExceeded(hard, used, reservation corev1.ResourceList) bool {
	for name, current := range used {
		limit, found := hard[name]
		if !found {
			continue
		}
		// Quantity is a value copy that still shares the underlying Dec pointer: in-place Sub would corrupt the caller's hard table
		// (KI-9: it once silently corrupted the persisted status.host.quotaHard), so DeepCopy first.
		limit = limit.DeepCopy()
		reserved, _ := reservationForQuotaKey(reservation, name)
		limit.Sub(reserved)
		if current.Cmp(limit) > 0 {
			return true
		}
	}
	return false
}

func reservationForQuotaKey(reservation corev1.ResourceList, quotaKey corev1.ResourceName) (resource.Quantity, bool) {
	if value, found := reservation[quotaKey]; found {
		return value, true
	}
	key := string(quotaKey)
	for _, prefix := range []string{"requests.", "limits."} {
		if strings.HasPrefix(key, prefix) {
			if value, found := reservation[corev1.ResourceName(strings.TrimPrefix(key, prefix))]; found {
				return value, true
			}
		}
	}
	return resource.Quantity{}, false
}

func ClassifyQuotaDenial(message string) DenialLayer {
	lower := strings.ToLower(message)
	if strings.Contains(lower, "host resourcequota") || strings.Contains(lower, "resourcequota exceeded") {
		return DenialHostQuota
	}
	if strings.Contains(lower, "child logical capacity") || strings.Contains(lower, "child capacity") {
		return DenialChildEntitlement
	}
	return DenialUnknown
}

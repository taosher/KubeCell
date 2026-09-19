package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/platform"
)

func TestResolveVirtualClusterLocksPlatformVersions(t *testing.T) {
	cluster, cell, class := resolutionObjects()
	resolved, err := resolveVirtualCluster(cluster, cell, class)
	if err != nil {
		t.Fatalf("resolveVirtualCluster() error = %v", err)
	}
	if resolved.K3kVersion != platform.K3kVersion {
		t.Fatalf("K3kVersion = %q, want platform constant %q", resolved.K3kVersion, platform.K3kVersion)
	}
	if resolved.ChildK3sVersion != platform.ChildK3sVersion {
		t.Fatalf("ChildK3sVersion = %q, want platform constant %q", resolved.ChildK3sVersion, platform.ChildK3sVersion)
	}
	if resolved.K3kChildVersion == "" || resolved.ChildImageTag == "" {
		t.Fatalf("resolved child version = %#v, want K3k spec and image tag", resolved)
	}
	if resolved.MachineProfileName != "ascend-910b" || resolved.ProfileHash == "" {
		t.Fatalf("resolved profile = %#v, want machine profile name and fingerprint", resolved)
	}
	if resolved.CellUID != string(cell.UID) || resolved.ClassUID != string(class.UID) || resolved.ManagedClusterName != cell.Spec.ManagedClusterRef.Name {
		t.Fatalf("resolved identity = %#v", resolved)
	}
}

func TestResolveVirtualClusterFingerprintStableAndSensitive(t *testing.T) {
	cluster, cell, class := resolutionObjects()
	first, err := resolveVirtualCluster(cluster, cell, class)
	if err != nil {
		t.Fatalf("resolveVirtualCluster() error = %v", err)
	}
	second, err := resolveVirtualCluster(cluster, cell, class)
	if err != nil {
		t.Fatalf("resolveVirtualCluster() error = %v", err)
	}
	if first.ProfileHash != second.ProfileHash {
		t.Fatalf("profile hash not stable: %q vs %q", first.ProfileHash, second.ProfileHash)
	}
	_, mutatedCell, _ := resolutionObjects()
	mutatedCell.Spec.MachineProfile.Devices = append(mutatedCell.Spec.MachineProfile.Devices, api.DeviceContract{Name: "extra", ResourceName: "example.com/extra"})
	mutated, err := resolveVirtualCluster(cluster, mutatedCell, class)
	if err != nil {
		t.Fatalf("resolveVirtualCluster() error = %v", err)
	}
	if mutated.ProfileHash == first.ProfileHash {
		t.Fatal("profile hash ignores device contracts")
	}
	_, _, mutatedClass := resolutionObjects()
	mutatedClass.Spec.StorageClassName = "other-topolvm"
	storageMutated, err := resolveVirtualCluster(cluster, cell, mutatedClass)
	if err != nil {
		t.Fatalf("resolveVirtualCluster() error = %v", err)
	}
	if storageMutated.ProfileHash == first.ProfileHash {
		t.Fatal("profile hash ignores storage class")
	}
}

func TestResolveVirtualClusterRejectsUndeclaredDeviceEntitlement(t *testing.T) {
	cluster, cell, class := resolutionObjects()
	class.Spec.Entitlement.WorkloadHard[corev1.ResourceName("requests.nvidia.com/gpu")] = resource.MustParse("1")
	class.Spec.Entitlement.WorkloadHard[corev1.ResourceName("limits.nvidia.com/gpu")] = resource.MustParse("1")
	if _, err := resolveVirtualCluster(cluster, cell, class); err == nil {
		t.Fatal("resolveVirtualCluster accepted an undeclared device resource")
	}
}

func TestResolveVirtualClusterRejectsMismatchedDeviceRequestLimit(t *testing.T) {
	cluster, cell, class := resolutionObjects()
	class.Spec.Entitlement.WorkloadHard[corev1.ResourceName("requests.huawei.com/Ascend910")] = resource.MustParse("1")
	class.Spec.Entitlement.WorkloadHard[corev1.ResourceName("limits.huawei.com/Ascend910")] = resource.MustParse("2")
	if _, err := resolveVirtualCluster(cluster, cell, class); err == nil {
		t.Fatal("resolveVirtualCluster accepted mismatched device request and limit")
	}
}

func TestResolveVirtualClusterRejectsEmptyMachineProfile(t *testing.T) {
	cluster, cell, class := resolutionObjects()
	cell.Spec.MachineProfile.Name = ""
	if _, err := resolveVirtualCluster(cluster, cell, class); err == nil {
		t.Fatal("resolveVirtualCluster accepted an empty machine profile")
	}
}

func TestResolveVirtualClusterMapsStorageClass(t *testing.T) {
	cluster, cell, class := resolutionObjects()
	resolved, err := resolveVirtualCluster(cluster, cell, class)
	if err != nil {
		t.Fatalf("resolveVirtualCluster() error = %v", err)
	}
	mapping := resolved.Storage.Class
	if mapping.ChildName != class.Spec.StorageClassName || mapping.HostName != class.Spec.StorageClassName {
		t.Fatalf("storage mapping = %#v, want certified class %q", mapping, class.Spec.StorageClassName)
	}
	if mapping.VolumeBindingMode != storagev1.VolumeBindingWaitForFirstConsumer {
		t.Fatalf("volumeBindingMode = %q, want WaitForFirstConsumer", mapping.VolumeBindingMode)
	}
	if mapping.ReclaimPolicy != corev1.PersistentVolumeReclaimRetain {
		t.Fatalf("reclaimPolicy = %q, want Retain", mapping.ReclaimPolicy)
	}
	if !mapping.AllowVolumeExpansion {
		t.Fatalf("allowVolumeExpansion = false, want platform constant true")
	}
}

func TestResolveVirtualClusterCarriesQuotaAndReservation(t *testing.T) {
	cluster, cell, class := resolutionObjects()
	resolved, err := resolveVirtualCluster(cluster, cell, class)
	if err != nil {
		t.Fatalf("resolveVirtualCluster() error = %v", err)
	}
	for key, quantity := range class.Spec.Entitlement.WorkloadHard {
		got, found := resolved.Quota[key]
		if !found || !got.Equal(quantity) {
			t.Fatalf("quota[%q] = %v, want %s", key, got, quantity.String())
		}
	}
	if resolved.Reservation.Policy != api.ReservationPolicyIncludedInHostQuota {
		t.Fatalf("reservation policy = %q, want IncludedInHostQuota", resolved.Reservation.Policy)
	}
	for key, quantity := range platform.ControlPlaneReservation() {
		if got := resolved.Reservation.ControlPlane[key]; !got.Equal(quantity) {
			t.Fatalf("control-plane reservation[%q] = %s, want %s", key, got.String(), quantity.String())
		}
	}
	for key, quantity := range platform.ReflectedSystemReservation() {
		if got := resolved.Reservation.ReflectedSystem[key]; !got.Equal(quantity) {
			t.Fatalf("reflected-system reservation[%q] = %s, want %s", key, got.String(), quantity.String())
		}
	}
	if len(resolved.AcceleratorKeys) != 1 || resolved.AcceleratorKeys[0] != "huawei.com/Ascend910" {
		t.Fatalf("acceleratorKeys = %v, want declared device resource", resolved.AcceleratorKeys)
	}
	if resolved.HostPathTenantName != "" || resolved.HostPathTenantRoot != "" {
		t.Fatalf("resolved = %#v, want no HostPath tenant identity", resolved)
	}
}

func TestSameResolvedProviderDetectsQuotaAndStorageDrift(t *testing.T) {
	cluster, cell, class := resolutionObjects()
	current, err := resolveVirtualCluster(cluster, cell, class)
	if err != nil {
		t.Fatalf("resolveVirtualCluster() error = %v", err)
	}
	desired := current
	desired.Quota = corev1.ResourceList{"requests.cpu": resource.MustParse("2")}
	if sameResolvedProvider(current, desired) {
		t.Fatal("sameResolvedProvider ignored quota drift")
	}
	desired = current
	desired.Storage.Class.HostName = "another-topolvm"
	if sameResolvedProvider(current, desired) {
		t.Fatal("sameResolvedProvider ignored storage drift")
	}
	if !sameResolvedProvider(current, current) {
		t.Fatal("sameResolvedProvider rejected identical snapshots")
	}
}

func resolutionObjects() (*api.VirtualCluster, *api.Cell, *api.VirtualNodeClass) {
	cluster := &api.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "vc", Namespace: "tenant", UID: "vc-uid"},
		Spec:       api.VirtualClusterSpec{CellRef: api.ObjectReference{Name: "cell-a", Namespace: "kubecell-system"}, ClassRef: api.ObjectReference{Name: "small"}},
	}
	cell := &api.Cell{
		ObjectMeta: metav1.ObjectMeta{Name: "cell-a", Namespace: "kubecell-system", UID: "cell-uid"},
		Spec: api.CellSpec{
			ManagedClusterRef: api.ObjectReference{Name: "cell-a"},
			MachineProfile:    api.MachineProfile{Name: "ascend-910b", Devices: []api.DeviceContract{{Name: "ascend", ResourceName: "huawei.com/Ascend910"}}},
		},
	}
	class := &api.VirtualNodeClass{
		ObjectMeta: metav1.ObjectMeta{Name: "small", UID: "class-uid"},
		Spec: api.VirtualNodeClassSpec{
			Entitlement: api.EntitlementSpec{WorkloadHard: api.ResourceList{
				"requests.cpu": resource.MustParse("1"), "limits.cpu": resource.MustParse("1"),
				"requests.huawei.com/Ascend910": resource.MustParse("1"), "limits.huawei.com/Ascend910": resource.MustParse("1"),
			}},
			StorageClassName: "topolvm-provisioner",
		},
	}
	return cluster, cell, class
}

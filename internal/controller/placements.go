package controller

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/kubecell/kubecell/api/v1alpha1"
	"github.com/kubecell/kubecell/internal/controller/helpers"
	hostprovider "github.com/kubecell/kubecell/internal/provider/host"
)

type VirtualNodeHostReader interface {
	GetResourceQuota(context.Context, string, string) (*corev1.ResourceQuota, error)
	ListPods(context.Context, string) (*corev1.PodList, error)
	GetPersistentVolumeClaim(context.Context, string, string) (*corev1.PersistentVolumeClaim, error)
	GetPersistentVolume(context.Context, string) (*corev1.PersistentVolume, error)
	ListNodes(context.Context) (*corev1.NodeList, error)
}

func hostPVLocalNode(pv *corev1.PersistentVolume) (string, bool) {
	return hostprovider.PVLocalNode(pv)
}

type VirtualNodeHostReaderFactory interface {
	ForCell(context.Context, *api.Cell) (VirtualNodeHostReader, error)
}

func projectPlacements(ctx context.Context, reader VirtualNodeHostReader, namespace, k3kClusterName string, pods []corev1.Pod) []api.WorkloadPlacement {
	placements := make([]api.WorkloadPlacement, 0, len(pods))
	for _, pod := range pods {
		if pod.Labels[helpers.K3kClusterNameLabel] != k3kClusterName {
			continue
		}
		phase := string(pod.Status.Phase)
		if phase == "" {
			phase = string(corev1.PodPending)
		}
		failureLayer := ""
		if pod.Status.Phase == corev1.PodPending {
			failureLayer = "HostSchedulingBlocked"
		}
		placement := api.WorkloadPlacement{ChildUID: pod.Annotations["kubecell.io/child-uid"], HostUID: string(pod.UID), Phase: phase, HostNode: pod.Spec.NodeName, Requests: helpers.EffectivePodRequests(&pod), FailureLayer: failureLayer, ObservedAt: metav1.Now()}
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim == nil || reader == nil {
				continue
			}
			claim, err := reader.GetPersistentVolumeClaim(ctx, namespace, volume.PersistentVolumeClaim.ClaimName)
			if err != nil || claim == nil || claim.Spec.VolumeName == "" {
				if failureLayer == "HostSchedulingBlocked" {
					placement.FailureLayer = "StorageProvisioningBlocked"
				}
				continue
			}
			pv, err := reader.GetPersistentVolume(ctx, claim.Spec.VolumeName)
			if err != nil || pv == nil {
				continue
			}
			if pvNode, ok := hostPVLocalNode(pv); ok {
				placement.PVNode = pvNode
				if placement.HostNode != "" && placement.HostNode != pvNode {
					placement.FailureLayer = "StorageLocalityBlocked"
				}
			}
		}
		placements = append(placements, placement)
	}
	sort.Slice(placements, func(i, j int) bool { return placements[i].HostUID < placements[j].HostUID })
	return placements
}

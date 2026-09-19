package host

import (
	"context"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	api "github.com/kubecell/kubecell/api/v1alpha1"
)

type BaselineObservation struct {
	NamespaceReady  bool
	K3kCRDsReady    bool
	ControllerReady bool
	CNIReady        bool
	TopoLVMReady    bool
}

type BaselineReader interface {
	ObserveBaseline(context.Context, *api.Cell) (BaselineObservation, error)
}

type BaselineInputs struct {
	K3kNamespaceExists     bool
	K3kCRDs                []extensionsv1.CustomResourceDefinition
	K3kDeployments         []apps.Deployment
	TopoLVMNamespaceExists bool
	TopoLVMDeployments     []apps.Deployment
	TopoLVMNodeDaemonSets  []apps.DaemonSet
	TopoLVMLvmdDaemonSets  []apps.DaemonSet
	TopoLVMStorageClasses  []storagev1.StorageClass
	TopoLVMCSIDrivers      []storagev1.CSIDriver
	Nodes                  []corev1.Node
}

func EvaluateBaseline(inputs BaselineInputs) BaselineObservation {
	return BaselineObservation{
		NamespaceReady:  inputs.K3kNamespaceExists,
		K3kCRDsReady:    establishedCRD(inputs.K3kCRDs, "clusters.k3k.io") && establishedCRD(inputs.K3kCRDs, "virtualclusterpolicies.k3k.io"),
		ControllerReady: availableDeployment(inputs.K3kDeployments),
		CNIReady:        networkReadyNode(inputs.Nodes),
		TopoLVMReady: inputs.TopoLVMNamespaceExists &&
			availableDeployment(inputs.TopoLVMDeployments) &&
			readyDaemonSet(inputs.TopoLVMNodeDaemonSets) &&
			readyDaemonSet(inputs.TopoLVMLvmdDaemonSets) &&
			validTopoLVMStorageClass(inputs.TopoLVMStorageClasses) &&
			validTopoLVMCSIDriver(inputs.TopoLVMCSIDrivers),
	}
}

func validTopoLVMStorageClass(classes []storagev1.StorageClass) bool {
	for _, class := range classes {
		if class.Provisioner == "topolvm.io" && class.VolumeBindingMode != nil && *class.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
			return true
		}
	}
	return false
}

func validTopoLVMCSIDriver(drivers []storagev1.CSIDriver) bool {
	for _, driver := range drivers {
		if driver.Name == "topolvm.io" {
			return true
		}
	}
	return false
}

func establishedCRD(crds []extensionsv1.CustomResourceDefinition, name string) bool {
	for _, crd := range crds {
		if crd.Name != name {
			continue
		}
		for _, condition := range crd.Status.Conditions {
			if condition.Type == extensionsv1.Established && condition.Status == extensionsv1.ConditionTrue {
				return true
			}
		}
	}
	return false
}

func availableDeployment(deployments []apps.Deployment) bool {
	for _, deployment := range deployments {
		if deployment.Spec.Replicas != nil && deployment.Status.ObservedGeneration < deployment.Generation {
			continue
		}
		if deployment.Status.AvailableReplicas >= 1 {
			return true
		}
	}
	return false
}

func readyDaemonSet(daemonSets []apps.DaemonSet) bool {
	for _, daemonSet := range daemonSets {
		if daemonSet.Status.DesiredNumberScheduled > 0 &&
			daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled &&
			daemonSet.Status.UpdatedNumberScheduled == daemonSet.Status.DesiredNumberScheduled {
			return true
		}
	}
	return false
}

func networkReadyNode(nodes []corev1.Node) bool {
	for _, node := range nodes {
		ready := false
		networkUnavailable := false
		for _, condition := range node.Status.Conditions {
			switch condition.Type {
			case corev1.NodeReady:
				ready = condition.Status == corev1.ConditionTrue
			case corev1.NodeNetworkUnavailable:
				networkUnavailable = condition.Status == corev1.ConditionTrue
			}
		}
		if ready && !networkUnavailable {
			return true
		}
	}
	return false
}

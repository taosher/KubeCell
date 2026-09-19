package host

import (
	"testing"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEvaluateBaselineRequiresEveryHostLayer(t *testing.T) {
	inputs := BaselineInputs{
		K3kNamespaceExists:     true,
		K3kCRDs:                []extensionsv1.CustomResourceDefinition{established("clusters.k3k.io"), established("virtualclusterpolicies.k3k.io")},
		K3kDeployments:         []apps.Deployment{availableDeploymentForTest("k3k")},
		TopoLVMNamespaceExists: true,
		TopoLVMDeployments:     []apps.Deployment{availableDeploymentForTest("topolvm-controller")},
		TopoLVMNodeDaemonSets:  []apps.DaemonSet{readyDS("topolvm-node")},
		TopoLVMLvmdDaemonSets:  []apps.DaemonSet{readyDS("topolvm-lvmd-0")},
		TopoLVMStorageClasses:  []storagev1.StorageClass{topolvmStorageClass()},
		TopoLVMCSIDrivers:      []storagev1.CSIDriver{{ObjectMeta: metav1.ObjectMeta{Name: "topolvm.io"}}},
		Nodes:                  []corev1.Node{readyNode("node-a")},
	}
	observation := EvaluateBaseline(inputs)
	if !observation.NamespaceReady || !observation.K3kCRDsReady || !observation.ControllerReady || !observation.CNIReady || !observation.TopoLVMReady {
		t.Fatalf("observation = %#v, want all baseline components ready", observation)
	}
	inputs.TopoLVMNodeDaemonSets[0].Status.NumberReady = 0
	observation = EvaluateBaseline(inputs)
	if observation.TopoLVMReady {
		t.Fatalf("TopoLVMReady = true with an unready node DaemonSet")
	}
	inputs.TopoLVMStorageClasses = nil
	if observation := EvaluateBaseline(inputs); observation.TopoLVMReady {
		t.Fatal("TopoLVMReady = true without the certified StorageClass")
	}
}

func topolvmStorageClass() storagev1.StorageClass {
	binding := storagev1.VolumeBindingWaitForFirstConsumer
	return storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "topolvm-provisioner"}, Provisioner: "topolvm.io", VolumeBindingMode: &binding}
}

func established(name string) extensionsv1.CustomResourceDefinition {
	return extensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: extensionsv1.CustomResourceDefinitionStatus{Conditions: []extensionsv1.CustomResourceDefinitionCondition{{Type: extensionsv1.Established, Status: extensionsv1.ConditionTrue}}}}
}

func availableDeploymentForTest(name string) apps.Deployment {
	return apps.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Generation: 1}, Spec: apps.DeploymentSpec{Replicas: int32Ptr(1)}, Status: apps.DeploymentStatus{ObservedGeneration: 1, AvailableReplicas: 1}}
}

func readyDS(name string) apps.DaemonSet {
	return apps.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: apps.DaemonSetStatus{DesiredNumberScheduled: 1, NumberReady: 1, UpdatedNumberScheduled: 1}}
}

func readyNode(name string) corev1.Node {
	return corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}
}

func int32Ptr(value int32) *int32 { return &value }

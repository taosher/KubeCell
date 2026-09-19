package helpers

func OwnedHostLabels(cellName, clusterNamespace, clusterName, clusterUID, profileHash string) map[string]string {
	return map[string]string{
		LabelManagedBy:               ManagedByKubecell,
		LabelCellName:                cellName,
		LabelVirtualClusterName:      clusterName,
		LabelVirtualClusterNamespace: clusterNamespace,
		LabelVirtualClusterUID:       clusterUID,
		LabelExecutionMode:           ExecutionModeShared,
		LabelProfileHash:             profileHash,
	}
}

const (
	AnnotationReflectedSystemCPU    = "kubecell.io/reflected-system-cpu"
	AnnotationReflectedSystemMemory = "kubecell.io/reflected-system-memory"
	AnnotationHostPathTenantName    = "kubecell.io/hostpath-tenant-name"
	AnnotationHostPathTenantRoot    = "kubecell.io/hostpath-tenant-root"
	AnnotationHostPathEnabled       = "kubecell.io/hostpath-enabled"
)

func MatchesOwnedHostLabels(labels map[string]string, cellName, clusterNamespace, clusterName, clusterUID string) bool {
	return labels[LabelManagedBy] == ManagedByKubecell &&
		labels[LabelCellName] == cellName &&
		labels[LabelVirtualClusterNamespace] == clusterNamespace &&
		labels[LabelVirtualClusterName] == clusterName &&
		labels[LabelVirtualClusterUID] == clusterUID &&
		labels[LabelExecutionMode] == ExecutionModeShared
}

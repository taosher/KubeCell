package k3k

import (
	"fmt"

	"sigs.k8s.io/yaml"
)

func RewriteKubeconfigEndpoint(input []byte, endpoint string) ([]byte, error) {
	var config map[string]any
	if err := yaml.Unmarshal(input, &config); err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	clusters, ok := config["clusters"].([]any)
	if !ok || len(clusters) == 0 {
		return nil, fmt.Errorf("kubeconfig has no clusters")
	}
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint is empty")
	}
	for index, rawCluster := range clusters {
		clusterEntry, ok := rawCluster.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("kubeconfig cluster %d is malformed", index)
		}
		clusterConfig, ok := clusterEntry["cluster"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("kubeconfig cluster %d has no cluster data", index)
		}
		clusterConfig["server"] = endpoint
	}
	output, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal kubeconfig: %w", err)
	}
	return output, nil
}

package generator

import (
	"strings"
	"testing"

	"github.com/gsid-nl/kdef/internal/types"
)

func TestGenerateDaemonSet(t *testing.T) {
	ds := types.DaemonSetConfig{
		Name:      "node-exporter",
		Namespace: "monitoring",
		Containers: []types.ContainerConfig{
			{
				Name:  "exporter",
				Image: "quay.io/prometheus/node-exporter:v1.8.0",
				Ports: []types.PortConfig{
					{Number: 9100, Name: "metrics"},
				},
			},
		},
		Service: &types.ServiceConfig{
			Ports: []types.ServicePortConfig{
				{Number: 9100, Name: "metrics", Target: 9100},
			},
		},
	}

	manifests := GenerateDaemonSet(ds)
	if len(manifests) != 2 {
		t.Fatalf("expected 2 manifests (DaemonSet+Service), got %d", len(manifests))
	}

	yamlBytes, err := RenderYAML(manifests)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	yaml := string(yamlBytes)

	if !strings.Contains(yaml, "kind: DaemonSet") {
		t.Error("yaml missing DaemonSet kind")
	}
	if !strings.Contains(yaml, "kind: Service") {
		t.Error("yaml missing Service kind")
	}
	if strings.Contains(yaml, "replicas:") {
		t.Error("DaemonSet must not have replicas")
	}
	if !strings.Contains(yaml, "quay.io/prometheus/node-exporter") {
		t.Error("yaml missing image")
	}
}

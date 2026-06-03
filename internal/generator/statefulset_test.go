package generator

import (
	"strings"
	"testing"

	"github.com/gsid-nl/kdef/internal/types"
)

func TestGenerateStatefulSet(t *testing.T) {
	sts := types.StatefulSetConfig{
		Name:        "postgres",
		Namespace:   "production",
		ServiceName: "postgres",
		Replicas:    3,
		Containers: []types.ContainerConfig{
			{
				Name:  "postgres",
				Image: "postgres:16",
				Ports: []types.PortConfig{
					{Number: 5432, Name: "pg"},
				},
				Volumes: []types.VolumeConfig{
					{Name: "data", MountPath: "/var/lib/postgresql/data"},
				},
			},
		},
		VolumeClaims: []types.VolumeClaimTemplate{
			{
				Name:         "data",
				MountPath:    "/var/lib/postgresql/data",
				Storage:      "50Gi",
				StorageClass: "fast-ssd",
			},
		},
		Service: &types.ServiceConfig{
			Ports: []types.ServicePortConfig{
				{Number: 5432, Name: "pg", Target: 5432},
			},
		},
	}

	manifests := GenerateStatefulSet(sts, nil)
	if len(manifests) != 2 {
		t.Fatalf("expected 2 manifests (StatefulSet+Service), got %d", len(manifests))
	}

	yamlBytes, err := RenderYAML(manifests)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	yaml := string(yamlBytes)

	if !strings.Contains(yaml, "kind: StatefulSet") {
		t.Error("yaml missing StatefulSet kind")
	}
	if !strings.Contains(yaml, "serviceName: postgres") {
		t.Error("yaml missing serviceName")
	}
	if !strings.Contains(yaml, "replicas: 3") {
		t.Error("yaml missing replicas: 3")
	}
	if !strings.Contains(yaml, "volumeClaimTemplates:") {
		t.Error("yaml missing volumeClaimTemplates")
	}
	if !strings.Contains(yaml, "storage: 50Gi") {
		t.Error("yaml missing storage size")
	}
	if !strings.Contains(yaml, "clusterIP: None") {
		t.Error("expected headless Service (clusterIP: None) because service_name matches")
	}
	// Pod-level Volumes should NOT re-declare "data" — it's provided by volumeClaimTemplates
	if strings.Contains(yaml, "  volumes:\n  - emptyDir: {}\n    name: data") {
		t.Error("data volume should not be in pod spec (provided by volumeClaimTemplates)")
	}
}

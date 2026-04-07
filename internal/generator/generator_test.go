package generator

import (
	"strings"
	"testing"

	"github.com/gsid-nl/kdef/internal/types"
)

func fullDeployment() types.DeploymentConfig {
	return types.DeploymentConfig{
		Name:     "timepickr-api",
		Replicas: 3,
		Containers: []types.ContainerConfig{
			{
				Name:  "api",
				Image: "registry.gsid.nl/timepickr/api:v2.4.1",
				Ports: []types.PortConfig{
					{Number: 8080, Name: "http", Health: "/health", Ready: "/ready"},
				},
				Env: []types.EnvEntry{
					{Name: "APP_ENV", Value: "production"},
					{Name: "LOG_LVL", Value: "info"},
				},
				Resources: &types.ResourcesConfig{
					CPURequest:    "200m",
					CPULimit:      "1000m",
					MemoryRequest: "256Mi",
					MemoryLimit:   "512Mi",
				},
			},
		},
		Service: &types.ServiceConfig{
			Ports: []types.ServicePortConfig{
				{Number: 8080, Name: "http", Target: 8080},
			},
		},
		Ingress: &types.IngressConfig{
			Host: "api.timepickr.net",
			TLS:  true,
		},
	}
}

func TestGenerateDeploymentV2(t *testing.T) {
	dep := fullDeployment()
	manifests := GenerateDeploymentV2(dep)

	if len(manifests) < 3 {
		t.Fatalf("expected at least 3 manifests (deployment+service+ingress), got %d", len(manifests))
	}

	yamlBytes, err := RenderYAML(manifests)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	yaml := string(yamlBytes)

	if !strings.Contains(yaml, "kind: Deployment") {
		t.Error("yaml missing Deployment")
	}
	if !strings.Contains(yaml, "kind: Service") {
		t.Error("yaml missing Service")
	}
	if !strings.Contains(yaml, "kind: Ingress") {
		t.Error("yaml missing Ingress")
	}
	if !strings.Contains(yaml, "replicas: 3") {
		t.Error("yaml missing replicas: 3")
	}
	if !strings.Contains(yaml, "registry.gsid.nl/timepickr/api:v2.4.1") {
		t.Error("yaml missing image")
	}
}

func TestGenerateDeploymentNoService(t *testing.T) {
	dep := types.DeploymentConfig{
		Name:     "worker",
		Replicas: 1,
		Containers: []types.ContainerConfig{
			{Name: "worker", Image: "nginx"},
		},
		// No Service block = no Service generated
	}
	manifests := GenerateDeploymentV2(dep)

	yamlBytes, err := RenderYAML(manifests)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	yaml := string(yamlBytes)
	if !strings.Contains(yaml, "kind: Deployment") {
		t.Error("yaml missing Deployment")
	}
	if strings.Contains(yaml, "kind: Service") {
		t.Error("yaml should not contain Service for worker-type deployment")
	}
}

package generator

import (
	"strings"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"

	"github.com/gsid-nl/kdef/internal/types"
)

func fullDeployment() types.DeploymentConfig {
	return types.DeploymentConfig{
		Name:     "my-api",
		Replicas: 3,
		Containers: []types.ContainerConfig{
			{
				Name:  "api",
				Image: "registry.example.com/my-app/api:v2.4.1",
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
		Ingresses: []types.IngressConfig{
			{
				Host: "api.example.com",
				TLS:  true,
			},
		},
	}
}

func TestGenerateDeploymentV2(t *testing.T) {
	dep := fullDeployment()
	manifests := GenerateDeploymentV2(dep, nil)

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
	if !strings.Contains(yaml, "registry.example.com/my-app/api:v2.4.1") {
		t.Error("yaml missing image")
	}
}

func TestGenerateDeploymentV2_MultipleIngresses(t *testing.T) {
	dep := types.DeploymentConfig{
		Name: "web",
		Containers: []types.ContainerConfig{
			{
				Name:  "web",
				Image: "nginx:1.27",
				Ports: []types.PortConfig{{Number: 80, Name: "http"}},
			},
		},
		Ingresses: []types.IngressConfig{
			{Host: "www.example.com", TLS: true},
			{
				Host: "example.com",
				Annotations: map[string]string{
					"nginx.ingress.kubernetes.io/permanent-redirect": "https://www.example.com$request_uri",
				},
			},
		},
	}

	manifests := GenerateDeploymentV2(dep, nil)

	var ingressNames []string
	for _, m := range manifests {
		if ing, ok := m.Object.(*networkingv1.Ingress); ok {
			ingressNames = append(ingressNames, ing.Name)
		}
	}

	if len(ingressNames) != 2 {
		t.Fatalf("expected 2 Ingress manifests, got %d (%v)", len(ingressNames), ingressNames)
	}
	if ingressNames[0] != "web" || ingressNames[1] != "web-2" {
		t.Errorf("expected ingress names [web web-2], got %v", ingressNames)
	}

	yamlBytes, err := RenderYAML(manifests)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	yaml := string(yamlBytes)
	if !strings.Contains(yaml, "permanent-redirect") {
		t.Error("redirect annotation missing from second ingress")
	}
}

func TestGeneratePersistentVolumeClaim(t *testing.T) {
	pvc := types.PersistentVolumeClaimConfig{
		Name:         "app-data",
		Namespace:    "production",
		StorageClass: "gp3",
		AccessModes:  []string{"ReadWriteOnce"},
		Storage:      "10Gi",
	}

	result := GeneratePersistentVolumeClaim(pvc)

	yamlBytes, err := RenderYAML([]Manifest{{Object: result}})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	yaml := string(yamlBytes)

	if !strings.Contains(yaml, "kind: PersistentVolumeClaim") {
		t.Error("yaml missing PersistentVolumeClaim kind")
	}
	if !strings.Contains(yaml, "name: app-data") {
		t.Error("yaml missing name")
	}
	if !strings.Contains(yaml, "namespace: production") {
		t.Error("yaml missing namespace")
	}
	if !strings.Contains(yaml, "gp3") {
		t.Error("yaml missing storage class")
	}
	if !strings.Contains(yaml, "ReadWriteOnce") {
		t.Error("yaml missing access mode")
	}
	if !strings.Contains(yaml, "10Gi") {
		t.Error("yaml missing storage size")
	}
}

func TestGeneratePersistentVolumeClaimDefaults(t *testing.T) {
	pvc := types.PersistentVolumeClaimConfig{
		Name:    "minimal",
		Storage: "5Gi",
	}

	result := GeneratePersistentVolumeClaim(pvc)

	if len(result.Spec.AccessModes) != 1 || result.Spec.AccessModes[0] != "ReadWriteOnce" {
		t.Errorf("expected default access mode ReadWriteOnce, got %v", result.Spec.AccessModes)
	}
	if result.Spec.StorageClassName != nil {
		t.Error("storage class should be nil when not specified")
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
	manifests := GenerateDeploymentV2(dep, nil)

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

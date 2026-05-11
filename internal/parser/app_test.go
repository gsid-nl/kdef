package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

func testEvalContext() *hcl.EvalContext {
	return &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"var": cty.ObjectVal(map[string]cty.Value{
				"image_tag":   cty.StringVal("v2.4.1"),
				"environment": cty.StringVal("production"),
			}),
		},
	}
}

func TestParseDeploymentBlock(t *testing.T) {
	src := `
deployment "my-api" {
  namespace        = "my-app"
  image_pull_secrets = ["regcred"]

  container "api" {
    image = "registry.example.com/my-app/api:${var.image_tag}"
    image_pull_policy = "Always"

    port "8080" "http" {
      health = "/health"
      ready  = "/ready"
    }

    env {
      APP_ENV = var.environment
      LOG_LVL = "info"
    }

    resources {
      cpu    = "200m..1000m"
      memory = "256Mi..512Mi"
    }
  }

  scale {
    replicas = 3
  }

  service {}

  ingress {
    host = "api.example.com"
    tls  = true
  }
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "api.kdef")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	result, diags := ParseFile(path, testEvalContext())
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}

	if len(result.Deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(result.Deployments))
	}

	dep := result.Deployments[0]

	if dep.Name != "my-api" {
		t.Errorf("name: expected 'my-api', got %q", dep.Name)
	}
	if dep.Namespace != "my-app" {
		t.Errorf("namespace: expected 'my-app', got %q", dep.Namespace)
	}
	if dep.Replicas != 3 {
		t.Errorf("replicas: expected 3, got %d", dep.Replicas)
	}
	if len(dep.Containers) != 1 {
		t.Fatalf("containers: expected 1, got %d", len(dep.Containers))
	}

	c := dep.Containers[0]
	if c.Image != "registry.example.com/my-app/api:v2.4.1" {
		t.Errorf("image: expected interpolated tag v2.4.1, got %q", c.Image)
	}
	if len(c.Ports) != 1 || c.Ports[0].Number != 8080 {
		t.Errorf("ports: expected [8080]")
	}

	foundEnv := false
	for _, e := range c.Env {
		if e.Name == "APP_ENV" && e.Value == "production" {
			foundEnv = true
		}
	}
	if !foundEnv {
		t.Error("env: missing APP_ENV=production")
	}

	if len(dep.Ingresses) == 0 {
		t.Fatal("ingress: expected at least one ingress block")
	}
	if dep.Ingresses[0].Host != "api.example.com" {
		t.Errorf("ingress host: expected 'api.example.com', got %q", dep.Ingresses[0].Host)
	}
}

func TestParseMinimalDeployment(t *testing.T) {
	src := `
deployment "simple" {
  container "simple" {
    image = "nginx:latest"
  }
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "simple.kdef")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	result, diags := ParseFile(path, &hcl.EvalContext{})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}

	if len(result.Deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(result.Deployments))
	}

	dep := result.Deployments[0]
	if dep.Name != "simple" {
		t.Errorf("name: expected 'simple', got %q", dep.Name)
	}
	if dep.Replicas != 1 {
		t.Errorf("replicas: expected default 1, got %d", dep.Replicas)
	}
	if len(dep.Containers) != 1 {
		t.Fatalf("containers: expected 1, got %d", len(dep.Containers))
	}
	if dep.Containers[0].Image != "nginx:latest" {
		t.Errorf("image: expected 'nginx:latest', got %q", dep.Containers[0].Image)
	}
}

func TestParseCronJob(t *testing.T) {
	src := `
cronjob "send-reminders" {
  schedule    = "*/5 * * * *"
  image       = "registry.example.com/my-app/api:latest"
  command     = ["php", "bin/console", "app:send-reminders"]
  concurrency = "Forbid"
  deadline    = "4m"
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "crons.kdef")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	result, diags := ParseFile(path, &hcl.EvalContext{})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}

	if len(result.CronJobs) != 1 {
		t.Fatalf("expected 1 cronjob, got %d", len(result.CronJobs))
	}

	cj := result.CronJobs[0]
	if cj.Name != "send-reminders" {
		t.Errorf("name: expected 'send-reminders', got %q", cj.Name)
	}
	if cj.Schedule != "*/5 * * * *" {
		t.Errorf("schedule: expected '*/5 * * * *', got %q", cj.Schedule)
	}
	if cj.Concurrency != "Forbid" {
		t.Errorf("concurrency: expected 'Forbid', got %q", cj.Concurrency)
	}
}

func TestParseMixedFile(t *testing.T) {
	src := `
deployment "web" {
  container "web" {
    image = "nginx:latest"
  }
  service {}
}

cronjob "cleanup" {
  schedule = "0 0 * * *"
  image    = "nginx:latest"
  command  = ["cleanup"]
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "all.kdef")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	result, diags := ParseFile(path, &hcl.EvalContext{})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}

	if len(result.Deployments) != 1 {
		t.Errorf("deployments: expected 1, got %d", len(result.Deployments))
	}
	if len(result.CronJobs) != 1 {
		t.Errorf("cronjobs: expected 1, got %d", len(result.CronJobs))
	}
}

func TestParsePersistentVolumeClaimBlock(t *testing.T) {
	src := `
persistentvolumeclaim "app-data" {
  namespace     = "production"
  storage_class = "gp3"
  access_modes  = ["ReadWriteOnce"]
  storage       = "10Gi"
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "pvc.kdef")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	result, diags := ParseFile(path, &hcl.EvalContext{})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}

	if len(result.PersistentVolumeClaims) != 1 {
		t.Fatalf("expected 1 PVC, got %d", len(result.PersistentVolumeClaims))
	}

	pvc := result.PersistentVolumeClaims[0]
	if pvc.Name != "app-data" {
		t.Errorf("name = %q, want %q", pvc.Name, "app-data")
	}
	if pvc.Namespace != "production" {
		t.Errorf("namespace = %q, want %q", pvc.Namespace, "production")
	}
	if pvc.StorageClass != "gp3" {
		t.Errorf("storage_class = %q, want %q", pvc.StorageClass, "gp3")
	}
	if len(pvc.AccessModes) != 1 || pvc.AccessModes[0] != "ReadWriteOnce" {
		t.Errorf("access_modes = %v, want [ReadWriteOnce]", pvc.AccessModes)
	}
	if pvc.Storage != "10Gi" {
		t.Errorf("storage = %q, want %q", pvc.Storage, "10Gi")
	}
}

func TestParsePersistentVolumeClaimDefaults(t *testing.T) {
	src := `
persistentvolumeclaim "minimal" {
  storage = "5Gi"
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "pvc.kdef")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	result, diags := ParseFile(path, &hcl.EvalContext{})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Error())
	}

	if len(result.PersistentVolumeClaims) != 1 {
		t.Fatalf("expected 1 PVC, got %d", len(result.PersistentVolumeClaims))
	}

	pvc := result.PersistentVolumeClaims[0]
	if pvc.Name != "minimal" {
		t.Errorf("name = %q, want %q", pvc.Name, "minimal")
	}
	if pvc.StorageClass != "" {
		t.Errorf("storage_class should be empty, got %q", pvc.StorageClass)
	}
	if len(pvc.AccessModes) != 0 {
		t.Errorf("access_modes should be empty, got %v", pvc.AccessModes)
	}
	if pvc.Storage != "5Gi" {
		t.Errorf("storage = %q, want %q", pvc.Storage, "5Gi")
	}
}

func TestParseResourceRange(t *testing.T) {
	tests := []struct {
		input   string
		wantReq string
		wantLim string
	}{
		{"200m..1000m", "200m", "1000m"},
		{"256Mi..512Mi", "256Mi", "512Mi"},
		{"500m", "500m", "500m"},
	}

	for _, tt := range tests {
		req, lim := parseResourceRange(tt.input)
		if req != tt.wantReq || lim != tt.wantLim {
			t.Errorf("parseResourceRange(%q) = (%q, %q), want (%q, %q)",
				tt.input, req, lim, tt.wantReq, tt.wantLim)
		}
	}
}

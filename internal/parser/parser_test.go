package parser

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func projectRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

func TestLoadSimple(t *testing.T) {
	dir := filepath.Join(projectRoot(), "testdata", "simple")
	config, err := Load(dir, nil)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if len(config.Deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(config.Deployments))
	}

	dep := config.Deployments[0]
	if dep.Name != "simple-api" {
		t.Errorf("name: expected simple-api, got %q", dep.Name)
	}
	if len(dep.Containers) != 1 || dep.Containers[0].Image != "nginx:latest" {
		t.Errorf("image: expected nginx:latest")
	}
}

func TestLoadRootKdef(t *testing.T) {
	dir := filepath.Join(projectRoot(), "testdata", "with-root")
	config, err := Load(dir, nil)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	// Namespaces
	if len(config.Namespaces) != 1 || config.Namespaces[0] != "production" {
		t.Errorf("namespaces = %v, want [production]", config.Namespaces)
	}

	// ServiceAccounts
	if len(config.ServiceAccounts) != 1 {
		t.Fatalf("expected 1 service account, got %d", len(config.ServiceAccounts))
	}
	if config.ServiceAccounts[0].Name != "default" {
		t.Errorf("service account name = %q, want %q", config.ServiceAccounts[0].Name, "default")
	}
	if len(config.ServiceAccounts[0].ImagePullSecrets) != 1 || config.ServiceAccounts[0].ImagePullSecrets[0] != "registrykey" {
		t.Errorf("image pull secrets = %v, want [registrykey]", config.ServiceAccounts[0].ImagePullSecrets)
	}

	// Deployments
	if len(config.Deployments) != 2 {
		t.Fatalf("expected 2 deployments, got %d", len(config.Deployments))
	}

	for _, dep := range config.Deployments {
		if dep.Namespace != "production" {
			t.Errorf("deployment %q: namespace = %q, want %q", dep.Name, dep.Namespace, "production")
		}
		if dep.ServiceAccountName != "default" {
			t.Errorf("deployment %q: service_account = %q, want %q", dep.Name, dep.ServiceAccountName, "default")
		}
	}

	// ConfigMaps should also get namespace injected
	if len(config.ConfigMaps) != 1 {
		t.Fatalf("expected 1 configmap, got %d", len(config.ConfigMaps))
	}
	if config.ConfigMaps[0].Namespace != "production" {
		t.Errorf("configmap namespace = %q, want %q", config.ConfigMaps[0].Namespace, "production")
	}
}

func TestLoadRootKdef_ScopedServiceAccounts(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "mon"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "root.kdef"), []byte(`
namespaces = ["production", "monitoring"]

service_account "default" {
  image_pull_secrets = ["shared"]
}
service_account "default" {
  namespace          = "monitoring"
  image_pull_secrets = ["mon-reg"]
}

deployments = {
  "app" = {
    path            = "app"
    namespace       = "production"
    service_account = "default"
  }
  "mon" = {
    path            = "mon"
    namespace       = "monitoring"
    service_account = "default"
  }
}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app", "app.kdef"), []byte(`
deployment "app" {
  container "app" {
    image = "nginx"
  }
}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mon", "mon.kdef"), []byte(`
deployment "mon" {
  container "mon" {
    image = "nginx"
  }
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	config, err := Load(dir, nil)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if len(config.ServiceAccounts) != 2 {
		t.Fatalf("expected 2 service_account entries, got %d", len(config.ServiceAccounts))
	}
}

func TestLoadRootKdef_DuplicateServiceAccountRejected(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "root.kdef"), []byte(`
namespaces = ["production"]

service_account "default" {
  image_pull_secrets = ["a"]
}
service_account "default" {
  image_pull_secrets = ["b"]
}

deployments = {
  "app" = { path = "app", namespace = "production" }
}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app", "app.kdef"), []byte(`
deployment "app" {
  container "app" {
    image = "nginx"
  }
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir, nil)
	if err == nil {
		t.Fatal("expected error for duplicate service_account")
	}
	if !strings.Contains(err.Error(), "Duplicate service_account") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadRootKdefValidationError(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "app")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// root.kdef with namespace restriction
	if err := os.WriteFile(filepath.Join(dir, "root.kdef"), []byte(`
namespaces = ["production"]
deployments = {
  "app" = {
    path = "app"
    namespace = "staging"
  }
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(subDir, "app.kdef"), []byte(`
deployment "app" {
  container "app" { image = "nginx" }
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir, nil)
	if err == nil {
		t.Fatal("expected validation error for wrong namespace")
	}
	if !strings.Contains(err.Error(), "not in root.kdef namespaces list") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadRootKdefMissingNamespace(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "app")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "root.kdef"), []byte(`
namespaces = ["production"]
deployments = {
  "app" = {
    path = "app"
  }
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(subDir, "app.kdef"), []byte(`
deployment "app" {
  container "app" { image = "nginx" }
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir, nil)
	if err == nil {
		t.Fatal("expected validation error for missing namespace")
	}
	if !strings.Contains(err.Error(), "has no namespace") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadWithOverrides(t *testing.T) {
	dir := filepath.Join(projectRoot(), "testdata", "with-ingress")
	overrides := map[string]string{"image_tag": "v1.0.0"}

	config, err := Load(dir, overrides)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if len(config.Deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(config.Deployments))
	}

	dep := config.Deployments[0]
	if dep.Containers[0].Image != "registry.example.com/acme/api:v1.0.0" {
		t.Errorf("image: expected v1.0.0 override, got %q", dep.Containers[0].Image)
	}
	if dep.Ingress == nil {
		t.Fatal("expected ingress block")
	}
	if dep.Ingress.Host != "api.acme.dev" {
		t.Errorf("ingress host: expected api.acme.dev, got %q", dep.Ingress.Host)
	}
}

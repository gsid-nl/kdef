package parser

import (
	"path/filepath"
	"runtime"
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

	if len(config.Deployments) != 2 {
		t.Fatalf("expected 2 deployments, got %d", len(config.Deployments))
	}

	names := map[string]bool{}
	for _, dep := range config.Deployments {
		names[dep.Name] = true
	}
	if !names["frontend"] {
		t.Error("missing deployment: frontend")
	}
	if !names["backend"] {
		t.Error("missing deployment: backend")
	}

	if len(config.ConfigMaps) != 1 {
		t.Fatalf("expected 1 configmap, got %d", len(config.ConfigMaps))
	}
	if config.ConfigMaps[0].Name != "backend-config" {
		t.Errorf("configmap name = %q, want %q", config.ConfigMaps[0].Name, "backend-config")
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

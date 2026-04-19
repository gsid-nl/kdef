package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanImagesWalk_ThreeLevels(t *testing.T) {
	root := t.TempDir()
	mid := filepath.Join(root, "monitoring")
	leaf := filepath.Join(mid, "node-exporter")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(root, "images.kdef"), `images {
  grafana       = "grafana:root"
  node-exporter = "node-exporter:root"
}`)
	writeFile(t, filepath.Join(mid, "extras.kdef"), `images {
  node-exporter = "node-exporter:mid"
  prometheus    = "prometheus:mid"
}`)
	writeFile(t, filepath.Join(leaf, "override.kdef"), `images {
  node-exporter = "node-exporter:leaf"
}`)

	got, err := ScanImagesWalk(leaf, root)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"grafana":       "grafana:root",
		"node-exporter": "node-exporter:leaf",
		"prometheus":    "prometheus:mid",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d images, want %d: %+v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("image %q: got %q, want %q", k, got[k], v)
		}
	}
}

func TestScanImagesWalk_SiblingNotVisible(t *testing.T) {
	root := t.TempDir()
	sibA := filepath.Join(root, "alloy")
	sibB := filepath.Join(root, "monitoring", "node-exporter")
	if err := os.MkdirAll(sibA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibB, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(root, "images.kdef"), `images {
  shared = "shared:root"
}`)
	writeFile(t, filepath.Join(filepath.Dir(sibB), "monitoring-only.kdef"), `images {
  node-exporter = "node-exporter:monitoring"
}`)

	alloyImages, err := ScanImagesWalk(sibA, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, leaked := alloyImages["node-exporter"]; leaked {
		t.Errorf("node-exporter leaked from monitoring/ into alloy/: %+v", alloyImages)
	}
	if alloyImages["shared"] != "shared:root" {
		t.Errorf("alloy missing shared from root: %+v", alloyImages)
	}

	neImages, err := ScanImagesWalk(sibB, root)
	if err != nil {
		t.Fatal(err)
	}
	if neImages["node-exporter"] != "node-exporter:monitoring" {
		t.Errorf("node-exporter not found via monitoring/: %+v", neImages)
	}
}

func TestScanImagesWalk_NoRootDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "images.kdef"), `images {
  only = "only:local"
}`)

	got, err := ScanImagesWalk(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if got["only"] != "only:local" {
		t.Errorf("expected local-only scan: %+v", got)
	}
}

func TestLoadVariablesWalk_ThreeLevels(t *testing.T) {
	root := t.TempDir()
	mid := filepath.Join(root, "monitoring")
	leaf := filepath.Join(mid, "node-exporter")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(root, "vars.kdef"), `variable "shared" {
  type    = "string"
  default = "from-root"
}
variable "leaf_wins" {
  type    = "string"
  default = "root"
}`)
	writeFile(t, filepath.Join(mid, "vars.kdef"), `variable "leaf_wins" {
  type    = "string"
  default = "mid"
}
variable "mid_only" {
  type    = "string"
  default = "mid"
}`)
	writeFile(t, filepath.Join(leaf, "vars.kdef"), `variable "leaf_wins" {
  type    = "string"
  default = "leaf"
}`)

	loaded, diags := LoadVariablesWalk(leaf, root, nil)
	if diags.HasErrors() {
		t.Fatalf("diags: %v", diags)
	}

	if v := loaded.Variables["shared"]; v.Default.AsString() != "from-root" {
		t.Errorf("shared: %v", v.Default)
	}
	if v := loaded.Variables["leaf_wins"]; v.Default.AsString() != "leaf" {
		t.Errorf("leaf_wins: got %v, want leaf", v.Default)
	}
	if v := loaded.Variables["mid_only"]; v.Default.AsString() != "mid" {
		t.Errorf("mid_only: %v", v.Default)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

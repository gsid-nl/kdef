package parser

import (
	"testing"
)

const daemonsetSrc = `
daemonset "node-exporter" {
  namespace = "monitoring"

  container "exporter" {
    image = "quay.io/prometheus/node-exporter:v1.8.0"

    port "9100" "metrics" {
      tcp_ready = true
    }
  }

  service {
    port "9100" "metrics" {}
  }
}
`

const statefulsetSrc = `
statefulset "postgres" {
  namespace    = "production"
  service_name = "postgres"

  scale {
    replicas = 3
  }

  container "postgres" {
    image = "postgres:16"

    port "5432" "pg" {
      tcp_ready = true
    }

    volume "data" {
      mount_path = "/var/lib/postgresql/data"
    }
  }

  volume_claim "data" {
    mount_path    = "/var/lib/postgresql/data"
    storage       = "50Gi"
    storage_class = "fast-ssd"
  }

  service {
    port "5432" "pg" {}
  }
}
`

func TestParseDaemonSet(t *testing.T) {
	result, diags := ParseBytes([]byte(daemonsetSrc), "ds.kdef", nil)
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}
	if len(result.DaemonSets) != 1 {
		t.Fatalf("expected 1 daemonset, got %d", len(result.DaemonSets))
	}
	ds := result.DaemonSets[0]
	if ds.Name != "node-exporter" {
		t.Errorf("name: got %q, want node-exporter", ds.Name)
	}
	if ds.Namespace != "monitoring" {
		t.Errorf("namespace: got %q, want monitoring", ds.Namespace)
	}
	if len(ds.Containers) != 1 || ds.Containers[0].Image != "quay.io/prometheus/node-exporter:v1.8.0" {
		t.Errorf("container image mismatch")
	}
	if ds.Service == nil {
		t.Error("expected service block to be parsed")
	}
}

func TestParseStatefulSet(t *testing.T) {
	result, diags := ParseBytes([]byte(statefulsetSrc), "sts.kdef", nil)
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}
	if len(result.StatefulSets) != 1 {
		t.Fatalf("expected 1 statefulset, got %d", len(result.StatefulSets))
	}
	sts := result.StatefulSets[0]
	if sts.ServiceName != "postgres" {
		t.Errorf("service_name: got %q, want postgres", sts.ServiceName)
	}
	if sts.Replicas != 3 {
		t.Errorf("replicas: got %d, want 3", sts.Replicas)
	}
	if len(sts.VolumeClaims) != 1 {
		t.Fatalf("expected 1 volume_claim, got %d", len(sts.VolumeClaims))
	}
	vc := sts.VolumeClaims[0]
	if vc.Storage != "50Gi" || vc.StorageClass != "fast-ssd" || vc.MountPath != "/var/lib/postgresql/data" {
		t.Errorf("volume_claim fields wrong: %+v", vc)
	}
}

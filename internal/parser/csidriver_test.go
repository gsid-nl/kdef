package parser

import (
	"strings"
	"testing"
)

const csiSrc = `
csidriver "storage.example.com" {
  lifecycle_modes = ["Ephemeral"]
  fs_group_policy = "None"
}

daemonset "csi" {
  namespace = "files"

  container "plugin" {
    image = "registry.example.com/files:1"

    volume "pods" {
      mount_path        = "/var/lib/kubelet/pods"
      host_path         = "/var/lib/kubelet/pods"
      mount_propagation = "Bidirectional"
    }
  }
}

deployment "cms" {
  namespace = "cms"

  container "php" {
    image = "registry.example.com/cms:1"

    volume "data" {
      mount_path        = "/data/cms"
      csi_driver        = "storage.example.com"
      csi_attributes    = { bucket = "shop", uid = "1000" }
      csi_secret        = "files-shop"
      mount_propagation = "HostToContainer"
    }
  }
}
`

func TestParseCSIDriverAndVolumes(t *testing.T) {
	result, diags := ParseBytes([]byte(csiSrc), "csi.kdef", nil)
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}
	if len(result.CSIDrivers) != 1 || result.CSIDrivers[0].LifecycleModes[0] != "Ephemeral" || result.CSIDrivers[0].AttachRequired {
		t.Fatalf("csidriver: %+v", result.CSIDrivers)
	}
	pods := result.DaemonSets[0].Containers[0].Volumes[0]
	if pods.MountPropagation != "Bidirectional" {
		t.Errorf("daemonset volume: %+v", pods)
	}
	data := result.Deployments[0].Containers[0].Volumes[0]
	if data.CSIDriver != "storage.example.com" || data.CSIAttributes["bucket"] != "shop" || data.CSISecret != "files-shop" || data.MountPropagation != "HostToContainer" {
		t.Errorf("csi volume: %+v", data)
	}
}

func TestParseCSIErrors(t *testing.T) {
	for src, want := range map[string]string{
		"csidriver \"x.example.com\" {\n  lifecycle_modes = [\"Forever\"]\n}":                                                                                                                      "Invalid lifecycle mode",
		"deployment \"a\" {\n  container \"c\" {\n    image = \"i\"\n    volume \"v\" {\n      mount_path = \"/x\"\n      csi_secret = \"s\"\n    }\n  }\n}":                                       "csi_driver required",
		"deployment \"a\" {\n  container \"c\" {\n    image = \"i\"\n    volume \"v\" {\n      mount_path = \"/x\"\n      host_path = \"/x\"\n      mount_propagation = \"Shared\"\n    }\n  }\n}": "Invalid mount_propagation",
	} {
		_, diags := ParseBytes([]byte(src), "x.kdef", nil)
		if !diags.HasErrors() || !strings.Contains(diags.Error(), want) {
			t.Errorf("want %q, got %v", want, diags)
		}
	}
}

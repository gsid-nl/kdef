package parser

import (
	"strings"
	"testing"
)

func devicesSrc(resources, podSecurity string) string {
	return `
deployment "llm" {
  namespace = "ai"

  container "llm" {
    image = "llama"

    resources {
      cpu = "2..8"
` + resources + `
    }
  }

  security_context {
    fs_group = 1000
` + podSecurity + `
  }
}
`
}

func TestParseDevicesAndSupplementalGroups(t *testing.T) {
	src := devicesSrc(`      devices = { "squat.ai/dri" = 1, "nvidia.com/gpu" = 2 }`, `    supplemental_groups = [992, 44]`)
	result, diags := ParseBytes([]byte(src), "llm.kdef", nil)
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}
	dep := result.Deployments[0]
	devices := dep.Containers[0].Resources.Devices
	if devices["squat.ai/dri"] != 1 || devices["nvidia.com/gpu"] != 2 || len(devices) != 2 {
		t.Errorf("Devices: got %v", devices)
	}
	groups := dep.SecurityContext.SupplementalGroups
	if len(groups) != 2 || groups[0] != 992 || groups[1] != 44 {
		t.Errorf("SupplementalGroups: got %v, want [992 44]", groups)
	}
	if dep.SecurityContext.FSGroup == nil || *dep.SecurityContext.FSGroup != 1000 {
		t.Errorf("FSGroup: got %v, want 1000", dep.SecurityContext.FSGroup)
	}
}

func TestParseDevicesInvalid(t *testing.T) {
	for name, devices := range map[string]string{
		"no domain":    `{ "gpu" = 1 }`,
		"zero":         `{ "squat.ai/dri" = 0 }`,
		"fraction":     `{ "squat.ai/dri" = 1.5 }`,
		"text count":   `{ "squat.ai/dri" = "1" }`,
		"not a map":    `["squat.ai/dri"]`,
		"plain string": `"squat.ai/dri"`,
	} {
		t.Run(name, func(t *testing.T) {
			src := devicesSrc(`      devices = `+devices, "")
			_, diags := ParseBytes([]byte(src), "llm.kdef", nil)
			if !diags.HasErrors() || !strings.Contains(diags.Error(), "devices") {
				t.Fatalf("expected a devices error, got %v", diags)
			}
		})
	}
}

func TestParseSupplementalGroupsInvalid(t *testing.T) {
	for name, groups := range map[string]string{
		"negative": `[-1]`,
		"fraction": `[9.5]`,
		"text":     `["render"]`,
		"number":   `992`,
	} {
		t.Run(name, func(t *testing.T) {
			src := devicesSrc("", `    supplemental_groups = `+groups)
			_, diags := ParseBytes([]byte(src), "llm.kdef", nil)
			if !diags.HasErrors() || !strings.Contains(diags.Error(), "supplemental_groups") {
				t.Fatalf("expected a supplemental_groups error, got %v", diags)
			}
		})
	}
}

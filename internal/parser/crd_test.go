package parser

import (
	"strings"
	"testing"
)

const crdSrc = `
crd "volumes.storage.example.com" {
  kind        = "Volume"
  scope       = "Cluster"
  short_names = ["wfv"]

  version "v1alpha1" {
    status_subresource = true

    schema = {
      type = "object"
      properties = {
        spec = {
          type = "object"
          properties = {
            replicas = { type = "array", items = { type = "string" } }
          }
        }
      }
    }

    column "Replicas" {
      type      = "string"
      json_path = ".spec.replicas"
    }
  }
}
`

func TestParseCRD(t *testing.T) {
	result, diags := ParseBytes([]byte(crdSrc), "crd.kdef", nil)
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}
	if len(result.CRDs) != 1 {
		t.Fatalf("expected 1 crd, got %d", len(result.CRDs))
	}
	crd := result.CRDs[0]
	if crd.Group != "storage.example.com" || crd.Plural != "volumes" {
		t.Errorf("group/plural: got %q/%q", crd.Group, crd.Plural)
	}
	if crd.Singular != "volume" || crd.ListKind != "VolumeList" {
		t.Errorf("defaults: got singular %q, listKind %q", crd.Singular, crd.ListKind)
	}
	if crd.Scope != "Cluster" {
		t.Errorf("scope: got %q", crd.Scope)
	}
	v := crd.Versions[0]
	if !v.Served || !v.Storage || !v.StatusSubresource {
		t.Errorf("version flags: served=%v storage=%v status=%v", v.Served, v.Storage, v.StatusSubresource)
	}
	if v.Schema["type"] != "object" {
		t.Errorf("schema not decoded: %v", v.Schema)
	}
	if len(v.Columns) != 1 || v.Columns[0].JSONPath != ".spec.replicas" {
		t.Errorf("columns: %+v", v.Columns)
	}
}

func TestParseCRDSchemaFromYAMLText(t *testing.T) {
	src := `
crd "widgets.example.com" {
  kind = "Widget"
  version "v1" {
    schema = <<-EOT
      type: object
      x-kubernetes-preserve-unknown-fields: true
    EOT
  }
}
`
	result, diags := ParseBytes([]byte(src), "crd.kdef", nil)
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}
	s := result.CRDs[0].Versions[0].Schema
	if s["x-kubernetes-preserve-unknown-fields"] != true {
		t.Errorf("yaml schema not decoded: %v", s)
	}
	if result.CRDs[0].Scope != "Namespaced" {
		t.Errorf("scope default: got %q", result.CRDs[0].Scope)
	}
}

func TestParseCRDErrors(t *testing.T) {
	cases := map[string]struct {
		src  string
		want string
	}{
		"name without group": {
			src:  "crd \"volumes\" {\n  kind = \"Volume\"\n  version \"v1\" {}\n}",
			want: "Invalid CRD name",
		},
		"bad scope": {
			src:  "crd \"volumes.example.com\" {\n  kind  = \"Volume\"\n  scope = \"Global\"\n  version \"v1\" {}\n}",
			want: "Invalid CRD scope",
		},
		"no versions": {
			src:  `crd "volumes.example.com" { kind = "Volume" }`,
			want: "CRD has no versions",
		},
		"two versions, no storage": {
			src:  "crd \"volumes.example.com\" {\n  kind = \"Volume\"\n  version \"v1\" {}\n  version \"v2\" {}\n}",
			want: "exactly one storage version",
		},
		"bad column type": {
			src: `crd "volumes.example.com" {
  kind = "Volume"
  version "v1" {
    column "Age" {
      type      = "time"
      json_path = ".metadata.creationTimestamp"
    }
  }
}`,
			want: "Invalid column type",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, diags := ParseBytes([]byte(tc.src), "crd.kdef", nil)
			if !diags.HasErrors() || !strings.Contains(diags.Error(), tc.want) {
				t.Errorf("want error %q, got %v", tc.want, diags)
			}
		})
	}
}

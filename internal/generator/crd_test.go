package generator

import (
	"strings"
	"testing"

	"github.com/gsid-nl/kdef/internal/types"
)

func TestGenerateCRD(t *testing.T) {
	crd := types.CRDConfig{
		Name:       "volumes.storage.example.com",
		Group:      "storage.example.com",
		Plural:     "volumes",
		Kind:       "Volume",
		Singular:   "volume",
		ListKind:   "VolumeList",
		ShortNames: []string{"wfv"},
		Scope:      "Cluster",
		Versions: []types.CRDVersionConfig{
			{
				Name:              "v1alpha1",
				Served:            true,
				Storage:           true,
				StatusSubresource: true,
				Schema: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
				Columns: []types.CRDColumnConfig{
					{Name: "Replicas", Type: "string", JSONPath: ".spec.replicas"},
				},
			},
		},
	}

	yamlBytes, err := RenderYAML([]Manifest{{Object: GenerateCRD(crd)}})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	yaml := string(yamlBytes)

	for _, want := range []string{
		"apiVersion: apiextensions.k8s.io/v1",
		"kind: CustomResourceDefinition",
		"name: volumes.storage.example.com",
		"group: storage.example.com",
		"scope: Cluster",
		"listKind: VolumeList",
		"- wfv",
		"openAPIV3Schema:",
		"jsonPath: .spec.replicas",
		// empty objects carry meaning in a CRD and must survive cleanYAML
		"status: {}",
		"properties: {}",
		"app.kubernetes.io/managed-by: kdef",
	} {
		if !strings.Contains(yaml, want) {
			t.Errorf("yaml missing %q:\n%s", want, yaml)
		}
	}
	if strings.Contains(yaml, "namespace:") {
		t.Error("CRD is cluster-scoped and must not carry a namespace")
	}
}

func TestGenerateCRDDefaultSchema(t *testing.T) {
	crd := types.CRDConfig{
		Name: "widgets.example.com", Group: "example.com", Plural: "widgets",
		Kind: "Widget", Singular: "widget", ListKind: "WidgetList", Scope: "Namespaced",
		Versions: []types.CRDVersionConfig{{Name: "v1", Served: true, Storage: true}},
	}
	yamlBytes, err := RenderYAML([]Manifest{{Object: GenerateCRD(crd)}})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if !strings.Contains(string(yamlBytes), "x-kubernetes-preserve-unknown-fields: true") {
		t.Errorf("default schema missing:\n%s", yamlBytes)
	}
	if strings.Contains(string(yamlBytes), "subresources") {
		t.Error("subresources rendered without status_subresource")
	}
}

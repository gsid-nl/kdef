package generator

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/gsid-nl/kdef/internal/types"
)

// GenerateCRD creates a CustomResourceDefinition from a CRDConfig. It is built
// unstructured so the schema passes through exactly as written.
func GenerateCRD(crd types.CRDConfig) *unstructured.Unstructured {
	names := map[string]interface{}{
		"kind":     crd.Kind,
		"listKind": crd.ListKind,
		"plural":   crd.Plural,
		"singular": crd.Singular,
	}
	if len(crd.ShortNames) > 0 {
		names["shortNames"] = stringsToInterfaces(crd.ShortNames)
	}
	if len(crd.Categories) > 0 {
		names["categories"] = stringsToInterfaces(crd.Categories)
	}

	versions := make([]interface{}, 0, len(crd.Versions))
	for _, v := range crd.Versions {
		schema := v.Schema
		if schema == nil {
			schema = map[string]interface{}{
				"type":                                 "object",
				"x-kubernetes-preserve-unknown-fields": true,
			}
		}
		version := map[string]interface{}{
			"name":    v.Name,
			"served":  v.Served,
			"storage": v.Storage,
			"schema":  map[string]interface{}{"openAPIV3Schema": schema},
		}
		if v.StatusSubresource {
			version["subresources"] = map[string]interface{}{"status": map[string]interface{}{}}
		}
		if len(v.Columns) > 0 {
			cols := make([]interface{}, 0, len(v.Columns))
			for _, c := range v.Columns {
				col := map[string]interface{}{
					"name":     c.Name,
					"type":     c.Type,
					"jsonPath": c.JSONPath,
				}
				if c.Description != "" {
					col["description"] = c.Description
				}
				if c.Format != "" {
					col["format"] = c.Format
				}
				if c.Priority != 0 {
					col["priority"] = c.Priority
				}
				cols = append(cols, col)
			}
			version["additionalPrinterColumns"] = cols
		}
		versions = append(versions, version)
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"name": crd.Name,
			},
			"spec": map[string]interface{}{
				"group":    crd.Group,
				"names":    names,
				"scope":    crd.Scope,
				"versions": versions,
			},
		},
	}
	obj.SetLabels(kdefLabels(map[string]string{"app.kubernetes.io/name": crd.Name}))
	return obj
}

func stringsToInterfaces(in []string) []interface{} {
	out := make([]interface{}, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

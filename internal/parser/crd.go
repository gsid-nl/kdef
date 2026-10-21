package parser

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"sigs.k8s.io/yaml"

	"github.com/gsid-nl/kdef/internal/types"
)

func parseCRDBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.CRDConfig, hcl.Diagnostics) {
	crd := types.CRDConfig{
		Name:  block.Labels[0],
		Scope: "Namespaced",
	}

	var diags hcl.Diagnostics
	plural, group, ok := strings.Cut(crd.Name, ".")
	if !ok || plural == "" || group == "" || plural != strings.ToLower(plural) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid CRD name",
			Detail:   fmt.Sprintf("crd %q must be named \"<plural>.<group>\" with a lowercase plural, e.g. \"volumes.example.com\"", crd.Name),
			Subject:  block.LabelRanges[0].Ptr(),
		})
	}
	crd.Plural = plural
	crd.Group = group

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "kind", Required: true},
			{Name: "singular"},
			{Name: "list_kind"},
			{Name: "short_names"},
			{Name: "categories"},
			{Name: "scope"},
		},
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "version", LabelNames: []string{"name"}},
		},
	}

	content, moreDiags := block.Body.Content(schema)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return crd, diags
	}

	collectString := func(name string, dest *string) {
		attr, ok := content.Attributes[name]
		if !ok {
			return
		}
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			*dest = val.AsString()
		}
	}
	collectList := func(name string, dest *[]string) {
		attr, ok := content.Attributes[name]
		if !ok {
			return
		}
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() || !val.CanIterateElements() {
			return
		}
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			*dest = append(*dest, v.AsString())
		}
	}

	collectString("kind", &crd.Kind)
	collectString("singular", &crd.Singular)
	collectString("list_kind", &crd.ListKind)
	collectString("scope", &crd.Scope)
	collectList("short_names", &crd.ShortNames)
	collectList("categories", &crd.Categories)

	if crd.Singular == "" {
		crd.Singular = strings.ToLower(crd.Kind)
	}
	if crd.ListKind == "" {
		crd.ListKind = crd.Kind + "List"
	}
	if crd.Scope != "Namespaced" && crd.Scope != "Cluster" {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid CRD scope",
			Detail:   fmt.Sprintf("scope %q must be either %q or %q", crd.Scope, "Namespaced", "Cluster"),
			Subject:  content.Attributes["scope"].Range.Ptr(),
		})
	}

	// storage defaults to true only when there is a single version; with
	// several, exactly one must say so.
	storageSet := make([]bool, 0, len(content.Blocks))
	for _, b := range content.Blocks {
		v, set, moreDiags := parseCRDVersionBlock(b, ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			crd.Versions = append(crd.Versions, v)
			storageSet = append(storageSet, set)
		}
	}
	if diags.HasErrors() {
		return crd, diags
	}

	if len(crd.Versions) == 0 {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "CRD has no versions",
			Detail:   fmt.Sprintf("crd %q needs at least one version block", crd.Name),
			Subject:  block.DefRange.Ptr(),
		})
		return crd, diags
	}
	if len(crd.Versions) == 1 && !storageSet[0] {
		crd.Versions[0].Storage = true
	}
	storage := 0
	for _, v := range crd.Versions {
		if v.Storage {
			storage++
		}
	}
	if storage != 1 {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "CRD needs exactly one storage version",
			Detail:   fmt.Sprintf("crd %q has %d versions with storage = true", crd.Name, storage),
			Subject:  block.DefRange.Ptr(),
		})
	}

	return crd, diags
}

// parseCRDVersionBlock also reports whether storage was set explicitly.
func parseCRDVersionBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.CRDVersionConfig, bool, hcl.Diagnostics) {
	v := types.CRDVersionConfig{
		Name:   block.Labels[0],
		Served: true,
	}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "served"},
			{Name: "storage"},
			{Name: "schema"},
			{Name: "status_subresource"},
		},
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "column", LabelNames: []string{"name"}},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return v, false, diags
	}

	collectBool := func(name string, dest *bool) bool {
		attr, ok := content.Attributes[name]
		if !ok {
			return false
		}
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			*dest = val.True()
		}
		return true
	}

	collectBool("served", &v.Served)
	storageSet := collectBool("storage", &v.Storage)
	collectBool("status_subresource", &v.StatusSubresource)

	if attr, ok := content.Attributes["schema"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			s, err := crdSchemaValue(val)
			if err != nil {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid CRD schema",
					Detail:   err.Error(),
					Subject:  attr.Range.Ptr(),
				})
			} else {
				v.Schema = s
			}
		}
	}

	for _, b := range content.Blocks {
		col, moreDiags := parseCRDColumnBlock(b, ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			v.Columns = append(v.Columns, col)
		}
	}

	return v, storageSet, diags
}

// crdSchemaValue accepts the OpenAPI v3 schema either as YAML/JSON text
// (typically file("x.schema.yaml")) or as an HCL object.
func crdSchemaValue(val cty.Value) (map[string]interface{}, error) {
	var data []byte
	if val.Type() == cty.String {
		data = []byte(val.AsString())
	} else {
		b, err := ctyjson.SimpleJSONValue{Value: val}.MarshalJSON()
		if err != nil {
			return nil, err
		}
		data = b
	}

	var schema map[string]interface{}
	if err := yaml.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("schema must be a YAML or JSON object: %w", err)
	}
	if len(schema) == 0 {
		return nil, fmt.Errorf("schema is empty")
	}
	return schema, nil
}

var crdColumnTypes = []string{"integer", "number", "string", "boolean", "date"}

func parseCRDColumnBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.CRDColumnConfig, hcl.Diagnostics) {
	col := types.CRDColumnConfig{Name: block.Labels[0]}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "type", Required: true},
			{Name: "json_path", Required: true},
			{Name: "description"},
			{Name: "format"},
			{Name: "priority"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return col, diags
	}

	for name, dest := range map[string]*string{
		"type":        &col.Type,
		"json_path":   &col.JSONPath,
		"description": &col.Description,
		"format":      &col.Format,
	} {
		attr, ok := content.Attributes[name]
		if !ok {
			continue
		}
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			*dest = val.AsString()
		}
	}

	if attr, ok := content.Attributes["priority"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			col.Priority, _ = val.AsBigFloat().Int64()
		}
	}

	valid := false
	for _, t := range crdColumnTypes {
		if col.Type == t {
			valid = true
		}
	}
	if !valid && !diags.HasErrors() {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid column type",
			Detail:   fmt.Sprintf("column %q type %q must be one of %s", col.Name, col.Type, strings.Join(crdColumnTypes, ", ")),
			Subject:  content.Attributes["type"].Range.Ptr(),
		})
	}

	return col, diags
}

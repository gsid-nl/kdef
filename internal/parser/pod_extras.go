package parser

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/gsid-nl/kdef/internal/types"
)

// parseTolerationBlock parses a single `toleration {}` block.
func parseTolerationBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.TolerationConfig, hcl.Diagnostics) {
	t := types.TolerationConfig{}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "key"},
			{Name: "operator"},
			{Name: "value"},
			{Name: "effect"},
			{Name: "toleration_seconds"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return t, diags
	}

	if attr, ok := content.Attributes["key"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			t.Key = val.AsString()
		}
	}
	if attr, ok := content.Attributes["operator"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			t.Operator = val.AsString()
		}
	}
	if attr, ok := content.Attributes["value"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			t.Value = val.AsString()
		}
	}
	if attr, ok := content.Attributes["effect"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			t.Effect = val.AsString()
		}
	}
	if attr, ok := content.Attributes["toleration_seconds"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			n, _ := val.AsBigFloat().Int64()
			t.TolerationSeconds = &n
		}
	}

	return t, diags
}

// parseNodeSelectorAttr parses a `node_selector = { ... }` attribute value into a map.
func parseNodeSelectorAttr(attr *hcl.Attribute, ctx *hcl.EvalContext) (map[string]string, hcl.Diagnostics) {
	val, diags := attr.Expr.Value(ctx)
	if diags.HasErrors() || !val.CanIterateElements() {
		return nil, diags
	}
	result := make(map[string]string)
	for it := val.ElementIterator(); it.Next(); {
		k, v := it.Element()
		result[k.AsString()] = v.AsString()
	}
	return result, diags
}

package parser

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/gsid-nl/kdef/internal/types"
)

func parseSealedSecretBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.SealedSecretConfig, hcl.Diagnostics) {
	ss := types.SealedSecretConfig{
		Name: block.Labels[0],
		Type: "Opaque",
		Data: make(map[string]string),
	}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "namespace"},
			{Name: "type"},
			{Name: "data"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return ss, diags
	}

	if attr, ok := content.Attributes["namespace"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ss.Namespace = val.AsString()
		}
	}

	if attr, ok := content.Attributes["type"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ss.Type = val.AsString()
		}
	}

	if attr, ok := content.Attributes["data"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			for it := val.ElementIterator(); it.Next(); {
				k, v := it.Element()
				ss.Data[k.AsString()] = v.AsString()
			}
		}
	}

	return ss, diags
}

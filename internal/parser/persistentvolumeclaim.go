package parser

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/gsid-nl/kdef/internal/types"
)

func parsePersistentVolumeClaimBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.PersistentVolumeClaimConfig, hcl.Diagnostics) {
	pvc := types.PersistentVolumeClaimConfig{
		Name: block.Labels[0],
	}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "namespace"},
			{Name: "storage_class"},
			{Name: "access_modes"},
			{Name: "storage", Required: true},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return pvc, diags
	}

	if attr, ok := content.Attributes["namespace"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			pvc.Namespace = val.AsString()
		}
	}

	if attr, ok := content.Attributes["storage_class"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			pvc.StorageClass = val.AsString()
		}
	}

	if attr, ok := content.Attributes["access_modes"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				pvc.AccessModes = append(pvc.AccessModes, v.AsString())
			}
		}
	}

	storageVal, moreDiags := content.Attributes["storage"].Expr.Value(ctx)
	diags = append(diags, moreDiags...)
	if !moreDiags.HasErrors() {
		pvc.Storage = storageVal.AsString()
	}

	return pvc, diags
}

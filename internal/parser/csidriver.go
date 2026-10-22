package parser

import (
	"fmt"
	"slices"

	"github.com/hashicorp/hcl/v2"

	"github.com/gsid-nl/kdef/internal/types"
)

func parseCSIDriverBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.CSIDriverConfig, hcl.Diagnostics) {
	d := types.CSIDriverConfig{Name: block.Labels[0]}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "attach_required"},
			{Name: "pod_info_on_mount"},
			{Name: "requires_republish"},
			{Name: "fs_group_policy"},
			{Name: "lifecycle_modes"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return d, diags
	}

	for name, dest := range map[string]*bool{
		"attach_required":    &d.AttachRequired,
		"pod_info_on_mount":  &d.PodInfoOnMount,
		"requires_republish": &d.RequiresRepublish,
	} {
		if attr, ok := content.Attributes[name]; ok {
			val, moreDiags := attr.Expr.Value(ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				*dest = val.True()
			}
		}
	}

	if attr, ok := content.Attributes["fs_group_policy"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			d.FSGroupPolicy = val.AsString()
			if !slices.Contains([]string{"None", "File", "ReadWriteOnceWithFSType"}, d.FSGroupPolicy) {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid fs_group_policy",
					Detail:   fmt.Sprintf("fs_group_policy %q must be \"None\", \"File\" or \"ReadWriteOnceWithFSType\"", d.FSGroupPolicy),
					Subject:  attr.Range.Ptr(),
				})
			}
		}
	}

	if attr, ok := content.Attributes["lifecycle_modes"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				mode := v.AsString()
				if mode != "Persistent" && mode != "Ephemeral" {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Invalid lifecycle mode",
						Detail:   fmt.Sprintf("lifecycle mode %q must be \"Persistent\" or \"Ephemeral\"", mode),
						Subject:  attr.Range.Ptr(),
					})
					continue
				}
				d.LifecycleModes = append(d.LifecycleModes, mode)
			}
		}
	}

	return d, diags
}

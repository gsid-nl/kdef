package parser

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// extractIfBlocks uses PartialContent to separate "if" blocks from other content.
// It evaluates each if block's condition and, if true, returns the inner body
// for further parsing.
func extractIfBlocks(body hcl.Body, ctx *hcl.EvalContext) (hcl.Body, []hcl.Body, hcl.Diagnostics) {
	schema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "if"},
		},
	}

	content, remain, diags := body.PartialContent(schema)
	if diags.HasErrors() {
		return remain, nil, diags
	}

	var includedBodies []hcl.Body
	for _, block := range content.Blocks {
		if block.Type != "if" {
			continue
		}

		// The if block's body should have a "condition" attribute and inner blocks.
		// But to keep the syntax clean, we use the HCL native approach:
		// The condition is encoded as the first attribute "condition" in the if block body.
		ifSchema := &hcl.BodySchema{
			Attributes: []hcl.AttributeSchema{
				{Name: "condition", Required: true},
			},
		}

		ifContent, ifRemain, moreDiags := block.Body.PartialContent(ifSchema)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			continue
		}

		condVal, moreDiags := ifContent.Attributes["condition"].Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			continue
		}

		if condVal.True() {
			includedBodies = append(includedBodies, ifRemain)
		}
	}

	return remain, includedBodies, diags
}

// isHCLNativeBody checks if the body supports PartialContent (native HCL syntax).
func isHCLNativeBody(body hcl.Body) bool {
	_, ok := body.(*hclsyntax.Body)
	return ok
}

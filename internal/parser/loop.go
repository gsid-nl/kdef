package parser

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// expandForBlocks extracts "for" blocks, evaluates them by iterating over
// the collection, and returns FileResults for each iteration.
func expandForBlocks(body hcl.Body, ctx *hcl.EvalContext) (hcl.Body, []FileResult, hcl.Diagnostics) {
	schema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "for", LabelNames: []string{"iterator", "collection"}},
		},
	}

	content, remain, diags := body.PartialContent(schema)
	if diags.HasErrors() {
		return remain, nil, diags
	}

	var results []FileResult
	for _, block := range content.Blocks {
		if block.Type != "for" {
			continue
		}

		iteratorName := block.Labels[0]
		collectionExpr := block.Labels[1]

		// Resolve the collection from the eval context
		// The label is like "var.tenants" — we need to walk the context
		collectionVal, err := resolveReference(collectionExpr, ctx)
		if err != nil {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid for collection",
				Detail:   fmt.Sprintf("Cannot resolve %q: %s", collectionExpr, err),
				Subject:  block.DefRange.Ptr(),
			})
			continue
		}

		if !collectionVal.CanIterateElements() {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Collection not iterable",
				Detail:   fmt.Sprintf("%q is not a list or set", collectionExpr),
				Subject:  block.DefRange.Ptr(),
			})
			continue
		}

		// Extract inner blocks from the for block body using PartialContent
		innerSchema := &hcl.BodySchema{
			Blocks: []hcl.BlockHeaderSchema{
				{Type: "deployment", LabelNames: []string{"name"}},
				{Type: "cronjob", LabelNames: []string{"name"}},
				{Type: "configmap", LabelNames: []string{"name"}},
			},
		}

		innerContent, moreDiags := block.Body.Content(innerSchema)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			continue
		}

		// Get the source bytes from the file for re-parsing
		forBodySrc, forFilename, err := extractBlocksSource(innerContent.Blocks, block.Body)
		if err != nil {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Cannot extract for block source",
				Detail:   err.Error(),
				Subject:  block.DefRange.Ptr(),
			})
			continue
		}

		// Iterate over each element, re-parsing for each iteration
		for it := collectionVal.ElementIterator(); it.Next(); {
			_, itemVal := it.Element()

			childVars := make(map[string]cty.Value)
			for k, v := range ctx.Variables {
				childVars[k] = v
			}
			childVars[iteratorName] = itemVal

			childCtx := &hcl.EvalContext{
				Variables: childVars,
				Functions: ctx.Functions,
			}

			result, moreDiags := ParseBytes(forBodySrc, forFilename, childCtx)
			diags = append(diags, moreDiags...)
			results = append(results, result)
		}
	}

	return remain, results, diags
}

// extractBlocksSource extracts the raw source encompassing all blocks from the for body.
// It uses the DefRange of the blocks to find the source range.
func extractBlocksSource(blocks []*hcl.Block, hclBody hcl.Body) ([]byte, string, error) {
	syntaxBody, ok := hclBody.(*hclsyntax.Body)
	if !ok {
		return nil, "", fmt.Errorf("for blocks require native HCL syntax")
	}

	rng := syntaxBody.SrcRange
	filename := rng.Filename
	src, err := os.ReadFile(filename)
	if err != nil {
		return nil, "", fmt.Errorf("read source: %w", err)
	}

	start := rng.Start.Byte
	end := rng.End.Byte

	if start >= len(src) || end > len(src) {
		return nil, "", fmt.Errorf("source range out of bounds")
	}

	extracted := src[start:end]

	// The SrcRange includes the enclosing { and }. Strip them for re-parsing.
	if len(extracted) > 0 && extracted[0] == '{' {
		extracted = extracted[1:]
	}
	if len(extracted) > 0 && extracted[len(extracted)-1] == '}' {
		extracted = extracted[:len(extracted)-1]
	}

	return extracted, filename + ":for", nil
}

// resolveReference resolves a dotted reference like "var.tenants" against the eval context.
func resolveReference(ref string, ctx *hcl.EvalContext) (cty.Value, error) {
	// Split on dots: "var.tenants" → ["var", "tenants"]
	parts := splitDots(ref)
	if len(parts) == 0 {
		return cty.NilVal, fmt.Errorf("empty reference")
	}

	// Find the root variable
	rootName := parts[0]
	val, ok := findVariable(rootName, ctx)
	if !ok {
		return cty.NilVal, fmt.Errorf("variable %q not found", rootName)
	}

	// Walk the remaining attributes
	for _, attr := range parts[1:] {
		if !val.Type().IsObjectType() && !val.Type().IsMapType() {
			return cty.NilVal, fmt.Errorf("cannot access attribute %q on %s", attr, val.Type().FriendlyName())
		}
		if val.Type().IsObjectType() && !val.Type().HasAttribute(attr) {
			return cty.NilVal, fmt.Errorf("object has no attribute %q", attr)
		}
		val = val.GetAttr(attr)
	}

	return val, nil
}

func findVariable(name string, ctx *hcl.EvalContext) (cty.Value, bool) {
	if v, ok := ctx.Variables[name]; ok {
		return v, true
	}
	return cty.NilVal, false
}

func splitDots(s string) []string {
	var parts []string
	current := ""
	for _, c := range s {
		if c == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// LoadValuesFile loads a JSON values file and converts it to cty values
// that can be merged into the EvalContext.
func LoadValuesFile(filename string) (map[string]cty.Value, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read values file: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse values JSON: %w", err)
	}

	result := make(map[string]cty.Value)
	for key, value := range raw {
		ctyVal, err := jsonToCty(value)
		if err != nil {
			return nil, fmt.Errorf("convert value %q: %w", key, err)
		}
		result[key] = ctyVal
	}

	return result, nil
}

// jsonToCty converts a parsed JSON value to a cty.Value.
func jsonToCty(v interface{}) (cty.Value, error) {
	switch val := v.(type) {
	case string:
		return cty.StringVal(val), nil
	case float64:
		return cty.NumberFloatVal(val), nil
	case bool:
		return cty.BoolVal(val), nil
	case nil:
		return cty.NilVal, nil
	case []interface{}:
		if len(val) == 0 {
			return cty.EmptyTupleVal, nil
		}
		var vals []cty.Value
		for _, item := range val {
			cv, err := jsonToCty(item)
			if err != nil {
				return cty.NilVal, err
			}
			vals = append(vals, cv)
		}
		return cty.TupleVal(vals), nil
	case map[string]interface{}:
		if len(val) == 0 {
			return cty.EmptyObjectVal, nil
		}
		attrs := make(map[string]cty.Value)
		for k, item := range val {
			cv, err := jsonToCty(item)
			if err != nil {
				return cty.NilVal, err
			}
			attrs[k] = cv
		}
		return cty.ObjectVal(attrs), nil
	default:
		return cty.NilVal, fmt.Errorf("unsupported JSON type %T", v)
	}
}

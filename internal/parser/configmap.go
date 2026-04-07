package parser

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/gsid-nl/kdef/internal/types"
)

func parseConfigMapBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.ConfigMapConfig, hcl.Diagnostics) {
	cm := types.ConfigMapConfig{
		Name: block.Labels[0],
		Data: make(map[string]string),
	}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "namespace"},
			{Name: "data"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return cm, diags
	}

	if attr, ok := content.Attributes["namespace"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			cm.Namespace = val.AsString()
		}
	}

	// Data as a map: data = { "key" = "value", "file.conf" = file("path") }
	if attr, ok := content.Attributes["data"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			for it := val.ElementIterator(); it.Next(); {
				k, v := it.Element()
				cm.Data[k.AsString()] = v.AsString()
			}
		}
	}

	return cm, diags
}

// FileFunction returns a cty function that reads a file's contents as a string.
// The path is resolved relative to the given base directory.
func FileFunction(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "path", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return cty.NilVal, fmt.Errorf("read file %q: %w", path, err)
			}
			return cty.StringVal(string(data)), nil
		},
	})
}

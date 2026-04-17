package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// ScanImages scans all .kdef files in a directory for images {} blocks
// and returns a map of image name → full image reference (registry/name:tag).
func ScanImages(dir string) (map[string]string, error) {
	images := make(map[string]string)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return images, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".kdef") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		found, diags := parseImagesFromFile(path)
		if diags.HasErrors() {
			return images, diags
		}
		for k, v := range found {
			images[k] = v
		}
	}

	return images, nil
}

func parseImagesFromFile(filename string) (map[string]string, hcl.Diagnostics) {
	images := make(map[string]string)

	parser := hclparse.NewParser()
	file, diags := parser.ParseHCLFile(filename)
	if diags.HasErrors() {
		return images, diags
	}

	// Use PartialContent to extract only images blocks without failing on other blocks
	schema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "images"},
		},
	}

	content, _, diags := file.Body.PartialContent(schema)
	if diags.HasErrors() {
		return images, diags
	}

	for _, block := range content.Blocks {
		if block.Type != "images" {
			continue
		}

		attrs, moreDiags := block.Body.JustAttributes()
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			continue
		}

		for name, attr := range attrs {
			val, moreDiags := attr.Expr.Value(nil)
			diags = append(diags, moreDiags...)
			if moreDiags.HasErrors() {
				continue
			}
			if val.Type() == cty.String {
				images[name] = val.AsString()
			}
		}
	}

	return images, diags
}

// ScanImagesFromBytes parses images from raw bytes (for LSP, unsaved content).
func ScanImagesFromBytes(src []byte, filename string) map[string]string {
	images := make(map[string]string)

	p := hclparse.NewParser()
	file, diags := p.ParseHCL(src, filename)
	if diags.HasErrors() {
		return images
	}

	schema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "images"},
		},
	}

	content, _, diags := file.Body.PartialContent(schema)
	if diags.HasErrors() {
		return images
	}

	for _, block := range content.Blocks {
		if block.Type != "images" {
			continue
		}
		attrs, moreDiags := block.Body.JustAttributes()
		if moreDiags.HasErrors() {
			continue
		}
		for name, attr := range attrs {
			val, moreDiags := attr.Expr.Value(nil)
			if moreDiags.HasErrors() {
				continue
			}
			if val.Type() == cty.String {
				images[name] = val.AsString()
			}
		}
	}

	return images
}

// ImageFunction returns a cty function that resolves image names to full references.
func ImageFunction(images map[string]string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "name", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			name := args[0].AsString()
			ref, ok := images[name]
			if !ok {
				return cty.NilVal, fmt.Errorf("image %q not found; define it in an images {} block", name)
			}
			return cty.StringVal(ref), nil
		},
	})
}

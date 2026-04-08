package parser

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/gsid-nl/kdef/internal/types"
)

var enumPattern = regexp.MustCompile(`^enum\[(.+)]$`)

// VariableFileResult holds variables and import paths from a variable file.
type VariableFileResult struct {
	Variables       map[string]types.VariableDecl
	Imports         []string // resolved absolute paths
	IngressDefaults *types.IngressDefaults
}

// ParseVariableFile parses a .kdef file containing variable declarations and import statements.
func ParseVariableFile(filename string) (map[string]types.VariableDecl, hcl.Diagnostics) {
	result, diags := ParseVariableFileWithImports(filename)
	if diags.HasErrors() {
		return nil, diags
	}
	return result.Variables, diags
}

// ParseVariableFileWithImports parses variables and returns any import paths found.
func ParseVariableFileWithImports(filename string) (VariableFileResult, hcl.Diagnostics) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCLFile(filename)
	if diags.HasErrors() {
		return VariableFileResult{}, diags
	}

	return parseVariableBodyWithImports(file.Body, filename)
}

// ParseVariableBytes parses variable declarations from raw bytes (for testing).
func ParseVariableBytes(src []byte, filename string) (map[string]types.VariableDecl, hcl.Diagnostics) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCLFile(filename)
	if diags.HasErrors() {
		// Try parsing as bytes
		file, diags = parser.ParseHCL(src, filename)
		if diags.HasErrors() {
			return nil, diags
		}
	}
	return parseVariableBody(file.Body)
}

func parseVariableBody(body hcl.Body) (map[string]types.VariableDecl, hcl.Diagnostics) {
	result, diags := parseVariableBodyWithImports(body, "")
	return result.Variables, diags
}

func parseVariableBodyWithImports(body hcl.Body, sourceFilename string) (VariableFileResult, hcl.Diagnostics) {
	result := VariableFileResult{
		Variables: make(map[string]types.VariableDecl),
	}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "import"},
		},
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "variable", LabelNames: []string{"name"}},
			{Type: "ingress_defaults"},
		},
	}

	content, diags := body.Content(schema)
	if diags.HasErrors() {
		return result, diags
	}

	// Extract import paths
	if attr, ok := content.Attributes["import"]; ok {
		val, moreDiags := attr.Expr.Value(nil)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			// Can be a single string or a list of strings
			if val.Type() == cty.String {
				result.Imports = append(result.Imports, resolveImportPath(val.AsString(), sourceFilename))
			} else if val.CanIterateElements() {
				for it := val.ElementIterator(); it.Next(); {
					_, v := it.Element()
					result.Imports = append(result.Imports, resolveImportPath(v.AsString(), sourceFilename))
				}
			}
		}
	}

	vars := result.Variables

	for _, block := range content.Blocks {
		switch block.Type {
		case "variable":
			varDecl, moreDiags := parseVariableBlock(block)
			diags = append(diags, moreDiags...)
			if moreDiags.HasErrors() {
				continue
			}
			vars[varDecl.Name] = varDecl
		case "ingress_defaults":
			defaults, moreDiags := parseIngressDefaults(block)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				result.IngressDefaults = defaults
			}
		}
	}

	return result, diags
}

func parseIngressDefaults(block *hcl.Block) (*types.IngressDefaults, hcl.Diagnostics) {
	defaults := &types.IngressDefaults{}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "tls"},
			{Name: "tls_secret"},
			{Name: "issuer"},
			{Name: "annotations"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return defaults, diags
	}

	// Annotations as a map: annotations = { "key" = "value" }
	if attr, ok := content.Attributes["annotations"]; ok {
		val, moreDiags := attr.Expr.Value(nil)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			defaults.Annotations = flattenAnnotations("", val)
		}
	}

	if attr, ok := content.Attributes["tls"]; ok {
		val, moreDiags := attr.Expr.Value(nil)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			b := val.True()
			defaults.TLS = &b
		}
	}

	if attr, ok := content.Attributes["tls_secret"]; ok {
		val, moreDiags := attr.Expr.Value(nil)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			defaults.TLSSecret = val.AsString()
		}
	}

	if attr, ok := content.Attributes["issuer"]; ok {
		val, moreDiags := attr.Expr.Value(nil)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			defaults.Issuer = val.AsString()
		}
	}

	return defaults, diags
}

// resolveImportPath resolves an import path relative to the source file's directory.
func resolveImportPath(importPath string, sourceFilename string) string {
	if filepath.IsAbs(importPath) {
		return importPath
	}
	if sourceFilename == "" {
		return importPath
	}
	return filepath.Join(filepath.Dir(sourceFilename), importPath)
}

func parseVariableBlock(block *hcl.Block) (types.VariableDecl, hcl.Diagnostics) {
	varDecl := types.VariableDecl{
		Name:     block.Labels[0],
		Required: true,
	}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "type", Required: true},
			{Name: "default"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return varDecl, diags
	}

	// Parse type
	typeVal, moreDiags := content.Attributes["type"].Expr.Value(nil)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return varDecl, diags
	}

	typeStr := typeVal.AsString()
	ctyType, allowedValues, err := resolveVarType(typeStr)
	if err != nil {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid variable type",
			Detail:   fmt.Sprintf("Unsupported variable type %q: %s", typeStr, err),
			Subject:  content.Attributes["type"].Expr.Range().Ptr(),
		})
		return varDecl, diags
	}
	varDecl.Type = typeStr
	varDecl.CtyType = ctyType
	varDecl.AllowedValues = allowedValues

	// Parse default
	if attr, ok := content.Attributes["default"]; ok {
		defaultVal, moreDiags := attr.Expr.Value(nil)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			varDecl.Default = &defaultVal
			varDecl.Required = false
		}
	}

	return varDecl, diags
}

func resolveVarType(typeStr string) (cty.Type, []string, error) {
	switch typeStr {
	case "string":
		return cty.String, nil, nil
	case "number":
		return cty.Number, nil, nil
	case "bool":
		return cty.Bool, nil, nil
	default:
		// Check for enum[...]
		matches := enumPattern.FindStringSubmatch(typeStr)
		if matches == nil {
			return cty.NilType, nil, fmt.Errorf("unknown type %q (supported: string, number, bool, enum[...])", typeStr)
		}
		// Parse allowed values from enum["a", "b", "c"]
		raw := matches[1]
		var allowed []string
		for _, v := range strings.Split(raw, ",") {
			v = strings.TrimSpace(v)
			v = strings.Trim(v, `"'`)
			if v != "" {
				allowed = append(allowed, v)
			}
		}
		if len(allowed) == 0 {
			return cty.NilType, nil, fmt.Errorf("enum type must have at least one allowed value")
		}
		return cty.String, allowed, nil
	}
}

// BuildEvalContext creates an hcl.EvalContext from parsed variable declarations,
// optional overrides (from --set flags), additional cty values (from --values files),
// and an image registry (from images {} blocks).
func BuildEvalContext(vars map[string]types.VariableDecl, overrides map[string]string, extraValues map[string]cty.Value, images map[string]string, baseDir ...string) (*hcl.EvalContext, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	varValues := make(map[string]cty.Value)

	// Start with extra values from JSON files
	for k, v := range extraValues {
		varValues[k] = v
	}

	for name, v := range vars {
		// Check overrides first
		if override, ok := overrides[name]; ok {
			varValues[name] = cty.StringVal(override)
			continue
		}

		// Use default if available (don't overwrite extra values)
		if _, alreadySet := varValues[name]; alreadySet {
			continue
		}
		if v.Default != nil {
			varValues[name] = *v.Default
			continue
		}

		// Required variable with no value
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Missing required variable",
			Detail:   fmt.Sprintf("Variable %q has no default value and was not provided via --set or --values", name),
		})
	}

	ctx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"var": cty.ObjectVal(varValues),
		},
		Functions: map[string]function.Function{
			"secret": secretFunction(),
			"file":   FileFunction(resolveBaseDir(baseDir)),
			"image":  ImageFunction(images),
		},
	}

	return ctx, diags
}

func resolveBaseDir(dirs []string) string {
	if len(dirs) > 0 {
		return dirs[0]
	}
	return "."
}

// secretFunction returns a cty function that creates a secret reference marker.
// Usage in kdef: secret("secret-name", "key-name")
// Returns a cty object with __secret_name and __secret_key that the env parser recognizes.
func secretFunction() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "secret_name", Type: cty.String},
			{Name: "secret_key", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.Object(map[string]cty.Type{
			"__secret_name": cty.String,
			"__secret_key":  cty.String,
		})),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			return cty.ObjectVal(map[string]cty.Value{
				"__secret_name": args[0],
				"__secret_key":  args[1],
			}), nil
		},
	})
}

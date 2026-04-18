package parser

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"

	"github.com/gsid-nl/kdef/internal/types"
)

// OverrideResult holds parsed overrides from an environment file.
type OverrideResult struct {
	VarOverrides map[string]string
	AppOverrides map[string]AppOverride
}

// AppOverride holds fields that can be overridden for an app.
type AppOverride struct {
	Replicas  *int32
	Resources *types.ResourcesConfig
}

// ParseOverrideFile parses an environment override file (e.g. environments/production.kdef).
func ParseOverrideFile(filename string) (OverrideResult, hcl.Diagnostics) {
	result := OverrideResult{
		VarOverrides: make(map[string]string),
		AppOverrides: make(map[string]AppOverride),
	}

	parser := hclparse.NewParser()
	file, diags := parser.ParseHCLFile(filename)
	if diags.HasErrors() {
		return result, diags
	}

	schema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "use_vars"},
			{Type: "override", LabelNames: []string{"type", "name"}},
		},
	}

	content, moreDiags := file.Body.Content(schema)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return result, diags
	}

	for _, block := range content.Blocks {
		switch block.Type {
		case "use_vars":
			attrs, moreDiags := block.Body.JustAttributes()
			diags = append(diags, moreDiags...)
			if moreDiags.HasErrors() {
				continue
			}
			for name, attr := range attrs {
				val, moreDiags := attr.Expr.Value(nil)
				diags = append(diags, moreDiags...)
				if !moreDiags.HasErrors() {
					result.VarOverrides[name] = val.AsString()
				}
			}

		case "override":
			if block.Labels[0] != "app" {
				continue // only app overrides for now
			}
			appName := block.Labels[1]
			override, moreDiags := parseAppOverride(block)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				result.AppOverrides[appName] = override
			}
		}
	}

	return result, diags
}

func parseAppOverride(block *hcl.Block) (AppOverride, hcl.Diagnostics) {
	override := AppOverride{}

	schema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "scale"},
			{Type: "resources"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return override, diags
	}

	for _, b := range content.Blocks {
		switch b.Type {
		case "scale":
			scaleSchema := &hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{Name: "replicas", Required: true},
				},
			}
			sc, moreDiags := b.Body.Content(scaleSchema)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				val, moreDiags := sc.Attributes["replicas"].Expr.Value(nil)
				diags = append(diags, moreDiags...)
				if !moreDiags.HasErrors() {
					n, _ := val.AsBigFloat().Int64()
					r := int32(n)
					override.Replicas = &r
				}
			}

		case "resources":
			resSchema := &hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{Name: "cpu"},
					{Name: "memory"},
				},
			}
			rc, moreDiags := b.Body.Content(resSchema)
			diags = append(diags, moreDiags...)
			if moreDiags.HasErrors() {
				continue
			}

			res := &types.ResourcesConfig{}
			if attr, ok := rc.Attributes["cpu"]; ok {
				val, moreDiags := attr.Expr.Value(nil)
				diags = append(diags, moreDiags...)
				if !moreDiags.HasErrors() {
					req, lim := parseResourceRange(val.AsString())
					res.CPURequest = req
					res.CPULimit = lim
				}
			}
			if attr, ok := rc.Attributes["memory"]; ok {
				val, moreDiags := attr.Expr.Value(nil)
				diags = append(diags, moreDiags...)
				if !moreDiags.HasErrors() {
					req, lim := parseResourceRange(val.AsString())
					res.MemoryRequest = req
					res.MemoryLimit = lim
				}
			}
			override.Resources = res
		}
	}

	return override, diags
}

// ApplyOverrides applies parsed overrides to a KdefConfig in-place.
func ApplyOverrides(config *types.KdefConfig, overrides OverrideResult) {
	for i, dep := range config.Deployments {
		if o, ok := overrides.AppOverrides[dep.Name]; ok {
			if o.Replicas != nil {
				config.Deployments[i].Replicas = *o.Replicas
			}
			// Resources override would need to target specific containers
			// For now, skip resource overrides on deployments
		}
	}
	for i, sts := range config.StatefulSets {
		if o, ok := overrides.AppOverrides[sts.Name]; ok {
			if o.Replicas != nil {
				config.StatefulSets[i].Replicas = *o.Replicas
			}
		}
	}
}

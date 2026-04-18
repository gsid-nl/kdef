package parser

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/gsid-nl/kdef/internal/types"
)

func parseClusterRoleBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.ClusterRoleConfig, hcl.Diagnostics) {
	cr := types.ClusterRoleConfig{
		Name: block.Labels[0],
	}

	schema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "rule"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return cr, diags
	}

	for _, b := range content.Blocks {
		if b.Type != "rule" {
			continue
		}
		rule, moreDiags := parsePolicyRuleBlock(b, ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			cr.Rules = append(cr.Rules, rule)
		}
	}

	return cr, diags
}

func parsePolicyRuleBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.PolicyRuleConfig, hcl.Diagnostics) {
	r := types.PolicyRuleConfig{}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "api_groups"},
			{Name: "resources"},
			{Name: "resource_names"},
			{Name: "verbs", Required: true},
			{Name: "non_resource_urls"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return r, diags
	}

	collectList := func(name string, dest *[]string) {
		attr, ok := content.Attributes[name]
		if !ok {
			return
		}
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() || !val.CanIterateElements() {
			return
		}
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			*dest = append(*dest, v.AsString())
		}
	}

	collectList("api_groups", &r.APIGroups)
	collectList("resources", &r.Resources)
	collectList("resource_names", &r.ResourceNames)
	collectList("verbs", &r.Verbs)
	collectList("non_resource_urls", &r.NonResourceURLs)

	return r, diags
}

func parseClusterRoleBindingBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.ClusterRoleBindingConfig, hcl.Diagnostics) {
	crb := types.ClusterRoleBindingConfig{
		Name: block.Labels[0],
	}

	schema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "role_ref"},
			{Type: "subject"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return crb, diags
	}

	for _, b := range content.Blocks {
		switch b.Type {
		case "role_ref":
			rr, moreDiags := parseRoleRefBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				crb.RoleRef = rr
			}
		case "subject":
			s, moreDiags := parseSubjectBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				crb.Subjects = append(crb.Subjects, s)
			}
		}
	}

	return crb, diags
}

func parseRoleRefBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.RoleRefConfig, hcl.Diagnostics) {
	rr := types.RoleRefConfig{Kind: "ClusterRole"}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "kind"},
			{Name: "name", Required: true},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return rr, diags
	}

	if attr, ok := content.Attributes["kind"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			rr.Kind = val.AsString()
		}
	}

	nameVal, moreDiags := content.Attributes["name"].Expr.Value(ctx)
	diags = append(diags, moreDiags...)
	if !moreDiags.HasErrors() {
		rr.Name = nameVal.AsString()
	}

	return rr, diags
}

func parseSubjectBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.SubjectConfig, hcl.Diagnostics) {
	s := types.SubjectConfig{Kind: "ServiceAccount"}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "kind"},
			{Name: "name", Required: true},
			{Name: "namespace"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return s, diags
	}

	if attr, ok := content.Attributes["kind"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			s.Kind = val.AsString()
		}
	}

	nameVal, moreDiags := content.Attributes["name"].Expr.Value(ctx)
	diags = append(diags, moreDiags...)
	if !moreDiags.HasErrors() {
		s.Name = nameVal.AsString()
	}

	if attr, ok := content.Attributes["namespace"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			s.Namespace = val.AsString()
		}
	}

	return s, diags
}

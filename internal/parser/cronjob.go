package parser

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/gsid-nl/kdef/internal/types"
)

func parseCronJobBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.CronJobConfig, hcl.Diagnostics) {
	cj := types.CronJobConfig{
		Name:        block.Labels[0],
		Concurrency: "Allow",
	}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "image", Required: true},
			{Name: "schedule", Required: true},
			{Name: "container_name"},
			{Name: "namespace"},
			{Name: "image_pull_policy"},
			{Name: "image_pull_secrets"},
			{Name: "service_account"},
			{Name: "command"},
			{Name: "concurrency"},
			{Name: "deadline"},
			{Name: "restart"},
		},
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "env"},
			{Type: "env_from"},
			{Type: "resources"},
			{Type: "volume", LabelNames: []string{"name"}},
			{Type: "security_context"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return cj, diags
	}

	// Required attributes
	imageVal, moreDiags := content.Attributes["image"].Expr.Value(ctx)
	diags = append(diags, moreDiags...)
	if !moreDiags.HasErrors() {
		cj.Image = imageVal.AsString()
	}

	schedVal, moreDiags := content.Attributes["schedule"].Expr.Value(ctx)
	diags = append(diags, moreDiags...)
	if !moreDiags.HasErrors() {
		cj.Schedule = schedVal.AsString()
	}

	// Optional string attributes
	for _, field := range []struct {
		name string
		dest *string
	}{
		{"namespace", &cj.Namespace},
		{"image_pull_policy", &cj.ImagePullPolicy},
		{"service_account", &cj.ServiceAccountName},
		{"container_name", &cj.ContainerName},
		{"concurrency", &cj.Concurrency},
		{"deadline", &cj.Deadline},
		{"restart", &cj.Restart},
	} {
		if attr, ok := content.Attributes[field.name]; ok {
			val, moreDiags := attr.Expr.Value(ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				*field.dest = val.AsString()
			}
		}
	}

	// Command (list of strings)
	if attr, ok := content.Attributes["command"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				cj.Command = append(cj.Command, v.AsString())
			}
		}
	}

	// ImagePullSecrets (list of strings)
	if attr, ok := content.Attributes["image_pull_secrets"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				cj.ImagePullSecrets = append(cj.ImagePullSecrets, v.AsString())
			}
		}
	}

	// Blocks
	for _, b := range content.Blocks {
		switch b.Type {
		case "env":
			entries, moreDiags := parseEnvBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				cj.Env = entries
			}
		case "env_from":
			ef, moreDiags := parseEnvFromBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				cj.EnvFrom = append(cj.EnvFrom, ef)
			}
		case "resources":
			res, moreDiags := parseResourcesBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				cj.Resources = &res
			}
		case "volume":
			vol, moreDiags := parseVolumeBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				cj.Volumes = append(cj.Volumes, vol)
			}
		case "security_context":
			sc, moreDiags := parseSecurityContextBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				cj.SecurityContext = &sc
			}
		}
	}

	// Normalize concurrency
	switch cj.Concurrency {
	case "forbid", "Forbid":
		cj.Concurrency = "Forbid"
	case "replace", "Replace":
		cj.Concurrency = "Replace"
	default:
		cj.Concurrency = "Allow"
	}

	return cj, diags
}

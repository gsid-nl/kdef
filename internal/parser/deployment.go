package parser

import (
	"fmt"
	"strconv"

	"github.com/hashicorp/hcl/v2"

	"github.com/gsid-nl/kdef/internal/types"
)

func parseDeploymentBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.DeploymentConfig, hcl.Diagnostics) {
	dep := types.DeploymentConfig{
		Name:     block.Labels[0],
		Replicas: 1,
	}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "name"},
			{Name: "namespace"},
			{Name: "labels"},
			{Name: "selector"},
			{Name: "image_pull_secrets"},
			{Name: "service_account"},
			{Name: "node_selector"},
			{Name: "raw"},
		},
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "container", LabelNames: []string{"name"}},
			{Type: "init", LabelNames: []string{"name"}},
			{Type: "scale"},
			{Type: "volume", LabelNames: []string{"name"}},
			{Type: "security_context"},
			{Type: "service"},
			{Type: "ingress"},
			{Type: "autoscale"},
			{Type: "toleration"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return dep, diags
	}

	// Name override
	if attr, ok := content.Attributes["name"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			dep.Name = val.AsString()
		}
	}

	// Namespace
	if attr, ok := content.Attributes["namespace"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			dep.Namespace = val.AsString()
		}
	}

	// Labels
	if attr, ok := content.Attributes["labels"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			dep.Labels = make(map[string]string)
			for it := val.ElementIterator(); it.Next(); {
				k, v := it.Element()
				dep.Labels[k.AsString()] = v.AsString()
			}
		}
	}

	// Selector
	if attr, ok := content.Attributes["selector"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			dep.Selector = make(map[string]string)
			for it := val.ElementIterator(); it.Next(); {
				k, v := it.Element()
				dep.Selector[k.AsString()] = v.AsString()
			}
		}
	}

	// ImagePullSecrets
	if attr, ok := content.Attributes["image_pull_secrets"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				dep.ImagePullSecrets = append(dep.ImagePullSecrets, v.AsString())
			}
		}
	}

	// ServiceAccount
	if attr, ok := content.Attributes["service_account"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			dep.ServiceAccountName = val.AsString()
		}
	}

	// Raw
	if attr, ok := content.Attributes["raw"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			dep.Raw = val.AsString()
		}
	}

	// NodeSelector
	if attr, ok := content.Attributes["node_selector"]; ok {
		ns, moreDiags := parseNodeSelectorAttr(attr, ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			dep.NodeSelector = ns
		}
	}

	// Process blocks
	for _, b := range content.Blocks {
		switch b.Type {
		case "container":
			c, moreDiags := parseContainerBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				dep.Containers = append(dep.Containers, c)
			}
		case "init":
			ic, moreDiags := parseInitContainerBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				dep.InitContainers = append(dep.InitContainers, ic)
			}
		case "scale":
			scaleSchema := &hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{Name: "replicas", Required: true},
				},
			}
			sc, moreDiags := b.Body.Content(scaleSchema)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				val, moreDiags := sc.Attributes["replicas"].Expr.Value(ctx)
				diags = append(diags, moreDiags...)
				if !moreDiags.HasErrors() {
					n, _ := val.AsBigFloat().Int64()
					dep.Replicas = int32(n)
				}
			}
		case "volume":
			vol, moreDiags := parseVolumeBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				dep.Volumes = append(dep.Volumes, vol)
			}
		case "security_context":
			sc, moreDiags := parseSecurityContextBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				dep.SecurityContext = &sc
			}
		case "service":
			svc, moreDiags := parseServiceBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				dep.Service = &svc
			}
		case "ingress":
			ing, moreDiags := parseIngressBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				dep.Ingress = &ing
			}
		case "autoscale":
			as, moreDiags := parseAutoscaleBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				dep.Autoscale = &as
			}
		case "toleration":
			t, moreDiags := parseTolerationBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				dep.Tolerations = append(dep.Tolerations, t)
			}
		}
	}

	return dep, diags
}

func parseContainerBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.ContainerConfig, hcl.Diagnostics) {
	c := types.ContainerConfig{
		Name: block.Labels[0],
	}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "image", Required: true},
			{Name: "image_pull_policy"},
			{Name: "command"},
			{Name: "args"},
			{Name: "working_dir"},
		},
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "port", LabelNames: []string{"number", "name"}},
			{Type: "env"},
			{Type: "env_from"},
			{Type: "resources"},
			{Type: "volume", LabelNames: []string{"name"}},
			{Type: "security_context"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return c, diags
	}

	imageVal, moreDiags := content.Attributes["image"].Expr.Value(ctx)
	diags = append(diags, moreDiags...)
	if !moreDiags.HasErrors() {
		c.Image = imageVal.AsString()
	}

	if attr, ok := content.Attributes["image_pull_policy"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			c.ImagePullPolicy = val.AsString()
			if d := validateImagePullPolicy(c.ImagePullPolicy, attr); d != nil {
				diags = append(diags, d)
			}
		}
	}

	if attr, ok := content.Attributes["command"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				c.Command = append(c.Command, v.AsString())
			}
		}
	}

	if attr, ok := content.Attributes["args"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				c.Args = append(c.Args, v.AsString())
			}
		}
	}

	if attr, ok := content.Attributes["working_dir"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			c.WorkingDir = val.AsString()
		}
	}

	for _, b := range content.Blocks {
		switch b.Type {
		case "port":
			port, moreDiags := parsePortBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				c.Ports = append(c.Ports, port)
			}
		case "env":
			entries, moreDiags := parseEnvBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				c.Env = entries
			}
		case "env_from":
			ef, moreDiags := parseEnvFromBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				c.EnvFrom = append(c.EnvFrom, ef)
			}
		case "resources":
			res, moreDiags := parseResourcesBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				c.Resources = &res
			}
		case "volume":
			vol, moreDiags := parseVolumeBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				c.Volumes = append(c.Volumes, vol)
			}
		case "security_context":
			sc, moreDiags := parseSecurityContextBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				c.SecurityContext = &sc
			}
		}
	}

	return c, diags
}

func parseServiceBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.ServiceConfig, hcl.Diagnostics) {
	svc := types.ServiceConfig{}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "name"},
			{Name: "type"},
		},
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "port", LabelNames: []string{"number", "name"}},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return svc, diags
	}

	if attr, ok := content.Attributes["name"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			svc.Name = val.AsString()
		}
	}

	if attr, ok := content.Attributes["type"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			svc.Type = val.AsString()
		}
	}

	for _, b := range content.Blocks {
		if b.Type == "port" {
			sp, moreDiags := parseServicePortBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				svc.Ports = append(svc.Ports, sp)
			}
		}
	}

	return svc, diags
}

func parseServicePortBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.ServicePortConfig, hcl.Diagnostics) {
	sp := types.ServicePortConfig{
		Name: block.Labels[1],
	}

	num, err := strconv.Atoi(block.Labels[0])
	if err != nil {
		return sp, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Invalid port number",
			Detail:   fmt.Sprintf("Port number %q is not a valid integer", block.Labels[0]),
			Subject:  block.DefRange.Ptr(),
		}}
	}
	sp.Number = int32(num)
	sp.Target = sp.Number // default target = same as port

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "target"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return sp, diags
	}

	if attr, ok := content.Attributes["target"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			n, _ := val.AsBigFloat().Int64()
			sp.Target = int32(n)
		}
	}

	return sp, diags
}

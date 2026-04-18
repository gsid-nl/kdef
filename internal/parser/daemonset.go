package parser

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/gsid-nl/kdef/internal/types"
)

func parseDaemonSetBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.DaemonSetConfig, hcl.Diagnostics) {
	ds := types.DaemonSetConfig{
		Name: block.Labels[0],
	}

	attrs := []hcl.AttributeSchema{
		{Name: "name"},
		{Name: "namespace"},
		{Name: "labels"},
		{Name: "selector"},
		{Name: "image_pull_secrets"},
		{Name: "service_account"},
		{Name: "node_selector"},
		{Name: "raw"},
	}
	attrs = append(attrs, hostPodAttrs()...)
	schema := &hcl.BodySchema{
		Attributes: attrs,
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "container", LabelNames: []string{"name"}},
			{Type: "init", LabelNames: []string{"name"}},
			{Type: "volume", LabelNames: []string{"name"}},
			{Type: "security_context"},
			{Type: "service"},
			{Type: "toleration"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return ds, diags
	}

	if attr, ok := content.Attributes["name"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ds.Name = val.AsString()
		}
	}

	if attr, ok := content.Attributes["namespace"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ds.Namespace = val.AsString()
		}
	}

	if attr, ok := content.Attributes["labels"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			ds.Labels = make(map[string]string)
			for it := val.ElementIterator(); it.Next(); {
				k, v := it.Element()
				ds.Labels[k.AsString()] = v.AsString()
			}
		}
	}

	if attr, ok := content.Attributes["selector"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			ds.Selector = make(map[string]string)
			for it := val.ElementIterator(); it.Next(); {
				k, v := it.Element()
				ds.Selector[k.AsString()] = v.AsString()
			}
		}
	}

	if attr, ok := content.Attributes["image_pull_secrets"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				ds.ImagePullSecrets = append(ds.ImagePullSecrets, v.AsString())
			}
		}
	}

	if attr, ok := content.Attributes["service_account"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ds.ServiceAccountName = val.AsString()
		}
	}

	if attr, ok := content.Attributes["raw"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ds.Raw = val.AsString()
		}
	}

	if attr, ok := content.Attributes["node_selector"]; ok {
		ns, moreDiags := parseNodeSelectorAttr(attr, ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ds.NodeSelector = ns
		}
	}

	diags = append(diags, parseHostPodAttrs(content, ctx, &ds.HostNetwork, &ds.HostPID, &ds.HostIPC, &ds.DNSPolicy)...)

	for _, b := range content.Blocks {
		switch b.Type {
		case "container":
			c, moreDiags := parseContainerBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				ds.Containers = append(ds.Containers, c)
			}
		case "init":
			ic, moreDiags := parseInitContainerBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				ds.InitContainers = append(ds.InitContainers, ic)
			}
		case "volume":
			vol, moreDiags := parseVolumeBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				ds.Volumes = append(ds.Volumes, vol)
			}
		case "security_context":
			sc, moreDiags := parseSecurityContextBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				ds.SecurityContext = &sc
			}
		case "service":
			svc, moreDiags := parseServiceBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				ds.Service = &svc
			}
		case "toleration":
			t, moreDiags := parseTolerationBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				ds.Tolerations = append(ds.Tolerations, t)
			}
		}
	}

	return ds, diags
}

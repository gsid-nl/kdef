package parser

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/gsid-nl/kdef/internal/types"
)

func parseStatefulSetBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.StatefulSetConfig, hcl.Diagnostics) {
	sts := types.StatefulSetConfig{
		Name:     block.Labels[0],
		Replicas: 1,
	}

	attrs := []hcl.AttributeSchema{
		{Name: "name"},
		{Name: "namespace"},
		{Name: "labels"},
		{Name: "selector"},
		{Name: "image_pull_secrets"},
		{Name: "service_account"},
		{Name: "service_name"},
		{Name: "pod_management_policy"},
		{Name: "node_selector"},
		{Name: "raw"},
	}
	attrs = append(attrs, hostPodAttrs()...)
	schema := &hcl.BodySchema{
		Attributes: attrs,
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "container", LabelNames: []string{"name"}},
			{Type: "init", LabelNames: []string{"name"}},
			{Type: "scale"},
			{Type: "volume", LabelNames: []string{"name"}},
			{Type: "volume_claim", LabelNames: []string{"name"}},
			{Type: "security_context"},
			{Type: "service"},
			{Type: "ingress"},
			{Type: "toleration"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return sts, diags
	}

	if attr, ok := content.Attributes["name"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			sts.Name = val.AsString()
		}
	}

	if attr, ok := content.Attributes["namespace"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			sts.Namespace = val.AsString()
		}
	}

	if attr, ok := content.Attributes["labels"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			sts.Labels = make(map[string]string)
			for it := val.ElementIterator(); it.Next(); {
				k, v := it.Element()
				sts.Labels[k.AsString()] = v.AsString()
			}
		}
	}

	if attr, ok := content.Attributes["selector"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			sts.Selector = make(map[string]string)
			for it := val.ElementIterator(); it.Next(); {
				k, v := it.Element()
				sts.Selector[k.AsString()] = v.AsString()
			}
		}
	}

	if attr, ok := content.Attributes["image_pull_secrets"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				sts.ImagePullSecrets = append(sts.ImagePullSecrets, v.AsString())
			}
		}
	}

	if attr, ok := content.Attributes["service_account"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			sts.ServiceAccountName = val.AsString()
		}
	}

	if attr, ok := content.Attributes["service_name"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			sts.ServiceName = val.AsString()
		}
	}

	if attr, ok := content.Attributes["pod_management_policy"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			sts.PodManagementPolicy = val.AsString()
		}
	}

	if attr, ok := content.Attributes["raw"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			sts.Raw = val.AsString()
		}
	}

	if attr, ok := content.Attributes["node_selector"]; ok {
		ns, moreDiags := parseNodeSelectorAttr(attr, ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			sts.NodeSelector = ns
		}
	}

	diags = append(diags, parseHostPodAttrs(content, ctx, &sts.HostNetwork, &sts.HostPID, &sts.HostIPC, &sts.DNSPolicy)...)

	for _, b := range content.Blocks {
		switch b.Type {
		case "container":
			c, moreDiags := parseContainerBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				sts.Containers = append(sts.Containers, c)
			}
		case "init":
			ic, moreDiags := parseInitContainerBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				sts.InitContainers = append(sts.InitContainers, ic)
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
					sts.Replicas = int32(n)
				}
			}
		case "volume":
			vol, moreDiags := parseVolumeBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				sts.Volumes = append(sts.Volumes, vol)
			}
		case "volume_claim":
			vct, moreDiags := parseVolumeClaimTemplateBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				sts.VolumeClaims = append(sts.VolumeClaims, vct)
			}
		case "security_context":
			sc, moreDiags := parseSecurityContextBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				sts.SecurityContext = &sc
			}
		case "service":
			svc, moreDiags := parseServiceBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				sts.Service = &svc
			}
		case "ingress":
			ing, moreDiags := parseIngressBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				sts.Ingress = &ing
			}
		case "toleration":
			t, moreDiags := parseTolerationBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				sts.Tolerations = append(sts.Tolerations, t)
			}
		}
	}

	return sts, diags
}

func parseVolumeClaimTemplateBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.VolumeClaimTemplate, hcl.Diagnostics) {
	vct := types.VolumeClaimTemplate{
		Name: block.Labels[0],
	}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "mount_path", Required: true},
			{Name: "sub_path"},
			{Name: "read_only"},
			{Name: "storage_class"},
			{Name: "access_modes"},
			{Name: "storage", Required: true},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return vct, diags
	}

	mountVal, moreDiags := content.Attributes["mount_path"].Expr.Value(ctx)
	diags = append(diags, moreDiags...)
	if !moreDiags.HasErrors() {
		vct.MountPath = mountVal.AsString()
	}

	storageVal, moreDiags := content.Attributes["storage"].Expr.Value(ctx)
	diags = append(diags, moreDiags...)
	if !moreDiags.HasErrors() {
		vct.Storage = storageVal.AsString()
	}

	if attr, ok := content.Attributes["sub_path"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			vct.SubPath = val.AsString()
		}
	}

	if attr, ok := content.Attributes["read_only"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			vct.ReadOnly = val.True()
		}
	}

	if attr, ok := content.Attributes["storage_class"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			vct.StorageClass = val.AsString()
		}
	}

	if attr, ok := content.Attributes["access_modes"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				vct.AccessModes = append(vct.AccessModes, v.AsString())
			}
		}
	}

	return vct, diags
}

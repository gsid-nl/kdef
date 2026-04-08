package parser

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"

	"github.com/gsid-nl/kdef/internal/types"
)

// ParseFile parses a .kdef file containing app, worker, and/or cronjob blocks.
func ParseFile(filename string, ctx *hcl.EvalContext) (FileResult, hcl.Diagnostics) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCLFile(filename)
	if diags.HasErrors() {
		return FileResult{}, diags
	}
	return parseFileBody(file.Body, ctx)
}

// ParseBytes parses blocks from raw bytes (for testing).
func ParseBytes(src []byte, filename string, ctx *hcl.EvalContext) (FileResult, hcl.Diagnostics) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL(src, filename)
	if diags.HasErrors() {
		return FileResult{}, diags
	}
	return parseFileBody(file.Body, ctx)
}

// FileResult holds all parsed blocks from a single file.
type FileResult struct {
	Deployments   []types.DeploymentConfig
	CronJobs      []types.CronJobConfig
	ConfigMaps    []types.ConfigMapConfig
	SealedSecrets []types.SealedSecretConfig
}

func parseFileBody(body hcl.Body, ctx *hcl.EvalContext) (FileResult, hcl.Diagnostics) {
	var result FileResult

	// Pre-processing: extract and evaluate "for" blocks first
	remaining, forResults, diags := expandForBlocks(body, ctx)
	if diags.HasErrors() {
		return result, diags
	}
	for _, fr := range forResults {
		result.CronJobs = append(result.CronJobs, fr.CronJobs...)
		result.ConfigMaps = append(result.ConfigMaps, fr.ConfigMaps...)
		result.Deployments = append(result.Deployments, fr.Deployments...)
		result.SealedSecrets = append(result.SealedSecrets, fr.SealedSecrets...)
	}

	// Pre-processing: extract and evaluate "if" blocks
	remaining, includedBodies, moreDiags := extractIfBlocks(remaining, ctx)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return result, diags
	}

	// Parse all bodies: the remaining top-level body + any included if-block bodies
	bodies := append([]hcl.Body{remaining}, includedBodies...)

	for _, b := range bodies {
		moreDiags := parseBlocksFromBody(b, ctx, &result)
		diags = append(diags, moreDiags...)
	}

	return result, diags
}

var topLevelSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "deployment", LabelNames: []string{"name"}},
		{Type: "cronjob", LabelNames: []string{"name"}},
		{Type: "configmap", LabelNames: []string{"name"}},
		{Type: "sealedsecret", LabelNames: []string{"name"}},
		{Type: "images"},
	},
}

func parseBlocksFromBody(body hcl.Body, ctx *hcl.EvalContext, result *FileResult) hcl.Diagnostics {
	content, diags := body.Content(topLevelSchema)
	if diags.HasErrors() {
		return diags
	}

	for _, block := range content.Blocks {
		switch block.Type {
		case "deployment":
			dep, moreDiags := parseDeploymentBlock(block, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				result.Deployments = append(result.Deployments, dep)
			}
		case "cronjob":
			cj, moreDiags := parseCronJobBlock(block, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				result.CronJobs = append(result.CronJobs, cj)
			}
		case "configmap":
			cm, moreDiags := parseConfigMapBlock(block, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				result.ConfigMaps = append(result.ConfigMaps, cm)
			}
		case "sealedsecret":
			ss, moreDiags := parseSealedSecretBlock(block, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				result.SealedSecrets = append(result.SealedSecrets, ss)
			}
		}
	}

	return diags
}

// --- Shared parsing functions used by deployment, cronjob, etc. ---

func parsePortBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.PortConfig, hcl.Diagnostics) {
	port := types.PortConfig{
		Name: block.Labels[1],
	}

	// Parse port number from label
	num, err := strconv.Atoi(block.Labels[0])
	if err != nil {
		return port, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Invalid port number",
			Detail:   fmt.Sprintf("Port number %q is not a valid integer", block.Labels[0]),
			Subject:  block.DefRange.Ptr(),
		}}
	}
	port.Number = int32(num)

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "health"},
			{Name: "ready"},
			{Name: "tcp_health"},
			{Name: "tcp_ready"},
			{Name: "initial_delay"},
			{Name: "period"},
			{Name: "timeout"},
			{Name: "failures"},
			{Name: "successes"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return port, diags
	}

	if attr, ok := content.Attributes["tcp_health"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			port.TCPHealth = val.True()
		}
	}

	if attr, ok := content.Attributes["tcp_ready"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			port.TCPReady = val.True()
		}
	}

	if attr, ok := content.Attributes["health"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			port.Health = val.AsString()
		}
	}

	if attr, ok := content.Attributes["ready"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			port.Ready = val.AsString()
		}
	}

	// Probe tuning
	for _, field := range []struct {
		name string
		dest **int32
	}{
		{"initial_delay", &port.InitialDelay},
		{"period", &port.Period},
		{"timeout", &port.Timeout},
		{"failures", &port.Failures},
		{"successes", &port.Successes},
	} {
		if attr, ok := content.Attributes[field.name]; ok {
			val, moreDiags := attr.Expr.Value(ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				n, _ := val.AsBigFloat().Int64()
				v := int32(n)
				*field.dest = &v
			}
		}
	}

	return port, diags
}

func parseIngressBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.IngressConfig, hcl.Diagnostics) {
	ingress := types.IngressConfig{}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "host"},
			{Name: "hosts"},
			{Name: "name"},
			{Name: "service_name"},
			{Name: "port"},
			{Name: "tls"},
			{Name: "tls_secret"},
			{Name: "issuer"},
			{Name: "annotations"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return ingress, diags
	}

	// Ingress resource name (optional, defaults to app name)
	if attr, ok := content.Attributes["name"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ingress.Name = val.AsString()
		}
	}

	// Service name (optional, defaults to app name)
	if attr, ok := content.Attributes["service_name"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ingress.ServiceName = val.AsString()
		}
	}

	// Backend port (optional, defaults to first app port)
	if attr, ok := content.Attributes["port"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			n, _ := val.AsBigFloat().Int64()
			ingress.Port = int32(n)
		}
	}

	// Single host
	if attr, ok := content.Attributes["host"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ingress.Host = val.AsString()
		}
	}

	// Multiple hosts
	if attr, ok := content.Attributes["hosts"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				ingress.Hosts = append(ingress.Hosts, v.AsString())
			}
		}
	}

	if attr, ok := content.Attributes["tls"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ingress.TLS = val.True()
		}
	}

	if attr, ok := content.Attributes["tls_secret"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ingress.TLSSecret = val.AsString()
		}
	}

	if attr, ok := content.Attributes["issuer"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ingress.Issuer = val.AsString()
		}
	}

	// Annotations as a map, supports nested grouping:
	//   annotations = { "nginx.ingress.kubernetes.io" = { "enable-cors" = "true" } }
	// flattens to: "nginx.ingress.kubernetes.io/enable-cors" = "true"
	if attr, ok := content.Attributes["annotations"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			ingress.Annotations = flattenAnnotations("", val)
		}
	}

	return ingress, diags
}

// flattenAnnotations recursively flattens nested annotation maps.
// { "nginx.ingress.kubernetes.io" = { "enable-cors" = "true" } }
// becomes: { "nginx.ingress.kubernetes.io/enable-cors" = "true" }
func flattenAnnotations(prefix string, val cty.Value) map[string]string {
	result := make(map[string]string)

	for it := val.ElementIterator(); it.Next(); {
		k, v := it.Element()
		key := k.AsString()
		if prefix != "" {
			key = prefix + "/" + key
		}

		if v.Type().IsObjectType() || v.Type().IsMapType() {
			for fk, fv := range flattenAnnotations(key, v) {
				result[fk] = fv
			}
		} else {
			result[key] = v.AsString()
		}
	}

	return result
}

func parseResourcesBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.ResourcesConfig, hcl.Diagnostics) {
	res := types.ResourcesConfig{}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "cpu"},
			{Name: "memory"},
			{Name: "ephemeral_storage"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return res, diags
	}

	if attr, ok := content.Attributes["cpu"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			req, lim := parseResourceRange(val.AsString())
			res.CPURequest = req
			res.CPULimit = lim
		}
	}

	if attr, ok := content.Attributes["memory"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			req, lim := parseResourceRange(val.AsString())
			res.MemoryRequest = req
			res.MemoryLimit = lim
		}
	}

	if attr, ok := content.Attributes["ephemeral_storage"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			req, lim := parseResourceRange(val.AsString())
			res.EphemeralStorageRequest = req
			res.EphemeralStorageLimit = lim
		}
	}

	return res, diags
}

// parseResourceRange splits "200m..1000m" into request and limit.
// If no ".." is present, uses the value for both.
func parseResourceRange(s string) (request, limit string) {
	parts := strings.SplitN(s, "..", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return s, s
}

func parseEnvBlock(block *hcl.Block, ctx *hcl.EvalContext) ([]types.EnvEntry, hcl.Diagnostics) {
	attrs, diags := block.Body.JustAttributes()
	if diags.HasErrors() {
		return nil, diags
	}

	// Collect keys and sort for deterministic output
	var keys []string
	for name := range attrs {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	var entries []types.EnvEntry
	for _, name := range keys {
		attr := attrs[name]
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			continue
		}

		entry := types.EnvEntry{Name: name}

		// Check if this is a secret reference (cty object with __secret_name)
		if val.Type().IsObjectType() && val.Type().HasAttribute("__secret_name") {
			entry.SecretName = val.GetAttr("__secret_name").AsString()
			entry.SecretKey = val.GetAttr("__secret_key").AsString()
		} else if val.Type().IsObjectType() && val.Type().HasAttribute("__configmap_name") {
			entry.ConfigMapName = val.GetAttr("__configmap_name").AsString()
			entry.ConfigMapKey = val.GetAttr("__configmap_key").AsString()
		} else if val.Type() == cty.String {
			entry.Value = val.AsString()
		} else {
			entry.Value = fmt.Sprintf("%v", val.GoString())
		}

		entries = append(entries, entry)
	}

	return entries, diags
}

func parseVolumeBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.VolumeConfig, hcl.Diagnostics) {
	vol := types.VolumeConfig{
		Name: block.Labels[0],
	}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "mount_path", Required: true},
			{Name: "sub_path"},
			{Name: "read_only"},
			{Name: "secret"},
			{Name: "config_map"},
			{Name: "empty_dir"},
			{Name: "pvc"},
			{Name: "host_path"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return vol, diags
	}

	mountVal, moreDiags := content.Attributes["mount_path"].Expr.Value(ctx)
	diags = append(diags, moreDiags...)
	if !moreDiags.HasErrors() {
		vol.MountPath = mountVal.AsString()
	}

	if attr, ok := content.Attributes["sub_path"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			vol.SubPath = val.AsString()
		}
	}

	if attr, ok := content.Attributes["read_only"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			vol.ReadOnly = val.True()
		}
	}

	if attr, ok := content.Attributes["secret"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			vol.Secret = val.AsString()
		}
	}

	if attr, ok := content.Attributes["config_map"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			vol.ConfigMap = val.AsString()
		}
	}

	if attr, ok := content.Attributes["empty_dir"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			vol.EmptyDir = val.True()
		}
	}

	if attr, ok := content.Attributes["pvc"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			vol.PVC = val.AsString()
		}
	}

	if attr, ok := content.Attributes["host_path"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			vol.HostPath = val.AsString()
		}
	}

	return vol, diags
}

func parseEnvFromBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.EnvFromEntry, hcl.Diagnostics) {
	ef := types.EnvFromEntry{}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "config_map"},
			{Name: "secret"},
			{Name: "prefix"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return ef, diags
	}

	if attr, ok := content.Attributes["config_map"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ef.ConfigMap = val.AsString()
		}
	}

	if attr, ok := content.Attributes["secret"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ef.Secret = val.AsString()
		}
	}

	if attr, ok := content.Attributes["prefix"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ef.Prefix = val.AsString()
		}
	}

	return ef, diags
}

func parseSecurityContextBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.SecurityContextConfig, hcl.Diagnostics) {
	sc := types.SecurityContextConfig{}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "run_as_user"},
			{Name: "run_as_group"},
			{Name: "run_as_non_root"},
			{Name: "read_only_root"},
			{Name: "fs_group"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return sc, diags
	}

	if attr, ok := content.Attributes["run_as_user"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			n, _ := val.AsBigFloat().Int64()
			sc.RunAsUser = &n
		}
	}

	if attr, ok := content.Attributes["run_as_group"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			n, _ := val.AsBigFloat().Int64()
			sc.RunAsGroup = &n
		}
	}

	if attr, ok := content.Attributes["run_as_non_root"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			b := val.True()
			sc.RunAsNonRoot = &b
		}
	}

	if attr, ok := content.Attributes["read_only_root"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			b := val.True()
			sc.ReadOnlyRoot = &b
		}
	}

	if attr, ok := content.Attributes["fs_group"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			n, _ := val.AsBigFloat().Int64()
			sc.FSGroup = &n
		}
	}

	return sc, diags
}

func parseAutoscaleBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.AutoscaleConfig, hcl.Diagnostics) {
	as := types.AutoscaleConfig{}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "min", Required: true},
			{Name: "max", Required: true},
			{Name: "cpu"},
			{Name: "memory"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return as, diags
	}

	minVal, moreDiags := content.Attributes["min"].Expr.Value(ctx)
	diags = append(diags, moreDiags...)
	if !moreDiags.HasErrors() {
		n, _ := minVal.AsBigFloat().Int64()
		as.Min = int32(n)
	}

	maxVal, moreDiags := content.Attributes["max"].Expr.Value(ctx)
	diags = append(diags, moreDiags...)
	if !moreDiags.HasErrors() {
		n, _ := maxVal.AsBigFloat().Int64()
		as.Max = int32(n)
	}

	if attr, ok := content.Attributes["cpu"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			n, _ := val.AsBigFloat().Int64()
			v := int32(n)
			as.CPUPercent = &v
		}
	}

	if attr, ok := content.Attributes["memory"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			n, _ := val.AsBigFloat().Int64()
			v := int32(n)
			as.MemPercent = &v
		}
	}

	return as, diags
}

func parseInitContainerBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.InitContainerConfig, hcl.Diagnostics) {
	ic := types.InitContainerConfig{
		Name: block.Labels[0],
	}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "image", Required: true},
			{Name: "image_pull_policy"},
			{Name: "command"},
			{Name: "volumes"},
		},
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "env"},
			{Type: "env_from"},
			{Type: "security_context"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return ic, diags
	}

	imageVal, moreDiags := content.Attributes["image"].Expr.Value(ctx)
	diags = append(diags, moreDiags...)
	if !moreDiags.HasErrors() {
		ic.Image = imageVal.AsString()
	}

	if attr, ok := content.Attributes["image_pull_policy"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ic.ImagePullPolicy = val.AsString()
		}
	}

	if attr, ok := content.Attributes["command"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				ic.Command = append(ic.Command, v.AsString())
			}
		}
	}

	if attr, ok := content.Attributes["volumes"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				ic.VolumeMounts = append(ic.VolumeMounts, v.AsString())
			}
		}
	}

	for _, b := range content.Blocks {
		switch b.Type {
		case "env":
			entries, moreDiags := parseEnvBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				ic.Env = entries
			}
		case "env_from":
			ef, moreDiags := parseEnvFromBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				ic.EnvFrom = append(ic.EnvFrom, ef)
			}
		case "security_context":
			sc, moreDiags := parseSecurityContextBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				ic.SecurityContext = &sc
			}
		}
	}

	return ic, diags
}

func parseSidecarBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.SidecarConfig, hcl.Diagnostics) {
	sc := types.SidecarConfig{
		Name: block.Labels[0],
	}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "image", Required: true},
			{Name: "command"},
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
		return sc, diags
	}

	imageVal, moreDiags := content.Attributes["image"].Expr.Value(ctx)
	diags = append(diags, moreDiags...)
	if !moreDiags.HasErrors() {
		sc.Image = imageVal.AsString()
	}

	if attr, ok := content.Attributes["command"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				sc.Command = append(sc.Command, v.AsString())
			}
		}
	}

	for _, b := range content.Blocks {
		switch b.Type {
		case "port":
			port, moreDiags := parsePortBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				sc.Ports = append(sc.Ports, port)
			}
		case "env":
			entries, moreDiags := parseEnvBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				sc.Env = entries
			}
		case "env_from":
			ef, moreDiags := parseEnvFromBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				sc.EnvFrom = append(sc.EnvFrom, ef)
			}
		case "resources":
			res, moreDiags := parseResourcesBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				sc.Resources = &res
			}
		case "volume":
			vol, moreDiags := parseVolumeBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				sc.Volumes = append(sc.Volumes, vol)
			}
		case "security_context":
			secCtx, moreDiags := parseSecurityContextBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				sc.SecurityContext = &secCtx
			}
		}
	}

	return sc, diags
}

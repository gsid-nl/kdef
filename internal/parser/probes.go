package parser

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"

	"github.com/gsid-nl/kdef/internal/types"
)

func parseProbesBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.ProbesConfig, hcl.Diagnostics) {
	p := types.ProbesConfig{}

	schema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "liveness"},
			{Type: "readiness"},
			{Type: "startup"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return p, diags
	}

	for _, b := range content.Blocks {
		probe, moreDiags := parseProbeBlock(b, ctx)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			continue
		}
		switch b.Type {
		case "liveness":
			p.Liveness = &probe
		case "readiness":
			p.Readiness = &probe
		case "startup":
			p.Startup = &probe
		}
	}

	return p, diags
}

func parseProbeBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.ProbeConfig, hcl.Diagnostics) {
	pc := types.ProbeConfig{}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "exec"},
			{Name: "tcp_socket_port"},
			{Name: "initial_delay"},
			{Name: "period"},
			{Name: "timeout"},
			{Name: "failure_threshold"},
			{Name: "success_threshold"},
		},
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "http_get"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return pc, diags
	}

	if attr, ok := content.Attributes["exec"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				pc.Exec = append(pc.Exec, v.AsString())
			}
		}
	}

	if attr, ok := content.Attributes["tcp_socket_port"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			n, _ := val.AsBigFloat().Int64()
			pc.TCPPort = int32(n)
		}
	}

	diags = append(diags, parseProbeTuningAttrs(content, ctx, &pc)...)

	for _, b := range content.Blocks {
		if b.Type == "http_get" {
			path, port, moreDiags := parseHTTPGetBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				pc.HTTPPath = path
				pc.HTTPPort = port
			}
		}
	}

	// Validate: exactly one handler
	handlers := 0
	if len(pc.Exec) > 0 {
		handlers++
	}
	if pc.TCPPort > 0 {
		handlers++
	}
	if pc.HTTPPath != "" {
		handlers++
	}
	if handlers == 0 {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Probe has no handler",
			Detail:   fmt.Sprintf("%s probe must define exactly one of: http_get {}, tcp_socket_port, or exec", block.Type),
			Subject:  block.DefRange.Ptr(),
		})
	} else if handlers > 1 {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Probe has multiple handlers",
			Detail:   fmt.Sprintf("%s probe must define exactly one of: http_get {}, tcp_socket_port, or exec", block.Type),
			Subject:  block.DefRange.Ptr(),
		})
	}

	return pc, diags
}

func parseHTTPGetBlock(block *hcl.Block, ctx *hcl.EvalContext) (string, int32, hcl.Diagnostics) {
	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "path", Required: true},
			{Name: "port", Required: true},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return "", 0, diags
	}

	pathVal, moreDiags := content.Attributes["path"].Expr.Value(ctx)
	diags = append(diags, moreDiags...)
	path := ""
	if !moreDiags.HasErrors() {
		path = pathVal.AsString()
	}

	portVal, moreDiags := content.Attributes["port"].Expr.Value(ctx)
	diags = append(diags, moreDiags...)
	var port int32
	if !moreDiags.HasErrors() {
		n, _ := portVal.AsBigFloat().Int64()
		port = int32(n)
	}

	return path, port, diags
}

func parseProbeTuningAttrs(content *hcl.BodyContent, ctx *hcl.EvalContext, pc *types.ProbeConfig) hcl.Diagnostics {
	var diags hcl.Diagnostics
	intAttr := func(name string, dst **int32) {
		attr, ok := content.Attributes[name]
		if !ok {
			return
		}
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			return
		}
		n, _ := val.AsBigFloat().Int64()
		v := int32(n)
		*dst = &v
	}
	intAttr("initial_delay", &pc.InitialDelay)
	intAttr("period", &pc.Period)
	intAttr("timeout", &pc.Timeout)
	intAttr("failure_threshold", &pc.Failures)
	intAttr("success_threshold", &pc.Successes)
	return diags
}

func parseLifecycleBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.LifecycleConfig, hcl.Diagnostics) {
	lc := types.LifecycleConfig{}

	schema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "pre_stop"},
			{Type: "post_start"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return lc, diags
	}

	for _, b := range content.Blocks {
		h, moreDiags := parseLifecycleHandlerBlock(b, ctx)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			continue
		}
		switch b.Type {
		case "pre_stop":
			lc.PreStop = &h
		case "post_start":
			lc.PostStart = &h
		}
	}

	return lc, diags
}

func parseLifecycleHandlerBlock(block *hcl.Block, ctx *hcl.EvalContext) (types.LifecycleHandler, hcl.Diagnostics) {
	h := types.LifecycleHandler{}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "exec"},
		},
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "http_get"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return h, diags
	}

	if attr, ok := content.Attributes["exec"]; ok {
		val, moreDiags := attr.Expr.Value(ctx)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				h.Exec = append(h.Exec, v.AsString())
			}
		}
	}

	for _, b := range content.Blocks {
		if b.Type == "http_get" {
			path, port, moreDiags := parseHTTPGetBlock(b, ctx)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				h.HTTPPath = path
				h.HTTPPort = port
			}
		}
	}

	handlers := 0
	if len(h.Exec) > 0 {
		handlers++
	}
	if h.HTTPPath != "" {
		handlers++
	}
	if handlers == 0 {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Lifecycle hook has no handler",
			Detail:   fmt.Sprintf("%s hook must define exactly one of: http_get {} or exec", block.Type),
			Subject:  block.DefRange.Ptr(),
		})
	} else if handlers > 1 {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Lifecycle hook has multiple handlers",
			Detail:   fmt.Sprintf("%s hook must define exactly one of: http_get {} or exec", block.Type),
			Subject:  block.DefRange.Ptr(),
		})
	}

	return h, diags
}

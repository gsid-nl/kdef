package parser

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/zclconf/go-cty/cty"

	"github.com/gsid-nl/kdef/internal/types"
)

// LoadOptions configures how Load processes .kdef files.
type LoadOptions struct {
	Dir        string
	Overrides  map[string]string // from --set flags
	ValuesFile string            // from --values flag (JSON file)
	Env        string            // from --env flag (loads environments/<env>.kdef)
	VarsFrom   []string          // additional vars.kdef paths to import
}

// Load parses all .kdef files in the given directory and returns a KdefConfig.
func Load(dir string, overrides map[string]string) (*types.KdefConfig, error) {
	return LoadWithOptions(LoadOptions{Dir: dir, Overrides: overrides})
}

// LoadWithOptions parses all .kdef files using the given options.
func LoadWithOptions(opts LoadOptions) (*types.KdefConfig, error) {
	config := &types.KdefConfig{
		Variables: make(map[string]types.VariableDecl),
	}

	// Phase 1: parse vars.kdef (with imports)
	varsFile := filepath.Join(opts.Dir, "vars.kdef")
	if _, err := os.Stat(varsFile); err == nil {
		result, diags := ParseVariableFileWithImports(varsFile)
		if diags.HasErrors() {
			return nil, diags
		}

		// Process imports first (imported vars have lower precedence)
		for _, importPath := range result.Imports {
			importedVars, diags := ParseVariableFile(importPath)
			if diags.HasErrors() {
				return nil, diags
			}
			for k, v := range importedVars {
				config.Variables[k] = v
			}
		}

		// CLI --vars-from (higher precedence than file imports)
		for _, vf := range opts.VarsFrom {
			vars, diags := ParseVariableFile(vf)
			if diags.HasErrors() {
				return nil, diags
			}
			for k, v := range vars {
				config.Variables[k] = v
			}
		}

		// Local vars override everything
		for k, v := range result.Variables {
			config.Variables[k] = v
		}

		// Capture ingress defaults
		if result.IngressDefaults != nil {
			config.IngressDefaults = result.IngressDefaults
		}
	}

	// Load extra values from JSON file
	var extraValues map[string]cty.Value
	if opts.ValuesFile != "" {
		var err error
		extraValues, err = LoadValuesFile(opts.ValuesFile)
		if err != nil {
			return nil, err
		}
	}

	// Build EvalContext
	ctx, diags := BuildEvalContext(config.Variables, opts.Overrides, extraValues, opts.Dir)
	if diags.HasErrors() {
		return nil, diags
	}

	// Phase 2: find and parse all definition files (everything except vars.kdef)
	entries, err := os.ReadDir(opts.Dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".kdef") {
			continue
		}
		if entry.Name() == "vars.kdef" {
			continue
		}

		path := filepath.Join(opts.Dir, entry.Name())
		result, diags := ParseFile(path, ctx)
		if diags.HasErrors() {
			return nil, diags
		}
		config.CronJobs = append(config.CronJobs, result.CronJobs...)
		config.ConfigMaps = append(config.ConfigMaps, result.ConfigMaps...)
		config.Deployments = append(config.Deployments, result.Deployments...)
	}

	// Phase 3: apply ingress defaults to all apps
	if config.IngressDefaults != nil {
		applyIngressDefaults(config)
	}

	// Phase 4: apply environment overrides if --env is specified
	if opts.Env != "" {
		envFile := filepath.Join(opts.Dir, "environments", opts.Env+".kdef")
		if _, err := os.Stat(envFile); err == nil {
			overrideResult, diags := ParseOverrideFile(envFile)
			if diags.HasErrors() {
				return nil, diags
			}

			// If env file has var overrides, we need to re-parse with those vars
			// For now, just apply the structural overrides
			ApplyOverrides(config, overrideResult)

			// Merge var overrides into the set flags for reference
			for k, v := range overrideResult.VarOverrides {
				if opts.Overrides == nil {
					opts.Overrides = make(map[string]string)
				}
				if _, exists := opts.Overrides[k]; !exists {
					opts.Overrides[k] = v
				}
			}
		}
	}

	return config, nil
}

// applyIngressDefaults merges ingress defaults into all deployments that have an ingress block.
func applyIngressDefaults(config *types.KdefConfig) {
	defaults := config.IngressDefaults

	for i := range config.Deployments {
		dep := &config.Deployments[i]
		if dep.Ingress == nil {
			continue
		}

		if defaults.TLS != nil && !dep.Ingress.TLS {
			dep.Ingress.TLS = *defaults.TLS
		}
		if defaults.TLSSecret != "" && dep.Ingress.TLSSecret == "" {
			dep.Ingress.TLSSecret = defaults.TLSSecret
		}
		if defaults.Issuer != "" && dep.Ingress.Issuer == "" {
			dep.Ingress.Issuer = defaults.Issuer
		}
		if len(defaults.Annotations) > 0 {
			merged := make(map[string]string)
			for k, v := range defaults.Annotations {
				merged[k] = v
			}
			for k, v := range dep.Ingress.Annotations {
				merged[k] = v
			}
			dep.Ingress.Annotations = merged
		}
	}
}

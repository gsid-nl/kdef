package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
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
// If the directory contains a root.kdef with a deployments list,
// each listed subdirectory is loaded as an independent project and
// all results are merged.
func LoadWithOptions(opts LoadOptions) (*types.KdefConfig, error) {
	// Check for root.kdef — multi-project mode
	rootFile := filepath.Join(opts.Dir, "root.kdef")
	if _, err := os.Stat(rootFile); err == nil {
		return loadRootProject(rootFile, opts)
	}

	return loadSingleProject(opts)
}

func loadSingleProject(opts LoadOptions) (*types.KdefConfig, error) {
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
			importResult, diags := ParseVariableFileWithImports(importPath)
			if diags.HasErrors() {
				return nil, diags
			}
			for k, v := range importResult.Variables {
				config.Variables[k] = v
			}
			if importResult.IngressDefaults != nil && config.IngressDefaults == nil {
				config.IngressDefaults = importResult.IngressDefaults
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

	// Pre-scan for images {} blocks across all .kdef files
	images, err := ScanImages(opts.Dir)
	if err != nil {
		return nil, err
	}

	// Build EvalContext
	ctx, diags := BuildEvalContext(config.Variables, opts.Overrides, extraValues, images, opts.Dir)
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
		config.SealedSecrets = append(config.SealedSecrets, result.SealedSecrets...)
		config.PersistentVolumeClaims = append(config.PersistentVolumeClaims, result.PersistentVolumeClaims...)
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

// loadRootProject parses a root.kdef file and loads each listed subdirectory
// as an independent kdef project, merging all results into a single KdefConfig.
func loadRootProject(rootFile string, opts LoadOptions) (*types.KdefConfig, error) {
	root, err := parseRootFile(rootFile)
	if err != nil {
		return nil, fmt.Errorf("parse root.kdef: %w", err)
	}

	if len(root.Deployments) == 0 {
		return nil, fmt.Errorf("root.kdef contains no deployments")
	}

	merged := &types.KdefConfig{
		Variables:       make(map[string]types.VariableDecl),
		Namespaces:      root.Namespaces,
		IngressDefaults: root.IngressDefaults,
	}

	// Collect service accounts for manifest generation
	for _, sa := range root.ServiceAccounts {
		merged.ServiceAccounts = append(merged.ServiceAccounts, sa)
	}

	for name, entry := range root.Deployments {
		subDir := filepath.Join(filepath.Dir(rootFile), entry.Path)
		if _, err := os.Stat(subDir); err != nil {
			return nil, fmt.Errorf("deployment %q: directory %q not found: %w", name, entry.Path, err)
		}

		// Resolve env: CLI > per-deployment > global root
		env := root.Env
		if entry.Env != "" {
			env = entry.Env
		}
		if opts.Env != "" {
			env = opts.Env
		}

		// Resolve set: global root < per-deployment < CLI
		overrides := make(map[string]string)
		for k, v := range root.Set {
			overrides[k] = v
		}
		for k, v := range entry.Set {
			overrides[k] = v
		}
		for k, v := range opts.Overrides {
			overrides[k] = v
		}

		subOpts := LoadOptions{
			Dir:        subDir,
			Overrides:  overrides,
			ValuesFile: opts.ValuesFile,
			Env:        env,
			VarsFrom:   opts.VarsFrom,
		}

		config, err := LoadWithOptions(subOpts)
		if err != nil {
			return nil, fmt.Errorf("load %q: %w", name, err)
		}

		// Inject namespace from root.kdef into blocks that don't have one
		if entry.Namespace != "" {
			injectNamespace(config, entry.Namespace)
		}

		// Inject service_account into deployments and cronjobs
		if entry.ServiceAccount != "" {
			injectServiceAccount(config, entry.ServiceAccount)
		}

		merged.Deployments = append(merged.Deployments, config.Deployments...)
		merged.CronJobs = append(merged.CronJobs, config.CronJobs...)
		merged.ConfigMaps = append(merged.ConfigMaps, config.ConfigMaps...)
		merged.SealedSecrets = append(merged.SealedSecrets, config.SealedSecrets...)
		merged.PersistentVolumeClaims = append(merged.PersistentVolumeClaims, config.PersistentVolumeClaims...)
	}

	// Validate: every resource must have a namespace
	if err := validateNamespaces(merged, root); err != nil {
		return nil, err
	}

	// Apply ingress defaults from root.kdef
	if merged.IngressDefaults != nil {
		applyIngressDefaults(merged)
	}

	return merged, nil
}

// injectNamespace sets the namespace on all blocks that don't already have one.
func injectNamespace(config *types.KdefConfig, namespace string) {
	for i := range config.Deployments {
		if config.Deployments[i].Namespace == "" {
			config.Deployments[i].Namespace = namespace
		}
	}
	for i := range config.CronJobs {
		if config.CronJobs[i].Namespace == "" {
			config.CronJobs[i].Namespace = namespace
		}
	}
	for i := range config.ConfigMaps {
		if config.ConfigMaps[i].Namespace == "" {
			config.ConfigMaps[i].Namespace = namespace
		}
	}
	for i := range config.SealedSecrets {
		if config.SealedSecrets[i].Namespace == "" {
			config.SealedSecrets[i].Namespace = namespace
		}
	}
	for i := range config.PersistentVolumeClaims {
		if config.PersistentVolumeClaims[i].Namespace == "" {
			config.PersistentVolumeClaims[i].Namespace = namespace
		}
	}
}

// injectServiceAccount sets the service account on deployments and cronjobs that don't already have one.
func injectServiceAccount(config *types.KdefConfig, serviceAccount string) {
	for i := range config.Deployments {
		if config.Deployments[i].ServiceAccountName == "" {
			config.Deployments[i].ServiceAccountName = serviceAccount
		}
	}
	for i := range config.CronJobs {
		if config.CronJobs[i].ServiceAccountName == "" {
			config.CronJobs[i].ServiceAccountName = serviceAccount
		}
	}
}

// validateNamespaces checks that every resource has a namespace and that all
// namespaces are in the allowed list (if one is defined in root.kdef).
func validateNamespaces(config *types.KdefConfig, root *types.RootConfig) error {
	allowed := make(map[string]bool)
	for _, ns := range root.Namespaces {
		allowed[ns] = true
	}
	hasAllowList := len(allowed) > 0

	// Also validate service_account references
	definedSAs := make(map[string]bool)
	for name := range root.ServiceAccounts {
		definedSAs[name] = true
	}

	check := func(kind, name, namespace string) error {
		if namespace == "" {
			return fmt.Errorf("%s %q has no namespace (set it in root.kdef or in the .kdef file)", kind, name)
		}
		if hasAllowList && !allowed[namespace] {
			return fmt.Errorf("%s %q uses namespace %q which is not in root.kdef namespaces list", kind, name, namespace)
		}
		return nil
	}

	for _, dep := range config.Deployments {
		if err := check("deployment", dep.Name, dep.Namespace); err != nil {
			return err
		}
		if dep.ServiceAccountName != "" && len(definedSAs) > 0 && !definedSAs[dep.ServiceAccountName] {
			return fmt.Errorf("deployment %q uses service_account %q which is not defined in root.kdef", dep.Name, dep.ServiceAccountName)
		}
	}
	for _, cj := range config.CronJobs {
		if err := check("cronjob", cj.Name, cj.Namespace); err != nil {
			return err
		}
	}
	for _, cm := range config.ConfigMaps {
		if err := check("configmap", cm.Name, cm.Namespace); err != nil {
			return err
		}
	}
	for _, ss := range config.SealedSecrets {
		if err := check("sealedsecret", ss.Name, ss.Namespace); err != nil {
			return err
		}
	}
	for _, pvc := range config.PersistentVolumeClaims {
		if err := check("persistentvolumeclaim", pvc.Name, pvc.Namespace); err != nil {
			return err
		}
	}

	return nil
}

// parseRootFile parses a root.kdef file and returns the RootConfig.
func parseRootFile(filename string) (*types.RootConfig, error) {
	p := hclparse.NewParser()
	file, diags := p.ParseHCLFile(filename)
	if diags.HasErrors() {
		return nil, diags
	}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "namespaces"},
			{Name: "deployments", Required: true},
			{Name: "env"},
			{Name: "set"},
		},
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "service_account", LabelNames: []string{"name"}},
			{Type: "ingress_defaults"},
		},
	}

	content, diags := file.Body.Content(schema)
	if diags.HasErrors() {
		return nil, diags
	}

	root := &types.RootConfig{
		ServiceAccounts: make(map[string]types.ServiceAccountConfig),
	}

	// Parse namespaces
	if attr, ok := content.Attributes["namespaces"]; ok {
		val, moreDiags := attr.Expr.Value(nil)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				root.Namespaces = append(root.Namespaces, v.AsString())
			}
		}
	}

	// Parse global env
	if attr, ok := content.Attributes["env"]; ok {
		val, moreDiags := attr.Expr.Value(nil)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			root.Env = val.AsString()
		}
	}

	// Parse global set
	if attr, ok := content.Attributes["set"]; ok {
		val, moreDiags := attr.Expr.Value(nil)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			root.Set = make(map[string]string)
			for it := val.ElementIterator(); it.Next(); {
				k, v := it.Element()
				root.Set[k.AsString()] = v.AsString()
			}
		}
	}

	// Parse blocks
	for _, block := range content.Blocks {
		switch block.Type {
		case "service_account":
			sa, moreDiags := parseServiceAccountBlock(block)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				root.ServiceAccounts[sa.Name] = sa
			}
		case "ingress_defaults":
			id, moreDiags := parseIngressDefaultsBlock(block)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				root.IngressDefaults = id
			}
		}
	}

	// Parse deployments map
	deploymentsAttr := content.Attributes["deployments"]
	val, moreDiags := deploymentsAttr.Expr.Value(nil)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return nil, diags
	}

	root.Deployments = make(map[string]types.DeploymentEntry)
	for it := val.ElementIterator(); it.Next(); {
		k, v := it.Element()
		name := k.AsString()
		entry := types.DeploymentEntry{}

		if v.Type().HasAttribute("path") {
			entry.Path = v.GetAttr("path").AsString()
		} else {
			// Default path to the deployment name
			entry.Path = name
		}
		if v.Type().HasAttribute("namespace") {
			entry.Namespace = v.GetAttr("namespace").AsString()
		}
		if v.Type().HasAttribute("service_account") {
			entry.ServiceAccount = v.GetAttr("service_account").AsString()
		}
		if v.Type().HasAttribute("env") {
			entry.Env = v.GetAttr("env").AsString()
		}
		if v.Type().HasAttribute("set") {
			setVal := v.GetAttr("set")
			if setVal.CanIterateElements() {
				entry.Set = make(map[string]string)
				for sit := setVal.ElementIterator(); sit.Next(); {
					sk, sv := sit.Element()
					entry.Set[sk.AsString()] = sv.AsString()
				}
			}
		}

		root.Deployments[name] = entry
	}

	if diags.HasErrors() {
		return nil, diags
	}

	return root, nil
}

// parseServiceAccountBlock parses a service_account block.
func parseServiceAccountBlock(block *hcl.Block) (types.ServiceAccountConfig, hcl.Diagnostics) {
	sa := types.ServiceAccountConfig{
		Name: block.Labels[0],
	}

	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "image_pull_secrets"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return sa, diags
	}

	if attr, ok := content.Attributes["image_pull_secrets"]; ok {
		val, moreDiags := attr.Expr.Value(nil)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				sa.ImagePullSecrets = append(sa.ImagePullSecrets, v.AsString())
			}
		}
	}

	return sa, diags
}

// parseIngressDefaultsBlock parses an ingress_defaults block from root.kdef.
func parseIngressDefaultsBlock(block *hcl.Block) (*types.IngressDefaults, hcl.Diagnostics) {
	id := &types.IngressDefaults{}

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
		return nil, diags
	}

	if attr, ok := content.Attributes["tls"]; ok {
		val, moreDiags := attr.Expr.Value(nil)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			b := val.True()
			id.TLS = &b
		}
	}

	if attr, ok := content.Attributes["tls_secret"]; ok {
		val, moreDiags := attr.Expr.Value(nil)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			id.TLSSecret = val.AsString()
		}
	}

	if attr, ok := content.Attributes["issuer"]; ok {
		val, moreDiags := attr.Expr.Value(nil)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			id.Issuer = val.AsString()
		}
	}

	if attr, ok := content.Attributes["annotations"]; ok {
		val, moreDiags := attr.Expr.Value(nil)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() && val.CanIterateElements() {
			id.Annotations = flattenAnnotations("", val)
		}
	}

	return id, diags
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

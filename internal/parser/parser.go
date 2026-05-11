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
	RootDir    string            // root project directory (set when loading sub-projects)
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

	// Phase 0+1: walk from root down to leaf, loading vars.kdef at each level.
	// Deeper levels override shallower ones on name collision.
	loadedVars, diags := LoadVariablesWalk(opts.Dir, opts.RootDir, opts.VarsFrom)
	if diags.HasErrors() {
		return nil, diags
	}
	for k, v := range loadedVars.Variables {
		config.Variables[k] = v
	}
	if loadedVars.IngressDefaults != nil {
		config.IngressDefaults = loadedVars.IngressDefaults
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

	// Pre-scan for images: walk from root down to leaf, deeper levels override.
	images, err := ScanImagesWalk(opts.Dir, opts.RootDir)
	if err != nil {
		return nil, err
	}

	// Build EvalContext
	ctx, diags := BuildEvalContext(config.Variables, opts.Overrides, extraValues, images, opts.Dir)
	if diags.HasErrors() {
		return nil, diags
	}

	// Phase 2: parse local definition files (root-level defs are parsed once in loadRootProject)
	skipFiles := map[string]bool{"vars.kdef": true, "root.kdef": true}

	entries, err := os.ReadDir(opts.Dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".kdef") || skipFiles[entry.Name()] {
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
		config.DaemonSets = append(config.DaemonSets, result.DaemonSets...)
		config.StatefulSets = append(config.StatefulSets, result.StatefulSets...)
		config.Secrets = append(config.Secrets, result.Secrets...)
		config.SealedSecrets = append(config.SealedSecrets, result.SealedSecrets...)
		config.PersistentVolumeClaims = append(config.PersistentVolumeClaims, result.PersistentVolumeClaims...)
		config.ClusterRoles = append(config.ClusterRoles, result.ClusterRoles...)
		config.ClusterRoleBindings = append(config.ClusterRoleBindings, result.ClusterRoleBindings...)
	}

	// Phase 3: apply ingress defaults to all apps
	if config.IngressDefaults != nil {
		applyIngressDefaults(config)
	}

	// Phase 3b: detect ingress conflicts (duplicate resource names or cert secret names)
	if err := validateIngresses(config); err != nil {
		return nil, err
	}

	// Phase 4: apply environment overrides if --env is specified
	if opts.Env != "" {
		envFile := filepath.Join(opts.Dir, "environments", opts.Env+".kdef")
		if _, err := os.Stat(envFile); err == nil {
			overrideResult, diags := ParseOverrideFile(envFile)
			if diags.HasErrors() {
				return nil, diags
			}

			ApplyOverrides(config, overrideResult)

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

	// Validate cross-resource references (configmaps, secrets, PVCs)
	validateReferences(config)

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

	// Parse root-level definition files once (configmaps, etc.)
	// These use the root directory context so file() paths resolve correctly.
	rootDir := filepath.Dir(rootFile)
	rootDefs, err := parseRootDefinitionFiles(rootDir, root, opts)
	if err != nil {
		return nil, err
	}
	merged.CronJobs = append(merged.CronJobs, rootDefs.CronJobs...)
	merged.ConfigMaps = append(merged.ConfigMaps, rootDefs.ConfigMaps...)
	merged.Deployments = append(merged.Deployments, rootDefs.Deployments...)
	merged.DaemonSets = append(merged.DaemonSets, rootDefs.DaemonSets...)
	merged.StatefulSets = append(merged.StatefulSets, rootDefs.StatefulSets...)
	merged.Secrets = append(merged.Secrets, rootDefs.Secrets...)
	merged.SealedSecrets = append(merged.SealedSecrets, rootDefs.SealedSecrets...)
	merged.PersistentVolumeClaims = append(merged.PersistentVolumeClaims, rootDefs.PersistentVolumeClaims...)
	merged.ClusterRoles = append(merged.ClusterRoles, rootDefs.ClusterRoles...)
	merged.ClusterRoleBindings = append(merged.ClusterRoleBindings, rootDefs.ClusterRoleBindings...)

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
			RootDir:    filepath.Dir(rootFile),
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
		merged.DaemonSets = append(merged.DaemonSets, config.DaemonSets...)
		merged.StatefulSets = append(merged.StatefulSets, config.StatefulSets...)
		merged.CronJobs = append(merged.CronJobs, config.CronJobs...)
		merged.ConfigMaps = append(merged.ConfigMaps, config.ConfigMaps...)
		merged.Secrets = append(merged.Secrets, config.Secrets...)
		merged.SealedSecrets = append(merged.SealedSecrets, config.SealedSecrets...)
		merged.PersistentVolumeClaims = append(merged.PersistentVolumeClaims, config.PersistentVolumeClaims...)
		merged.ClusterRoles = append(merged.ClusterRoles, config.ClusterRoles...)
		merged.ClusterRoleBindings = append(merged.ClusterRoleBindings, config.ClusterRoleBindings...)
	}

	// Validate: every resource must have a namespace
	if err := validateNamespaces(merged, root); err != nil {
		return nil, err
	}

	// Validate cross-resource references (configmaps, secrets, PVCs)
	validateReferences(merged)

	// Apply ingress defaults from root.kdef
	if merged.IngressDefaults != nil {
		applyIngressDefaults(merged)
	}

	// Detect ingress conflicts (duplicate resource names or cert secret names)
	if err := validateIngresses(merged); err != nil {
		return nil, err
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
	for i := range config.DaemonSets {
		if config.DaemonSets[i].Namespace == "" {
			config.DaemonSets[i].Namespace = namespace
		}
	}
	for i := range config.StatefulSets {
		if config.StatefulSets[i].Namespace == "" {
			config.StatefulSets[i].Namespace = namespace
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
	for i := range config.Secrets {
		if config.Secrets[i].Namespace == "" {
			config.Secrets[i].Namespace = namespace
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
	for i := range config.DaemonSets {
		if config.DaemonSets[i].ServiceAccountName == "" {
			config.DaemonSets[i].ServiceAccountName = serviceAccount
		}
	}
	for i := range config.StatefulSets {
		if config.StatefulSets[i].ServiceAccountName == "" {
			config.StatefulSets[i].ServiceAccountName = serviceAccount
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

	// Also validate service_account references. An SA "name" is considered
	// defined if there's at least one service_account block with that name
	// (scoped or default).
	definedSAs := make(map[string]bool)
	for _, sa := range root.ServiceAccounts {
		definedSAs[sa.Name] = true
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
	for _, ds := range config.DaemonSets {
		if err := check("daemonset", ds.Name, ds.Namespace); err != nil {
			return err
		}
		if ds.ServiceAccountName != "" && len(definedSAs) > 0 && !definedSAs[ds.ServiceAccountName] {
			return fmt.Errorf("daemonset %q uses service_account %q which is not defined in root.kdef", ds.Name, ds.ServiceAccountName)
		}
	}
	for _, sts := range config.StatefulSets {
		if err := check("statefulset", sts.Name, sts.Namespace); err != nil {
			return err
		}
		if sts.ServiceAccountName != "" && len(definedSAs) > 0 && !definedSAs[sts.ServiceAccountName] {
			return fmt.Errorf("statefulset %q uses service_account %q which is not defined in root.kdef", sts.Name, sts.ServiceAccountName)
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
	for _, s := range config.Secrets {
		if err := check("secret", s.Name, s.Namespace); err != nil {
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

// validateIngresses checks that no two ingress blocks across the config produce:
//   - the same effective K8s Ingress resource name within a namespace, or
//   - the same cert-manager Certificate secret name within a namespace
//     (only generated when tls is on and tls_secret is not set).
//
// Conflicts are returned as a fatal error. The check runs after ingress_defaults
// have been applied so that defaulted fields (TLS, TLSSecret) are accounted for.
func validateIngresses(config *types.KdefConfig) error {
	type origin struct {
		kind     string
		workload string
		index    int
	}

	resourceNames := make(map[string]origin) // key: "namespace/name"
	secretNames := make(map[string]origin)   // key: "namespace/secret"

	check := func(kind, workload, namespace string, ingresses []types.IngressConfig) error {
		// Detect duplicates *within* the workload too: e.g. two blocks both
		// explicitly named "foo" would otherwise only collide cross-namespace.
		localNames := make(map[string]int)
		for i, ing := range ingresses {
			name := ing.ResourceName(workload, i)
			if prev, dup := localNames[name]; dup {
				return fmt.Errorf("%s %q: ingress blocks #%d and #%d both resolve to resource name %q — set a distinct `name` on at least one",
					kind, workload, prev+1, i+1, name)
			}
			localNames[name] = i

			key := namespace + "/" + name
			if prev, dup := resourceNames[key]; dup {
				return fmt.Errorf("%s %q: ingress #%d resolves to resource name %q which is already used by %s %q (ingress #%d) in namespace %q",
					kind, workload, i+1, name, prev.kind, prev.workload, prev.index+1, namespace)
			}
			resourceNames[key] = origin{kind: kind, workload: workload, index: i}

			if secret := ing.CertificateSecretName(); secret != "" {
				skey := namespace + "/" + secret
				if prev, dup := secretNames[skey]; dup {
					return fmt.Errorf("%s %q: ingress #%d would generate cert-manager Certificate secret %q which is already requested by %s %q (ingress #%d) in namespace %q — set `tls_secret` on one of them, or change the first host so the derived secret names differ",
						kind, workload, i+1, secret, prev.kind, prev.workload, prev.index+1, namespace)
				}
				secretNames[skey] = origin{kind: kind, workload: workload, index: i}
			}
		}
		return nil
	}

	for _, dep := range config.Deployments {
		if err := check("deployment", dep.Name, dep.Namespace, dep.Ingresses); err != nil {
			return err
		}
	}
	for _, sts := range config.StatefulSets {
		if err := check("statefulset", sts.Name, sts.Namespace, sts.Ingresses); err != nil {
			return err
		}
	}
	return nil
}

// validateReferences checks that all configmap, secret, and PVC references in
// deployments and cronjobs point to resources that are actually defined.
// Missing references are added as warnings (not errors) to config.Warnings.
func validateReferences(config *types.KdefConfig) {
	// Build lookup sets of defined resources
	configMaps := make(map[string]bool)
	for _, cm := range config.ConfigMaps {
		configMaps[cm.Name] = true
	}

	secrets := make(map[string]bool)
	for _, s := range config.Secrets {
		secrets[s.Name] = true
	}
	for _, ss := range config.SealedSecrets {
		secrets[ss.Name] = true
	}

	pvcs := make(map[string]bool)
	for _, pvc := range config.PersistentVolumeClaims {
		pvcs[pvc.Name] = true
	}

	seen := make(map[string]bool) // deduplicate warnings

	warn := func(msg string) {
		if !seen[msg] {
			seen[msg] = true
			config.Warnings = append(config.Warnings, msg)
		}
	}

	// checkEnv validates env and env_from references for a workload.
	checkEnv := func(kind, name string, env []types.EnvEntry, envFrom []types.EnvFromEntry) {
		for _, e := range env {
			if e.ConfigMapName != "" && !configMaps[e.ConfigMapName] {
				warn(fmt.Sprintf("%s %q references configmap %q (env var %q) which is not defined", kind, name, e.ConfigMapName, e.Name))
			}
			if e.SecretName != "" && !secrets[e.SecretName] {
				warn(fmt.Sprintf("%s %q references secret %q (env var %q) which is not defined", kind, name, e.SecretName, e.Name))
			}
		}
		for _, ef := range envFrom {
			if ef.ConfigMap != "" && !configMaps[ef.ConfigMap] {
				warn(fmt.Sprintf("%s %q references configmap %q (env_from) which is not defined", kind, name, ef.ConfigMap))
			}
			if ef.Secret != "" && !secrets[ef.Secret] {
				warn(fmt.Sprintf("%s %q references secret %q (env_from) which is not defined", kind, name, ef.Secret))
			}
		}
	}

	// checkVolumes validates volume source references for a workload.
	checkVolumes := func(kind, name string, volumes []types.VolumeConfig) {
		for _, v := range volumes {
			if v.ConfigMap != "" && !configMaps[v.ConfigMap] {
				warn(fmt.Sprintf("%s %q references configmap %q (volume %q) which is not defined", kind, name, v.ConfigMap, v.Name))
			}
			if v.Secret != "" && !secrets[v.Secret] {
				warn(fmt.Sprintf("%s %q references secret %q (volume %q) which is not defined", kind, name, v.Secret, v.Name))
			}
			if v.PVC != "" && !pvcs[v.PVC] {
				warn(fmt.Sprintf("%s %q references pvc %q (volume %q) which is not defined", kind, name, v.PVC, v.Name))
			}
		}
	}

	for _, dep := range config.Deployments {
		checkVolumes("deployment", dep.Name, dep.Volumes)
		for _, c := range dep.Containers {
			checkEnv("deployment", dep.Name, c.Env, c.EnvFrom)
			checkVolumes("deployment", dep.Name, c.Volumes)
		}
		for _, ic := range dep.InitContainers {
			checkEnv("deployment", dep.Name, ic.Env, ic.EnvFrom)
		}
	}

	for _, ds := range config.DaemonSets {
		checkVolumes("daemonset", ds.Name, ds.Volumes)
		for _, c := range ds.Containers {
			checkEnv("daemonset", ds.Name, c.Env, c.EnvFrom)
			checkVolumes("daemonset", ds.Name, c.Volumes)
		}
		for _, ic := range ds.InitContainers {
			checkEnv("daemonset", ds.Name, ic.Env, ic.EnvFrom)
		}
	}

	for _, sts := range config.StatefulSets {
		checkVolumes("statefulset", sts.Name, sts.Volumes)
		for _, c := range sts.Containers {
			checkEnv("statefulset", sts.Name, c.Env, c.EnvFrom)
			checkVolumes("statefulset", sts.Name, c.Volumes)
		}
		for _, ic := range sts.InitContainers {
			checkEnv("statefulset", sts.Name, ic.Env, ic.EnvFrom)
		}
	}

	for _, cj := range config.CronJobs {
		checkEnv("cronjob", cj.Name, cj.Env, cj.EnvFrom)
		checkVolumes("cronjob", cj.Name, cj.Volumes)
	}
}

// validImagePullPolicies lists the Kubernetes-accepted values.
var validImagePullPolicies = map[string]bool{
	"Always":       true,
	"IfNotPresent": true,
	"Never":        true,
}

// validateImagePullPolicy returns an HCL diagnostic if the policy is invalid.
func validateImagePullPolicy(policy string, attr *hcl.Attribute) *hcl.Diagnostic {
	if policy == "" || validImagePullPolicies[policy] {
		return nil
	}
	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  fmt.Sprintf("invalid image_pull_policy %q", policy),
		Detail:   "supported values: Always, IfNotPresent, Never",
		Subject:  attr.Expr.Range().Ptr(),
	}
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

	root := &types.RootConfig{}

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
	seenSA := make(map[string]bool) // key: name + "\x00" + namespace
	for _, block := range content.Blocks {
		switch block.Type {
		case "service_account":
			sa, moreDiags := parseServiceAccountBlock(block)
			diags = append(diags, moreDiags...)
			if moreDiags.HasErrors() {
				continue
			}
			key := sa.Name + "\x00" + sa.Namespace
			if seenSA[key] {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate service_account",
					Detail:   fmt.Sprintf("service_account %q for namespace %q is defined more than once", sa.Name, sa.Namespace),
					Subject:  block.DefRange.Ptr(),
				})
				continue
			}
			seenSA[key] = true
			root.ServiceAccounts = append(root.ServiceAccounts, sa)
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
			{Name: "namespace"},
			{Name: "image_pull_secrets"},
		},
	}

	content, diags := block.Body.Content(schema)
	if diags.HasErrors() {
		return sa, diags
	}

	if attr, ok := content.Attributes["namespace"]; ok {
		val, moreDiags := attr.Expr.Value(nil)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			sa.Namespace = val.AsString()
		}
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

// parseRootDefinitionFiles parses .kdef files in the root directory once,
// using the root directory as context for file() path resolution.
// This avoids re-parsing per sub-project and ensures correct paths.
func parseRootDefinitionFiles(rootDir string, root *types.RootConfig, opts LoadOptions) (*types.KdefConfig, error) {
	config := &types.KdefConfig{
		Variables: make(map[string]types.VariableDecl),
	}

	// Load root-level vars for the eval context
	varsFile := filepath.Join(rootDir, "vars.kdef")
	if _, err := os.Stat(varsFile); err == nil {
		result, diags := ParseVariableFileWithImports(varsFile)
		if diags.HasErrors() {
			return nil, diags
		}
		for _, importPath := range result.Imports {
			importResult, diags := ParseVariableFileWithImports(importPath)
			if diags.HasErrors() {
				return nil, diags
			}
			for k, v := range importResult.Variables {
				config.Variables[k] = v
			}
		}
		for k, v := range result.Variables {
			config.Variables[k] = v
		}
	}

	// Scan root-level images
	images, err := ScanImages(rootDir)
	if err != nil {
		return nil, err
	}

	// Build eval context with root directory for file() resolution
	ctx, diags := BuildEvalContext(config.Variables, opts.Overrides, nil, images, rootDir)
	if diags.HasErrors() {
		return nil, diags
	}

	skipFiles := map[string]bool{"vars.kdef": true, "root.kdef": true}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".kdef") || skipFiles[entry.Name()] {
			continue
		}

		path := filepath.Join(rootDir, entry.Name())
		result, diags := ParseFile(path, ctx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("parse root-level %s: %w", entry.Name(), diags)
		}
		config.CronJobs = append(config.CronJobs, result.CronJobs...)
		config.ConfigMaps = append(config.ConfigMaps, result.ConfigMaps...)
		config.Deployments = append(config.Deployments, result.Deployments...)
		config.DaemonSets = append(config.DaemonSets, result.DaemonSets...)
		config.StatefulSets = append(config.StatefulSets, result.StatefulSets...)
		config.Secrets = append(config.Secrets, result.Secrets...)
		config.SealedSecrets = append(config.SealedSecrets, result.SealedSecrets...)
		config.PersistentVolumeClaims = append(config.PersistentVolumeClaims, result.PersistentVolumeClaims...)
		config.ClusterRoles = append(config.ClusterRoles, result.ClusterRoles...)
		config.ClusterRoleBindings = append(config.ClusterRoleBindings, result.ClusterRoleBindings...)
	}

	return config, nil
}

// applyIngressDefaults merges ingress defaults into all workloads that have an ingress block.
func applyIngressDefaults(config *types.KdefConfig) {
	defaults := config.IngressDefaults

	apply := func(ing *types.IngressConfig) {
		if ing == nil {
			return
		}
		if defaults.TLS != nil && !ing.TLS {
			ing.TLS = *defaults.TLS
		}
		if defaults.TLSSecret != "" && ing.TLSSecret == "" {
			ing.TLSSecret = defaults.TLSSecret
		}
		if defaults.Issuer != "" && ing.Issuer == "" {
			ing.Issuer = defaults.Issuer
		}
		if len(defaults.Annotations) > 0 {
			merged := make(map[string]string)
			for k, v := range defaults.Annotations {
				merged[k] = v
			}
			for k, v := range ing.Annotations {
				merged[k] = v
			}
			ing.Annotations = merged
		}
	}

	for i := range config.Deployments {
		for j := range config.Deployments[i].Ingresses {
			apply(&config.Deployments[i].Ingresses[j])
		}
	}
	for i := range config.StatefulSets {
		for j := range config.StatefulSets[i].Ingresses {
			apply(&config.StatefulSets[i].Ingresses[j])
		}
	}
}

package types

// RootConfig holds the parsed root.kdef configuration.
type RootConfig struct {
	Namespaces      []string
	ServiceAccounts []ServiceAccountConfig
	IngressDefaults *IngressDefaults
	Env             string            // global default --env
	Set             map[string]string // global default --set overrides
	Deployments     map[string]DeploymentEntry
}

// ServiceAccountConfig defines a service account with its imagePullSecrets.
// Namespace is optional — an empty Namespace marks the declaration as a
// default that applies to every namespace referencing this SA name unless a
// more specific declaration (same Name, matching Namespace) exists.
type ServiceAccountConfig struct {
	Name             string
	Namespace        string
	ImagePullSecrets []string
}

// DeploymentEntry defines a sub-project entry in root.kdef.
type DeploymentEntry struct {
	Path           string
	Namespace      string
	ServiceAccount string
	Env            string            // per-deployment --env override
	Set            map[string]string // per-deployment --set overrides
}

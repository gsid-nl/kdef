package types

// RootConfig holds the parsed root.kdef configuration.
type RootConfig struct {
	Namespaces      []string
	ServiceAccounts map[string]ServiceAccountConfig
	IngressDefaults *IngressDefaults
	Env             string            // global default --env
	Set             map[string]string // global default --set overrides
	Deployments     map[string]DeploymentEntry
}

// ServiceAccountConfig defines a service account with its imagePullSecrets.
type ServiceAccountConfig struct {
	Name             string
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

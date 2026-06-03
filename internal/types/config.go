package types

// KdefConfig holds the fully parsed configuration from all .kdef files.
type KdefConfig struct {
	Variables       map[string]VariableDecl
	Deployments     []DeploymentConfig
	DaemonSets      []DaemonSetConfig
	StatefulSets    []StatefulSetConfig
	CronJobs        []CronJobConfig
	ConfigMaps              []ConfigMapConfig
	Secrets                 []SecretConfig
	SealedSecrets           []SealedSecretConfig
	PersistentVolumeClaims  []PersistentVolumeClaimConfig
	Namespaces              []string
	ServiceAccounts         []ServiceAccountConfig
	ClusterRoles            []ClusterRoleConfig
	ClusterRoleBindings     []ClusterRoleBindingConfig
	IngressDefaults         *IngressDefaults
	Warnings                []string // non-fatal validation warnings
}

// IngressDefaults defines shared settings inherited by all ingress blocks.
type IngressDefaults struct {
	Annotations map[string]string
	TLS         *bool
	TLSSecret   string
	Issuer      string
	Class       string // ingressClassName default for all ingress blocks
}

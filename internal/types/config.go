package types

// KdefConfig holds the fully parsed configuration from all .kdef files.
type KdefConfig struct {
	Variables       map[string]VariableDecl
	Deployments     []DeploymentConfig
	CronJobs        []CronJobConfig
	ConfigMaps              []ConfigMapConfig
	SealedSecrets           []SealedSecretConfig
	PersistentVolumeClaims  []PersistentVolumeClaimConfig
	Namespaces              []string
	ServiceAccounts         []ServiceAccountConfig
	IngressDefaults         *IngressDefaults
}

// IngressDefaults defines shared settings inherited by all ingress blocks.
type IngressDefaults struct {
	Annotations map[string]string
	TLS         *bool
	TLSSecret   string
	Issuer      string
}

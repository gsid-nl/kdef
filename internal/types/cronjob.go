package types

// CronJobConfig represents a parsed cronjob block.
type CronJobConfig struct {
	Name               string
	ContainerName      string // K8s container name (defaults to cronjob name)
	Namespace          string
	Image              string
	ImagePullPolicy    string
	ImagePullSecrets   []string
	ServiceAccountName string
	Command            []string
	Args               []string
	Schedule           string
	Env                []EnvEntry
	EnvFrom            []EnvFromEntry
	Resources          *ResourcesConfig
	Volumes            []VolumeConfig
	SecurityContext    *SecurityContextConfig
	NodeSelector       map[string]string
	Tolerations        []TolerationConfig
	HostNetwork        bool
	HostPID            bool
	HostIPC            bool
	DNSPolicy          string
	Concurrency        string // "Allow", "Forbid", "Replace"
	Deadline           string // e.g. "4m"
	Restart            string // "OnFailure", "Never" (default: "OnFailure")
}

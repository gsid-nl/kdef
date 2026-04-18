package types

// DaemonSetConfig represents a parsed daemonset block.
// DaemonSets run one pod per node, so there is no replicas field.
type DaemonSetConfig struct {
	Name               string
	Namespace          string
	Labels             map[string]string
	Selector           map[string]string
	ImagePullSecrets   []string
	ServiceAccountName string
	Containers         []ContainerConfig
	InitContainers     []InitContainerConfig
	Volumes            []VolumeConfig
	SecurityContext    *SecurityContextConfig
	NodeSelector       map[string]string
	Tolerations        []TolerationConfig
	HostNetwork        bool
	HostPID            bool
	HostIPC            bool
	DNSPolicy          string
	Service            *ServiceConfig
	Raw                string
}

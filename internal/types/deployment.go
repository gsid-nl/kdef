package types

// DeploymentConfig represents a parsed deployment block.
// This is the new explicit multi-container model.
type DeploymentConfig struct {
	Name               string
	Namespace          string
	Labels             map[string]string
	Selector           map[string]string
	ImagePullSecrets   []string
	ServiceAccountName string
	Replicas           int32
	Containers         []ContainerConfig
	InitContainers     []InitContainerConfig
	Volumes            []VolumeConfig
	SecurityContext    *SecurityContextConfig // pod-level
	Service            *ServiceConfig
	Ingress            *IngressConfig
	Autoscale          *AutoscaleConfig
	Raw                string
}

// ContainerConfig represents a container within a deployment.
type ContainerConfig struct {
	Name            string
	Image           string
	ImagePullPolicy string
	Command         []string
	WorkingDir      string
	Ports           []PortConfig
	Env             []EnvEntry
	EnvFrom         []EnvFromEntry
	Resources       *ResourcesConfig
	Volumes         []VolumeConfig // container-specific volume mounts
	SecurityContext *SecurityContextConfig
}

// ServiceConfig controls the generated Service resource.
type ServiceConfig struct {
	Name  string              // K8s resource name (defaults to deployment name)
	Type  string              // ClusterIP (default), NodePort, LoadBalancer
	Ports []ServicePortConfig // explicit port definitions
}

// ServicePortConfig defines a port on the Service.
type ServicePortConfig struct {
	Number int32  // service port number
	Name   string // port name
	Target int32  // target port (defaults to same as Number)
}

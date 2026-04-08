package types

// AppConfig represents a fully parsed and evaluated app block.
type AppConfig struct {
	Name               string
	Namespace          string
	Labels             map[string]string // custom labels (overrides defaults)
	Selector           map[string]string // custom selector (overrides defaults)
	Image              string
	ImagePullPolicy    string // Always, IfNotPresent, Never
	ImagePullSecrets   []string
	ServiceAccountName string
	Replicas           int32
	Ports              []PortConfig
	Env                []EnvEntry
	EnvFrom            []EnvFromEntry
	Ingress            *IngressConfig
	Resources          *ResourcesConfig
	Volumes            []VolumeConfig
	SecurityContext    *SecurityContextConfig
	InitContainers     []InitContainerConfig
	Sidecars           []SidecarConfig
	Autoscale          *AutoscaleConfig
	Raw                string // raw YAML to deep-merge into the Deployment
}

type AutoscaleConfig struct {
	Min         int32
	Max         int32
	CPUPercent  *int32 // target CPU utilization percentage
	MemPercent  *int32 // target memory utilization percentage
}

type PortConfig struct {
	Number    int32
	Name      string
	Health    string // liveness probe path (httpGet)
	Ready     string // readiness probe path (httpGet)
	TCPHealth bool   // use tcpSocket probe for liveness
	TCPReady  bool   // use tcpSocket probe for readiness
	// Probe tuning (applied to both liveness and readiness)
	InitialDelay *int32
	Period       *int32
	Timeout      *int32
	Failures     *int32
	Successes    *int32
}

type IngressConfig struct {
	Name        string            // K8s resource name (defaults to app name)
	ServiceName string            // backend service name (defaults to app name)
	Port        int32             // backend port number (defaults to first app port)
	Host        string            // single host (shorthand)
	Hosts       []string          // multiple hosts
	TLS         bool
	TLSSecret   string            // existing TLS secret name; if set, no Certificate is generated
	Issuer      string            // cert-manager ClusterIssuer name (default: "letsencrypt-production")
	Annotations map[string]string // nginx/traefik annotations
}

type ResourcesConfig struct {
	CPURequest              string
	CPULimit                string
	MemoryRequest           string
	MemoryLimit             string
	EphemeralStorageRequest string
	EphemeralStorageLimit   string
}

type VolumeConfig struct {
	Name      string
	MountPath string
	SubPath   string // mount a specific key from configmap/secret
	ReadOnly  bool
	// Source — exactly one of these is set
	Secret    string // secret name
	ConfigMap string // configmap name
	EmptyDir  bool
	PVC       string // PersistentVolumeClaim name
	HostPath  string // host path
}

// EnvEntry represents an environment variable — either a plain value, a secret reference, or a configmap reference.
type EnvEntry struct {
	Name          string
	Value         string // plain value (set if SecretName and ConfigMapName are empty)
	SecretName    string // K8s secret name (set for secret refs)
	SecretKey     string // key within the secret
	ConfigMapName string // K8s configmap name (set for configmap refs)
	ConfigMapKey  string // key within the configmap
}

// EnvFromEntry imports all keys from a ConfigMap or Secret as env vars.
type EnvFromEntry struct {
	ConfigMap string // import from configmap
	Secret    string // import from secret
	Prefix    string // optional prefix for all imported keys
}

// InitContainerConfig represents an init container.
type InitContainerConfig struct {
	Name            string
	Image           string
	ImagePullPolicy string
	Command         []string
	Env             []EnvEntry
	EnvFrom         []EnvFromEntry
	VolumeMounts    []string // volume names to mount (inherits mount paths from app volumes)
	SecurityContext *SecurityContextConfig
}

// SidecarConfig represents a sidecar container running alongside the main container.
type SidecarConfig struct {
	Name            string
	Image           string
	Command         []string
	Ports           []PortConfig
	Env             []EnvEntry
	EnvFrom         []EnvFromEntry
	Resources       *ResourcesConfig
	Volumes         []VolumeConfig // sidecar-specific volume mounts
	SecurityContext *SecurityContextConfig
}

type SecurityContextConfig struct {
	RunAsUser    *int64
	RunAsGroup   *int64
	RunAsNonRoot *bool
	ReadOnlyRoot *bool
	FSGroup      *int64
}

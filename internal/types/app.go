package types

import (
	"fmt"
	"strings"
)

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

// AllHosts returns Host and Hosts merged, deduplicated, in declaration order.
func (i IngressConfig) AllHosts() []string {
	var hosts []string
	seen := make(map[string]struct{})
	add := func(h string) {
		if h == "" {
			return
		}
		if _, ok := seen[h]; ok {
			return
		}
		seen[h] = struct{}{}
		hosts = append(hosts, h)
	}
	add(i.Host)
	for _, h := range i.Hosts {
		add(h)
	}
	return hosts
}

// ResourceName returns the effective K8s Ingress resource name for the index-th
// ingress block on a workload. If an explicit `name` is set it wins; otherwise
// the first block uses the workload name and later blocks get a numeric suffix.
func (i IngressConfig) ResourceName(workloadName string, index int) string {
	if i.Name != "" {
		return i.Name
	}
	if index == 0 {
		return workloadName
	}
	return fmt.Sprintf("%s-%d", workloadName, index+1)
}

// CertificateSecretName returns the secret name that the cert-manager Certificate
// generator will pick for this ingress, or "" when no Certificate is generated
// (TLS disabled, an explicit tls_secret was supplied, or no hosts are set).
func (i IngressConfig) CertificateSecretName() string {
	if !i.TLS || i.TLSSecret != "" {
		return ""
	}
	hosts := i.AllHosts()
	if len(hosts) == 0 {
		return ""
	}
	return fmt.Sprintf("%s-tls", strings.ReplaceAll(hosts[0], ".", "-"))
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
	Secret       string // secret name
	ConfigMap    string // configmap name
	EmptyDir     bool
	EmptyDirSize string // size limit for emptyDir (e.g. "1Gi"); ignored unless EmptyDir is true
	PVC          string // PersistentVolumeClaim name
	HostPath     string // host path
	HostPathType string // optional: DirectoryOrCreate, FileOrCreate, Directory, File, Socket, etc.
}

// ProbeConfig represents a Kubernetes probe (liveness, readiness, or startup).
// Exactly one probe handler (HTTPGet/TCPSocket/Exec) should be set.
type ProbeConfig struct {
	// HTTPGet
	HTTPPath string
	HTTPPort int32
	// TCPSocket
	TCPPort int32
	// Exec
	Exec []string
	// Tuning
	InitialDelay *int32
	Period       *int32
	Timeout      *int32
	Failures     *int32
	Successes    *int32
}

// ProbesConfig groups the three container probe types.
type ProbesConfig struct {
	Liveness  *ProbeConfig
	Readiness *ProbeConfig
	Startup   *ProbeConfig
}

// LifecycleHandler represents a single K8s lifecycle hook (preStop or postStart).
// Exactly one of Exec/HTTPPath must be set.
type LifecycleHandler struct {
	Exec     []string
	HTTPPath string
	HTTPPort int32
}

// LifecycleConfig groups the two container lifecycle hook types.
type LifecycleConfig struct {
	PreStop   *LifecycleHandler
	PostStart *LifecycleHandler
}

// EnvEntry represents an environment variable — either a plain value, a secret reference, a configmap reference, or a downward-API field reference.
type EnvEntry struct {
	Name          string
	Value         string // plain value (set if all other sources are empty)
	SecretName    string // K8s secret name (set for secret refs)
	SecretKey     string // key within the secret
	ConfigMapName string // K8s configmap name (set for configmap refs)
	ConfigMapKey  string // key within the configmap
	FieldPath     string // downward API field path (e.g. "spec.nodeName", "metadata.name")
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
	Args            []string
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
	Privileged   *bool // container-level only
}

// TolerationConfig matches Kubernetes Toleration semantics.
type TolerationConfig struct {
	Key               string
	Operator          string // "Equal" (default) or "Exists"
	Value             string
	Effect            string // "NoSchedule", "PreferNoSchedule", "NoExecute"
	TolerationSeconds *int64
}

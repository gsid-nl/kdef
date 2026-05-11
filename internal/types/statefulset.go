package types

// StatefulSetConfig represents a parsed statefulset block.
// StatefulSets manage pods with stable identities and per-pod persistent storage.
type StatefulSetConfig struct {
	Name                string
	Namespace           string
	Labels              map[string]string
	Selector            map[string]string
	ImagePullSecrets    []string
	ServiceAccountName  string
	Replicas            int32
	ServiceName         string // required: name of the governing headless Service
	PodManagementPolicy string // OrderedReady (default) or Parallel
	Containers          []ContainerConfig
	InitContainers      []InitContainerConfig
	Volumes             []VolumeConfig
	VolumeClaims        []VolumeClaimTemplate
	SecurityContext     *SecurityContextConfig
	NodeSelector        map[string]string
	Tolerations         []TolerationConfig
	HostNetwork         bool
	HostPID             bool
	HostIPC             bool
	DNSPolicy           string
	Service             *ServiceConfig
	Ingresses           []IngressConfig
	Raw                 string
}

// VolumeClaimTemplate defines a PVC template for a StatefulSet.
// Each pod gets its own PVC derived from this template.
type VolumeClaimTemplate struct {
	Name         string   // template name (referenced by container volume mounts)
	MountPath    string   // mount path inside the container
	SubPath      string
	ReadOnly     bool
	StorageClass string
	AccessModes  []string // defaults to ["ReadWriteOnce"]
	Storage      string   // e.g. "10Gi"
}

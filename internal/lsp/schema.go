package lsp

import protocol "github.com/tliron/glsp/protocol_3_16"

// AttrSchema describes an attribute within a block.
type AttrSchema struct {
	Name     string
	Doc      string
	Required bool
	Kind     protocol.CompletionItemKind
}

// BlockSchema describes a block type and its children.
type BlockSchema struct {
	Type       string
	Labels     int // number of labels (0 = no labels)
	Doc        string
	Attributes []AttrSchema
	SubBlocks  []BlockSchema
}

// FuncSchema describes a built-in function.
type FuncSchema struct {
	Name      string
	Signature string
	Doc       string
}

var builtinFunctions = []FuncSchema{
	{Name: "secret", Signature: `secret("secret-name", "key")`, Doc: "Reference a Kubernetes Secret key (generates valueFrom.secretKeyRef)"},
	{Name: "configmap", Signature: `configmap("configmap-name", "key")`, Doc: "Reference a ConfigMap key (generates valueFrom.configMapKeyRef)"},
	{Name: "file", Signature: `file("path")`, Doc: "Read file contents (resolved relative to project directory)"},
	{Name: "image", Signature: `image("name")`, Doc: "Resolve an image alias from an images {} block"},
}

var topLevelBlocks = []BlockSchema{
	deploymentSchema,
	daemonsetSchema,
	statefulsetSchema,
	cronjobSchema,
	configmapSchema,
	secretBlockSchema,
	sealedSecretSchema,
	pvcSchema,
	imagesSchema,
}

var deploymentSchema = BlockSchema{
	Type:   "deployment",
	Labels: 1,
	Doc:    "Kubernetes Deployment with containers, service, and ingress",
	Attributes: []AttrSchema{
		{Name: "namespace", Doc: "Kubernetes namespace"},
		{Name: "labels", Doc: "Custom pod labels (map)"},
		{Name: "selector", Doc: "Custom label selector (map)"},
		{Name: "image_pull_secrets", Doc: "List of image pull secret names"},
		{Name: "service_account", Doc: "ServiceAccount name"},
		{Name: "raw", Doc: "Raw YAML to deep-merge into the manifest"},
	},
	SubBlocks: []BlockSchema{
		containerSchema,
		initContainerSchema,
		sidecarSchema,
		scaleSchema,
		volumeSchema,
		securityContextSchema,
		serviceSchema,
		ingressSchema,
		autoscaleSchema,
	},
}

var daemonsetSchema = BlockSchema{
	Type:   "daemonset",
	Labels: 1,
	Doc:    "Kubernetes DaemonSet (one pod per node)",
	Attributes: []AttrSchema{
		{Name: "namespace", Doc: "Kubernetes namespace"},
		{Name: "labels", Doc: "Custom pod labels (map)"},
		{Name: "selector", Doc: "Custom label selector (map)"},
		{Name: "image_pull_secrets", Doc: "List of image pull secret names"},
		{Name: "service_account", Doc: "ServiceAccount name"},
		{Name: "raw", Doc: "Raw YAML to deep-merge into the manifest"},
	},
	SubBlocks: []BlockSchema{
		containerSchema,
		initContainerSchema,
		volumeSchema,
		securityContextSchema,
		serviceSchema,
	},
}

var statefulsetSchema = BlockSchema{
	Type:   "statefulset",
	Labels: 1,
	Doc:    "Kubernetes StatefulSet (stable pod identity and per-pod storage)",
	Attributes: []AttrSchema{
		{Name: "namespace", Doc: "Kubernetes namespace"},
		{Name: "labels", Doc: "Custom pod labels (map)"},
		{Name: "selector", Doc: "Custom label selector (map)"},
		{Name: "image_pull_secrets", Doc: "List of image pull secret names"},
		{Name: "service_account", Doc: "ServiceAccount name"},
		{Name: "service_name", Doc: "Governing headless Service name (required by K8s)"},
		{Name: "pod_management_policy", Doc: "Pod management policy: OrderedReady (default), Parallel"},
		{Name: "raw", Doc: "Raw YAML to deep-merge into the manifest"},
	},
	SubBlocks: []BlockSchema{
		containerSchema,
		initContainerSchema,
		scaleSchema,
		volumeSchema,
		volumeClaimSchema,
		securityContextSchema,
		serviceSchema,
		ingressSchema,
	},
}

var volumeClaimSchema = BlockSchema{
	Type:   "volume_claim",
	Labels: 1,
	Doc:    "StatefulSet volumeClaimTemplate (per-pod PVC)",
	Attributes: []AttrSchema{
		{Name: "mount_path", Doc: "Mount path inside the container", Required: true},
		{Name: "sub_path", Doc: "Subpath within the volume"},
		{Name: "read_only", Doc: "Mount as read-only (bool)"},
		{Name: "storage_class", Doc: "StorageClass name"},
		{Name: "access_modes", Doc: "Access modes (default [\"ReadWriteOnce\"])"},
		{Name: "storage", Doc: "Storage size (e.g. \"10Gi\")", Required: true},
	},
}

var containerSchema = BlockSchema{
	Type:   "container",
	Labels: 1,
	Doc:    "Application container",
	Attributes: []AttrSchema{
		{Name: "image", Doc: "Container image", Required: true},
		{Name: "image_pull_policy", Doc: "Pull policy: IfNotPresent, Always, Never"},
		{Name: "command", Doc: "Container command (list of strings)"},
		{Name: "working_dir", Doc: "Working directory inside the container"},
	},
	SubBlocks: []BlockSchema{
		portSchema,
		envBlockSchema,
		envFromSchema,
		resourcesSchema,
		volumeSchema,
		securityContextSchema,
	},
}

var initContainerSchema = BlockSchema{
	Type:   "init",
	Labels: 1,
	Doc:    "Init container (runs before app containers)",
	Attributes: []AttrSchema{
		{Name: "image", Doc: "Container image", Required: true},
		{Name: "image_pull_policy", Doc: "Pull policy: IfNotPresent, Always, Never"},
		{Name: "command", Doc: "Container command (list of strings)"},
		{Name: "volumes", Doc: "Volume mount names to inherit from deployment"},
	},
	SubBlocks: []BlockSchema{
		envBlockSchema,
		envFromSchema,
		securityContextSchema,
	},
}

var sidecarSchema = BlockSchema{
	Type:   "sidecar",
	Labels: 1,
	Doc:    "Sidecar container",
	Attributes: []AttrSchema{
		{Name: "image", Doc: "Container image", Required: true},
		{Name: "command", Doc: "Container command (list of strings)"},
	},
	SubBlocks: []BlockSchema{
		portSchema,
		envBlockSchema,
		envFromSchema,
		resourcesSchema,
		volumeSchema,
		securityContextSchema,
	},
}

var portSchema = BlockSchema{
	Type:   "port",
	Labels: 2,
	Doc:    "Container port (labels: number, name)",
	Attributes: []AttrSchema{
		{Name: "health", Doc: "HTTP liveness probe path"},
		{Name: "ready", Doc: "HTTP readiness probe path"},
		{Name: "tcp_health", Doc: "Use TCP liveness probe (bool)"},
		{Name: "tcp_ready", Doc: "Use TCP readiness probe (bool)"},
		{Name: "initial_delay", Doc: "Probe initial delay seconds"},
		{Name: "period", Doc: "Probe period seconds"},
		{Name: "timeout", Doc: "Probe timeout seconds"},
		{Name: "failures", Doc: "Failure threshold"},
		{Name: "successes", Doc: "Success threshold"},
	},
}

var envBlockSchema = BlockSchema{
	Type: "env",
	Doc:  "Environment variables (key = value pairs)",
}

var envFromSchema = BlockSchema{
	Type: "env_from",
	Doc:  "Import all keys from a ConfigMap or Secret",
	Attributes: []AttrSchema{
		{Name: "config_map", Doc: "ConfigMap name to import"},
		{Name: "secret", Doc: "Secret name to import"},
		{Name: "prefix", Doc: "Prefix for imported env var names"},
	},
}

var resourcesSchema = BlockSchema{
	Type: "resources",
	Doc:  "CPU, memory, and storage resource requests/limits",
	Attributes: []AttrSchema{
		{Name: "cpu", Doc: `CPU request..limit (e.g. "200m..1000m")`},
		{Name: "memory", Doc: `Memory request..limit (e.g. "256Mi..512Mi")`},
		{Name: "ephemeral_storage", Doc: `Ephemeral storage request..limit (e.g. "1Gi..2Gi")`},
	},
}

var volumeSchema = BlockSchema{
	Type:   "volume",
	Labels: 1,
	Doc:    "Volume mount",
	Attributes: []AttrSchema{
		{Name: "mount_path", Doc: "Mount path inside the container", Required: true},
		{Name: "sub_path", Doc: "Subpath within the volume"},
		{Name: "read_only", Doc: "Mount as read-only (bool)"},
		{Name: "secret", Doc: "Secret name as volume source"},
		{Name: "config_map", Doc: "ConfigMap name as volume source"},
		{Name: "empty_dir", Doc: "Use ephemeral empty directory (bool)"},
		{Name: "pvc", Doc: "PersistentVolumeClaim name"},
		{Name: "host_path", Doc: "Host path to mount"},
	},
}

var securityContextSchema = BlockSchema{
	Type: "security_context",
	Doc:  "Pod or container security context",
	Attributes: []AttrSchema{
		{Name: "run_as_user", Doc: "UID to run as"},
		{Name: "run_as_group", Doc: "GID to run as"},
		{Name: "run_as_non_root", Doc: "Require non-root (bool)"},
		{Name: "read_only_root", Doc: "Read-only root filesystem (bool)"},
		{Name: "fs_group", Doc: "Filesystem group ID"},
	},
}

var scaleSchema = BlockSchema{
	Type: "scale",
	Doc:  "Replica count",
	Attributes: []AttrSchema{
		{Name: "replicas", Doc: "Number of replicas", Required: true},
	},
}

var serviceSchema = BlockSchema{
	Type: "service",
	Doc:  "Kubernetes Service (omit for worker-style deployments)",
	Attributes: []AttrSchema{
		{Name: "name", Doc: "Service resource name (defaults to deployment name)"},
		{Name: "type", Doc: "Service type: ClusterIP, NodePort, LoadBalancer"},
	},
	SubBlocks: []BlockSchema{
		servicePortSchema,
	},
}

var servicePortSchema = BlockSchema{
	Type:   "port",
	Labels: 2,
	Doc:    "Service port (labels: number, name)",
	Attributes: []AttrSchema{
		{Name: "target", Doc: "Target port number (defaults to port number)"},
	},
}

var ingressSchema = BlockSchema{
	Type: "ingress",
	Doc:  "Kubernetes Ingress",
	Attributes: []AttrSchema{
		{Name: "name", Doc: "Ingress resource name"},
		{Name: "service_name", Doc: "Backend service name"},
		{Name: "port", Doc: "Backend port number"},
		{Name: "host", Doc: "Single hostname"},
		{Name: "hosts", Doc: "Multiple hostnames (list)"},
		{Name: "tls", Doc: "Enable TLS (bool)"},
		{Name: "tls_secret", Doc: "TLS secret name"},
		{Name: "issuer", Doc: "Cert-manager issuer name"},
		{Name: "annotations", Doc: "Ingress annotations (map, supports nesting)"},
	},
}

var autoscaleSchema = BlockSchema{
	Type: "autoscale",
	Doc:  "Horizontal Pod Autoscaler",
	Attributes: []AttrSchema{
		{Name: "min", Doc: "Minimum replicas", Required: true},
		{Name: "max", Doc: "Maximum replicas", Required: true},
		{Name: "cpu", Doc: "Target CPU utilization %"},
		{Name: "memory", Doc: "Target memory utilization %"},
	},
}

var cronjobSchema = BlockSchema{
	Type:   "cronjob",
	Labels: 1,
	Doc:    "Kubernetes CronJob",
	Attributes: []AttrSchema{
		{Name: "image", Doc: "Container image", Required: true},
		{Name: "schedule", Doc: "Cron schedule expression", Required: true},
		{Name: "namespace", Doc: "Kubernetes namespace"},
		{Name: "image_pull_policy", Doc: "Pull policy: IfNotPresent, Always, Never"},
		{Name: "image_pull_secrets", Doc: "List of image pull secret names"},
		{Name: "service_account", Doc: "ServiceAccount name"},
		{Name: "container_name", Doc: "Custom container name (defaults to cronjob name)"},
		{Name: "command", Doc: "Container command (list of strings)"},
		{Name: "concurrency", Doc: "Concurrency policy: Allow, Forbid, Replace"},
		{Name: "deadline", Doc: "Starting deadline (e.g. \"4m\")"},
		{Name: "restart", Doc: "Restart policy: OnFailure, Never"},
	},
	SubBlocks: []BlockSchema{
		envBlockSchema,
		envFromSchema,
		resourcesSchema,
		volumeSchema,
		securityContextSchema,
	},
}

var configmapSchema = BlockSchema{
	Type:   "configmap",
	Labels: 1,
	Doc:    "Kubernetes ConfigMap",
	Attributes: []AttrSchema{
		{Name: "namespace", Doc: "Kubernetes namespace"},
		{Name: "data", Doc: "Key-value data (supports file() function)"},
	},
}

var secretBlockSchema = BlockSchema{
	Type:   "secret",
	Labels: 1,
	Doc:    "Kubernetes Secret",
	Attributes: []AttrSchema{
		{Name: "namespace", Doc: "Kubernetes namespace"},
		{Name: "type", Doc: "Secret type (default: Opaque)"},
		{Name: "data", Doc: "Key-value secret data"},
	},
}

var sealedSecretSchema = BlockSchema{
	Type:   "sealedsecret",
	Labels: 1,
	Doc:    "Bitnami SealedSecret (encrypted, safe to commit)",
	Attributes: []AttrSchema{
		{Name: "namespace", Doc: "Kubernetes namespace"},
		{Name: "type", Doc: "Secret type (default: Opaque)"},
		{Name: "data", Doc: "Encrypted key-value data"},
	},
}

var pvcSchema = BlockSchema{
	Type:   "persistentvolumeclaim",
	Labels: 1,
	Doc:    "Kubernetes PersistentVolumeClaim",
	Attributes: []AttrSchema{
		{Name: "namespace", Doc: "Kubernetes namespace"},
		{Name: "storage_class", Doc: "StorageClass name"},
		{Name: "access_modes", Doc: "Access modes (e.g. [\"ReadWriteOnce\"])"},
		{Name: "storage", Doc: "Storage size (e.g. \"10Gi\")", Required: true},
	},
}

var imagesSchema = BlockSchema{
	Type: "images",
	Doc:  "Image registry (name = \"registry/image:tag\" pairs)",
}

// blockSchemaMap provides quick lookup by block type name.
var blockSchemaMap map[string]*BlockSchema

func init() {
	blockSchemaMap = make(map[string]*BlockSchema)
	registerBlock(&deploymentSchema)
	registerBlock(&daemonsetSchema)
	registerBlock(&statefulsetSchema)
	registerBlock(&volumeClaimSchema)
	registerBlock(&cronjobSchema)
	registerBlock(&configmapSchema)
	registerBlock(&secretBlockSchema)
	registerBlock(&sealedSecretSchema)
	registerBlock(&pvcSchema)
	registerBlock(&imagesSchema)
	registerBlock(&containerSchema)
	registerBlock(&initContainerSchema)
	registerBlock(&sidecarSchema)
	registerBlock(&scaleSchema)
	registerBlock(&serviceSchema)
	registerBlock(&ingressSchema)
	registerBlock(&autoscaleSchema)
	registerBlock(&portSchema)
	registerBlock(&envBlockSchema)
	registerBlock(&envFromSchema)
	registerBlock(&resourcesSchema)
	registerBlock(&volumeSchema)
	registerBlock(&securityContextSchema)
}

func registerBlock(b *BlockSchema) {
	blockSchemaMap[b.Type] = b
}

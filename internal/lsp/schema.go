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
	{Name: "field_ref", Signature: `field_ref("spec.nodeName")`, Doc: "Downward-API reference (generates valueFrom.fieldRef)"},
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
	clusterRoleSchema,
	clusterRoleBindingSchema,
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
		{Name: "node_selector", Doc: "Node labels required for scheduling (map)"},
		{Name: "host_network", Doc: "Share the host network namespace (bool)"},
		{Name: "host_pid", Doc: "Share the host PID namespace (bool)"},
		{Name: "host_ipc", Doc: "Share the host IPC namespace (bool)"},
		{Name: "dns_policy", Doc: "DNS policy: ClusterFirst (default), ClusterFirstWithHostNet, Default, None"},
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
		tolerationSchema,
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
		{Name: "node_selector", Doc: "Node labels required for scheduling (map)"},
		{Name: "host_network", Doc: "Share the host network namespace (bool)"},
		{Name: "host_pid", Doc: "Share the host PID namespace (bool)"},
		{Name: "host_ipc", Doc: "Share the host IPC namespace (bool)"},
		{Name: "dns_policy", Doc: "DNS policy: ClusterFirst (default), ClusterFirstWithHostNet, Default, None"},
		{Name: "raw", Doc: "Raw YAML to deep-merge into the manifest"},
	},
	SubBlocks: []BlockSchema{
		containerSchema,
		initContainerSchema,
		volumeSchema,
		securityContextSchema,
		serviceSchema,
		tolerationSchema,
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
		{Name: "node_selector", Doc: "Node labels required for scheduling (map)"},
		{Name: "host_network", Doc: "Share the host network namespace (bool)"},
		{Name: "host_pid", Doc: "Share the host PID namespace (bool)"},
		{Name: "host_ipc", Doc: "Share the host IPC namespace (bool)"},
		{Name: "dns_policy", Doc: "DNS policy: ClusterFirst (default), ClusterFirstWithHostNet, Default, None"},
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
		tolerationSchema,
	},
}

var tolerationSchema = BlockSchema{
	Type: "toleration",
	Doc:  "Pod toleration for node taints",
	Attributes: []AttrSchema{
		{Name: "key", Doc: "Taint key to tolerate"},
		{Name: "operator", Doc: `"Equal" (default) or "Exists"`},
		{Name: "value", Doc: "Taint value (for Equal operator)"},
		{Name: "effect", Doc: `"NoSchedule", "PreferNoSchedule", "NoExecute"`},
		{Name: "toleration_seconds", Doc: "How long to tolerate NoExecute (seconds)"},
	},
}

var clusterRoleSchema = BlockSchema{
	Type:   "clusterrole",
	Labels: 1,
	Doc:    "Kubernetes ClusterRole (cluster-scoped RBAC)",
	SubBlocks: []BlockSchema{
		policyRuleSchema,
	},
}

var policyRuleSchema = BlockSchema{
	Type: "rule",
	Doc:  "ClusterRole / Role policy rule",
	Attributes: []AttrSchema{
		{Name: "api_groups", Doc: "API groups (use [\"\"] for core)"},
		{Name: "resources", Doc: "Resource types (e.g. [\"pods\", \"nodes\"])"},
		{Name: "resource_names", Doc: "Optional named resource restrictions"},
		{Name: "verbs", Doc: "Verbs (e.g. [\"get\", \"list\", \"watch\"])", Required: true},
		{Name: "non_resource_urls", Doc: "Non-resource URLs (e.g. [\"/metrics\"])"},
	},
}

var clusterRoleBindingSchema = BlockSchema{
	Type:   "clusterrolebinding",
	Labels: 1,
	Doc:    "Kubernetes ClusterRoleBinding",
	SubBlocks: []BlockSchema{
		roleRefSchema,
		subjectSchema,
	},
}

var roleRefSchema = BlockSchema{
	Type: "role_ref",
	Doc:  "Reference to a ClusterRole or Role",
	Attributes: []AttrSchema{
		{Name: "kind", Doc: `"ClusterRole" (default) or "Role"`},
		{Name: "name", Doc: "Role name", Required: true},
	},
}

var subjectSchema = BlockSchema{
	Type: "subject",
	Doc:  "Binding subject (ServiceAccount, User, or Group)",
	Attributes: []AttrSchema{
		{Name: "kind", Doc: `"ServiceAccount" (default), "User", or "Group"`},
		{Name: "name", Doc: "Subject name", Required: true},
		{Name: "namespace", Doc: "Namespace (required for ServiceAccount kind)"},
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
		{Name: "command", Doc: "Container command (list of strings, overrides ENTRYPOINT)"},
		{Name: "args", Doc: "Container args (list of strings, overrides CMD)"},
		{Name: "working_dir", Doc: "Working directory inside the container"},
	},
	SubBlocks: []BlockSchema{
		portSchema,
		envBlockSchema,
		envFromSchema,
		resourcesSchema,
		volumeSchema,
		securityContextSchema,
		probesSchema,
		lifecycleSchema,
	},
}

var probeHandlerAttrs = []AttrSchema{
	{Name: "exec", Doc: "Exec probe command (list of strings)"},
	{Name: "tcp_socket_port", Doc: "TCP socket probe target port"},
	{Name: "initial_delay", Doc: "Seconds before first probe (initialDelaySeconds)"},
	{Name: "period", Doc: "Seconds between probes (periodSeconds)"},
	{Name: "timeout", Doc: "Per-probe timeout in seconds (timeoutSeconds)"},
	{Name: "failure_threshold", Doc: "Consecutive failures before restart (failureThreshold)"},
	{Name: "success_threshold", Doc: "Consecutive successes required (successThreshold)"},
}

var httpGetSchema = BlockSchema{
	Type: "http_get",
	Doc:  "HTTP GET probe handler",
	Attributes: []AttrSchema{
		{Name: "path", Doc: "HTTP path", Required: true},
		{Name: "port", Doc: "HTTP port", Required: true},
	},
}

var livenessProbeSchema = BlockSchema{
	Type:       "liveness",
	Doc:        "Liveness probe",
	Attributes: probeHandlerAttrs,
	SubBlocks:  []BlockSchema{httpGetSchema},
}

var readinessProbeSchema = BlockSchema{
	Type:       "readiness",
	Doc:        "Readiness probe",
	Attributes: probeHandlerAttrs,
	SubBlocks:  []BlockSchema{httpGetSchema},
}

var startupProbeSchema = BlockSchema{
	Type:       "startup",
	Doc:        "Startup probe",
	Attributes: probeHandlerAttrs,
	SubBlocks:  []BlockSchema{httpGetSchema},
}

var probesSchema = BlockSchema{
	Type: "probes",
	Doc:  "Container probes: liveness, readiness, startup",
	SubBlocks: []BlockSchema{
		livenessProbeSchema,
		readinessProbeSchema,
		startupProbeSchema,
	},
}

var lifecycleHandlerAttrs = []AttrSchema{
	{Name: "exec", Doc: "Exec command (list of strings)"},
}

var preStopSchema = BlockSchema{
	Type:       "pre_stop",
	Doc:        "preStop lifecycle hook",
	Attributes: lifecycleHandlerAttrs,
	SubBlocks:  []BlockSchema{httpGetSchema},
}

var postStartSchema = BlockSchema{
	Type:       "post_start",
	Doc:        "postStart lifecycle hook",
	Attributes: lifecycleHandlerAttrs,
	SubBlocks:  []BlockSchema{httpGetSchema},
}

var lifecycleSchema = BlockSchema{
	Type: "lifecycle",
	Doc:  "Container lifecycle hooks",
	SubBlocks: []BlockSchema{
		preStopSchema,
		postStartSchema,
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
		{Name: "args", Doc: "Container args (list of strings)"},
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
		{Name: "size_limit", Doc: `Size limit for emptyDir (e.g. "1Gi"); ignored unless empty_dir is true`},
		{Name: "pvc", Doc: "PersistentVolumeClaim name"},
		{Name: "host_path", Doc: "Host path to mount"},
		{Name: "host_path_type", Doc: `Host path type: "DirectoryOrCreate", "FileOrCreate", "Directory", "File", "Socket", etc.`},
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
		{Name: "fs_group", Doc: "Filesystem group ID (pod-level)"},
		{Name: "privileged", Doc: "Run container in privileged mode (bool, container-level)"},
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
	Doc:  "Kubernetes Ingress. Repeatable per workload — each `ingress {}` block becomes its own Ingress resource (and Certificate when `tls = true` without `tls_secret`). Resource name defaults to the workload name on the first block and is auto-suffixed (-2, -3, ...) on later blocks.",
	Attributes: []AttrSchema{
		{Name: "name", Doc: "Ingress resource name (overrides the auto-derived name)"},
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
		{Name: "args", Doc: "Container args (list of strings)"},
		{Name: "node_selector", Doc: "Node labels required for scheduling (map)"},
		{Name: "host_network", Doc: "Share the host network namespace (bool)"},
		{Name: "host_pid", Doc: "Share the host PID namespace (bool)"},
		{Name: "host_ipc", Doc: "Share the host IPC namespace (bool)"},
		{Name: "dns_policy", Doc: "DNS policy: ClusterFirst (default), ClusterFirstWithHostNet, Default, None"},
		{Name: "concurrency", Doc: "Concurrency policy: Allow, Forbid, Replace"},
		{Name: "deadline", Doc: "Starting deadline (e.g. \"4m\")"},
		{Name: "restart", Doc: "Restart policy: OnFailure, Never"},
		{Name: "suspend", Doc: "If true, Kubernetes skips scheduled runs (bool; default false)"},
	},
	SubBlocks: []BlockSchema{
		envBlockSchema,
		envFromSchema,
		resourcesSchema,
		volumeSchema,
		securityContextSchema,
		tolerationSchema,
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
	registerBlock(&tolerationSchema)
	registerBlock(&clusterRoleSchema)
	registerBlock(&clusterRoleBindingSchema)
	registerBlock(&policyRuleSchema)
	registerBlock(&roleRefSchema)
	registerBlock(&subjectSchema)
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
	registerBlock(&probesSchema)
	registerBlock(&livenessProbeSchema)
	registerBlock(&readinessProbeSchema)
	registerBlock(&startupProbeSchema)
	registerBlock(&httpGetSchema)
	registerBlock(&lifecycleSchema)
	registerBlock(&preStopSchema)
	registerBlock(&postStartSchema)
}

func registerBlock(b *BlockSchema) {
	blockSchemaMap[b.Type] = b
}

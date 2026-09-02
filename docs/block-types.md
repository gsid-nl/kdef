# Block Types

## `root.kdef` — Multi-Project Root

The `root.kdef` file is the entry point for multi-app repositories. It defines namespaces, service accounts, ingress defaults, and lists all sub-projects with their configuration.

```hcl
# root.kdef

namespaces = ["production", "staging"]

service_account "default" {
  image_pull_secrets = ["registrykey"]
}

# Optional: scope a service_account to a specific namespace.
# When a scoped block exists, it wins over the default for that namespace.
# Useful when the same SA name (e.g. "default") needs different
# image_pull_secrets per namespace.
service_account "default" {
  namespace          = "monitoring"
  image_pull_secrets = ["monitoring-registry"]
}

ingress_defaults {
  tls    = true
  issuer = "letsencrypt-production"
  class  = "nginx"                     # default ingressClassName for every ingress block
  annotations = {
    "nginx.ingress.kubernetes.io" = {
      "force-ssl-redirect" = "true"
    }
  }
}

# Global defaults (overridable per deployment and via CLI)
env = "production"
set = { "replicas" = "3" }

deployments = {
  "api" = {
    path            = "api"
    namespace       = "production"
    service_account = "default"
  }
  "api-staging" = {
    path            = "api"           # same app, different config
    namespace       = "staging"
    service_account = "default"
    env             = "staging"       # overrides global env
    set             = { "replicas" = "1" }
  }
  "worker" = {
    path      = "worker"
    namespace = "production"
  }
}
```

```
repo/
  root.kdef
  api/
    vars.kdef
    app.kdef
  worker/
    app.kdef
```

### Generated resources

- **Namespace** manifests for each entry in `namespaces`
- **ServiceAccount** manifests (with `imagePullSecrets`) in each namespace where they're referenced

### Namespace and service account injection

- `namespace` from a deployment entry is injected into all blocks (deployments, cronjobs, configmaps, secrets, etc.) that don't already specify one
- `service_account` is injected into deployments and cronjobs that don't already specify one
- Individual `.kdef` files can override the namespace, but it must be in the `namespaces` list
- Every resource must have a namespace — either from root.kdef or the `.kdef` file

### Override precedence (highest wins)

| Setting | CLI flags | Per-deployment in root.kdef | Global in root.kdef |
|---------|-----------|----------------------------|---------------------|
| `env`   | `--env`   | `env = "staging"`          | `env = "production"`|
| `set`   | `--set`   | `set = { ... }`            | `set = { ... }`     |

### Validation

- Namespace must be in the `namespaces` list (if defined)
- Service account must be defined as a `service_account` block (if referenced)
- Every resource must have a namespace — missing namespace is an error

All CLI commands work transparently:

```bash
kdef render --dir repo              # renders all apps
kdef validate --dir repo            # validates all apps
kdef diff --dir repo --env staging  # diffs all apps against cluster
kdef apply --dir repo --env prod    # applies all apps
```

### `ingress_defaults` — shared ingress settings

`ingress_defaults` is a root-level block (valid in `root.kdef`, `vars.kdef`, or an imported file). Every `ingress {}` block inherits its values; anything set on the ingress block itself wins.

| Attribute | Description |
| --- | --- |
| `mode` | `"ingress"` (default) or `"gateway"` — see below |
| `class` | Default `ingressClassName` (classic mode, defaults to `nginx`) |
| `gateway` | Gateway resource name (gateway mode) |
| `gateway_namespace` | Namespace of the Gateway resource (gateway mode, optional) |
| `tls` | Enable TLS on all ingress blocks |
| `tls_secret` | Default TLS secret name |
| `issuer` | Default cert-manager issuer |
| `annotations` | Default annotations, merged with per-ingress annotations (supports nesting) |

#### Ingress modes

With the default `mode = "ingress"`, each `ingress {}` block renders a classic `networking.k8s.io/v1` Ingress.

With `mode = "gateway"`, the same `ingress {}` blocks render Gateway API `HTTPRoute` resources (`gateway.networking.k8s.io/v1`) attached to the Gateway named by `gateway`. Hosts become `hostnames`, the backend service and port become the rule's `backendRef`, and annotations are copied onto the HTTPRoute. Certificates are still generated from `tls`/`issuer` as usual.

```hcl
# root.kdef

ingress_defaults {
  mode              = "gateway"
  gateway           = "public-gateway"
  gateway_namespace = "gateway-system"
  tls               = true
  issuer            = "letsencrypt-production"
}
```

The mode is repo-wide: switching it re-renders every ingress in the tree, so no `.kdef` app files need to change.

---

## `deployment` — Kubernetes Deployment

The primary block type. Defines a Deployment with explicit containers, an optional Service, and zero or more Ingresses (one resource per `ingress {}` block).

```hcl
deployment "web" {
  namespace          = "production"
  image_pull_secrets = ["registry-secret"]
  service_account    = "web-sa"

  # Custom labels and selector (for existing deployments)
  selector = { "app" = "web" }
  labels   = { "app" = "web" }

  scale {
    replicas = 3
  }

  # --- Containers ---

  container "web" {
    image             = "my-registry/web:${var.image_tag}"
    image_pull_policy = "Always"
    working_dir       = "/app"

    port "8080" "http" {
      health        = "/health"     # HTTP liveness probe
      ready         = "/ready"      # HTTP readiness probe
      initial_delay = 5
      period        = 10
    }

    # Or use TCP probes
    port "9000" "grpc" {
      tcp_health = true
      tcp_ready  = true
    }

    # Explicit probes (liveness/readiness/startup) with exec, TCP, or HTTP.
    # When present, these override any port-shorthand probes above.
    probes {
      liveness {
        exec          = ["pgrep", "worker"]
        initial_delay = 30
        period        = 10
      }
      readiness {
        http_get {
          path = "/ready"
          port = 8080
        }
      }
      startup {
        tcp_socket_port   = 8080
        failure_threshold = 30
      }
    }

    # Container lifecycle hooks
    lifecycle {
      pre_stop {
        exec = ["/bin/sh", "-c", "sleep 10"]
      }
      # post_start { http_get { path = "/warmup" port = 8080 } }
    }

    env {
      APP_ENV      = var.environment
      DATABASE_URL = secret("db-credentials", "url")
    }

    env_from {
      config_map = "app-config"
    }

    env_from {
      secret = "app-secrets"
      prefix = "SECRET_"
    }

    resources {
      cpu               = "200m..1000m"    # request..limit
      memory            = "256Mi..512Mi"
      ephemeral_storage = "1Gi..2Gi"
    }

    volume "config" {
      mount_path = "/etc/app/config.yaml"
      sub_path   = "config.yaml"
      config_map = "app-config"
    }

    volume "data" {
      mount_path = "/data"
      host_path  = "/mnt/data"
    }

    volume "cache" {
      mount_path = "/tmp"
      empty_dir  = true
      size_limit = "1Gi"              # optional: emptyDir sizeLimit
    }

    volume "certs" {
      mount_path = "/etc/tls"
      secret     = "tls-certs"
      read_only  = true
    }

    volume "storage" {
      mount_path = "/var/storage"
      pvc        = "my-pvc"
    }

    security_context {
      run_as_user     = 1000
      run_as_group    = 1000
      run_as_non_root = true
      read_only_root  = true
    }
  }

  container "nginx" {
    image = "nginx:stable-alpine"

    port "80" "http" {
      tcp_health = true
    }

    resources {
      cpu    = "100m..500m"
      memory = "64Mi..256Mi"
    }

    volume "shared" {
      mount_path = "/var/www/html"
      empty_dir  = true
    }
  }

  # --- Init Containers ---

  init "warmup" {
    image             = "my-registry/web:${var.image_tag}"
    image_pull_policy = "Always"
    command = [
      "/bin/sh",
      "-c",
      "php bin/console cache:warmup",
    ]
    volumes = ["shared"]    # reference deployment-level volumes by name

    env_from {
      config_map = "app-config"
    }

    security_context {
      run_as_user = 0
    }
  }

  # --- Pod-level Security Context ---

  security_context {
    fs_group = 1000
  }

  # --- Service ---

  service {
    name = "web-svc"                # defaults to deployment name
    port "80" "http" {}             # port 80, targetPort 80
    port "443" "https" {
      target = 8443                 # port 443 → targetPort 8443
    }
  }

  # --- Ingress ---
  #
  # A deployment may have one or more `ingress {}` blocks. Each block produces
  # one Kubernetes Ingress resource (and, when `tls = true` without an explicit
  # `tls_secret`, one cert-manager Certificate). With
  # `ingress_defaults { mode = "gateway" }` the same blocks produce Gateway API
  # HTTPRoutes instead — see "Ingress modes" above.
  #
  # Resource naming rules:
  #   - First block, no `name`  →  uses the deployment name (e.g. "web")
  #   - Later blocks, no `name` →  appended with -2, -3, ... (e.g. "web-2")
  #   - Set `name = "..."` to override explicitly
  #
  # kdef rejects (at parse time) any two ingresses whose resolved resource
  # names — or whose derived cert-manager Certificate secret names — would
  # collide within the same namespace. If you hit a Certificate collision,
  # either change the first host of one block or set `tls_secret` to BYO the
  # secret and skip Certificate generation.

  ingress {
    name         = "web"                # K8s resource name (override)
    service_name = "web-svc"            # backend service
    port         = 80                   # backend port
    host         = "web.example.com"
    class        = "nginx"              # ingressClassName (defaults to "nginx")
    tls          = true
    tls_secret   = "web-tls"            # existing TLS secret
    # Or use cert-manager:
    # issuer = "letsencrypt-production"

    annotations = {
      "nginx.ingress.kubernetes.io" = {
        "proxy-body-size"    = "50m"
        "ssl-redirect"       = "true"
        "proxy-read-timeout" = "120"
      }
    }
  }

  # A second ingress on the same deployment — e.g. an apex → www redirect.
  # Resource name is auto-suffixed: this becomes "web-2".
  ingress {
    host = "example.com"
    annotations = {
      "nginx.ingress.kubernetes.io/permanent-redirect" = "https://www.example.com$request_uri"
    }
  }

  # Multiple hosts on a single Ingress resource.
  # Resource name is auto-suffixed: "web-3".
  ingress {
    hosts = [
      "web.example.com",
      "www.example.com",
      "app.example.com",
    ]
    tls        = true
    tls_secret = "wildcard-tls"
  }

  # --- Autoscaling (HPA) ---

  autoscale {
    min = 2
    max = 10
    cpu = 70       # target CPU utilization %
    memory = 80    # target memory utilization %
  }

  # --- Rollout & history ---

  rollout {
    revision_history_limit = 3             # old ReplicaSets to keep (K8s default: 10)
    strategy               = "RollingUpdate" # or "Recreate"
    max_surge              = "25%"         # int or percentage string
    max_unavailable        = 1
    progress_deadline      = 600           # seconds before rollout is marked failed
  }

  # --- Raw YAML Escape Hatch ---

  raw = <<-EOT
    spec:
      template:
        spec:
          terminationGracePeriodSeconds: 60
          topologySpreadConstraints:
          - maxSkew: 1
            topologyKey: kubernetes.io/hostname
            whenUnsatisfiable: DoNotSchedule
  EOT
}
```

**No `service {}` block = no Service generated** (worker-style deployment):

```hcl
deployment "queue-consumer" {
  container "consumer" {
    image   = "my-registry/api:${var.image_tag}"
    command = ["php", "bin/console", "messenger:consume", "async"]
  }

  scale {
    replicas = 2
  }
}
```

## `daemonset` — Kubernetes DaemonSet

Runs one pod per node. Useful for log collectors, metrics agents, and node-level networking components.

The `daemonset` block accepts the same `container`, `init`, `volume`, `security_context`, `service`, and `toleration` sub-blocks as `deployment`, plus an optional top-level `node_selector`. It does **not** support `scale`, `autoscale`, or `ingress`.

```hcl
daemonset "promtail" {
  namespace       = "logs"
  service_account = "promtail"

  container "promtail" {
    image = "grafana/promtail:2.9.3"
    args  = ["-config.file=/etc/promtail/promtail.yaml"]

    port "9080" "http-metrics" {}

    env {
      HOSTNAME = field_ref("spec.nodeName")
    }

    volume "config" {
      mount_path = "/etc/promtail"
      config_map = "promtail-config"
    }

    volume "run" {
      mount_path     = "/run/promtail"
      host_path      = "/run/promtail"
      host_path_type = "DirectoryOrCreate"
    }

    volume "varlog" {
      mount_path = "/var/log"
      host_path  = "/var/log"
      read_only  = true
    }

    security_context {
      privileged  = true
      run_as_user = 0
    }

    resources {
      cpu    = "50m..200m"
      memory = "128Mi..256Mi"
    }
  }

  toleration { effect = "NoSchedule" operator = "Exists" }
  toleration { effect = "NoExecute"  operator = "Exists" }
}
```

## `statefulset` — Kubernetes StatefulSet

Manages pods with stable identities (`pod-0`, `pod-1`, …) and per-pod persistent storage via `volume_claim` templates. Use for databases, message brokers, and other stateful workloads.

A StatefulSet requires a `service_name` pointing at a governing headless Service. If you declare a `service` block whose name matches `service_name`, kdef emits it as a headless Service (`clusterIP: None`).

```hcl
statefulset "postgres" {
  namespace    = "production"
  service_name = "postgres"

  scale {
    replicas = 3
  }

  container "postgres" {
    image = "postgres:16"

    port "5432" "pg" {
      tcp_ready = true
    }

    env {
      POSTGRES_PASSWORD = secret("postgres", "password")
    }

    volume "data" {
      mount_path = "/var/lib/postgresql/data"
    }
  }

  # Per-pod persistent storage
  volume_claim "data" {
    mount_path    = "/var/lib/postgresql/data"
    storage       = "50Gi"
    storage_class = "fast-ssd"
    access_modes  = ["ReadWriteOnce"]
  }

  # Governing headless service (matches service_name → emitted as ClusterIP=None)
  service {
    port "5432" "pg" {}
  }
}
```

### Attributes

| Attribute               | Description                                                     |
|-------------------------|-----------------------------------------------------------------|
| `service_name`          | Name of the governing headless Service (required by Kubernetes)|
| `pod_management_policy` | `OrderedReady` (default) or `Parallel`                          |

### `volume_claim` block

Declares a `volumeClaimTemplates` entry. Each replica gets its own PVC named `<template>-<sts>-<ordinal>`.

| Attribute        | Required | Description                           |
|------------------|----------|---------------------------------------|
| `mount_path`     | yes      | Mount path inside the container       |
| `storage`        | yes      | Storage size (e.g. `"10Gi"`)          |
| `storage_class`  |          | StorageClass name                     |
| `access_modes`   |          | Defaults to `["ReadWriteOnce"]`       |
| `sub_path`       |          | Subpath within the volume             |
| `read_only`      |          | Mount as read-only                    |

## `cronjob` — Kubernetes CronJob

```hcl
cronjob "send-reminders" {
  namespace          = "production"
  schedule           = "*/5 * * * *"
  image              = "my-registry/api:${var.image_tag}"
  image_pull_policy  = "Always"
  image_pull_secrets = ["regcred"]
  container_name     = "my-custom-name"    # defaults to cronjob name

  command = [
    "/bin/sh",
    "-c",
    "php bin/console app:send-reminders",
  ]

  concurrency = "Forbid"    # Allow, Forbid, Replace
  deadline    = "4m"         # startingDeadlineSeconds
  restart     = "OnFailure"  # OnFailure (default), Never
  suspend     = false        # when true, k8s skips scheduled runs (default false)

  env {
    APP_NAME = "reminder-worker"
  }

  env_from {
    config_map = "app-config"
  }

  resources {
    cpu    = "100m..500m"
    memory = "128Mi..256Mi"
  }
}
```

## `configmap` — Kubernetes ConfigMap

```hcl
configmap "app-config" {
  namespace = "production"

  data = {
    "APP_ENV"  = "production"
    "APP_NAME" = "my-app"
  }
}

# Load files from disk
configmap "nginx-config" {
  namespace = "production"

  data = {
    "nginx.conf" = file("configs/nginx.conf")
  }
}
```

## `ingress` — Standalone Ingress

An `ingress` block can also stand on its own at the top level, outside any workload. This is the same block as the nested `ingress {}` inside a deployment, with two differences: it takes a label, and it carries its own namespace.

Use it when ingresses outnumber workloads, or when the list of hosts is data rather than something hand-written: many domains pointing at a single service, a tenant list generated from a database and committed as a values file. An Ingress only names a Service in Kubernetes, so nothing about it requires a workload block to sit in.

```hcl
ingress "cms-gsid-nl" {
  namespace    = "cms"                # optional; injected from root.kdef when omitted
  service_name = "cms-public-svc"     # required
  port         = 80                   # required
  hosts        = ["gsid.nl", "www.gsid.nl"]
  tls          = true
}
```

Rules specific to the standalone form:

- **The label is the resource name.** An explicit `name = "..."` wins over the label, which is how you derive a name inside a `for` loop: HCL does not allow interpolation in block labels.
- **`service_name` and `port` are required.** A nested block borrows both from its workload; a standalone block has nothing to borrow from, so kdef errors at parse time rather than silently emitting a `:80` backend.
- **`namespace` follows the usual injection rules.** Set it on the block, or let the `namespace` from the block's `root.kdef` entry fill it in.
- Everything else — `tls`, `tls_secret`, `issuer`, `class`, `annotations`, `host`/`hosts` — behaves exactly as in the nested form, `ingress_defaults` included.
- The `app.kubernetes.io/name` label on the generated Ingress and Certificate is set to `service_name`, so a standalone ingress still groups with the workload it fronts.
- Collision detection is shared with nested blocks: two ingresses that resolve to the same resource name, or that would derive the same cert-manager Certificate secret name, are rejected within a namespace regardless of which form they were written in.

Combined with `for`, this is how a generated domain list becomes ingresses — see [conditionals-and-loops.md](conditionals-and-loops.md).

`kdef import` still emits nested `ingress {}` blocks under the workload owning the matching Service; it does not produce standalone blocks.

## `secret` — Kubernetes Secret

Define plain Kubernetes Secrets. Generates `v1/Secret` manifests using `stringData` (Kubernetes handles the base64 encoding).

```hcl
secret "db-credentials" {
  namespace = "production"
  type      = "Opaque"    # optional, defaults to "Opaque"

  data = {
    username = "admin"
    password = var.db_password
  }
}
```

Supports all standard Kubernetes secret types:

| Type | Description |
|------|-------------|
| `Opaque` | Default, arbitrary key-value data |
| `kubernetes.io/tls` | TLS certificate and key |
| `kubernetes.io/dockerconfigjson` | Docker registry credentials |
| `kubernetes.io/basic-auth` | Basic authentication credentials |
| `kubernetes.io/ssh-auth` | SSH authentication credentials |

Load file contents with `file()`:

```hcl
secret "tls-certs" {
  namespace = "production"
  type      = "kubernetes.io/tls"

  data = {
    "tls.crt" = file("certs/server.crt")
    "tls.key" = file("certs/server.key")
  }
}
```

Pairs naturally with `secret()` references in deployments:

```hcl
secret "db-credentials" {
  namespace = "production"
  data = {
    DATABASE_URL = var.database_url
  }
}

deployment "api" {
  namespace = "production"
  container "api" {
    image = image("api")
    env {
      DATABASE_URL = secret("db-credentials", "DATABASE_URL")
    }
  }
}
```

> **Note:** Secret values are stored in plaintext in `.kdef` files. For secrets that must be safe to commit to git, use [`sealedsecret`](#sealedsecret--bitnami-sealed-secret) instead.

---

## `sealedsecret` — Bitnami Sealed Secret

Define encrypted secrets that are safe to commit to git. Generates [Bitnami SealedSecret](https://github.com/bitnami-labs/sealed-secrets) CRD manifests.

```hcl
sealedsecret "db-credentials" {
  namespace = "production"
  type      = "Opaque"    # optional, defaults to "Opaque"

  data = {
    DATABASE_URL = "AgBy3i4OJSWK+PiTySYZZA9rO43cGDEq..."
    PASSWORD     = "AgCE9F2h7GKJF8mL3nP5rS7tV9xB2dH4..."
  }
}
```

The values in `data` are kubeseal-encrypted ciphertexts. Use `kdef seal` to encrypt plaintext values. The sealed-secrets controller in your cluster decrypts them into regular Kubernetes Secrets at deploy time.

Pairs naturally with `secret()` references in deployments:

```hcl
sealedsecret "db-credentials" {
  namespace = "production"
  data = {
    DATABASE_URL = "AgBy3i4OJSWK+PiTySYZZA9rO43cGDEq..."
  }
}

deployment "api" {
  namespace = "production"
  container "api" {
    image = image("api")
    env {
      DATABASE_URL = secret("db-credentials", "DATABASE_URL")
    }
  }
}
```

## `persistentvolumeclaim` — Persistent Volume Claim

Define PersistentVolumeClaims to request storage from the cluster.

```hcl
persistentvolumeclaim "app-data" {
  namespace     = "production"
  storage_class = "gp3"
  access_modes  = ["ReadWriteOnce"]
  storage       = "10Gi"
}
```

| Attribute       | Required | Description                                                    |
|-----------------|----------|----------------------------------------------------------------|
| `namespace`     | no       | Kubernetes namespace                                           |
| `storage_class` | no       | StorageClass name (omit to use cluster default)                |
| `access_modes`  | no       | List of access modes (default: `["ReadWriteOnce"]`)            |
| `storage`       | **yes**  | Storage size, e.g. `"5Gi"`, `"500Mi"`                         |

Valid access modes: `ReadWriteOnce`, `ReadOnlyMany`, `ReadWriteMany`, `ReadWriteOncePod`.

Pairs naturally with `pvc` volume references in deployments:

```hcl
persistentvolumeclaim "app-data" {
  namespace     = "production"
  storage_class = "gp3"
  storage       = "10Gi"
}

deployment "api" {
  namespace = "production"
  container "api" {
    image = image("api")
  }

  volume "data" {
    mount_path = "/data"
    pvc        = "app-data"
  }
}
```

## `clusterrole` — Kubernetes ClusterRole

Cluster-scoped RBAC. Use `rule {}` sub-blocks to list policy rules.

```hcl
clusterrole "promtail" {
  rule {
    api_groups = [""]
    resources  = ["nodes", "nodes/proxy", "services", "endpoints", "pods"]
    verbs      = ["get", "watch", "list"]
  }

  rule {
    non_resource_urls = ["/metrics"]
    verbs             = ["get"]
  }
}
```

If `api_groups` is omitted and `resources` is set, it defaults to `[""]` (the core API group).

## `clusterrolebinding` — Kubernetes ClusterRoleBinding

Binds a ClusterRole to one or more subjects (typically ServiceAccounts). Use a `role_ref {}` block and one or more `subject {}` blocks.

```hcl
clusterrolebinding "promtail" {
  role_ref {
    name = "promtail"  # kind defaults to "ClusterRole"
  }

  subject {
    # kind defaults to "ServiceAccount"
    name      = "promtail"
    namespace = "logs"
  }
}
```

## Pod scheduling: `toleration`, `node_selector`, host namespaces

All workload block types (`deployment`, `daemonset`, `statefulset`, `cronjob`) accept:

- `node_selector` — map; pods only schedule on nodes whose labels match.
- `toleration {}` — zero or more sub-blocks; tolerate node taints.
- `host_network`, `host_pid`, `host_ipc` — bool; share the host's network / PID / IPC namespace. Typical for DaemonSets that need node-level visibility (node-exporter, log collectors, CNI agents).
- `dns_policy` — `ClusterFirst` (K8s default), `ClusterFirstWithHostNet` (use this with `host_network`), `Default`, or `None`.

```hcl
daemonset "node-exporter" {
  namespace    = "monitoring"
  host_network = true
  host_pid     = true
  dns_policy   = "ClusterFirstWithHostNet"

  container "exporter" {
    image = "quay.io/prometheus/node-exporter:v1.8.0"
    args  = [
      "--path.rootfs=/host",
      "--web.listen-address=:9100",
    ]
    port "9100" "metrics" { tcp_ready = true }

    volume "rootfs" {
      mount_path = "/host"
      host_path  = "/"
      read_only  = true
    }
  }

  toleration { operator = "Exists" effect = "NoSchedule" }
}
```

```hcl
daemonset "gpu-driver" {
  namespace     = "kube-system"
  node_selector = { "gpu" = "nvidia" }

  toleration {
    key      = "nvidia.com/gpu"
    operator = "Exists"
    effect   = "NoSchedule"
  }

  container "driver" {
    image = "nvidia/driver:550"
    security_context {
      privileged = true
    }
  }
}
```

## Downward API: `field_ref()`

Env vars can read pod/node metadata via `field_ref()`:

```hcl
env {
  NODE_NAME = field_ref("spec.nodeName")
  POD_NAME  = field_ref("metadata.name")
  POD_NS    = field_ref("metadata.namespace")
  POD_IP    = field_ref("status.podIP")
}
```

This generates `valueFrom.fieldRef` in the rendered manifest.

# kdef

**Declarative Kubernetes configuration language.**

kdef compiles human-readable `.kdef` files into standard Kubernetes YAML manifests. It sits between Kustomize (no variables, no loops) and Helm (too complex, opaque templates) — giving you typed variables, native loops, environment overrides, and transparent output.

```hcl
deployment "api" {
  namespace        = "acme-app"
  image_pull_secrets = ["regcred"]

  container "api" {
    image = "registry.example.com/acme/api:${var.image_tag}"

    port "9000" "http" {
      tcp_health    = true
      initial_delay = 3
      period        = 3
    }

    env_from { config_map = "env-configmap" }

    resources {
      cpu    = "300m..800m"
      memory = "256M..1G"
    }
  }

  container "nginx" {
    image = "nginx:stable-alpine"
    port "80" "http" { tcp_health = true }
  }

  service {
    port "80" "http" {}
  }

  ingress {
    host = "api.acme.dev"
    tls  = true
  }
}
```

## Install

```bash
# Build from source
make build

# The binary is self-contained, zero runtime dependencies
./kdef version
```

## Quick Start

```bash
# Import existing resources from a live cluster
kdef import --namespace my-app --output-dir k8s/

# Or import from YAML files
kdef import --from-file manifests.yaml --output-dir k8s/

# Render to YAML
kdef render --dir k8s/

# Compare against live cluster
kdef diff --dir k8s/

# Deploy (server-side apply)
kdef apply --dir k8s/
kdef apply --dir k8s/ --dry-run   # preview first
```

## Block Types

### `deployment` — Kubernetes Deployment

The primary block type. Defines a Deployment with explicit containers, optional Service, and optional Ingress.

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

  ingress {
    name         = "web.example.com"    # K8s resource name
    service_name = "web-svc"            # backend service
    port         = 80                   # backend port
    host         = "web.example.com"
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

  # Multiple hosts
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

### `cronjob` — Kubernetes CronJob

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

### `configmap` — Kubernetes ConfigMap

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

## Variables

### Declaration (`vars.kdef`)

```hcl
import = "../shared/global-vars.kdef"    # import from other files

variable "environment" {
  type    = "string"
  default = "staging"
}

variable "image_tag" {
  type    = "string"
  default = "latest"
}

variable "replicas" {
  type    = "number"
  default = 1
}

variable "debug" {
  type    = "bool"
  default = false
}

variable "environment" {
  type    = "enum[staging, production]"
  default = "staging"
}
```

### Usage

Variables are referenced as `var.name` in attribute values and `${var.name}` in string interpolation:

```hcl
container "api" {
  image = "my-registry/api:${var.image_tag}"
}

scale {
  replicas = var.replicas
}
```

### Overrides

```bash
# CLI flag
kdef render --dir k8s/ --set image_tag=v2.5.0

# JSON values file (for complex types like lists)
kdef render --dir k8s/ --values values/production.json

# Import variables from another file
kdef render --dir k8s/ --vars-from ../shared/vars.kdef
```

## Ingress Defaults

Define shared ingress settings in `vars.kdef` — every deployment's ingress inherits them:

```hcl
ingress_defaults {
  tls        = true
  tls_secret = "wildcard-tls"

  annotations = {
    "nginx.ingress.kubernetes.io" = {
      "force-ssl-redirect" = "true"
      "proxy-body-size"    = "50m"
      "proxy-read-timeout" = "120"
    }
  }
}
```

Per-deployment annotations merge on top (override specific keys).

## Conditionals

### Ternary (inline)

```hcl
scale {
  replicas = var.environment == "production" ? 3 : 1
}
```

### Top-level `if` blocks

```hcl
if {
  condition = var.environment == "production"

  deployment "extra-consumer" {
    container "consumer" {
      image = "my-registry/api:${var.image_tag}"
    }
  }
}
```

## For Loops

Generate multiple resources from a list variable:

```hcl
# values.json
{
  "tenants": [
    {"name": "acme", "domain": "acme.example.com", "plan": "enterprise"},
    {"name": "demo", "domain": "demo.example.com", "plan": "starter"}
  ]
}
```

```hcl
for "tenant" "var.tenants" {
  deployment "booking" {
    name = "booking-${tenant.name}"

    container "booking" {
      image = "my-registry/booking:${var.image_tag}"

      env {
        TENANT_ID = tenant.name
      }
    }

    scale {
      replicas = tenant.plan == "enterprise" ? 3 : 1
    }

    service {}

    ingress {
      host = tenant.domain
      tls  = true
    }
  }
}
```

```bash
kdef render --dir k8s/ --values values.json
```

## Environment Overrides

### `environments/production.kdef`

```hcl
use_vars {
  environment = "production"
  image_tag   = "v2.4.1"
}

override "app" "api" {
  scale {
    replicas = 3
  }
}
```

```bash
kdef render --dir k8s/ --env production
kdef diff --dir k8s/ --env production
kdef apply --dir k8s/ --env production
```

## Secret References

Reference Kubernetes Secrets in env blocks — generates `valueFrom.secretKeyRef`:

```hcl
env {
  APP_ENV      = var.environment                       # plain value
  DATABASE_URL = secret("db-credentials", "url")       # secret ref
  JWT_SECRET   = secret("jwt-keys", "secret")          # secret ref
}
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `kdef render` | Render `.kdef` files to Kubernetes YAML |
| `kdef diff` | Compare rendered manifests against live cluster |
| `kdef apply` | Deploy to cluster (server-side apply) |
| `kdef validate` | Check for type errors and missing references |
| `kdef import` | Generate `.kdef` from existing K8s resources |
| `kdef version` | Print version information |

### Common Flags

```bash
--dir <path>           # project directory (default: .)
--env <name>           # load environments/<name>.kdef
--set key=value        # override variables
--values <file>        # JSON values file for complex variables
--vars-from <file>     # import variable files
```

### Import

```bash
# From live cluster
kdef import --namespace my-app --output-dir k8s/

# From YAML files (e.g. helm template output)
kdef import --from-file manifests.yaml --output-dir k8s/

# Preview to stdout
kdef import --namespace my-app
```

The importer auto-detects:
- Deployments with Services/Ingresses → `deployment` blocks
- Deployments without Services → worker-style `deployment` blocks (no `service {}`)
- CronJobs → `cronjob` blocks
- ConfigMaps → `configmap` blocks
- Secret references in env vars → `secret()` calls
- Multi-host ingresses, probe settings, init containers, sidecars, volumes

### Apply

```bash
kdef apply --dir k8s/                     # apply to cluster
kdef apply --dir k8s/ --dry-run           # preview without applying
kdef apply --dir k8s/ --env production    # with environment overrides
```

Uses `kubectl apply --server-side --force-conflicts` for clean resource management.

## Project Structure

```
k8s/
├── vars.kdef                 # variables + ingress defaults + imports
├── api.kdef                  # deployment: api + nginx sidecar
├── web.kdef                  # deployment: web + nginx sidecar
├── admin.kdef                # deployment: admin (single container)
├── cronjobs.kdef             # cronjob definitions
├── configmaps.kdef           # configmap definitions
├── environments/
│   ├── staging.kdef          # staging overrides
│   └── production.kdef       # production overrides
└── values/
    ├── staging.json          # complex variable values
    └── production.json
```

## ArgoCD Integration

kdef includes a Config Management Plugin for ArgoCD. See [argocd-plugin/README.md](argocd-plugin/README.md) for setup instructions.

ArgoCD auto-discovers `.kdef` files and renders them using kdef.

## Comparison

### vs Traditional K8s Tools

| | Kustomize | Helm | kdef |
|---|---|---|---|
| Variables | No | Yes (values.yaml) | Yes (typed, with defaults) |
| Loops | No | Partial (range) | Yes (native `for`) |
| Conditionals | No | Yes (Go templates) | Yes (`if`, ternary) |
| Human readable | Mostly | No (Go templates) | Yes (HCL syntax) |
| Transparent output | Yes | `helm template` | Yes (always) |
| Type validation | No | No | Yes |
| Multi-container | Yes (raw YAML) | Yes (raw YAML) | Yes (explicit `container` blocks) |
| Import existing | No | No | Yes (`kdef import`) |
| Secret references | No | No | Yes (`secret()` function) |
| Escape hatch | Patches | Raw YAML | `raw` block (deep-merge) |
| Learning curve | Low | Moderate | Low |

### vs Configuration Languages

| | CUE | KCL | Pkl | kdef |
|---|---|---|---|---|
| Variables | Yes | Yes | Yes | Yes (typed, defaults) |
| Loops | Yes (comprehensions) | Yes | Yes | Yes (native `for`) |
| Conditionals | Yes | Yes | Yes | Yes (`if`, ternary) |
| Human readable | Moderate (JSON superset) | Yes (Python-like) | Yes (modern syntax) | Yes (HCL syntax) |
| Type system | Strong (constraints) | Strong (schema-centric) | Strong (static) | Basic (string, number, bool, enum) |
| Modules / Components | Yes (packages) | Yes (packages) | Yes (classes, modules) | Not yet (planned) |
| K8s-aware | No (generic config) | Yes (K8s schemas) | Yes (K8s templates) | Yes (deployment-centric) |
| Import existing manifests | Yes (`cue import`) | Yes (`kcl import`) | Yes (convert module) | Yes (`kdef import`) |
| Secret references | No | No | No | Yes (`secret()` function) |
| Multi-container pods | Manual (raw YAML) | Manual (raw YAML) | Manual (raw YAML) | Yes (explicit `container` blocks) |
| Service/Ingress generation | No (manual) | No (manual) | No (manual) | Yes (nested `service`/`ingress` blocks) |
| Env overrides | Manual | Manual | Manual | Built-in (`--env` flag) |
| Ingress defaults | No | No | No | Yes (`ingress_defaults`) |
| Escape hatch | N/A (you write everything) | N/A | N/A | `raw` block (deep-merge) |
| Learning curve | Steep | Moderate | Low | Low |
| Philosophy | General-purpose config constraint language | General-purpose config language (CNCF) | General-purpose config language (Apple) | Purpose-built K8s deployment tool |

**Key differences:**

- **CUE, KCL, Pkl** are general-purpose configuration languages. They're powerful but you still need to know the K8s API and write resource-level YAML structure. They replace the *templating* layer, not the *abstraction* layer.
- **kdef** is purpose-built for Kubernetes deployments. One `deployment` block generates a Deployment + Service + Ingress + Certificate + HPA. You think in terms of "my app has these containers, this service, this ingress" — not in terms of K8s API objects.

## Built With

- [Go](https://go.dev/)
- [HashiCorp HCL](https://github.com/hashicorp/hcl) — parser
- [Kubernetes API types](https://github.com/kubernetes/api) — typed K8s objects
- [Cobra](https://github.com/spf13/cobra) — CLI framework

## Author

**Guido Smit** — [GSID](https://gsid.nl)

- GitHub: [@gesmit](https://github.com/gesmit)
- Email: guido@gsid.nl

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

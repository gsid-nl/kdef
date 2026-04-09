# Block Types

## `root.kdef` — Multi-Project Root

The `root.kdef` file is the entry point for multi-app repositories. It defines namespaces, service accounts, ingress defaults, and lists all sub-projects with their configuration.

```hcl
# root.kdef

namespaces = ["production", "staging"]

service_account "default" {
  image_pull_secrets = ["registrykey"]
}

ingress_defaults {
  tls    = true
  issuer = "letsencrypt-production"
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

- `namespace` from a deployment entry is injected into all blocks (deployments, cronjobs, configmaps, etc.) that don't already specify one
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

---

## `deployment` — Kubernetes Deployment

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

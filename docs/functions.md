# Functions

kdef provides built-in functions for use in `.kdef` files.

## `image()` — Image Registry

Resolve a short image name to a full `registry/name:tag` reference from an `images {}` block.

```hcl
# images.kdef (or any .kdef file)
images {
  api   = "registry.example.com/my-app/api:1.2.3"
  cdn   = "registry.example.com/my-app/cdn:0.5.0"
  nginx = "registry.example.com/nginx:stable"
}
```

```hcl
# apps.kdef
deployment "api" {
  container "api" {
    image = image("api")
  }
  container "nginx" {
    image = image("nginx")
  }
}
```

The `images {}` block can be placed in any `.kdef` file — a dedicated `images.kdef`, inside `vars.kdef`, or alongside deployments. Multiple blocks in the same directory are merged.

### Scoping in multi-project repositories

In a repo with a `root.kdef`, `images {}` blocks are scoped to the directory they live in and all descendant directories. Resolution walks from the project root down to the subproject, with deeper levels overriding shallower ones on name collision.

```
logging/
├── root.kdef
├── images.kdef               # visible everywhere below
├── monitoring/
│   ├── images.kdef           # visible in monitoring/** only
│   ├── node-exporter/*.kdef
│   └── prometheus/*.kdef
└── alloy/*.kdef              # cannot see monitoring/images.kdef
```

An image name defined closer to the subproject wins. For example, a `node-exporter` key in `monitoring/images.kdef` overrides the same key in the root-level `images.kdef` for anything under `monitoring/`, but leaves the root definition visible to `alloy/`.

The same scoping rule applies to `vars.kdef` files: each level's declarations override ancestor levels.

## `secret()` — Secret References

Reference Kubernetes Secrets in env blocks — generates `valueFrom.secretKeyRef`:

```hcl
env {
  APP_ENV      = var.environment                       # plain value
  DATABASE_URL = secret("db-credentials", "url")       # secret ref
  JWT_SECRET   = secret("jwt-keys", "secret")          # secret ref
}
```

## `configmap()` — ConfigMap Key References

Reference individual keys from a ConfigMap — generates `valueFrom.configMapKeyRef`. Useful when the env var name differs from the ConfigMap key:

```hcl
env {
  # Maps NUXT_PUBLIC_BASE_URL to the FOS_API_URL key in env-configmap
  NUXT_PUBLIC_BASE_URL = configmap("env-configmap", "FOS_API_URL")
  NUXT_PUBLIC_CDN_URL  = configmap("env-configmap", "FOS_CDN_URL")
}
```

This is different from `env_from { config_map = "..." }` which imports *all* keys with their original names. Use `configmap()` when you need to remap a key to a different env var name.

## `file()` — File Contents

Read file contents into a ConfigMap data field:

```hcl
configmap "nginx-config" {
  data = {
    "nginx.conf" = file("configs/nginx.conf")
  }
}
```

Paths are resolved relative to the project directory.

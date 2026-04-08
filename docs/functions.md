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

The `images {}` block can be placed in any `.kdef` file — a dedicated `images.kdef`, inside `vars.kdef`, or alongside deployments. All `images {}` blocks across all files are merged before evaluation.

## `secret()` — Secret References

Reference Kubernetes Secrets in env blocks — generates `valueFrom.secretKeyRef`:

```hcl
env {
  APP_ENV      = var.environment                       # plain value
  DATABASE_URL = secret("db-credentials", "url")       # secret ref
  JWT_SECRET   = secret("jwt-keys", "secret")          # secret ref
}
```

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

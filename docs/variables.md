# Variables

## Declaration (`vars.kdef`)

```hcl
# Single import
import = "../shared/global-vars.kdef"

# Or import multiple files
import = [
  "../shared/global-vars.kdef",
  "defaults.kdef",
  "images.kdef",
]

variable "environment" {
  type    = "string"
  default = "staging"
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

Imported files can contain variables and `ingress_defaults`. Files are processed in order; later imports override earlier ones. `vars.kdef` itself always takes final precedence.

## Usage

Variables are referenced as `var.name` in attribute values and `${var.name}` in string interpolation:

```hcl
container "api" {
  image = "my-registry/api:${var.image_tag}"
}

scale {
  replicas = var.replicas
}
```

## Environment Variables (`env`)

Local environment variables are available via the `env` namespace. This works just like `var` — direct access and string interpolation are both supported:

```hcl
container "api" {
  image = "nginx:latest"

  env {
    HOME      = env.HOME
    DATA_PATH = "${env.HOME}/data"
    USER      = env.USER
  }
}

volume "data" {
  mount_path = "${env.HOME}/data"
}
```

All environment variables from the local shell are available. Referencing an unset variable is a render-time error:

```
Error: This object does not have an attribute named "NONEXISTENT_VAR".
```

This is useful for local development (machine-specific paths), CI/CD pipelines (injected secrets and config), and keeping values out of committed files.

## Overrides

```bash
# CLI flag
kdef render --dir k8s/ --set image_tag=v2.5.0

# JSON values file (for complex types like lists)
kdef render --dir k8s/ --values values/production.json

# Import variables from another file
kdef render --dir k8s/ --vars-from ../shared/vars.kdef
```

## Ingress Defaults

Define shared ingress settings in `vars.kdef` (or an imported file) — every deployment's ingress inherits them:

```hcl
ingress_defaults {
  tls    = true
  issuer = "letsencrypt-prod"

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

Can also be placed in a separate file and imported:

```hcl
# vars.kdef
import = "defaults.kdef"
```

```hcl
# defaults.kdef
ingress_defaults {
  tls    = true
  issuer = "letsencrypt-prod"
}
```

If `ingress_defaults` is defined in both `vars.kdef` and an imported file, `vars.kdef` takes precedence.

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

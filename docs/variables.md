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

# Structured values: lists of objects, maps, anything a --values file could carry.
variable "sites" {
  type = "list"

  default = [
    { name = "cor-it-nl", hosts = ["cor-it.nl", "www.cor-it.nl"] },
    { name = "gsid-nl", hosts = ["gsid.nl", "www.gsid.nl"] },
  ]
}
```

### Types

| Type | Notes |
| --- | --- |
| `string` | |
| `number` | |
| `bool` | |
| `enum[a, b, c]` | A string restricted to the listed values |
| `list` | A list of anything — objects, strings, nested lists. The shape is not checked, only that the default is iterable. |
| `any` | Same as `list` without the iterable check: maps, objects, scalars. |

A `list` or `any` variable is the in-repo alternative to a `--values` JSON file, and reads the same way at the point of use — `for "site" "var.sites"` does not care which one supplied the value. Declare it in `vars.kdef` when the data belongs to the repo; use `--values` when it is generated elsewhere.

`--set` carries strings only, so it cannot override a `list` or `any` variable; kdef errors rather than quietly replacing the value with a string. Use `--values` for that.

Imported files can contain variables and `ingress_defaults`. Files are processed in order; later imports override earlier ones. `vars.kdef` itself always takes final precedence.

### Scoping in multi-project repositories

In a repo with a `root.kdef`, `vars.kdef` files at any directory level apply to that directory and all descendants. Resolution walks from the project root down to the subproject, with deeper levels overriding shallower ones on name collision.

```
logging/
├── root.kdef
├── vars.kdef                 # visible everywhere below
├── monitoring/
│   ├── vars.kdef             # visible in monitoring/** only; overrides root vars
│   └── node-exporter/
│       ├── vars.kdef         # overrides both above, but only here
│       └── *.kdef
└── alloy/*.kdef              # cannot see monitoring/vars.kdef
```

The same scoping rule applies to `images {}` blocks (see [Functions — `image()`](functions.md#image--image-registry)).

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

Define shared ingress settings in `vars.kdef` (or an imported file) — **every** `ingress {}` block on every deployment/statefulset inherits them. If a workload has multiple ingress blocks, each one gets the defaults independently:

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

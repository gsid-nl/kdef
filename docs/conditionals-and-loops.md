# Conditionals and Loops

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
      image = image("api")
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
      image = image("booking")

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

### Looping over ingresses

`for` also accepts top-level [`ingress`](block-types.md#ingress--standalone-ingress) blocks, which is the usual way to drive a list of custom domains from data: one service, many registrable domains, the list generated elsewhere and committed.

```json
// sites.json
{
  "sites": [
    {"name": "cor-it-nl", "hosts": ["cor-it.nl", "www.cor-it.nl"]},
    {"name": "gsid-nl",   "hosts": ["gsid.nl", "www.gsid.nl"]}
  ]
}
```

```hcl
for "site" "var.sites" {
  ingress "cms" {
    name         = "cms-${site.name}"
    service_name = "cms-public-svc"
    port         = 80
    hosts        = site.hosts
  }
}
```

The block label is a placeholder here: HCL forbids interpolation in labels, so `name` supplies the per-iteration resource name. Without it every iteration would resolve to the same name and kdef would reject the collision.

TLS, issuer, class and shared annotations belong in `ingress_defaults` rather than being repeated per iteration. `hosts[0]` determines the derived Certificate secret name, so whatever generates the values file should emit a stable first element (order by the canonical host, not by row id) or the secret name churns between renders.

```bash
kdef render --dir k8s/ --values sites.json
```

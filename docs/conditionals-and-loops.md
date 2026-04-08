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

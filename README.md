<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/kdef-logo-dark.svg">
  <img src="docs/assets/kdef-logo.svg" alt="kdef" width="420">
</picture>

**Declarative Kubernetes configuration language.**

kdef compiles human-readable `.kdef` files into standard Kubernetes YAML manifests. It sits between Kustomize (no variables, no loops) and Helm (too complex, opaque templates) — giving you typed variables, native loops, environment overrides, and transparent output.

```hcl
deployment "api" {
  namespace        = "my-app"
  image_pull_secrets = ["regcred"]

  container "api" {
    image       = image("api")
    working_dir = "/app"

    port "8080" "http" {
      health = "/health"
    }

    env {
      APP_ENV      = var.environment
      DATABASE_URL = secret("db-credentials", "url")
      API_BASE_URL = configmap("app-config", "API_URL")
    }

    resources {
      cpu    = "300m..800m"
      memory = "256M..1G"
    }
  }

  container "nginx" {
    image = image("nginx")
    port "80" "http" { tcp_health = true }
  }

  service {
    port "80" "http" {}
  }

  ingress {
    host = "api.example.com"
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

### Cross-platform binaries

```bash
make build-all    # Linux, macOS, Windows (amd64 + arm64)
```

### Linux packages

```bash
make package      # .deb, .rpm, .apk (amd64 + arm64)
```

Requires [nfpm](https://nfpm.goreleaser.com) (`go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest`).

## Quick Start

```bash
# Import existing resources from a live cluster
kdef import --namespace my-app --output-dir k8s/

# Render to YAML
kdef render --dir k8s/

# Compare against live cluster
kdef diff --dir k8s/

# Deploy (server-side apply)
kdef apply --dir k8s/
kdef apply --dir k8s/ --dry-run   # preview first
```

## Documentation

- [Block Types](docs/block-types.md) — `root.kdef`, `deployment`, `cronjob`, `configmap`, `secret`, `sealedsecret`, `persistentvolumeclaim`
- [Variables](docs/variables.md) — typed variables, `env` (local environment), imports, ingress defaults, environment overrides
- [Functions](docs/functions.md) — `image()`, `secret()`, `configmap()`, `file()`
- [Conditionals and Loops](docs/conditionals-and-loops.md) — `if` blocks, `for` loops, ternary
- [CLI Commands](docs/cli.md) — render, diff, apply, validate, import, seal, seal-secret, install-hook
- [Comparison](docs/comparison.md) — vs Kustomize, Helm, CUE, KCL, Pkl
- [Editor Integration](#editor-integration) — VS Code extension, JetBrains LSP setup

## Project Structure

### Single-app layout

```
k8s/
├── vars.kdef                 # variables + ingress defaults + imports
├── images.kdef               # image registry (name -> registry/image:tag)
├── api.kdef                  # deployment: api + nginx sidecar
├── web.kdef                  # deployment: web + nginx sidecar
├── cronjobs.kdef             # cronjob definitions
├── configmaps.kdef           # configmap definitions
├── secrets.kdef              # secret / sealed secret definitions
├── environments/
│   ├── staging.kdef          # staging overrides
│   └── production.kdef       # production overrides
└── values/
    └── production.json       # complex variable values
```

### Multi-app layout with `root.kdef`

For repositories that ship several apps together, a `root.kdef` at the top declares shared namespaces, service accounts, ingress defaults, and the list of sub-projects. Root-level definition files (`configmaps.kdef`, `secrets.kdef`, ...) are parsed once and shared across all sub-projects.

```
repo/
├── root.kdef                 # namespaces, service_accounts, deployments {...}
├── configmaps.kdef           # shared configmaps (parsed once)
├── secrets.kdef              # shared secrets / sealed secrets (parsed once)
├── api/
│   ├── vars.kdef
│   └── app.kdef              # deployment "api"
├── worker/
│   └── app.kdef              # deployment "worker"
└── environments/
    ├── staging.kdef
    └── production.kdef
```

All CLI commands (`render`, `validate`, `diff`, `apply`) transparently walk every sub-project listed in `root.kdef`. See [Block Types › root.kdef](docs/block-types.md) for the full reference.

## Editor Integration

kdef includes a language server (`kdef-lsp`) that provides real-time diagnostics (error squiggles) as you type.

### VS Code

Install the kdef extension (`.vsix` included in [releases](https://github.com/gsid-nl/kdef/releases)) and make sure `kdef-lsp` is on your `PATH`:

```bash
# Install the language server
go install github.com/gsid-nl/kdef/cmd/kdef-lsp@latest

# Or build from source
make build    # produces ./kdef-lsp
```

The extension activates automatically for `.kdef` files. To use a custom path, set `kdef.lsp.path` in VS Code settings.

### JetBrains (IntelliJ, GoLand, WebStorm, PhpStorm)

JetBrains IDEs support LSP natively (2023.3+). To set up:

1. Install `kdef-lsp` (see above)
2. Go to **Settings** → **Languages & Frameworks** → **Language Servers**
3. Click **+** to add a new server
4. Set **Name** to `kdef`
5. Set **Command** to `kdef-lsp` (or the full path to the binary)
6. Under **File type mappings**, add `*.kdef`
7. Click **OK** and open any `.kdef` file

For syntax highlighting in JetBrains, install the [TextMate Bundles](https://plugins.jetbrains.com/plugin/7221-textmate-bundles) plugin and point it to `vscode-extension/syntaxes/kdef.tmLanguage.json`.

## GitOps Integration

### Flux CD

kdef includes a Kubernetes controller for Flux. Define a `KdefRelease` CR pointing at a Flux `GitRepository` and the controller renders and applies your `.kdef` files automatically:

```yaml
apiVersion: kdef.gsid.nl/v1alpha1
kind: KdefRelease
metadata:
  name: my-app
  namespace: flux-system
spec:
  sourceRef:
    kind: GitRepository
    name: my-app
  path: ./k8s/
  interval: 5m
  prune: true
```

See [flux-controller/README.md](flux-controller/README.md) for installation and full reference.

### ArgoCD

kdef includes a Config Management Plugin for ArgoCD. See [argocd-plugin/README.md](argocd-plugin/README.md) for setup instructions.

## Built With

- [Go](https://go.dev/)
- [HashiCorp HCL](https://github.com/hashicorp/hcl) — parser
- [Kubernetes API types](https://github.com/kubernetes/api) — typed K8s objects
- [Cobra](https://github.com/spf13/cobra) — CLI framework
- [nfpm](https://nfpm.goreleaser.com/) — Linux package builder (deb/rpm/apk)

## Author

**Guido Smit** — [GSID](https://gsid.nl)

- GitHub: [@gesmit](https://github.com/gesmit)
- Email: guido@gsid.nl

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

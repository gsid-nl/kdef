# kdef for Visual Studio Code

Syntax highlighting, snippets, and language support for [kdef](https://github.com/gsid-nl/kdef) — the declarative Kubernetes configuration language.

## Features

- Syntax highlighting for `.kdef` files
- Bracket matching and auto-closing
- Code folding for blocks
- 25+ snippets for common patterns
- String interpolation highlighting (`${var.name}`)

## Snippets

| Prefix | Description |
|--------|-------------|
| `deployment` | Deployment with container, service |
| `deployment-ingress` | Deployment with service and ingress |
| `deployment-multi` | Multi-container deployment with sidecar |
| `deployment-worker` | Worker deployment (no service) |
| `container` | Container block |
| `init` | Init container |
| `cronjob` | CronJob |
| `configmap` | ConfigMap |
| `configmap-file` | ConfigMap from file |
| `variable` | Variable declaration |
| `ingress` | Ingress block |
| `ingress-annotations` | Ingress with nginx annotations |
| `service` | Service block |
| `volume-configmap` | ConfigMap volume mount |
| `volume-secret` | Secret volume mount |
| `volume-empty` | EmptyDir volume mount |
| `env-from-configmap` | Import env from ConfigMap |
| `env-from-secret` | Import env from Secret |
| `secret` | Secret key reference |
| `resources` | Resources block |
| `security-context` | Security context |
| `autoscale` | HPA autoscale block |
| `for` | For loop |
| `if` | Conditional block |
| `ingress-defaults` | Shared ingress defaults |

## Install

### From VSIX

```bash
cd vscode-extension
npx @vscode/vsce package
code --install-extension kdef-0.2.0.vsix
```

### Development

1. Open the `vscode-extension` folder in VS Code
2. Press `F5` to launch the Extension Development Host
3. Open a `.kdef` file to see syntax highlighting

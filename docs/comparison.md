# Comparison

## vs Traditional K8s Tools

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
| Image registry | Yes (`kustomization.yaml`) | Yes (values.yaml) | Yes (`images {}` block + `image()`) |
| Local env variables | No | No | Yes (`env.HOME`, `"${env.VAR}"`) |
| Secret references | No | No | Yes (`secret()` function) |
| Sealed secrets | No | No | Yes (`sealedsecret` block + `kdef seal`) |
| Escape hatch | Patches | Raw YAML | `raw` block (deep-merge) |
| Learning curve | Low | Moderate | Low |

## vs Configuration Languages

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
| Image registry | No | No | No | Yes (`images {}` block + `image()`) |
| Local env variables | No | No | No | Yes (`env.HOME`, `"${env.VAR}"`) |
| Secret references | No | No | No | Yes (`secret()` function) |
| Sealed secrets | No | No | No | Yes (`sealedsecret` block + `kdef seal`) |
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

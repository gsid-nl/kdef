# CLI Commands

| Command | Description |
|---------|-------------|
| `kdef render` | Render `.kdef` files to Kubernetes YAML |
| `kdef diff` | Compare rendered manifests against live cluster |
| `kdef apply` | Deploy to cluster (server-side apply) |
| `kdef validate` | Check for type errors and missing references |
| `kdef import` | Generate `.kdef` from existing K8s resources |
| `kdef seal` | Encrypt a single value for use in `sealedsecret` blocks |
| `kdef seal-secret` | Seal an entire Kubernetes Secret into a `sealedsecret` block |
| `kdef install-hook` | Install a git pre-commit hook that runs `kdef validate` |
| `kdef version` | Print version information |

## Common Flags

```bash
--dir <path>           # project directory (default: .)
--env <name>           # load environments/<name>.kdef
--set key=value        # override variables
--values <file>        # JSON values file for complex variables
--vars-from <file>     # import variable files
```

## Import

```bash
# From live cluster
kdef import --namespace my-app --output-dir k8s/

# From YAML files (e.g. helm template output)
kdef import --from-file manifests.yaml --output-dir k8s/

# Preview to stdout
kdef import --namespace my-app
```

The importer auto-detects:
- Deployments with Services/Ingresses -> `deployment` blocks
- Deployments without Services -> worker-style `deployment` blocks (no `service {}`)
- DaemonSets -> `daemonset` blocks
- StatefulSets (including `volumeClaimTemplates`) -> `statefulset` blocks
- CronJobs -> `cronjob` blocks
- ConfigMaps -> `configmap` blocks
- ClusterRoles + ClusterRoleBindings -> `clusterrole` / `clusterrolebinding` blocks (from YAML files)
- Secret references in env vars -> `secret()` calls
- Downward-API env vars -> `field_ref()` calls
- Tolerations, `node_selector`, `host_path.type`, container `args`, privileged security contexts
- Multi-host ingresses, probe settings, init containers, sidecars, volumes

## Apply

```bash
kdef apply --dir k8s/                     # apply to cluster
kdef apply --dir k8s/ --dry-run           # preview without applying
kdef apply --dir k8s/ --env production    # with environment overrides
```

Uses `kubectl apply --server-side --force-conflicts` for clean resource management.

## Seal

Encrypt secret values using [kubeseal](https://github.com/bitnami-labs/sealed-secrets) for use in `sealedsecret` blocks. Requires `kubeseal` installed and a sealed-secrets controller running in the cluster.

```bash
# Encrypt a single value
kdef seal --secret db-credentials --key PASSWORD --value "hunter2"

# Encrypt from stdin
echo -n "hunter2" | kdef seal --secret db-credentials --key PASSWORD

# Specify namespace and controller
kdef seal --secret db-credentials --key PASSWORD --value "hunter2" \
  --namespace production --controller-name sealed-secrets
```

The command outputs the encrypted blob to stdout, plus a usage snippet showing how to paste it into a `.kdef` file.

## Seal Secret

Seal an entire Kubernetes Secret into a ready-to-use `sealedsecret` block. Fetches the Secret from the cluster or reads from a YAML file, decodes all values, encrypts each key with kubeseal, and outputs the complete block.

```bash
# From live cluster
kdef seal-secret --name db-credentials --namespace production

# From a YAML file
kdef seal-secret --from-file secret.yaml

# With custom controller
kdef seal-secret --name db-credentials --namespace production \
  --controller-name sealed-secrets
```

Output is a complete `sealedsecret` block ready to paste into a `.kdef` file.

## Install Hook

Install a git pre-commit hook that runs `kdef validate` before each commit, aborting the commit if validation fails. The hook walks up from `--dir` to find the repo root (also handles git worktrees and submodules) and writes `.git/hooks/pre-commit`.

```bash
# Fresh install in the current repo
kdef install-hook

# Run from anywhere inside the repo, or point at a different project
kdef install-hook --dir path/to/project

# Append the kdef check to an existing pre-commit hook (safe, idempotent)
kdef install-hook --append

# Overwrite an existing hook with a standalone kdef hook
kdef install-hook --force
```

The generated script skips validation with a warning if `kdef` is not in `PATH`, so cloning the repo on a machine without kdef installed does not block commits. With `--append`, the kdef block is wrapped in `# >>> kdef validate >>>` / `# <<< kdef validate <<<` sentinels and re-running `--append` is a no-op. `--append` and `--force` are mutually exclusive.

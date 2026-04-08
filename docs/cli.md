# CLI Commands

| Command | Description |
|---------|-------------|
| `kdef render` | Render `.kdef` files to Kubernetes YAML |
| `kdef diff` | Compare rendered manifests against live cluster |
| `kdef apply` | Deploy to cluster (server-side apply) |
| `kdef validate` | Check for type errors and missing references |
| `kdef import` | Generate `.kdef` from existing K8s resources |
| `kdef seal` | Encrypt values for use in `sealedsecret` blocks |
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
- CronJobs -> `cronjob` blocks
- ConfigMaps -> `configmap` blocks
- Secret references in env vars -> `secret()` calls
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

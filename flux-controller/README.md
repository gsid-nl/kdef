# kdef Flux Controller

Kubernetes controller that integrates kdef with [Flux CD](https://fluxcd.io/). Point it at a GitRepository containing `.kdef` files and it renders and applies them automatically.

## Install

```bash
# Install with Helm
helm install kdef-controller ./flux-controller/chart \
  --namespace flux-system
```

## Usage

### 1. Create a Flux GitRepository

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: my-app
  namespace: flux-system
spec:
  interval: 1m
  url: https://github.com/example/my-app.git
  ref:
    branch: main
```

### 2. Create a KdefRelease

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

The controller will:
1. Watch the GitRepository for new revisions
2. Download and extract the artifact
3. Run `kdef render` on the `.kdef` files
4. Apply the rendered manifests via server-side apply
5. Prune resources that are no longer in the output

### Environment overrides

```yaml
spec:
  env: production    # loads environments/production.kdef
```

### Variable overrides

```yaml
spec:
  set:
    image_tag: "v2.0.0"
    replicas: "3"
```

### Values from ConfigMap/Secret

```yaml
spec:
  valuesFrom:
    kind: Secret
    name: my-app-values
    key: values.json    # defaults to "values.json"
```

### Suspend reconciliation

```yaml
spec:
  suspend: true
```

## Status

```bash
kubectl get kdefreleases -n flux-system

NAME     READY   STATUS                                    REVISION       AGE
my-app   True    Applied revision: main@sha256:abc123...   main@sha...    5m
```

## CRD Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `sourceRef.kind` | string | yes | `GitRepository`, `OCIRepository`, or `Bucket` |
| `sourceRef.name` | string | yes | Name of the Flux source |
| `sourceRef.namespace` | string | no | Namespace of the source (defaults to KdefRelease namespace) |
| `path` | string | no | Path within the artifact to the kdef project directory |
| `env` | string | no | Environment name (loads `environments/<env>.kdef`) |
| `set` | map | no | Variable overrides (`--set` equivalent) |
| `valuesFrom.kind` | string | no | `ConfigMap` or `Secret` |
| `valuesFrom.name` | string | no | Name of the ConfigMap/Secret |
| `valuesFrom.key` | string | no | Key in the data (defaults to `values.json`) |
| `interval` | duration | yes | Reconciliation interval (e.g. `5m`, `1h`) |
| `prune` | bool | no | Delete resources removed from output |
| `targetNamespace` | string | no | Override namespace for all resources |
| `serviceAccountName` | string | no | ServiceAccount for impersonation |
| `suspend` | bool | no | Pause reconciliation |

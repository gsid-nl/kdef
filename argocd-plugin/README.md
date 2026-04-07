# kdef ArgoCD Plugin

Config Management Plugin for ArgoCD that renders `.kdef` files natively.

## Setup

### 1. Build the sidecar image

```bash
# From the kdef root directory
make build
cp kdef argocd-plugin/
cd argocd-plugin
docker build -t kdef-argocd-plugin:latest .
```

### 2. Add the sidecar to argocd-repo-server

```yaml
# In your ArgoCD values.yaml (Helm) or repo-server deployment
repoServer:
  extraContainers:
    - name: kdef
      image: kdef-argocd-plugin:latest
      command: ["/var/run/argocd/argocd-cmp-server"]
      securityContext:
        runAsNonRoot: true
        runAsUser: 999
      volumeMounts:
        - mountPath: /var/run/argocd
          name: var-files
        - mountPath: /home/argocd/cmp-server/plugins
          name: plugins
        - mountPath: /tmp
          name: cmp-tmp
  volumes:
    - name: cmp-tmp
      emptyDir: {}
```

### 3. Configure your ArgoCD Application

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: timepickr
spec:
  source:
    repoURL: https://github.com/gsid-nl/timepickr-k8s.git
    path: k8s/
    plugin:
      name: kdef
```

ArgoCD will automatically detect `.kdef` files and use the kdef plugin to render them.

## Environment Overrides

To use `--env` with ArgoCD, configure the plugin parameters:

```yaml
source:
  plugin:
    name: kdef
    env:
      - name: KDEF_ENV
        value: production
```

Then update `plugin.yaml` generate command:
```yaml
generate:
  command: ["sh", "-c", "kdef render --dir . --env ${KDEF_ENV:-staging}"]
```

# ArgoCD Config Management Plugin sidecar for kdef
FROM alpine:3.19

# Copy the kdef binary
COPY kdef /usr/local/bin/kdef

# Copy plugin config
COPY plugin.yaml /home/argocd/cmp-server/config/plugin.yaml

# Required by ArgoCD CMP
RUN chmod +x /usr/local/bin/kdef

# ArgoCD runs the plugin as the argocd user
USER 999

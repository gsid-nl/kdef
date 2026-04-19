#!/usr/bin/env bash
# Release script for kdef. Bumps versions, runs tests, builds all artifacts,
# and packages the VS Code extension, Helm chart, ArgoCD plugin, and binaries.
#
# Usage: scripts/release.sh <version>   e.g. scripts/release.sh 0.6.4
#
# Does NOT push to docker registries, create git tags, or publish — those
# steps are printed as reminders at the end.

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <version>   (e.g. $0 0.6.4)" >&2
  exit 1
fi

NEW_VERSION="$1"
if ! [[ "$NEW_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: version must be X.Y.Z, got $NEW_VERSION" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CURRENT_VERSION="$(grep -E '^VERSION \?=' Makefile | awk '{print $3}')"
echo "Current version: $CURRENT_VERSION"
echo "New version:     $NEW_VERSION"
echo

# Chart.yaml has its own version line (chart schema version). Bump it only if
# the kdef appVersion is actually changing — otherwise rerunning the script
# would inflate the chart version every time.
CURRENT_CHART_VERSION="$(grep -E '^version:' flux-controller/chart/Chart.yaml | awk '{print $2}')"
CURRENT_APP_VERSION="$(grep -E '^appVersion:' flux-controller/chart/Chart.yaml | awk '{print $2}' | tr -d '"')"
if [[ "$CURRENT_APP_VERSION" == "$NEW_VERSION" ]]; then
  NEW_CHART_VERSION="$CURRENT_CHART_VERSION"
  echo "Chart version:   $CURRENT_CHART_VERSION (unchanged — appVersion already at $NEW_VERSION)"
else
  CHART_MAJOR="${CURRENT_CHART_VERSION%.*}"
  CHART_PATCH="${CURRENT_CHART_VERSION##*.}"
  NEW_CHART_VERSION="${CHART_MAJOR}.$((CHART_PATCH + 1))"
  echo "Chart version:   $CURRENT_CHART_VERSION -> $NEW_CHART_VERSION"
fi
echo

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: required tool '$1' not found on PATH" >&2
    exit 1
  fi
}

require go
require node
require npm
require docker
require envsubst

NFPM="$(go env GOPATH)/bin/nfpm"
if [[ ! -x "$NFPM" ]]; then
  echo "error: nfpm not found at $NFPM — install with: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest" >&2
  exit 1
fi

# -----------------------------------------------------------------------------
# Pre-flight: clean git tree (we want reproducible release commits)
# -----------------------------------------------------------------------------
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "warning: working tree is dirty — version bumps will be included" >&2
fi

# -----------------------------------------------------------------------------
# 1. Bump version strings
# -----------------------------------------------------------------------------
echo "==> Bumping versions"

# Makefile
sed -i -E "s/^VERSION \?= .*/VERSION ?= ${NEW_VERSION}/" Makefile

# vscode-extension/package.json
node -e "
  const fs = require('fs');
  const p = './vscode-extension/package.json';
  const j = JSON.parse(fs.readFileSync(p, 'utf8'));
  j.version = '${NEW_VERSION}';
  fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
"

# flux-controller/chart/Chart.yaml — both version and appVersion
sed -i -E "s/^version: .*/version: ${NEW_CHART_VERSION}/" flux-controller/chart/Chart.yaml
sed -i -E "s/^appVersion: .*/appVersion: \"${NEW_VERSION}\"/" flux-controller/chart/Chart.yaml

echo "    Makefile         -> ${NEW_VERSION}"
echo "    vscode-extension -> ${NEW_VERSION}"
echo "    chart appVersion -> ${NEW_VERSION}"
echo "    chart version    -> ${NEW_CHART_VERSION}"
echo

# -----------------------------------------------------------------------------
# 2. Tests
# -----------------------------------------------------------------------------
echo "==> Running tests"
go test ./... >/dev/null
echo "    go test: ok"
echo

# -----------------------------------------------------------------------------
# 3. Build kdef + kdef-lsp (cross-platform) and install locally
# -----------------------------------------------------------------------------
echo "==> Building cross-platform binaries (make build-all)"
make build-all >/dev/null
echo "    dist/ populated"

echo "==> Installing kdef + kdef-lsp locally to ~/.local/bin"
install -m 0755 dist/kdef-linux-amd64     "$HOME/.local/bin/kdef"
install -m 0755 dist/kdef-lsp-linux-amd64 "$HOME/.local/bin/kdef-lsp"
echo "    installed:"
"$HOME/.local/bin/kdef" version
echo

# -----------------------------------------------------------------------------
# 4. Shell completions + linux packages (deb/rpm/apk)
#    'make package' does not clean dist/ on its own; its build-all dep already
#    ran in step 3 above. nfpm just writes .deb/.rpm/.apk next to the binaries.
# -----------------------------------------------------------------------------
echo "==> Generating shell completions"
make completions >/dev/null
echo "    completions/ written"

echo "==> Building .deb / .rpm / .apk packages"
make package >/dev/null
echo "    packages in dist/"
echo

# -----------------------------------------------------------------------------
# 5. Stage ArgoCD plugin binary
# -----------------------------------------------------------------------------
echo "==> Staging ArgoCD plugin binary"
cp dist/kdef-linux-amd64 argocd-plugin/kdef
echo "    argocd-plugin/kdef refreshed"
echo

# -----------------------------------------------------------------------------
# 6. Helm chart package
# -----------------------------------------------------------------------------
if command -v helm >/dev/null 2>&1; then
  echo "==> Packaging Helm chart"
  mkdir -p dist/charts
  helm package flux-controller/chart -d dist/charts >/dev/null
  echo "    dist/charts/kdef-controller-${NEW_CHART_VERSION}.tgz"
  echo
else
  echo "warning: helm not found — skipping chart package" >&2
fi

# -----------------------------------------------------------------------------
# 7. VS Code extension .vsix
# -----------------------------------------------------------------------------
echo "==> Packaging VS Code extension"
pushd vscode-extension >/dev/null
if [[ ! -d node_modules ]]; then
  npm install >/dev/null
fi
npm run package >/dev/null
ls -1 "kdef-${NEW_VERSION}.vsix" >/dev/null
echo "    vscode-extension/kdef-${NEW_VERSION}.vsix"
popd >/dev/null
echo

# -----------------------------------------------------------------------------
# 8. Summary + manual steps
# -----------------------------------------------------------------------------
cat <<EOF
================================================================================
Release v${NEW_VERSION} artifacts built.

  dist/kdef-linux-amd64, kdef-linux-arm64, kdef-darwin-{amd64,arm64}, kdef-windows-amd64.exe
  dist/kdef-lsp-linux-amd64, ... (same platforms)
  dist/kdef_${NEW_VERSION}_*.deb / *.rpm / *.apk
  dist/charts/kdef-controller-${NEW_CHART_VERSION}.tgz
  vscode-extension/kdef-${NEW_VERSION}.vsix
  argocd-plugin/kdef (linux-amd64 binary for Docker build)

Remaining manual steps:

  1. Update release-notes.md with a v${NEW_VERSION} section (if not done)

  2. Review and commit:
       git add -A
       git status
       git commit -m "Release v${NEW_VERSION}: <headline>"

  3. Tag and push:
       git tag v${NEW_VERSION}
       git push origin main --tags

  4. Build & push Docker images (registry-specific; see argocd-plugin/make,
     flux-controller/Dockerfile — the placeholder 'your-registry.example.com'
     must be replaced):
       # ArgoCD plugin
       docker buildx build --push -f argocd-plugin/Dockerfile \\
         -t <registry>/kdef-argocd-plugin:${NEW_VERSION} \\
         -t <registry>/kdef-argocd-plugin:latest argocd-plugin/
       # Flux controller
       docker buildx build --push -f flux-controller/Dockerfile \\
         -t ghcr.io/gsid-nl/kdef-controller:${NEW_VERSION} .

  5. Publish VS Code extension (optional):
       cd vscode-extension && npx vsce publish --packagePath kdef-${NEW_VERSION}.vsix

  6. Create GitHub release with dist/ binaries + .vsix + chart .tgz attached.
================================================================================
EOF

#!/usr/bin/env bash
# Release script for kdef. Bumps versions, runs tests, builds all artifacts,
# packages the VS Code extension, Helm chart, ArgoCD plugin, and binaries,
# then commits, tags, pushes, builds+pushes Docker images to ghcr.io, and
# creates the GitHub release with all artifacts attached.
#
# Usage:
#   scripts/release.sh <version>                    # full release
#   scripts/release.sh <version> --build-only       # stop after artifacts
#   scripts/release.sh <version> --no-docker        # skip image push
#   scripts/release.sh <version> --no-gh-release    # skip GH release
#   scripts/release.sh <version> --dry-run          # print side-effect cmds
#
# Example: scripts/release.sh 0.6.4

set -euo pipefail

BUILD_ONLY=0
NO_DOCKER=0
NO_GH_RELEASE=0
DRY_RUN=0
NEW_VERSION=""

for arg in "$@"; do
  case "$arg" in
    --build-only)    BUILD_ONLY=1 ;;
    --no-docker)     NO_DOCKER=1 ;;
    --no-gh-release) NO_GH_RELEASE=1 ;;
    --dry-run)       DRY_RUN=1 ;;
    -h|--help)
      sed -n '2,13p' "$0" | sed 's/^# \?//'
      exit 0 ;;
    -*)
      echo "error: unknown flag $arg" >&2
      exit 1 ;;
    *)
      if [[ -z "$NEW_VERSION" ]]; then
        NEW_VERSION="$arg"
      else
        echo "error: unexpected argument $arg" >&2
        exit 1
      fi ;;
  esac
done

if [[ -z "$NEW_VERSION" ]]; then
  echo "usage: $0 <version> [--build-only|--no-docker|--no-gh-release|--dry-run]" >&2
  exit 1
fi
if ! [[ "$NEW_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: version must be X.Y.Z, got $NEW_VERSION" >&2
  exit 1
fi

run() {
  if (( DRY_RUN )); then
    echo "DRY-RUN: $*"
  else
    "$@"
  fi
}

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

echo "==> Build phase complete."
echo "    dist/ contains kdef + kdef-lsp binaries, .deb/.rpm/.apk packages"
echo "    dist/charts/kdef-controller-${NEW_CHART_VERSION}.tgz"
echo "    vscode-extension/kdef-${NEW_VERSION}.vsix"
echo "    argocd-plugin/kdef (linux-amd64 binary staged for Docker build)"
echo

if (( BUILD_ONLY )); then
  echo "==> --build-only: stopping before side-effects. Artifacts in dist/."
  exit 0
fi

# -----------------------------------------------------------------------------
# 8. Commit, tag, push
# -----------------------------------------------------------------------------
if git diff --quiet && git diff --cached --quiet; then
  echo "==> Nothing to commit — assuming release commit already exists."
else
  echo "==> Staging and committing release"
  run git add -A
  run git commit -m "Release v${NEW_VERSION}"
fi

if git rev-parse "v${NEW_VERSION}" >/dev/null 2>&1; then
  echo "==> Tag v${NEW_VERSION} already exists locally — skipping tag"
else
  echo "==> Creating tag v${NEW_VERSION}"
  run git tag "v${NEW_VERSION}"
fi

echo "==> Pushing main + tags to origin"
run git push origin main --tags

# -----------------------------------------------------------------------------
# 9. Build + push Docker images to ghcr.io
# -----------------------------------------------------------------------------
if (( ! NO_DOCKER )); then
  echo "==> Building and pushing ArgoCD plugin image"
  run docker buildx build --platform linux/amd64 --push \
    -t "ghcr.io/gsid-nl/kdef-argocd-plugin:${NEW_VERSION}" \
    -t "ghcr.io/gsid-nl/kdef-argocd-plugin:latest" \
    -f argocd-plugin/Dockerfile argocd-plugin/

  echo "==> Building and pushing Flux controller image"
  run docker buildx build --platform linux/amd64 --push \
    -t "ghcr.io/gsid-nl/kdef-controller:${NEW_VERSION}" \
    -t "ghcr.io/gsid-nl/kdef-controller:latest" \
    -f flux-controller/Dockerfile .
else
  echo "==> --no-docker: skipping image build+push"
fi

# -----------------------------------------------------------------------------
# 10. Create GitHub release with artifacts attached
# -----------------------------------------------------------------------------
if (( ! NO_GH_RELEASE )); then
  if ! command -v gh >/dev/null 2>&1; then
    echo "warning: gh not found — skipping GitHub release" >&2
  elif gh release view "v${NEW_VERSION}" >/dev/null 2>&1; then
    echo "==> GitHub release v${NEW_VERSION} already exists — skipping"
  else
    echo "==> Creating GitHub release v${NEW_VERSION}"
    NOTES_FILE="$(mktemp)"
    trap 'rm -f "$NOTES_FILE"' EXIT
    if [[ -f release-notes.md ]]; then
      awk -v ver="v${NEW_VERSION}" '
        $0 ~ "^## kdef " ver { flag=1; next }
        /^---$/ && flag { exit }
        /^## kdef v/ && flag { exit }
        flag { print }
      ' release-notes.md > "$NOTES_FILE"
    fi
    if [[ ! -s "$NOTES_FILE" ]]; then
      echo "Release v${NEW_VERSION}." > "$NOTES_FILE"
    fi

    # Collect artifacts — glob so we don't hard-code filenames.
    shopt -s nullglob
    ARTIFACTS=(
      dist/kdef-linux-amd64
      dist/kdef-linux-arm64
      dist/kdef-darwin-amd64
      dist/kdef-darwin-arm64
      dist/kdef-windows-amd64.exe
      dist/kdef-lsp-linux-amd64
      dist/kdef-lsp-linux-arm64
      dist/kdef-lsp-darwin-amd64
      dist/kdef-lsp-darwin-arm64
      dist/kdef-lsp-windows-amd64.exe
    )
    ARTIFACTS+=( dist/kdef_${NEW_VERSION}_*.deb )
    ARTIFACTS+=( dist/kdef-${NEW_VERSION}-1.*.rpm )
    ARTIFACTS+=( dist/kdef_${NEW_VERSION}_*.apk )
    ARTIFACTS+=( dist/charts/kdef-controller-${NEW_CHART_VERSION}.tgz )
    ARTIFACTS+=( vscode-extension/kdef-${NEW_VERSION}.vsix )

    # Read headline from release-notes first line (skip blanks).
    HEADLINE="$(head -3 "$NOTES_FILE" | grep -m1 -v '^$' || true)"
    if [[ -z "$HEADLINE" ]]; then
      TITLE="v${NEW_VERSION}"
    else
      TITLE="v${NEW_VERSION} — ${HEADLINE}"
    fi

    run gh release create "v${NEW_VERSION}" \
      --title "$TITLE" \
      --notes-file "$NOTES_FILE" \
      "${ARTIFACTS[@]}"
  fi
else
  echo "==> --no-gh-release: skipping GitHub release"
fi

echo
echo "================================================================================"
echo "Release v${NEW_VERSION} complete."
echo "  GitHub:       https://github.com/gsid-nl/kdef/releases/tag/v${NEW_VERSION}"
echo "  Controller:   ghcr.io/gsid-nl/kdef-controller:${NEW_VERSION}"
echo "  ArgoCD:       ghcr.io/gsid-nl/kdef-argocd-plugin:${NEW_VERSION}"
echo
echo "Optional next step:"
echo "  cd vscode-extension && npx vsce publish --packagePath kdef-${NEW_VERSION}.vsix"
echo "================================================================================"

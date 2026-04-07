# Contributing to kdef

Thank you for your interest in contributing to kdef! This document provides guidelines and information about contributing to this project.

## How to Contribute

### Reporting Issues

- Use [GitHub Issues](https://github.com/gsid-nl/kdef/issues) to report bugs or request features
- Before creating a new issue, search existing issues to avoid duplicates
- Include clear steps to reproduce bugs, with your kdef version (`kdef version`)
- For feature requests, describe the use case and why existing features don't cover it

### Submitting Changes

1. **Fork** the repository
2. **Create a branch** from `main` for your changes:
   ```bash
   git checkout -b feature/my-feature
   ```
3. **Make your changes** — follow the code style below
4. **Test your changes**:
   ```bash
   make test
   make build
   # Test with your own .kdef files
   ./kdef render --dir your-test/
   ```
5. **Commit** with a clear message describing *why*, not just *what*
6. **Push** and create a Pull Request against `main`

### Pull Request Guidelines

- Keep PRs focused — one feature or fix per PR
- Include tests for new features
- Update testdata fixtures if the syntax changes
- Update README.md if adding user-facing features
- All tests must pass (`make test`)
- The binary must build cleanly (`make build`)

## Development Setup

### Prerequisites

- Go 1.22 or later
- kubectl (for testing `diff`/`apply` commands)
- A Kubernetes cluster (for integration testing)

### Building

```bash
# Build the binary
make build

# Run tests
make test

# Clean
make clean
```

### Project Structure

```
cmd/kdef/              — CLI entry point
internal/
  cli/                 — Cobra command definitions
  parser/              — HCL parsing (variables, deployments, cronjobs, etc.)
  generator/           — K8s object generation (Deployment, Service, Ingress, etc.)
  importer/            — Import from cluster/YAML files
  types/               — Shared type definitions
  version/             — Version info (set at build time)
testdata/              — Test fixtures
argocd-plugin/         — ArgoCD Config Management Plugin
vscode-extension/      — VS Code extension (syntax highlighting + snippets)
```

### Adding a New Block Type

1. Define the type in `internal/types/`
2. Add it to `KdefConfig` in `internal/types/config.go`
3. Create a parser in `internal/parser/`
4. Register it in `topLevelSchema` in `internal/parser/app.go`
5. Add the block handler in `parseBlocksFromBody`
6. Create a generator in `internal/generator/`
7. Wire it into `Generate()` in `internal/generator/generator.go`
8. Add importer support in `internal/importer/`
9. Add testdata fixtures
10. Update the README

### Adding a New Attribute

1. Add the field to the relevant type in `internal/types/`
2. Add it to the parser schema and parsing logic
3. Use it in the generator
4. Capture it in the importer
5. Add to testdata

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions focused — one responsibility per function
- Use descriptive variable names
- Add comments only where the logic isn't self-evident
- Prefer simplicity over abstraction

## Testing

- **Unit tests**: `go test ./...`
- **Testdata fixtures**: each directory in `testdata/` is a self-contained kdef project
- **Round-trip testing**: import → render → compare against original

```bash
# Run all tests
make test

# Test a specific fixture
./kdef render --dir testdata/with-ingress

# Round-trip test with a cluster
./kdef import --namespace my-app --output-dir /tmp/test/
./kdef render --dir /tmp/test/
./kdef diff --dir /tmp/test/
```

## Community

- **Issues**: [github.com/gsid-nl/kdef/issues](https://github.com/gsid-nl/kdef/issues)
- **Discussions**: [github.com/gsid-nl/kdef/discussions](https://github.com/gsid-nl/kdef/discussions)

## License

By contributing to kdef, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).

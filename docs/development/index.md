# Development Guide

## Repository layout

```
cmd/              # Manager entry point (main.go)
api/v1alpha1/     # CRD type definitions (Go structs + kubebuilder markers)
internal/
  controller/     # One reconciler per CRD + helpers
  concourse/      # HTTP client construction and cache
config/
  crd/            # Generated CRD YAML manifests
  rbac/           # Generated RBAC resources
  samples/        # Example CRs for all 7 resource types
  default/        # Default kustomization (metrics, cert patches)
test/
  e2e/            # End-to-end tests
  integration/    # Integration tests (require live Concourse)
docs/             # This documentation site
```

## Setting up the development environment

```bash
git clone https://github.com/jakobmoellerdev/concourse-operator
cd concourse-operator
go mod download
```

Optionally use the provided dev container:

```bash
# VSCode: "Reopen in Container"
# Or manually:
docker build -f .devcontainer/Dockerfile -t concourse-operator-dev .
```

## Running locally

```bash
# Install CRDs into your current cluster context
make install

# Run the operator process locally (hot-reloads on ^C + re-run)
make run
```

## Code generation

After modifying type definitions in `api/v1alpha1/`:

```bash
# Regenerate DeepCopy methods (zz_generated.deepcopy.go)
make generate

# Regenerate CRD YAML, RBAC from kubebuilder markers
make manifests
```

Always run both after changing types.

## Linting

```bash
make lint       # run golangci-lint
make lint-fix   # auto-fix where possible
```

The linter configuration lives in `.golangci.yml`.

## Building the container image

```bash
make docker-build IMG=<registry>/concourse-operator:dev
make docker-push  IMG=<registry>/concourse-operator:dev
```

## Commit conventions

This project uses [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add Resource pinnedVersion support
fix: evict cache entry on instance finalizer
docs: add authentication guide
test: add integration test for team deletion
chore: bump go-concourse to v4.2.2
```

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Write tests first (see [Testing](testing.md))
4. Implement the change
5. Run `make lint test` — both must pass
6. Submit a pull request against `main`

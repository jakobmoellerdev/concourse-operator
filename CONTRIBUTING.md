# Contributing

## Prerequisites

| Tool | Version | Purpose |
| ------ | --------- | --------- |
| Go | 1.26.2+ | Build and test |
| Docker | 17.03+ | Local Concourse fixture, image builds |
| kubectl | 1.11.3+ | Cluster interaction |
| Kind | latest | E2E tests only |

All other tools (controller-gen, kustomize, golangci-lint, setup-envtest) are installed locally under `bin/` by the Makefile on first use.

## Getting Started

```bash
git clone https://github.com/jakobmoellerdev/concourse-operator.git
cd concourse-operator
go mod tidy
```

## Development Workflow

After editing types in `api/v1alpha1/`, regenerate code and manifests:

```bash
make manifests   # regenerate CRDs and RBAC from marker comments
make generate    # regenerate DeepCopy methods
```

Format and lint before every commit:

```bash
make fmt
make vet
make lint        # runs golangci-lint
make lint-fix    # auto-fix where possible
```

## Running Tests

### Unit tests

No external dependencies needed — uses envtest (embedded etcd + API server).

```bash
make test
```

### Integration tests

Requires a running Concourse instance. The repo ships a docker-compose fixture.

```bash
make concourse-up          # starts Concourse at http://localhost:8080 (test/test)
make test-integration
make concourse-down
```

See [`docs/testing.md`](docs/testing.md) for details on what the tests cover and how credentials work.

### E2E tests

Requires Kind. The Makefile creates and destroys an isolated cluster automatically.

```bash
make setup-test-e2e
make test-e2e
make cleanup-test-e2e
```

## Local Operator Development

Run the operator against a local Concourse instance:

```bash
# 1. Start Concourse
make concourse-up

# 2. Create the credential secret (in whichever cluster make run targets)
kubectl create secret generic concourse-local-credentials \
  --from-literal=password=test

# 3. Install CRDs and run the operator
make install
make run

# 4. Apply sample CRs
kubectl apply -f config/samples/
```

The samples are ordered so that applying the whole directory works without conflicts (Instance → Team → Pipeline → Job / Build / Resource / Worker).

## Submitting Changes

- Use [Conventional Commits](https://www.conventionalcommits.org/): `feat`, `fix`, `test`, `refactor`, `docs`, `chore`, `ci`, `perf`
- All CI checks must pass before merge: `lint`, `test`, `test-integration`, `test-e2e`
- No extra secrets are needed — the Concourse compose file is self-contained and runs in CI as-is

## License

Apache 2.0 — see [LICENSE](LICENSE).

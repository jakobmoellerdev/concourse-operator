# Testing

## Test suites

=== "Unit tests"

    Run against a fake Kubernetes API server (envtest). No live Concourse required.

    ```bash
    make test
    ```

    Tests live in `internal/controller/*_controller_test.go` and use [Ginkgo](https://onsi.github.io/ginkgo/) + [Gomega](https://onsi.github.io/gomega/).

=== "Integration tests"

    Run against a real Concourse instance. Require `make concourse-up` first.

    ```bash
    make test-integration
    # equivalent:
    go test -tags integration -count=1 -timeout 120s ./test/integration/ -v
    ```

=== "E2E tests"

    Full end-to-end tests against a running cluster with the operator deployed.

    ```bash
    make test-e2e
    ```

---

## Local Concourse for integration tests

### Quick start

```bash
# 1. Start Concourse (uses the bundled docker-compose.yml)
make concourse-up

# 2. Run the integration tests
make test-integration

# 3. Tear down when done
make concourse-down
```

The Concourse instance starts at **http://localhost:8080** with credentials **test / test**.

### What `concourse-up` does

`make concourse-up` is shorthand for:

```bash
docker compose up -d
# then polls http://localhost:8080/api/v1/info until healthy
```

The `docker-compose.yml` at the repo root is the canonical [Concourse quick-start](https://concourse-ci.org/docs/getting-started/quick-start/) compose file. It starts:

| Service | Purpose |
|---------|---------|
| `concourse-db` | PostgreSQL database for Concourse state |
| `concourse` | Concourse web + worker in quickstart mode |

Key settings baked in:

| Setting | Value |
|---------|-------|
| URL | `http://localhost:8080` |
| Username | `test` |
| Password | `test` |
| Cluster name | `tutorial` |
| Worker runtime | `containerd` |

---

## Integration test coverage

Tests live under `test/integration/` and use the `integration` build tag so they are excluded from the normal `make test` run.

| Test group | What it verifies |
|------------|-----------------|
| Client connectivity | `GetInfo` returns version `8.2.1`, `ListWorkers` returns at least one worker |
| Team management | Create a team via `CreateOrUpdate`, list teams, delete team |
| Pipeline management | Set pipeline config, pause/unpause, delete |
| Resource management | List resource versions, trigger a resource check |
| Build management | Trigger a job build, fetch build status |

### Token acquisition

The tests obtain a short-lived OAuth bearer token at suite startup using the password grant:

```
POST http://localhost:8080/sky/issuer/token
  grant_type=password
  username=test
  password=test
  client: fly / Y29uY291cnNlLXdlYgo=
```

This is exactly what `fly login` does under the hood.

---

## Testing the operator against local Concourse

### 1. Create the credential secret

```bash
kubectl create secret generic concourse-local-credentials \
  --from-literal=password=test
```

### 2. Start the operator

```bash
make install   # install CRDs into your cluster
make run       # run the operator process locally
```

### 3. Apply samples

```bash
kubectl apply -f config/samples/
```

The samples reference `concourseinstance-sample` which points at `http://localhost:8080` using the `concourse-local-credentials` secret. They are ordered such that applying the full directory works without conflicts (Instance → Team → Pipeline → Job / Resource / Worker).

---

## CI workflow

The integration tests run automatically on every push and pull request via `.github/workflows/test-integration.yml`. The workflow:

1. Checks out the code.
2. Starts Concourse with `docker compose up -d` (uses the repo-local `docker-compose.yml`).
3. Polls `/api/v1/info` until Concourse is healthy (up to 120 s).
4. Runs `make test-integration`.

No extra secrets or infrastructure are needed — the compose file is self-contained.

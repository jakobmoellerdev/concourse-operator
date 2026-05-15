# Prerequisites

## For the Quick Start (local development)

| Tool | Minimum version | Purpose |
|------|----------------|---------|
| Docker (or Podman) | 17.03 | Running the bundled Concourse docker-compose stack |
| docker compose | v2 | `make concourse-up` / `make concourse-down` |
| kubectl | v1.11.3 | Apply CRDs and resources |
| Kubernetes cluster | v1.11.3 | Runtime for the operator ([kind](https://kind.sigs.k8s.io/) or [minikube](https://minikube.sigs.k8s.io/) recommended locally) |
| Go | 1.24.6 | `make run` — running the operator outside a container |
| git | any | Clone the repository |

## For in-cluster deployment (production / staging)

Everything above, plus:

| Tool | Minimum version | Purpose |
|------|----------------|---------|
| make | any recent | Build targets |
| A container registry | — | Push the operator image (`make docker-push IMG=...`) |
| Concourse CI | v8.2.1 | Target system — can be external, not the docker-compose one |

## Optional

| Tool | Purpose |
|------|---------|
| [cert-manager](https://cert-manager.io/) | TLS for the operator metrics endpoint only — not required for Concourse connectivity |
| kustomize | Customizing the deployment manifests; already bundled via `make` targets |

## Local Concourse stack

The bundled `docker-compose.yml` runs everything you need for local testing:

```
http://localhost:8080   Concourse web UI + API
username: test
password: test
```

No additional Concourse setup is needed for the Quick Start — `make concourse-up` handles everything. See [Quick Start](index.md) for the full walkthrough.

## Cluster access for the operator

When running in-cluster, the operator's `ServiceAccount` needs:

- Read access to `Secrets` in the same namespace (for Concourse credentials)
- Full CRUD on all 7 CRD types and their `status` subresources
- Create/patch access to `Events`

All RBAC resources are generated automatically by `make deploy`. No manual setup required.

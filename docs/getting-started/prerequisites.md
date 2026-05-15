# Prerequisites

## Required software

| Tool | Minimum version | Purpose |
|------|----------------|---------|
| kubectl | v1.11.3 | Apply CRDs and resources |
| Kubernetes cluster | v1.11.3 | Runtime for the operator |
| Concourse CI | v8.2.1 | Target system the operator manages |

## Building from source (optional)

Only needed if you are building the operator image yourself.

| Tool | Minimum version |
|------|----------------|
| Go | 1.24.6 |
| Docker | 17.03 |
| make | any recent |

## Cluster access

The operator needs:

- Permission to read `Secrets` in the namespaces where `Instance` resources live (for credentials)
- Permission to create `Events` (for status logging)
- RBAC resources are generated automatically by kustomize — see [Installation](installation.md)

## Network connectivity

The operator pod must be able to reach the Concourse ATC URL specified in each `Instance.spec.url`. If your Concourse is behind a private network or VPN, ensure the operator's namespace has the appropriate egress rules.

## Optional: cert-manager

[cert-manager](https://cert-manager.io/) is only required if you enable TLS for the operator's metrics endpoint (the `cert_metrics_manager_patch.yaml` kustomize patch). It is **not** required for connecting to Concourse.

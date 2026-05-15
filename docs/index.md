# concourse-operator

**Manage Concourse CI resources declaratively from Kubernetes.**

concourse-operator is a Kubernetes operator that brings the full Concourse CI lifecycle under GitOps control. Instead of clicking through the Concourse UI or running `fly` commands, you declare your pipelines, teams, and workers as Kubernetes custom resources — and the operator keeps Concourse in sync.

<div class="grid cards" markdown>

-   **7 Custom Resource Definitions**

    Cover every layer of Concourse: instances, teams, pipelines, jobs, builds, resources, and workers.

-   **Config drift detection**

    Pipeline configurations are SHA-256 hashed. The operator only calls the Concourse API when content changes.

-   **Thread-safe client cache**

    Authenticated Concourse clients are cached and evicted automatically when credentials rotate.

-   **GitOps-ready**

    All state lives in Kubernetes. Commit your CRs, push, and let the operator reconcile.

</div>

## Quick install

```bash
# Install CRDs
kubectl apply -k github.com/jakobmoellerdev/concourse-operator/config/crd

# Deploy the operator
kubectl apply -k github.com/jakobmoellerdev/concourse-operator/config/default
```

Then create a `Secret` with your Concourse password and a `Instance` pointing at your server. See [Getting Started](getting-started/index.md) for the full walkthrough.

## Project status

| Property | Value |
|----------|-------|
| API version | `v1alpha1` |
| Concourse compatibility | v8.2.1+ |
| Kubernetes | v1.11.3+ |
| Go | 1.24.6+ |

!!! warning "Alpha API"
    All resources are `v1alpha1`. Field names and default values may change in future releases.

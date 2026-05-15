# CRD Reference

All resources belong to the API group `concourse-ci.org/v1alpha1`.

The full reference is auto-generated from Go source via [crd-ref-docs](https://github.com/elastic/crd-ref-docs). Run `make docs-generate` to refresh it after changing types.

## Resources

| Kind | Purpose | Parent ref |
|------|---------|-----------|
| [Instance](api.md#instance) | Connection to a Concourse server | — (root) |
| [Team](api.md#team) | Concourse team + RBAC roles | `instanceRef` |
| [Pipeline](api.md#pipeline) | Pipeline configuration | `teamRef` |
| [Job](api.md#job) | Job pause state + build trigger | `pipelineRef` |
| [Build](api.md#build) | Build tracking and abort | `jobRef` |
| [Resource](api.md#resource) | Resource pin + check interval | `pipelineRef` |
| [Worker](api.md#worker) | Worker lifecycle management | `instanceRef` |

## Common field types

| Type | Description |
|------|-------------|
| [LocalObjectReference](api.md#localobjectreference) | References a resource in the same namespace |
| [SecretKeySelector](api.md#secretkeyselector) | Points at a key inside a Kubernetes `Secret` |

## kubectl quick reference

```bash
# List all operator resources
kubectl get instance,team,pipeline,job,build,resource,worker

# Describe a resource (shows events and conditions)
kubectl describe pipeline my-pipeline

# Watch build status
kubectl get build --watch
```

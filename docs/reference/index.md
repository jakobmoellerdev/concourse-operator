# CRD Reference

All resources belong to the API group `concourse-ci.org/v1alpha1`.

The full reference is auto-generated from Go source via [crd-ref-docs](https://github.com/elastic/crd-ref-docs). Run `make docs-generate` to refresh it after changing types.

## Resources

| Kind | Short names | Purpose | Parent ref |
| ------ | ------------- | --------- | ----------- |
| [Instance](api.md#instance) | `cci`, `concinst` | Connection to a Concourse server | — (root) |
| [Team](api.md#team) | `cct` | Concourse team + RBAC roles | `instanceRef` |
| [Pipeline](api.md#pipeline) | `ccp` | Pipeline configuration (`inline` or `configMapRef`) | `teamRef` |
| [Job](api.md#job) | `ccj` | Job pause state; create a Build CR to trigger | `pipelineRef` |
| [Build](api.md#build) | `ccb` | Build tracking, adopt (`buildID`), cancel | `jobRef` |
| [Resource](api.md#resource) | `ccr` | Pipeline resource projection (pin + check) | `pipelineRef` |
| [Worker](api.md#worker) | `ccw` | Worker lifecycle (`Running` / `Draining` / `Removed`) | `instanceRef` |

## Common field types

| Type | Description |
| ------ | ------------- |
| [LocalObjectReference](api.md#localobjectreference) | References a resource by `name` and optional `namespace` |
| [SecretKeySelector](api.md#secretkeyselector) | Points at a key inside a Kubernetes `Secret` |
| [ConfigMapKeyRef](api.md#configmapkeyref) | Points at a key inside a Kubernetes `ConfigMap` |
| [ReclaimPolicy](api.md#reclaimpolicy) | `Delete` or `Orphan` (Team / Pipeline) |

Cross-namespace `LocalObjectReference` values are allowed only when `Instance.spec.allowedNamespaces` includes the child's namespace (or `"*"`).

## kubectl quick reference

```bash
# List all operator resources (kinds or short names)
kubectl get instance,team,pipeline,job,build,resource,worker
kubectl get cci,cct,ccp,ccj,ccb,ccr,ccw

# Describe a resource (shows events and conditions)
kubectl describe pipeline my-pipeline

# Watch build status
kubectl get build --watch
```

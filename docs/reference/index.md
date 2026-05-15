# CRD Reference

All resources belong to the API group `concourse-ci.org/v1alpha1`.

## Resources

| Kind | Purpose | Parent ref |
|------|---------|-----------|
| [Instance](instance.md) | Connection to a Concourse server | — (root) |
| [Team](team.md) | Concourse team + RBAC roles | `instanceRef` |
| [Pipeline](pipeline.md) | Pipeline configuration | `teamRef` |
| [Job](job.md) | Job pause state + build trigger | `pipelineRef` |
| [Build](build.md) | Build tracking and abort | `jobRef` |
| [Resource](resource.md) | Resource pin + check interval | `pipelineRef` |
| [Worker](worker.md) | Worker lifecycle management | `instanceRef` |

## Common field types

### LocalObjectReference

Used for all `*Ref` fields. References a resource **in the same namespace**.

```yaml
instanceRef:
  name: my-concourse
```

### SecretKeySelector

Used for credential references. Points at a key inside a Kubernetes `Secret`.

```yaml
passwordRef:
  name: my-secret     # Secret name
  key: password       # Key within the Secret data map
```

### Status conditions

All resources expose a `conditions` array conforming to `metav1.Condition`:

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      reason: ReconcileSucceeded
      message: ""
      lastTransitionTime: "2026-01-01T00:00:00Z"
      observedGeneration: 3
```

## kubectl quick reference

```bash
# List all operator resources
kubectl get instance,team,pipeline,job,build,resource,worker

# Describe a resource (shows events and conditions)
kubectl describe pipeline my-pipeline

# Watch status
kubectl get build --watch

# Short names (set by kubebuilder printcolumns)
kubectl get ci    # Instance
kubectl get ct    # Team
kubectl get cp    # Pipeline
```

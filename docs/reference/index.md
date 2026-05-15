# CRD Reference

All resources belong to the API group `concourse.concourse-ci.org/v1alpha1`.

## Resources

| Kind | Purpose | Parent ref |
|------|---------|-----------|
| [ConcourseInstance](concourseinstance.md) | Connection to a Concourse server | — (root) |
| [ConcourseTeam](concourseteam.md) | Concourse team + RBAC roles | `instanceRef` |
| [ConcoursePipeline](concoursepipeline.md) | Pipeline configuration | `teamRef` |
| [ConcourseJob](concoursejob.md) | Job pause state + build trigger | `pipelineRef` |
| [ConcourseBuild](concoursebuild.md) | Build tracking and abort | `jobRef` |
| [ConcourseResource](concourseresource.md) | Resource pin + check interval | `pipelineRef` |
| [ConcourseWorker](concourseworker.md) | Worker lifecycle management | `instanceRef` |

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
kubectl get concourseinstance,concourseteam,concoursepipeline,concoursejob,concoursebuild,concourseresource,concourseworker

# Describe a resource (shows events and conditions)
kubectl describe concoursepipeline my-pipeline

# Watch status
kubectl get concoursebuild --watch

# Short names (set by kubebuilder printcolumns)
kubectl get ci    # ConcourseInstance
kubectl get ct    # ConcourseTeam
kubectl get cp    # ConcoursePipeline
```

# Resource

Manages a resource within a Concourse pipeline: controls pinned versions and check intervals.

## Example

```yaml
apiVersion: concourse-ci.org/v1alpha1
kind: Resource
metadata:
  name: my-repo
spec:
  pipelineRef:
    name: my-pipeline
  resourceName: my-repo
  pinnedVersion:
    ref: abc123def456
  checkInterval: 10m
```

## Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `pipelineRef` | [LocalObjectReference](index.md#localobjectreference) | yes | — | The pipeline that contains this resource. |
| `resourceName` | string | no | `metadata.name` | Resource name within the pipeline. Defaults to the CR name. |
| `pinnedVersion` | map[string]string | no | — | Version fields to pin. Structure depends on resource type. When set, calls `PinResourceVersion`; when cleared, calls `UnpinResource`. |
| `checkInterval` | duration | no | — | If set, the operator triggers a resource check at this interval. |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []metav1.Condition | `Ready` condition. |
| `lastChecked` | metav1.Time | Timestamp of the last resource check triggered by the operator. |
| `latestVersion` | map[string]string | Most recent version metadata from Concourse. |
| `pinnedVersionID` | integer | Concourse resource version ID currently pinned (0 if none). |
| `pinned` | boolean | Whether a version is currently pinned in Concourse. |
| `observedGeneration` | int64 | Generation of the spec that produced the current status. |

## Usage notes

- **Pinned version format:** The keys and values of `pinnedVersion` are resource-type specific. For `git` resources, a common pin is `{"ref": "<commit-sha>"}`. For `registry-image`, `{"digest": "sha256:..."}`.
- **Unpinning:** Remove `pinnedVersion` from the spec (or set it to `{}`) to unpin. The operator calls `UnpinResource` on the next reconcile.
- **Check interval:** `checkInterval` is independent of pin state. Set it to trigger periodic checks without waiting for Concourse's built-in interval.
- `resourceName` defaults to `metadata.name`. Use an explicit value when the Kubernetes name and Concourse resource name differ.

# ConcourseJob

Controls a Concourse job: manages its pause state and optionally triggers a new build.

## Example

```yaml
apiVersion: concourse.concourse-ci.org/v1alpha1
kind: ConcourseJob
metadata:
  name: my-job
spec:
  pipelineRef:
    name: my-pipeline
  jobName: build
  paused: false
  triggerBuild: false
```

## Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `pipelineRef` | [LocalObjectReference](index.md#localobjectreference) | yes | — | The pipeline that contains this job. |
| `jobName` | string | no | `metadata.name` | Job name within the pipeline. Defaults to the CR name. |
| `paused` | boolean | no | `false` | Whether the job should be paused in Concourse. |
| `triggerBuild` | boolean | no | `false` | If `true`, trigger a build on the next reconcile and create a `ConcourseBuild` CR. |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []metav1.Condition | `Ready` condition. |
| `paused` | boolean | Actual pause state in Concourse at last reconcile. |
| `nextBuildName` | string | Name of the `ConcourseBuild` CR created by the last `triggerBuild`. |
| `observedGeneration` | int64 | Generation of the spec that produced the current status. |

## Usage notes

- `jobName` defaults to `metadata.name`. Use an explicit `jobName` when the Kubernetes name and the Concourse job name differ.
- `triggerBuild: true` is a one-shot signal: once the build is triggered and a `ConcourseBuild` CR is created, you should set `triggerBuild: false` to avoid triggering on every subsequent reconcile. The operator does not automatically reset this field.
- The created `ConcourseBuild` CR name is stored in `status.nextBuildName`.

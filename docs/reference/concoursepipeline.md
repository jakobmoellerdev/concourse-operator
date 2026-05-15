# ConcoursePipeline

Manages a Concourse pipeline: uploads configuration, controls pause/expose state, and detects config drift via SHA-256 hashing.

## Examples

=== "Inline config"

    ```yaml
    apiVersion: concourse.concourse-ci.org/v1alpha1
    kind: ConcoursePipeline
    metadata:
      name: hello-world
    spec:
      teamRef:
        name: my-team
      pipelineName: hello-world
      config:
        inline: |
          jobs:
            - name: hello
              plan:
                - task: say-hello
                  config:
                    platform: linux
                    image_resource:
                      type: registry-image
                      source: { repository: alpine }
                    run:
                      path: echo
                      args: ["Hello, World!"]
      paused: false
      exposed: false
    ```

=== "ConfigMap reference"

    ```yaml
    apiVersion: concourse.concourse-ci.org/v1alpha1
    kind: ConcoursePipeline
    metadata:
      name: my-pipeline
    spec:
      teamRef:
        name: my-team
      config:
        configMapRef:
          name: pipeline-config
          key: pipeline.yml
      vars:
        - name: git-uri
          value: https://github.com/example/repo
        - name: git-password
          valueFrom:
            name: git-credentials
            key: password
    ```

## Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `teamRef` | [LocalObjectReference](index.md#localobjectreference) | yes | — | The team this pipeline belongs to. |
| `pipelineName` | string | no | `metadata.name` | Pipeline name in Concourse. Defaults to the CR name. |
| `config` | [PipelineConfig](#pipelineconfig) | yes | — | Pipeline YAML source. |
| `vars` | [][PipelineVar](#pipelinevar) | no | — | Variables passed to the pipeline config (like `fly set-pipeline -v`). |
| `paused` | boolean | no | `false` | Whether the pipeline should be paused in Concourse. |
| `exposed` | boolean | no | `false` | Whether the pipeline is publicly visible without authentication. |

### PipelineConfig

Exactly one of `inline` or `configMapRef` must be set.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `inline` | string | no | Pipeline YAML embedded directly in the CR. |
| `configMapRef` | [ConfigMapKeyRef](#configmapkeyref) | no | Reference to a ConfigMap key containing pipeline YAML. |

### ConfigMapKeyRef

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Name of the ConfigMap. |
| `key` | string | yes | Key within the ConfigMap. |

### PipelineVar

Exactly one of `value` or `valueFrom` must be set.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Variable name (used as `((name))` in pipeline YAML). |
| `value` | string | no | Plain-text value. |
| `valueFrom` | [SecretKeySelector](index.md#secretkeyselector) | no | Read value from a Kubernetes Secret. |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []metav1.Condition | `Ready` condition. |
| `pipelineID` | integer | Numeric pipeline ID assigned by Concourse. |
| `configHash` | string | SHA-256 hash of the last successfully applied pipeline YAML. |
| `paused` | boolean | Actual pause state in Concourse at last reconcile. |
| `exposed` | boolean | Actual exposed state in Concourse at last reconcile. |
| `observedGeneration` | int64 | Generation of the spec that produced the current status. |

## Usage notes

- **Drift detection:** The operator hashes the fully-resolved pipeline YAML (after variable substitution). If `status.configHash` matches the current hash, no `SetPipeline` call is made — only pause/expose state is reconciled.
- **ConfigMap changes:** The operator does not watch ConfigMaps for changes. To trigger reconciliation after updating a ConfigMap, touch the `ConcoursePipeline` (e.g. update an annotation).
- **Variable secrets:** Variables using `valueFrom` are read from Secrets at reconcile time. Rotating the Secret value requires a CR touch to re-trigger reconciliation.

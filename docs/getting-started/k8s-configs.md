# Kubernetes Config & Secret Injection

The `concourse-operator` allows you to automatically inject Kubernetes `ConfigMap` and `Secret` objects directly into your Concourse pipelines as versioned, first-class resource inputs.

---

## Overview

When you declare `spec.k8sConfigs` on a `Pipeline` Custom Resource, the operator automatically:

1. **Injects the `k8s-config` resource type** into your Concourse pipeline using a lightweight, minimal container image.
2. **Generates matching `k8s-config` resources** for every specified ConfigMap or Secret in the pipeline definition.
3. **Preserves standard Concourse semantics:** Your jobs consume the files using standard `get:` steps and can use Concourse's `load_var` step or mount directories directly into task containers.

```
Kubernetes Cluster                          Concourse CI
┌───────────────────────┐                  ┌─────────────────────────────────────────┐
│ ConfigMap / Secret    │                  │ Pipeline                                │
│ ┌───────────────────┐ │                  │   ┌─────────────────────────────────┐   │
│ │ app.json: {...}   │ │ ──(k8s-config)──►│   │ get: app-config (input folder)  │   │
│ │ settings.yaml     │ │                  │   │   └─► task: run-build           │   │
│ └───────────────────┘ │                  │   └─────────────────────────────────┘   │
└───────────────────────┘                  └─────────────────────────────────────────┘
```

---

## Example Usage

### 1. Create a ConfigMap in Kubernetes

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-app-config
  namespace: default
data:
  app.json: |
    {
      "environment": "staging",
      "port": 8080
    }
  settings.yaml: |
    debug: true
    log_level: info
```

### 2. Declare `k8sConfigs` in your Pipeline CR

```yaml
apiVersion: concourse-ci.org/v1alpha1
kind: Pipeline
metadata:
  name: app-deployment
  namespace: default
spec:
  teamRef:
    name: main

  # Declare Kubernetes ConfigMaps and Secrets to auto-wire:
  k8sConfigs:
    - name: app-config
      configMapRef:
        name: my-app-config
      trigger: true # Triggers downstream jobs when ConfigMap data changes!
    - name: app-secrets
      secretRef:
        name: my-app-secret

  config:
    inline: |
      jobs:
        - name: deploy
          plan:
            # 1. Fetch the ConfigMap files via get step
            - get: app-config
              trigger: true
            - get: app-secrets

            # 2. (Optional) Load values dynamically using load_var
            - load_var: settings
              file: app-config/app.json
              format: json

            # 3. Mount as input directory in task
            - task: run-deploy
              config:
                platform: linux
                image_resource:
                  type: registry-image
                  source: { repository: alpine }
                inputs:
                  - name: app-config
                  - name: app-secrets
                params:
                  APP_PORT: ((.:settings.port))
                run:
                  path: sh
                  args:
                    - -c
                    - |
                      echo "Reading config directory:"
                      ls -la app-config/
                      cat app-config/settings.yaml
                      echo "Configured port: $APP_PORT"
```

---

## How It Works

### Auto-Generated Concourse Configuration

When the operator reconciles the pipeline above, it automatically prepends the following to the configuration submitted to the Concourse ATC:

```yaml
resource_types:
  - name: k8s-config
    type: registry-image
    source:
      repository: ghcr.io/jakobmoellerdev/concourse-k8s-config-resource
      tag: latest

resources:
  - name: app-config
    type: k8s-config
    source:
      kind: ConfigMap
      name: my-app-config
      namespace: default
  - name: app-secrets
    type: k8s-config
    source:
      kind: Secret
      name: my-app-secret
      namespace: default
```

---

## Customizing the Resource Image

For air-gapped clusters, private enterprise registries, or specific image versions, you can override the `k8s-config` image at multiple levels:

### Per-Pipeline Override

```yaml
apiVersion: concourse-ci.org/v1alpha1
kind: Pipeline
metadata:
  name: airgapped-pipeline
spec:
  teamRef: { name: main }
  k8sConfigImage:
    repository: private-registry.internal/ci/concourse-k8s-config-resource
    tag: v1.0.0
  k8sConfigs:
    - name: app-config
      configMapRef: { name: my-config }
  config: ...
```

### Global Instance Default Override

```yaml
apiVersion: concourse-ci.org/v1alpha1
kind: Instance
metadata:
  name: my-concourse
spec:
  url: https://ci.example.com
  auth: ...
  defaults:
    k8sConfigImage:
      repository: harbor.corp.internal/concourse/k8s-config-resource
      tag: stable
```

---

## Cross-Namespace References

By default, referenced ConfigMaps and Secrets are resolved in the same namespace as the `Pipeline` CR. You can reference objects in other namespaces by specifying `namespace` on the reference:

```yaml
k8sConfigs:
  - name: shared-certs
    secretRef:
      name: wildcard-tls
      namespace: cert-manager
```

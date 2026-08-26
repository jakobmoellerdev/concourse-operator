# Tutorial: From Zero to Running Pipeline

This tutorial walks through every layer of the operator — from connecting to Concourse to pinning a resource version and managing a worker. All state lives in Kubernetes.

## What you will build

A fully operator-managed Concourse setup:

- An `Instance` connected to a local Concourse server
- A `Team` with owner access
- A `Pipeline` running a hello-world job
- A `Job` and `Build` to track the run
- A `Resource` with a pinned version
- A `Worker` lifecycle demonstration

---

## Part 1: Start a local Concourse

The repo ships a `docker-compose.yml` with the official Concourse quickstart:

```bash
make concourse-up
# waits until http://localhost:8080 is healthy
```

Concourse is now at `http://localhost:8080` — credentials `test / test`.

---

## Part 2: Install the operator

```bash
# Install CRDs
make install

# Run the operator locally (keeps logs in your terminal)
make run
```

Leave this terminal open. Open a second terminal for `kubectl` commands.

---

## Part 3: Connect to Concourse

Create the credentials secret and the `Instance`:

```bash
kubectl create secret generic concourse-local-credentials \
  --from-literal=password=test
```

```yaml title="01-instance.yaml"
apiVersion: concourse-ci.org/v1alpha1
kind: Instance
metadata:
  name: tutorial
spec:
  url: http://localhost:8080
  auth:
    password:
      username: test
      passwordRef:
        name: concourse-local-credentials
        key: password
  healthProbeInterval: 1m
```

```bash
kubectl apply -f 01-instance.yaml
kubectl wait --for=condition=Ready instance/tutorial --timeout=60s
kubectl get instance tutorial
# NAME       URL                    VERSION   WORKERS   READY
# tutorial   http://localhost:8080  8.2.1     1         True
```

Short name: `kubectl get cci tutorial` (also `concinst`).

---

## Part 4: Create a team

```yaml title="02-team.yaml"
apiVersion: concourse-ci.org/v1alpha1
kind: Team
metadata:
  name: tutorial-team
spec:
  instanceRef:
    name: tutorial
  teamName: main
  allowDestroy: false
  reclaimPolicy: Delete
  roles:
    - role: owner
      users:
        - local:test
```

```bash
kubectl apply -f 02-team.yaml
kubectl wait --for=condition=Ready team/tutorial-team --timeout=30s
```

Open the Concourse UI — the `main` team now has the configured owner.

`teamName` and `instanceRef` are immutable after create. Deleting this CR deletes the Concourse team unless you set `reclaimPolicy: Orphan`. Destroying the reserved `main` team also requires `allowDestroy: true`.

---

## Part 5: Deploy a pipeline

```yaml title="03-pipeline.yaml"
apiVersion: concourse-ci.org/v1alpha1
kind: Pipeline
metadata:
  name: tutorial-pipeline
spec:
  teamRef:
    name: tutorial-team
  pipelineName: hello-world
  reclaimPolicy: Delete
  config:
    inline: |
      resources:
        - name: timer
          type: time
          source:
            interval: 1h

      jobs:
        - name: hello
          plan:
            - get: timer
              trigger: true
            - task: say-hello
              config:
                platform: linux
                image_resource:
                  type: registry-image
                  source: { repository: alpine }
                run:
                  path: echo
                  args: ["Hello from concourse-operator!"]
  paused: false
  exposed: true
```

```bash
kubectl apply -f 03-pipeline.yaml
kubectl wait --for=condition=Ready pipeline/tutorial-pipeline --timeout=30s
```

The pipeline appears in the Concourse UI. Because `exposed: true`, it is visible without login.

`config` must be exactly one of `inline` or `configMapRef`. Deleting this CR deletes the Concourse pipeline unless `reclaimPolicy: Orphan`.

---

## Part 6: Trigger and track a build

Create a `Job` referencing the `hello` job, then create a `Build` to trigger it. Jobs do not have a `triggerBuild` field.

```yaml title="04-job.yaml"
apiVersion: concourse-ci.org/v1alpha1
kind: Job
metadata:
  name: tutorial-hello
spec:
  pipelineRef:
    name: tutorial-pipeline
  jobName: hello
  paused: false
```

```yaml title="04-build.yaml"
apiVersion: concourse-ci.org/v1alpha1
kind: Build
metadata:
  name: tutorial-hello-build
spec:
  jobRef:
    name: tutorial-hello
```

```bash
kubectl apply -f 04-job.yaml
kubectl apply -f 04-build.yaml
```

The operator triggers a Concourse build once (no `spec.buildID` means create). Watch it:

```bash
kubectl get build --watch
# NAME                     BUILD   STATUS    AGE
# tutorial-hello-build     1       started   3s
# tutorial-hello-build     1       succeeded 18s
```

The `status.apiURL` field contains a direct link to the build log in the Concourse UI. Set `spec.canceled: true` to abort a running build.

---

## Part 7: Pin a resource version

```yaml title="05-resource.yaml"
apiVersion: concourse-ci.org/v1alpha1
kind: Resource
metadata:
  name: tutorial-timer
spec:
  pipelineRef:
    name: tutorial-pipeline
  resourceName: timer
  checkInterval: 10m
```

```bash
kubectl apply -f 05-resource.yaml
kubectl get resource tutorial-timer
# NAME             PINNED   READY
# tutorial-timer   false    True
```

To pin a specific version, patch `pinnedVersion`:

```bash
kubectl patch resource tutorial-timer \
  --type=merge \
  -p '{"spec":{"pinnedVersion":{"time":"2026-01-01T00:00:00Z"}}}'
```

The operator looks up the version ID and calls Concourse's pin API. Remove the field to unpin.

---

## Part 8: Worker lifecycle

```yaml title="06-worker.yaml"
apiVersion: concourse-ci.org/v1alpha1
kind: Worker
metadata:
  name: tutorial-worker
spec:
  instanceRef:
    name: tutorial
  workerName: worker-1
  lifecycle: Running
```

```bash
kubectl apply -f 06-worker.yaml
kubectl get worker tutorial-worker
# NAME              WORKER     PHASE    PLATFORM   READY
# tutorial-worker   worker-1   running  linux      True
```

`workerName` is optional and defaults to `metadata.name`. To land (drain) the worker:

```bash
kubectl patch worker tutorial-worker \
  --type=merge -p '{"spec":{"lifecycle":"Draining"}}'
```

`lifecycle` values: `Running` (leave in the pool), `Draining` (land / graceful drain), `Removed` (prune). Observed `status.phase` is one of `running`, `landing`, `landed`, `retiring`, `stalled`, `missing`.

---

## Part 9: GitOps workflow

Commit all YAML files to your repository. Configure a tool like Flux or Argo CD to apply the directory. From this point:

- Changing `spec.config.inline` in `Pipeline` → operator detects SHA diff → updates Concourse
- Rotating the password `Secret` → operator watches the Secret → evicts cache → reconnects
- Creating a new `Build` CR → operator triggers one Concourse build

---

## Cleanup

Deleting Team and Pipeline CRs deletes the corresponding Concourse objects unless `reclaimPolicy: Orphan`. This tutorial uses the default `Delete` policy. The reserved `main` team is not destroyed unless `allowDestroy: true`.

```bash
kubectl delete -f 06-worker.yaml -f 05-resource.yaml -f 04-build.yaml \
  -f 04-job.yaml -f 03-pipeline.yaml -f 02-team.yaml -f 01-instance.yaml
kubectl delete secret concourse-local-credentials
make concourse-down
```

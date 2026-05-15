# Tutorial: From Zero to Running Pipeline

This tutorial walks through every layer of the operator — from connecting to Concourse to pinning a resource version and managing a worker. All state lives in Kubernetes.

## What you will build

A fully operator-managed Concourse setup:

- A `Instance` connected to a local Concourse server
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
  basicAuth:
    username: test
    passwordRef:
      name: concourse-local-credentials
      key: password
  interval: 1m
```

```bash
kubectl apply -f 01-instance.yaml
kubectl wait --for=condition=Ready concourseinstance/tutorial --timeout=60s
kubectl get concourseinstance tutorial
# NAME       URL                    VERSION   WORKERS   READY
# tutorial   http://localhost:8080  8.2.1     1         True
```

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
  roles:
    - role: owner
      users:
        - local:test
```

```bash
kubectl apply -f 02-team.yaml
kubectl wait --for=condition=Ready concourseteam/tutorial-team --timeout=30s
```

Open the Concourse UI — the `main` team now has the configured owner.

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
kubectl wait --for=condition=Ready concoursepipeline/tutorial-pipeline --timeout=30s
```

The pipeline appears in the Concourse UI. Because `exposed: true`, it is visible without login.

---

## Part 6: Trigger and track a build

Create a `Job` referencing the `hello` job, then set `triggerBuild: true`:

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
  triggerBuild: true
```

```bash
kubectl apply -f 04-job.yaml
```

The operator triggers a build and creates a `Build` CR. Watch it:

```bash
kubectl get concoursebuild --watch
# NAME                      BUILD   STATUS    AGE
# tutorial-hello-build-1    1       started   3s
# tutorial-hello-build-1    1       succeeded 18s
```

The `status.apiURL` field contains a direct link to the build log in the Concourse UI.

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
kubectl get concourseresource tutorial-timer
# NAME             PINNED   LAST-CHECKED   READY
# tutorial-timer   false    10s ago        True
```

To pin a specific version, patch the `pinnedVersion`:

```bash
kubectl patch concourseresource tutorial-timer \
  --type=merge \
  -p '{"spec":{"pinnedVersion":{"time":"2026-01-01T00:00:00Z"}}}'
```

The operator calls Concourse's pin API. Remove the field to unpin.

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
  desiredState: active
```

```bash
kubectl apply -f 06-worker.yaml
kubectl get concourseworker tutorial-worker
# NAME              PLATFORM   CONTAINERS   VOLUMES   STATE    READY
# tutorial-worker   linux      0            0         running  True
```

To land (drain) the worker:

```bash
kubectl patch concourseworker tutorial-worker \
  --type=merge -p '{"spec":{"desiredState":"land"}}'
```

---

## Part 9: GitOps workflow

Commit all six YAML files to your repository. Configure a tool like Flux or Argo CD to apply the directory. From this point:

- Changing `spec.config.inline` in `Pipeline` → operator detects SHA diff → updates Concourse
- Rotating the password `Secret` → annotate `Instance` → operator evicts cache → reconnects
- Setting `triggerBuild: false` → operator stops triggering builds on the next reconcile

---

## Cleanup

```bash
kubectl delete -f 06-worker.yaml -f 05-resource.yaml -f 04-job.yaml \
  -f 03-pipeline.yaml -f 02-team.yaml -f 01-instance.yaml
kubectl delete secret concourse-local-credentials
make concourse-down
```

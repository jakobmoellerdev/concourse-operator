# Quick Start

This guide takes you from a blank machine to a running Concourse pipeline managed entirely by the operator. No prior Concourse or Kubernetes knowledge assumed.

**Time: ~15 minutes.**

---

## What you'll need

| Requirement | Notes |
|-------------|-------|
| Docker (or compatible OCI runtime) | For running the local Concourse stack |
| A Kubernetes cluster with `kubectl` configured | [kind](https://kind.sigs.k8s.io/) or [minikube](https://minikube.sigs.k8s.io/) work fine locally |
| Go 1.24.6+ | Only needed for `make run` (local operator); skip if deploying via image |
| `git` | To clone the repo |

---

## Step 0: Clone the repository

```bash
git clone https://github.com/jakobmoellerdev/concourse-operator
cd concourse-operator
```

---

## Step 1: Start a local Concourse

The repo ships a ready-to-use `docker-compose.yml` based on the official Concourse quickstart.

```bash
make concourse-up
```

This starts two containers:

| Container | Purpose |
|-----------|---------|
| `concourse-db` | PostgreSQL — Concourse state store |
| `concourse` | Concourse web + worker (containerd runtime) |

The target polls `http://localhost:8080/api/v1/info` and waits until Concourse is healthy before returning.

**Verify:**

```bash
curl -s http://localhost:8080/api/v1/info | python3 -m json.tool
# {"version":"8.2.1","worker_version":"2.3","external_url":"http://localhost:8080","cluster_name":"tutorial"}
```

Open [http://localhost:8080](http://localhost:8080) in a browser. Log in with **test / test**.

---

## Step 2: Install the operator CRDs

```bash
make install
```

Verify the CRDs are registered:

```bash
kubectl get crd | grep concourse-ci.org
# builds.concourse-ci.org
# instances.concourse-ci.org
# jobs.concourse-ci.org
# pipelines.concourse-ci.org
# resources.concourse-ci.org
# teams.concourse-ci.org
# workers.concourse-ci.org
```

---

## Step 3: Run the operator

=== "Local (no image needed)"

    ```bash
    make run
    ```

    Runs the operator against your current `kubectl` context. Logs stream to the terminal. Leave this running and open a second terminal for the next steps.

=== "In-cluster (requires image build)"

    ```bash
    make docker-build docker-push IMG=<registry>/concourse-operator:dev
    make deploy IMG=<registry>/concourse-operator:dev
    kubectl -n concourse-operator-system rollout status deployment/concourse-operator-controller-manager
    ```

---

## Step 4: Create the credentials Secret

The operator reads Concourse credentials from a Kubernetes Secret. For the local stack the password is `test`.

```bash
kubectl create secret generic concourse-local-credentials \
  --from-literal=password=test
```

---

## Step 5: Connect the operator to Concourse

```yaml title="instance.yaml"
apiVersion: concourse-ci.org/v1alpha1
kind: Instance
metadata:
  name: local
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
kubectl apply -f instance.yaml
kubectl wait --for=condition=Ready instance/local --timeout=60s
kubectl get instance local
# NAME    URL                    VERSION   WORKERS   READY   AGE
# local   http://localhost:8080  8.2.1     1         True    5s
```

---

## Step 6: Create a team

```yaml title="team.yaml"
apiVersion: concourse-ci.org/v1alpha1
kind: Team
metadata:
  name: my-team
spec:
  instanceRef:
    name: local
  teamName: main
  roles:
    - role: owner
      users:
        - local:test
```

```bash
kubectl apply -f team.yaml
kubectl wait --for=condition=Ready team/my-team --timeout=30s
```

---

## Step 7: Deploy a pipeline

```yaml title="pipeline.yaml"
apiVersion: concourse-ci.org/v1alpha1
kind: Pipeline
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
                  args: ["Hello from concourse-operator!"]
  paused: false
  exposed: true
```

```bash
kubectl apply -f pipeline.yaml
kubectl wait --for=condition=Ready pipeline/hello-world --timeout=30s
```

Open [http://localhost:8080](http://localhost:8080) — the `hello-world` pipeline is live.

---

## Step 8: Trigger a build

```yaml title="job.yaml"
apiVersion: concourse-ci.org/v1alpha1
kind: Job
metadata:
  name: hello
spec:
  pipelineRef:
    name: hello-world
  jobName: hello
  triggerBuild: true
```

```bash
kubectl apply -f job.yaml
```

The operator triggers a build and creates a `Build` CR. Watch it complete:

```bash
kubectl get build --watch
# NAME              JOB     BUILD-ID   STATUS      AGE
# hello-build-...   hello   1          started     2s
# hello-build-...   hello   1          succeeded   14s
```

---

## Step 9: Verify end to end

```bash
kubectl get instance,team,pipeline,job,build
```

All resources should show `READY=True`. The build output is visible in Concourse at the `apiURL` printed in `kubectl describe build <name>`.

---

## Teardown

```bash
kubectl delete -f job.yaml -f pipeline.yaml -f team.yaml -f instance.yaml
kubectl delete secret concourse-local-credentials
make concourse-down
```

---

## Next steps

- [Full installation guide](installation.md) — in-cluster deploy, cert-manager, Helm
- [Tutorial](../tutorial/index.md) — resources, version pinning, worker lifecycle, GitOps
- [CRD Reference](../reference/index.md) — all fields for all resource types

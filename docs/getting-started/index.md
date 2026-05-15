# Quick Start

Get from zero to a running pipeline managed by the operator in five steps.

## Prerequisites

- A Kubernetes cluster with `kubectl` configured
- A running Concourse CI instance (v8.2.1+)
- The operator deployed — see [Installation](installation.md) if not done yet

## Step 1: Create credentials

Store your Concourse password (or token) in a Kubernetes `Secret`:

```bash
kubectl create secret generic concourse-credentials \
  --from-literal=password=<your-password>
```

## Step 2: Connect to Concourse

```yaml title="concourseinstance.yaml"
apiVersion: concourse.concourse-ci.org/v1alpha1
kind: ConcourseInstance
metadata:
  name: my-concourse
spec:
  url: https://concourse.example.com
  basicAuth:
    username: admin
    passwordRef:
      name: concourse-credentials
      key: password
  interval: 5m
```

```bash
kubectl apply -f concourseinstance.yaml
kubectl wait --for=condition=Ready concourseinstance/my-concourse --timeout=60s
```

## Step 3: Create a team

```yaml title="team.yaml"
apiVersion: concourse.concourse-ci.org/v1alpha1
kind: ConcourseTeam
metadata:
  name: my-team
spec:
  instanceRef:
    name: my-concourse
  teamName: main
  roles:
    - role: owner
      users:
        - local:admin
```

```bash
kubectl apply -f team.yaml
```

## Step 4: Deploy a pipeline

```yaml title="pipeline.yaml"
apiVersion: concourse.concourse-ci.org/v1alpha1
kind: ConcoursePipeline
metadata:
  name: hello-world
spec:
  teamRef:
    name: my-team
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
```

```bash
kubectl apply -f pipeline.yaml
```

## Step 5: Verify

```bash
kubectl get concourseinstance,concourseteam,concoursepipeline
```

You should see `Ready=True` on all three. Open your Concourse UI — the pipeline is there.

## Next steps

- [Installation guide](installation.md) — full deploy options
- [CRD Reference](../reference/index.md) — all fields for every resource
- [Tutorial](../tutorial/index.md) — end-to-end walkthrough with builds and resources

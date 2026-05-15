# Installation

## Method 1: Kustomize (recommended)

### 1. Build and push the operator image

```bash
make docker-build docker-push IMG=<registry>/concourse-operator:latest
```

### 2. Deploy

```bash
make deploy IMG=<registry>/concourse-operator:latest
```

This installs the CRDs and creates the `concourse-operator-system` namespace with the manager `Deployment`, `ServiceAccount`, and RBAC resources.

### 3. Verify

```bash
kubectl -n concourse-operator-system get pods
# NAME                                           READY   STATUS    RESTARTS
# concourse-operator-controller-manager-xxxxx    2/2     Running   0
```

---

## Method 2: Apply CRDs only

If you want to manage the operator deployment yourself, install just the CRDs:

```bash
make install
# or
kubectl apply -k config/crd
```

Then run the operator locally (useful for development):

```bash
make run
```

---

## Method 3: Single bundle YAML

Generate a single installable manifest and apply it directly:

```bash
make build-installer IMG=<registry>/concourse-operator:latest
kubectl apply -f dist/install.yaml
```

---

## RBAC

The operator creates three roles per CRD:

| Role suffix | Permissions |
|-------------|-------------|
| `-admin` | Full CRUD + status subresource + finalizers |
| `-editor` | Create, update, patch (no delete) |
| `-viewer` | Get, list, watch |

The manager's own `ClusterRole` (`concourse-operator-manager-role`) includes:

- Read/write access to all 7 CRDs and their status subresources
- Read access to `Secrets` (for credentials)
- Create/patch access to `Events`

---

## Metrics and monitoring

The manager exposes Prometheus metrics on port `8443` (with TLS) via the `/metrics` endpoint.

To enable TLS on the metrics endpoint, uncomment the cert-manager patches in `config/default/kustomization.yaml`:

```yaml
patches:
  - path: cert_metrics_manager_patch.yaml
    target:
      kind: Deployment
```

A `ServiceMonitor` resource is available in `config/prometheus/` for Prometheus Operator integration.

---

## Uninstalling

```bash
# Remove all operator-managed CRs first (otherwise finalizers will block)
kubectl delete concourseinstance,concourseteam,concoursepipeline,concoursejob,concoursebuild,concourseresource,concourseworker --all --all-namespaces

# Remove operator and CRDs
make undeploy
make uninstall
```

!!! danger "Data loss"
    Deleting Instance resources triggers finalizers that clean up the client cache. All other CRs are purely declarative — deleting them removes the Kubernetes object but does not delete the corresponding resource from Concourse.

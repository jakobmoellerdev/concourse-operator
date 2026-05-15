# Authentication

The operator authenticates to Concourse on behalf of each `ConcourseInstance`. Two authentication methods are supported; they are mutually exclusive.

## Basic auth

Username and password. The password is read from a Kubernetes `Secret`.

```yaml
apiVersion: concourse.concourse-ci.org/v1alpha1
kind: ConcourseInstance
metadata:
  name: my-concourse
spec:
  url: https://concourse.example.com
  basicAuth:
    username: admin
    passwordRef:
      name: concourse-credentials   # Secret name
      key: password                 # Key within the Secret
```

Create the Secret:

```bash
kubectl create secret generic concourse-credentials \
  --from-literal=password=<your-password>
```

**When to use:** local test environments, or when your Concourse admin account uses local auth.

---

## Token auth (Bearer token)

A pre-issued bearer token. Useful for service accounts or long-lived tokens.

```yaml
apiVersion: concourse.concourse-ci.org/v1alpha1
kind: ConcourseInstance
metadata:
  name: my-concourse
spec:
  url: https://concourse.example.com
  tokenAuth:
    tokenRef:
      name: concourse-token
      key: token
```

Create the Secret:

```bash
kubectl create secret generic concourse-token \
  --from-literal=token=<bearer-token>
```

!!! warning "Mutual exclusivity"
    Setting both `basicAuth` and `tokenAuth` is invalid. The operator will set `Ready=False` with reason `InvalidSpec`.

---

## TLS configuration

TLS settings are independent of the auth method and can be combined with either.

### Custom CA certificate

```yaml
spec:
  url: https://concourse.internal
  basicAuth:
    username: admin
    passwordRef:
      name: concourse-credentials
      key: password
  tls:
    caRef:
      name: concourse-ca
      key: ca.crt
```

```bash
kubectl create secret generic concourse-ca \
  --from-file=ca.crt=/path/to/ca.crt
```

### Insecure skip verify

```yaml
spec:
  tls:
    insecureSkipVerify: true
```

!!! danger "Development only"
    `insecureSkipVerify: true` disables certificate validation entirely. Never use this in production.

---

## Credential rotation

When you update the `Secret` that holds your password or token, the operator does **not** automatically detect the `Secret` change. To trigger a refresh:

1. Update the `Secret` with the new credential.
2. Annotate or patch the `ConcourseInstance` to bump its `resourceVersion` (e.g. add/change an annotation).

This evicts the cached client and forces a new client build with the updated credentials.

---

## Security best practices

- Use RBAC to restrict which `ServiceAccounts` can read the credential `Secrets`.
- Prefer token auth for production — avoids transmitting a password on every reconcile.
- Mount credential `Secrets` in the operator's namespace only; use `LocalObjectReference` so CRs cannot reference secrets in other namespaces.
- Never set `insecureSkipVerify: true` in production deployments.

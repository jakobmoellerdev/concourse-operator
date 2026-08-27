# Authentication

The operator authenticates to Concourse on behalf of each `Instance`. Two authentication methods are supported; they are mutually exclusive. Exactly one of `spec.auth.password` or `spec.auth.token` must be set.

## Password grant

Local-user credentials used for an OAuth2 password grant against Concourse's `sky/issuer` token endpoint. The password is read from a Kubernetes `Secret`.

```yaml
apiVersion: concourse-ci.org/v1alpha1
kind: Instance
metadata:
  name: my-concourse
spec:
  url: https://concourse.example.com
  auth:
    password:
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

Optional OAuth2 client fields (default to the well-known `fly` client):

```yaml
spec:
  auth:
    password:
      username: admin
      passwordRef:
        name: concourse-credentials
        key: password
      clientID: fly
      clientSecretRef:
        name: concourse-oauth-client
        key: secret
```

**When to use:** local test environments, or when your Concourse admin account uses local auth.

---

## Token auth (Bearer token)

A pre-issued bearer token. Useful for service accounts or long-lived tokens.

```yaml
apiVersion: concourse-ci.org/v1alpha1
kind: Instance
metadata:
  name: my-concourse
spec:
  url: https://concourse.example.com
  auth:
    token:
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
    Exactly one of `spec.auth.password` or `spec.auth.token` must be set. Setting both (or neither) is invalid. The operator will set `Ready=False` with reason `InvalidSpec`.

---

## TLS configuration

TLS settings are independent of the auth method and can be combined with either. `caRef` and `insecureSkipVerify` are mutually exclusive.

### Custom CA certificate

```yaml
spec:
  url: https://concourse.internal
  auth:
    password:
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

The Instance controller watches Secrets used for auth (`passwordRef`, `tokenRef`, `clientSecretRef`) and TLS (`caRef`). Updating those Secrets triggers a reconcile automatically — you do **not** need to annotate the `Instance`.

1. Update the `Secret` with the new credential.
2. The controller rebuilds the cached client and records the Secret generation on `status.authSecretGeneration`.

---

## Security best practices

- Use RBAC to restrict which `ServiceAccounts` can read the credential `Secrets`.
- Prefer token auth for production — avoids transmitting a password on every reconcile.
- Auth and TLS Secrets are resolved in the Instance's own namespace (`SecretKeySelector` has `name` + `key` only).
- Cross-namespace Team/Worker refs are allowed only when `Instance.spec.allowedNamespaces` includes the child's namespace (or `"*"`).
- Never set `insecureSkipVerify: true` in production deployments.

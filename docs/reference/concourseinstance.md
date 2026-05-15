# ConcourseInstance

Root resource. Represents a connection to a Concourse CI server. All other resources resolve an authenticated client through a `ConcourseInstance`.

## Example

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
      name: concourse-credentials
      key: password
  tls:
    insecureSkipVerify: false
  interval: 5m
```

## Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `url` | string | yes | — | Base URL of the Concourse ATC. Must match `^https?://`. |
| `basicAuth` | [BasicAuth](#basicauth) | no | — | Username/password authentication. Mutually exclusive with `tokenAuth`. |
| `tokenAuth` | [TokenAuth](#tokenauth) | no | — | Bearer token authentication. Mutually exclusive with `basicAuth`. |
| `tls` | [TLSConfig](#tlsconfig) | no | — | TLS configuration. Orthogonal to auth method. |
| `interval` | duration | no | `5m` | How often to reconcile even without a spec change. |

### BasicAuth

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `username` | string | yes | Concourse username. |
| `passwordRef` | [SecretKeySelector](index.md#secretkeyselector) | yes | Reference to the Secret key holding the password. |

### TokenAuth

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tokenRef` | [SecretKeySelector](index.md#secretkeyselector) | yes | Reference to the Secret key holding the bearer token. |

### TLSConfig

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `caRef` | [SecretKeySelector](index.md#secretkeyselector) | no | — | PEM-encoded CA certificate for TLS verification. |
| `insecureSkipVerify` | boolean | no | `false` | Disable TLS certificate verification. **Development only.** |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []metav1.Condition | `Ready` and `Authenticated` conditions. |
| `version` | string | Concourse server version reported by `/api/v1/info`. |
| `workerCount` | integer | Number of healthy workers at last reconcile. |
| `observedGeneration` | int64 | Generation of the spec that produced the current status. |

### Conditions

| Type | True when |
|------|-----------|
| `Ready` | Connection succeeded and all dependent resources are in sync. |
| `Authenticated` | Credentials were accepted by Concourse. |

## Usage notes

- Exactly one of `basicAuth` or `tokenAuth` must be set; both or neither is invalid.
- The `interval` field controls the minimum reconciliation frequency. The operator also reconciles on any spec change.
- A finalizer (`concourse.concourse-ci.org/instance-finalizer`) is added to each instance to clean up the client cache on deletion.
- Changing the `url` or credential `Secret` contents requires bumping `resourceVersion` (e.g. by updating an annotation) to evict the cached client.

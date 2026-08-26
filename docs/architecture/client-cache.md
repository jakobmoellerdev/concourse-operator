# Client Cache

## Purpose

Each reconciler needs an authenticated `concourse.Client` to call the Concourse API. Building a new HTTP client on every reconciliation would be wasteful — it involves reading a `Secret`, constructing an HTTP transport, and potentially TLS handshakes.

The client cache stores ready-to-use `concourse.Client` instances and evicts them when the Instance or its referenced Secrets change.

## Cache key

```
"{namespace}/{name}@{resourceVersion}"
```

The `resourceVersion` of the `Instance` object is included in the key. When the `Instance` spec changes, the next reconcile produces a cache miss and a fresh client is built.

## Secret watches

The Instance controller watches Kubernetes `Secret` objects. If a Secret used by an Instance changes — `auth.password.passwordRef`, `auth.password.clientSecretRef`, `auth.token.tokenRef`, or `tls.caRef` — the Instance is enqueued automatically.

You do **not** need to annotate the Instance after rotating credentials. The reconcile rebuilds the cached client and records the Secret generation on `status.authSecretGeneration`.

## Thread safety

The cache is protected by a `sync.Mutex`. All reconcilers share the same cache instance injected at startup — concurrent access from multiple goroutines is safe.

```go
type Cache struct {
    mu    sync.Mutex
    store map[string]goconcourse.Client
}
```

Source: `internal/concourse/client_cache.go`

## Lifecycle

| Event | Cache action |
| ------- | ------------- |
| First reconcile of an Instance | Cache miss → build client → store |
| Subsequent reconcile, spec unchanged | Cache hit → reuse |
| Spec change (URL, auth method, TLS) | Cache miss (new resourceVersion) → build client → store |
| Referenced Secret rotated | Secret watch enqueues Instance → evict / rebuild |
| `Instance` deleted (finalizer runs) | Entry removed from cache |

## Eviction on deletion

`InstanceReconciler` adds the finalizer `concourse-ci.org/instance-finalizer` to every `Instance`. When the object is deleted, the finalizer handler removes the client from the cache before allowing the deletion to complete.

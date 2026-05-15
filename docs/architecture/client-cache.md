# Client Cache

## Purpose

Each reconciler needs an authenticated `concourse.Client` to call the Concourse API. Building a new HTTP client on every reconciliation would be wasteful — it involves reading a `Secret`, constructing an HTTP transport, and potentially TLS handshakes.

The client cache stores ready-to-use `concourse.Client` instances and evicts them only when credentials change.

## Cache key

```
"{namespace}/{name}@{resourceVersion}"
```

The `resourceVersion` of the `Instance` object is included in the key. When the `Instance` spec changes (e.g. a password rotation updates the referenced `Secret`), the operator updates the `Instance`, bumping `resourceVersion`. The next reconcile produces a cache miss, triggering a fresh client build with the new credentials.

## Thread safety

The cache is protected by a `sync.Mutex`. All reconcilers share the same cache instance injected at startup — concurrent access from multiple goroutines is safe.

```go
type ClientCache struct {
    mu      sync.Mutex
    clients map[string]concourse.Client
}
```

Source: `internal/concourse/client_cache.go`

## Lifecycle

| Event | Cache action |
|-------|-------------|
| First reconcile of an Instance | Cache miss → build client → store |
| Subsequent reconcile, spec unchanged | Cache hit → reuse |
| Spec change (credential rotation, URL update) | Cache miss (new resourceVersion) → build client → store |
| `Instance` deleted (finalizer runs) | Entry removed from cache |

## Eviction on deletion

`InstanceReconciler` adds the finalizer `concourse-ci.org/instance-finalizer` to every `Instance`. When the object is deleted, the finalizer handler removes the client from the cache before allowing the deletion to complete.

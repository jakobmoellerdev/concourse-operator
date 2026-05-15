/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package concourse

import (
	"context"
	"sync"

	goconcourse "github.com/concourse/concourse/go-concourse/concourse"
	"sigs.k8s.io/controller-runtime/pkg/client"

	concourcev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
)

// Cache stores one Concourse client per Instance, keyed by
// "{namespace}/{name}@{resourceVersion}". When the resourceVersion changes
// (i.e. spec updated), a new client is built and the old one replaced.
type Cache struct {
	mu    sync.Mutex
	store map[string]goconcourse.Client
}

// NewCache returns an empty Cache.
func NewCache() *Cache {
	return &Cache{store: make(map[string]goconcourse.Client)}
}

// cacheKey produces a stable key for a Instance.
func cacheKey(instance *concourcev1alpha1.Instance) string {
	return instance.Namespace + "/" + instance.Name + "@" + instance.ResourceVersion
}

// Get returns a cached client for the given instance, or nil if not cached.
func (c *Cache) Get(instance *concourcev1alpha1.Instance) (goconcourse.Client, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cl, ok := c.store[cacheKey(instance)]
	return cl, ok
}

// Set stores a client for the given instance, evicting stale entries for the
// same instance name (different resourceVersion).
func (c *Cache) Set(instance *concourcev1alpha1.Instance, cl goconcourse.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := instance.Namespace + "/" + instance.Name + "@"
	for k := range c.store {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.store, k)
		}
	}
	c.store[cacheKey(instance)] = cl
}

// Evict removes all cached clients for the given instance name.
func (c *Cache) Evict(instance *concourcev1alpha1.Instance) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := instance.Namespace + "/" + instance.Name + "@"
	for k := range c.store {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.store, k)
		}
	}
}

// GetOrBuild returns a cached client for the instance, building a new one if
// not present. It stores the built client in the cache before returning.
func (c *Cache) GetOrBuild(ctx context.Context, k8sClient client.Client, instance *concourcev1alpha1.Instance) (goconcourse.Client, error) {
	if cl, ok := c.Get(instance); ok {
		return cl, nil
	}
	httpClient, err := BuildHTTPClient(ctx, k8sClient, instance.Namespace, instance.Spec)
	if err != nil {
		return nil, err
	}
	cl := NewConcourseClient(instance.Spec.URL, httpClient)
	c.Set(instance, cl)
	return cl, nil
}

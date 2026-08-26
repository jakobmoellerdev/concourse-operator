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

package selfregistration

import (
	"context"
	"strconv"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// managedListLimit is the per-kind LIST cap the registrar applies to avoid
// pod-startup lag on clusters where the operator manages a very large
// population. The counts are still authoritative up to this cap; the
// managedItems list is separately capped at ManagedItemsMax.
const managedListLimit int64 = 1000

// ManagedItemsMax is the maximum number of Instance entries the registrar
// projects into the LM CR's status.managedItems. Chosen to fit a compact UI
// card without blowing the CR up — this is a diagnostic surface, not a
// materialised view of every Instance the operator manages.
const ManagedItemsMax = 20

// managedKinds enumerates the concourse-ci.org CRDs the registrar surfaces
// on the LM CR's status.managedResources. Order is stable so log/debug
// output is deterministic. Keys are the map keys emitted onto the CR;
// values are the corresponding List kinds.
var managedKinds = []struct {
	// Key is the status.managedResources map key (kubectl-lower-plural).
	Key string
	// ListKind is the singular Kind's List variant used to enumerate CRs.
	ListKind string
}{
	{Key: "instances", ListKind: "InstanceList"},
	{Key: "teams", ListKind: "TeamList"},
	{Key: "pipelines", ListKind: "PipelineList"},
	{Key: "jobs", ListKind: "JobList"},
	{Key: "resources", ListKind: "ResourceList"},
	{Key: "workers", ListKind: "WorkerList"},
	{Key: "builds", ListKind: "BuildList"},
}

// instanceListGVK is the GVK of concourse-ci.org/v1alpha1 InstanceList,
// used both for the count-per-kind sweep and for the managedItems detail
// projection. Kept as a package var to avoid repeatedly building it.
var instanceListGVK = schema.GroupVersionKind{
	Group:   ConcourseGroup,
	Version: "v1alpha1",
	Kind:    "InstanceList",
}

// buildListGVK is the GVK of concourse-ci.org/v1alpha1 BuildList, used to
// count Builds by their .status.concourseStatus phase.
var buildListGVK = schema.GroupVersionKind{
	Group:   ConcourseGroup,
	Version: "v1alpha1",
	Kind:    "BuildList",
}

// ManagedResourcesResolver returns a snapshot of the operator-managed
// resource counts and a bounded list of managed Instance items to surface
// on the LM CR at every heartbeat. Called once per heartbeat tick — errors
// on individual kinds MUST NOT abort the tick; they should degrade the
// snapshot instead. Returning (nil, nil, nil) is legitimate when nothing
// is knowable (e.g. no CRDs installed yet).
type ManagedResourcesResolver func(context.Context) (
	counts map[string]string,
	items []ManagedItemInfo,
	err error,
)

// ManagedItemInfo mirrors the maestro delivery API's ManagedItem — kept as
// a plain struct so this package does not depend on maestro Go types. The
// registrar projects []ManagedItemInfo into unstructured for the CR write.
type ManagedItemInfo struct {
	Kind      string
	Namespace string
	Name      string
	URL       string
	Ready     bool
}

// DefaultManagedResourcesResolver returns a resolver that lists every
// concourse-ci.org/v1alpha1 CRD via unstructured on the passed client and
// projects the result onto (counts, items). It is designed so that a
// missing / not-yet-installed CRD only produces a V(2) debug log and drops
// out of the snapshot — startup and every subsequent heartbeat still
// complete cleanly.
func DefaultManagedResourcesResolver(c client.Client, log logr.Logger) ManagedResourcesResolver {
	return func(ctx context.Context) (map[string]string, []ManagedItemInfo, error) {
		counts := make(map[string]string, len(managedKinds)+1)

		var activeBuilds int

		for _, k := range managedKinds {
			gvk := schema.GroupVersionKind{
				Group:   ConcourseGroup,
				Version: "v1alpha1",
				Kind:    k.ListKind,
			}
			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(gvk)
			if err := c.List(ctx, list, client.Limit(managedListLimit)); err != nil {
				// Missing CRD / RBAC denial / API not reachable — degrade
				// this kind and continue. This is deliberately V(2): on a
				// fresh cluster the CRDs are installed racy with the
				// operator and we do not want a noisy warning per tick.
				log.V(2).Info("could not list managed kind; skipping",
					"kind", k.ListKind, "err", err.Error())
				continue
			}
			counts[k.Key] = strconv.Itoa(len(list.Items))

			// When this is the Build list, also count phase == running as
			// activeBuilds. Read .status.concourseStatus which the operator
			// writes verbatim from Concourse's own build status.
			if gvk == buildListGVK {
				for i := range list.Items {
					phase, _, _ := unstructured.NestedString(
						list.Items[i].Object, "status", "concourseStatus")
					if phase == "running" || phase == "started" || phase == "pending" {
						activeBuilds++
					}
				}
			}
		}

		// activeBuilds is emitted only when the Build list was actually
		// reachable (so it doesn't appear as 0 on clusters where the CRD
		// isn't installed).
		if _, ok := counts["builds"]; ok {
			counts["activeBuilds"] = strconv.Itoa(activeBuilds)
		}

		items, err := listManagedInstances(ctx, c, log)
		if err != nil {
			// A hard failure listing Instances still returns the counts we
			// gathered so the tick isn't wasted.
			return counts, nil, err
		}

		if len(counts) == 0 && len(items) == 0 {
			return nil, nil, nil
		}
		return counts, items, nil
	}
}

// listManagedInstances enumerates concourse Instance CRs and projects the
// first ManagedItemsMax entries onto ManagedItemInfo. The URL prefers
// .status.externalURL (what Concourse itself reports) over .spec.url so a
// user-configured DNS override reflects reality.
func listManagedInstances(ctx context.Context, c client.Client, log logr.Logger) ([]ManagedItemInfo, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(instanceListGVK)
	if err := c.List(ctx, list, client.Limit(managedListLimit)); err != nil {
		log.V(2).Info("could not list Instances; skipping managedItems",
			"err", err.Error())
		return nil, nil
	}

	n := len(list.Items)
	if n > ManagedItemsMax {
		n = ManagedItemsMax
	}
	out := make([]ManagedItemInfo, 0, n)
	for i := 0; i < n; i++ {
		it := &list.Items[i]
		url, _, _ := unstructured.NestedString(it.Object, "status", "externalURL")
		if url == "" {
			url, _, _ = unstructured.NestedString(it.Object, "spec", "url")
		}
		out = append(out, ManagedItemInfo{
			Kind:      "Instance",
			Namespace: it.GetNamespace(),
			Name:      it.GetName(),
			URL:       url,
			Ready:     instanceReady(it),
		})
	}
	return out, nil
}

// instanceReady reports whether the passed Instance CR has a Ready
// condition with status == True. Coarse; treats missing / False /
// Unknown all as false.
func instanceReady(obj *unstructured.Unstructured) bool {
	conds, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}
	for _, raw := range conds {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		t, _, _ := unstructured.NestedString(m, "type")
		s, _, _ := unstructured.NestedString(m, "status")
		if t == "Ready" {
			return s == "True"
		}
	}
	return false
}

// managedItemsToUnstructured marshals []ManagedItemInfo into the []any of
// maps the unstructured LM CR expects on status.managedItems. Empty in →
// nil out so the JSON omitempty contract on the maestro API is respected
// and empty ticks don't clobber a previously-published list.
func managedItemsToUnstructured(items []ManagedItemInfo) []any {
	if len(items) == 0 {
		return nil
	}
	out := make([]any, 0, len(items))
	for _, it := range items {
		m := map[string]any{
			"kind": it.Kind,
			"name": it.Name,
		}
		if it.Namespace != "" {
			m["namespace"] = it.Namespace
		}
		if it.URL != "" {
			m["url"] = it.URL
		}
		if it.Ready {
			m["ready"] = true
		}
		out = append(out, m)
	}
	return out
}

// managedResourcesToUnstructured widens a map[string]string into the
// map[string]any the unstructured LM CR expects. Nil / empty in → nil out
// so the CRD's `omitempty` contract stays honest.
func managedResourcesToUnstructured(counts map[string]string) map[string]any {
	if len(counts) == 0 {
		return nil
	}
	out := make(map[string]any, len(counts))
	for k, v := range counts {
		out[k] = v
	}
	return out
}

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
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// DefaultHeartbeatInterval is the default cadence for status.lastHeartbeatTime
// refreshes. Chosen small enough that a single missed tick is tolerable
// (e.g. during a leader flap) but large enough not to hammer the API.
const DefaultHeartbeatInterval = 30 * time.Second

// FieldManager is the field-manager name used for merge patches authored by
// the self-registration runnable. Shared across replicas so their idempotent
// writes coalesce cleanly.
const FieldManager = "concourse-selfregistration"

// LifecycleManagerGVK is the maestro delivery LifecycleManager GVK — cluster
// scoped, v1alpha1. We talk to it through unstructured so this package does
// not require a Go dep on the maestro API module.
var LifecycleManagerGVK = schema.GroupVersionKind{
	Group:   "delivery.maestro.sap.com",
	Version: "v1alpha1",
	Kind:    "LifecycleManager",
}

// ConcourseGroup is the API group of the CRDs this operator serves. Kinds
// registered against this group are auto-enumerated into the LM CR's
// spec.supportedResourceTypes when SupportedResourceTypes is left empty.
const ConcourseGroup = "concourse-ci.org"

// Options configures a Registrar. See New for defaulting rules.
type Options struct {
	// Client is a controller-runtime client for the LOCAL cluster. Required.
	Client client.Client
	// Identity is the pod identity (namespace + SA) derived from the SA
	// mount by FromServiceAccountMount. Required unless Name is set.
	Identity Identity
	// Name overrides the derived "<namespace>-<serviceaccount>" name.
	// Empty means "derive from Identity".
	Name string
	// Type is the lifecycle manager type identifier (e.g. "concourse"). Required.
	Type string
	// SupportedWorkloadTypes lists workload type identifiers this LM claims
	// to reconcile. May be empty for v0.
	SupportedWorkloadTypes []string
	// SupportedResourceTypes lists resource type identifiers this LM
	// reconciles, in kubectl api-resources notation (<plural>.<group>).
	// When nil, the registrar auto-populates the list from the passed
	// runtime.Scheme by enumerating every kind registered under
	// ConcourseGroup.
	SupportedResourceTypes []string
	// Scheme is only consulted when SupportedResourceTypes is nil, to
	// auto-populate the list from the operator's registered API kinds.
	// May be nil when SupportedResourceTypes is explicit.
	Scheme *runtime.Scheme
	// HeartbeatInterval is the cadence of status.lastHeartbeatTime refreshes.
	// Defaults to DefaultHeartbeatInterval when zero.
	HeartbeatInterval time.Duration
	// InstanceInfoResolver, if non-nil, is called once at Start to compute
	// the InstanceInfo. Defaults to reading the Downward API env + querying
	// the pod. Overridable for tests.
	InstanceInfoResolver InstanceInfoResolver
	// Log is the logger used for lifecycle events. Defaults to a discard logger.
	Log logr.Logger
	// now overrides time.Now for tests. Nil means real time.
	now func() time.Time
}

// Registrar is a controller-runtime Runnable that self-registers a maestro
// LifecycleManager CR at Start, refreshes its status.lastHeartbeatTime on a
// ticker, and best-effort clears status.ready on shutdown.
type Registrar struct {
	client                 client.Client
	name                   string
	typ                    string
	supportedWorkloadTypes []string
	supportedResourceTypes []string
	interval               time.Duration
	resolver               InstanceInfoResolver
	instanceOnce           *InstanceInfo
	log                    logr.Logger
	now                    func() time.Time
}

// Compile-time proof: Registrar satisfies manager.Runnable and
// manager.LeaderElectionRunnable.
var (
	_ manager.Runnable               = (*Registrar)(nil)
	_ manager.LeaderElectionRunnable = (*Registrar)(nil)
)

// DerivedName returns the deterministic "<namespace>-<serviceaccount>"
// registration name. Exported so cmd/main.go can log it early.
//
// The LifecycleManager CR is cluster-scoped and namespaces may collide across
// clusters (two clusters could each host `concourse-system/controller-manager`),
// so prefixing with the namespace is stable and unambiguous within a single
// cluster. Callers may override via Options.Name.
func DerivedName(id Identity) string {
	return id.Namespace + "-" + id.ServiceAccount
}

// irregularPlurals overrides the naive `strings.ToLower(kind)+"s"` rule for
// kinds whose plural form is irregular. None of the concourse-operator kinds
// currently require an override — the map is here so future contributors
// have a clear extension point rather than having to touch call sites.
var irregularPlurals = map[string]string{}

// metav1CommonKinds are the standard metav1 helper types every scheme
// picks up when a v1alpha1 API's AddToScheme calls
// metav1.AddToGroupVersion. They are not real API resources and must be
// filtered out of the LM CR's supportedResourceTypes.
var metav1CommonKinds = map[string]struct{}{
	"CreateOptions": {},
	"DeleteOptions": {},
	"GetOptions":    {},
	"ListOptions":   {},
	"PatchOptions":  {},
	"UpdateOptions": {},
	"WatchEvent":    {},
}

// SupportedResourceTypesFromScheme returns the set of kubectl-style resource
// identifiers ("<plural>.<group>") for every Kind registered under group in
// scheme's known types map. The list is deterministic (alphabetical) and
// deduplicated across API versions. Standard metav1 helper types (like
// CreateOptions) get filtered out.
func SupportedResourceTypesFromScheme(scheme *runtime.Scheme, group string) []string {
	if scheme == nil {
		return nil
	}
	seen := map[string]struct{}{}
	for gvk := range scheme.AllKnownTypes() {
		if gvk.Group != group {
			continue
		}
		// Filter out list kinds — they don't correspond to API resources.
		if strings.HasSuffix(gvk.Kind, "List") {
			continue
		}
		// Filter out metav1's ambient helper types.
		if _, ok := metav1CommonKinds[gvk.Kind]; ok {
			continue
		}
		plural, ok := irregularPlurals[gvk.Kind]
		if !ok {
			plural = strings.ToLower(gvk.Kind) + "s"
		}
		seen[plural+"."+group] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// New validates opts and returns a Registrar ready for mgr.Add.
func New(opts Options) (*Registrar, error) {
	if opts.Client == nil {
		return nil, errors.New("Options.Client is required")
	}
	if opts.Type == "" {
		return nil, errors.New("Options.Type is required")
	}
	name := opts.Name
	if name == "" {
		if opts.Identity.Namespace == "" || opts.Identity.ServiceAccount == "" {
			return nil, errors.New("Options.Name is empty and Options.Identity is incomplete: cannot derive a registration name")
		}
		name = DerivedName(opts.Identity)
	}
	interval := opts.HeartbeatInterval
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}
	resourceTypes := opts.SupportedResourceTypes
	if resourceTypes == nil && opts.Scheme != nil {
		resourceTypes = SupportedResourceTypesFromScheme(opts.Scheme, ConcourseGroup)
	}
	now := opts.now
	if now == nil {
		now = time.Now
	}
	log := opts.Log
	// A default zero logr.Logger silently discards, matching controller-runtime.

	resolver := opts.InstanceInfoResolver
	if resolver == nil {
		resolver = DefaultInstanceInfoResolver(opts.Client, opts.Identity, now(), log)
	}

	return &Registrar{
		client:                 opts.Client,
		name:                   name,
		typ:                    opts.Type,
		supportedWorkloadTypes: append([]string(nil), opts.SupportedWorkloadTypes...),
		supportedResourceTypes: append([]string(nil), resourceTypes...),
		interval:               interval,
		resolver:               resolver,
		log:                    log,
		now:                    now,
	}, nil
}

// Name returns the LifecycleManager CR name this Registrar owns.
func (r *Registrar) Name() string { return r.name }

// SupportedResourceTypes returns the resource-type list this registrar will
// publish onto the LM CR. Exported for tests and startup logging.
func (r *Registrar) SupportedResourceTypes() []string {
	return append([]string(nil), r.supportedResourceTypes...)
}

// NeedLeaderElection gates the runnable on leader election.
//
// Returning true is a deliberate tradeoff: only the elected leader
// heartbeats, so replicas don't fight for the last write on a shared
// cluster-scoped CR. Every write to the LM CR is idempotent (same spec,
// monotonic timestamp), so letting all replicas write would be correct but
// wasteful. A leader flap will skip at most one heartbeat, and
// DefaultHeartbeatInterval (30s) is short enough that this is well within
// any reasonable freshness SLA on the consumer (maestro tower / delivery
// reconciler) side.
func (r *Registrar) NeedLeaderElection() bool { return true }

// Start performs the initial ensure + status update, then ticks
// status.lastHeartbeatTime until ctx is cancelled. On cancellation it makes
// a best-effort attempt to flip status.ready=false so consumers see a fresh
// "not ready" signal rather than a stale ready+expired heartbeat.
func (r *Registrar) Start(ctx context.Context) error {
	// Resolve InstanceInfo ONCE, before the first write. The pod identity
	// does not change over the lifetime of the process, so subsequent ticks
	// simply re-attach the cached value.
	if r.resolver != nil {
		info, err := r.resolver(ctx)
		if err != nil {
			r.log.V(1).Info("resolving InstanceInfo failed; status.instance will be omitted",
				"err", err.Error())
		}
		r.instanceOnce = info
	}

	if err := r.ensureSpec(ctx); err != nil {
		r.log.Error(err, "self-registration spec ensure failed; will retry on next tick",
			"name", r.name)
	} else if err := r.updateStatus(ctx, true); err != nil {
		r.log.Error(err, "self-registration status update failed; will retry on next tick",
			"name", r.name)
	} else {
		r.log.Info("self-registered LifecycleManager",
			"name", r.name, "type", r.typ,
			"supportedResourceTypes", r.supportedResourceTypes)
	}

	tick := time.NewTicker(r.interval)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			// Best-effort: unbind from ctx so a cancelled ctx can still reach the API.
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := r.updateStatus(shutdownCtx, false); err != nil {
				r.log.V(1).Info("best-effort shutdown status update failed",
					"name", r.name, "err", err.Error())
			}
			cancel()
			return nil
		case <-tick.C:
			// Re-ensure spec occasionally too, in case the CR was recreated
			// out-of-band with a stale spec. Cheap: it's a get + one patch.
			if err := r.ensureSpec(ctx); err != nil {
				r.log.Error(err, "heartbeat spec ensure failed", "name", r.name)
				continue
			}
			if err := r.updateStatus(ctx, true); err != nil {
				r.log.Error(err, "heartbeat status update failed", "name", r.name)
			}
		}
	}
}

// newLifecycleManager returns an empty *Unstructured pre-tagged with the
// maestro LifecycleManager GVK. Every read/patch path funnels through this
// helper so the GVK is set exactly once in this file.
func newLifecycleManager() *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(LifecycleManagerGVK)
	return u
}

// ensureSpec creates the LifecycleManager CR if missing, otherwise updates
// only its spec fields — deliberately leaving out-of-band labels/annotations
// (and any status subresource content) intact.
func (r *Registrar) ensureSpec(ctx context.Context) error {
	mlm := newLifecycleManager()
	mlm.SetName(r.name)
	_, err := controllerutil.CreateOrUpdate(ctx, r.client, mlm, func() error {
		spec := map[string]any{
			"type": r.typ,
		}
		// Emit the list fields only when non-empty so the CR stays tidy
		// (empty slices marshal to `[]` which noisily overwrites nil).
		if len(r.supportedWorkloadTypes) > 0 {
			spec["supportedWorkloadTypes"] = toAnySlice(r.supportedWorkloadTypes)
		}
		if len(r.supportedResourceTypes) > 0 {
			spec["supportedResourceTypes"] = toAnySlice(r.supportedResourceTypes)
		}
		mlm.Object["spec"] = spec
		return nil
	})
	if err != nil {
		return fmt.Errorf("ensuring LifecycleManager %q: %w", r.name, err)
	}
	return nil
}

// updateStatus patches only status.ready, status.lastHeartbeatTime, and
// status.instance via the status subresource. Uses a MergeFrom patch so
// concurrent replicas coalesce.
func (r *Registrar) updateStatus(ctx context.Context, ready bool) error {
	current := newLifecycleManager()
	if err := r.client.Get(ctx, client.ObjectKey{Name: r.name}, current); err != nil {
		if apierrors.IsNotFound(err) {
			// Spec ensure hasn't landed yet; skip this tick.
			return nil
		}
		return fmt.Errorf("getting LifecycleManager %q: %w", r.name, err)
	}
	base := current.DeepCopy()

	status, _, err := unstructured.NestedMap(current.Object, "status")
	if err != nil || status == nil {
		status = map[string]any{}
	}
	status["ready"] = ready
	nowStr := metav1.NewTime(r.now()).UTC().Format(time.RFC3339)
	status["lastHeartbeatTime"] = nowStr
	if r.instanceOnce != nil {
		status["instance"] = instanceInfoToMap(r.instanceOnce)
	}
	current.Object["status"] = status

	if err := r.client.Status().Patch(ctx, current, client.MergeFrom(base),
		client.FieldOwner(FieldManager)); err != nil {
		return fmt.Errorf("patching LifecycleManager %q status: %w", r.name, err)
	}
	return nil
}

// instanceInfoToMap projects an InstanceInfo into the unstructured shape
// consumed by the maestro tower. Empty fields are omitted so the CR stays
// aligned with the maestro API's `,omitempty` semantics.
func instanceInfoToMap(info *InstanceInfo) map[string]any {
	m := map[string]any{}
	if info.PodName != "" {
		m["podName"] = info.PodName
	}
	if info.PodNamespace != "" {
		m["podNamespace"] = info.PodNamespace
	}
	if info.PodUID != "" {
		m["podUID"] = info.PodUID
	}
	if info.NodeName != "" {
		m["nodeName"] = info.NodeName
	}
	if info.ServiceAccount != "" {
		m["serviceAccount"] = info.ServiceAccount
	}
	if info.Image != "" {
		m["image"] = info.Image
	}
	if info.Version != "" {
		m["version"] = info.Version
	}
	if !info.StartedAt.IsZero() {
		m["startedAt"] = metav1.NewTime(info.StartedAt).UTC().Format(time.RFC3339)
	}
	return m
}

// toAnySlice widens a []string to a []any so it round-trips through
// unstructured.Unstructured without triggering DeepCopyJSON panics on the
// concrete slice type.
func toAnySlice(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

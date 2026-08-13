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
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	concoursev1alpha1 "github.com/jakobmoellerdev/concourse-operator/api/v1alpha1"
)

// registrarScheme returns a runtime.Scheme with the LifecycleManager GVK
// registered as an Unstructured, plus the concourse operator API group so
// SupportedResourceTypesFromScheme has real kinds to enumerate.
func registrarScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, concoursev1alpha1.AddToScheme(s))
	// The fake client needs to know how to list/get the maestro LM CR.
	// Register the Unstructured shell against its GVK.
	s.AddKnownTypeWithName(LifecycleManagerGVK, &unstructured.Unstructured{})
	s.AddKnownTypeWithName(schema.GroupVersionKind{
		Group:   LifecycleManagerGVK.Group,
		Version: LifecycleManagerGVK.Version,
		Kind:    LifecycleManagerGVK.Kind + "List",
	}, &unstructured.UnstructuredList{})
	return s
}

// newLMShell returns a bare Unstructured with the LM GVK and given name so
// the fake client's status subresource support has a valid seed object.
func newLMShell(name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(LifecycleManagerGVK)
	u.SetName(name)
	return u
}

// newFakeClient returns a fake client that understands the LifecycleManager
// status subresource (required so Status().Patch doesn't panic).
func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(registrarScheme(t)).
		WithStatusSubresource(newLMShell("")).
		WithObjects(objs...).
		Build()
}

// testIdentity is the canonical identity used by all tests here.
var testIdentity = Identity{
	Namespace:      "concourse-system",
	ServiceAccount: "controller-manager",
}

// TestDerivedName pins the "<namespace>-<serviceaccount>" contract.
func TestDerivedName(t *testing.T) {
	assert.Equal(t, "concourse-system-controller-manager", DerivedName(testIdentity))
}

// TestSupportedResourceTypesFromScheme enumerates every kind under the
// concourse-ci.org group registered in the operator's own scheme. This is
// how the LM CR gets an accurate `supportedResourceTypes` list without a
// hand-maintained slice.
func TestSupportedResourceTypesFromScheme(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, concoursev1alpha1.AddToScheme(s))

	got := SupportedResourceTypesFromScheme(s, ConcourseGroup)
	// All 7 CRDs the concourse-operator ships, formatted <plural>.<group>.
	want := []string{
		"builds.concourse-ci.org",
		"instances.concourse-ci.org",
		"jobs.concourse-ci.org",
		"pipelines.concourse-ci.org",
		"resources.concourse-ci.org",
		"teams.concourse-ci.org",
		"workers.concourse-ci.org",
	}
	assert.Equal(t, want, got)

	// Unknown group → empty.
	assert.Empty(t, SupportedResourceTypesFromScheme(s, "nope.example.com"))

	// Nil scheme is a no-op, not a crash.
	assert.Nil(t, SupportedResourceTypesFromScheme(nil, ConcourseGroup))
}

// TestNew_Validation covers the invariants New enforces on construction.
func TestNew_Validation(t *testing.T) {
	c := newFakeClient(t)

	// nil client
	_, err := New(Options{Type: "concourse", Identity: testIdentity})
	require.Error(t, err)

	// empty type
	_, err = New(Options{Client: c, Identity: testIdentity})
	require.Error(t, err)

	// missing name AND incomplete identity
	_, err = New(Options{Client: c, Type: "concourse"})
	require.Error(t, err)

	// override name compensates for empty identity
	r, err := New(Options{Client: c, Type: "concourse", Name: "override"})
	require.NoError(t, err)
	assert.Equal(t, "override", r.Name())

	// interval defaulting
	r, err = New(Options{Client: c, Type: "concourse", Identity: testIdentity})
	require.NoError(t, err)
	assert.Equal(t, DefaultHeartbeatInterval, r.interval)
	assert.Equal(t, "concourse-system-controller-manager", r.Name())
}

// TestNew_SchemeAutoPopulatesResourceTypes proves the New defaulting path:
// when SupportedResourceTypes is nil, the constructor derives the list from
// the passed Scheme.
func TestNew_SchemeAutoPopulatesResourceTypes(t *testing.T) {
	c := newFakeClient(t)
	s := runtime.NewScheme()
	require.NoError(t, concoursev1alpha1.AddToScheme(s))

	r, err := New(Options{
		Client:   c,
		Type:     "concourse",
		Identity: testIdentity,
		Scheme:   s,
	})
	require.NoError(t, err)
	got := r.SupportedResourceTypes()
	assert.Contains(t, got, "pipelines.concourse-ci.org")
	assert.Contains(t, got, "teams.concourse-ci.org")
	assert.Len(t, got, 7)
}

// TestRegistrar_NeedLeaderElection pins the leader-election contract.
func TestRegistrar_NeedLeaderElection(t *testing.T) {
	c := newFakeClient(t)
	r, err := New(Options{Client: c, Type: "concourse", Identity: testIdentity})
	require.NoError(t, err)
	assert.True(t, r.NeedLeaderElection())
}

// TestRegistrar_CreatesFreshCR proves Start registers a new LM CR when none
// exists and marks it Ready with a fresh heartbeat + instance identity.
func TestRegistrar_CreatesFreshCR(t *testing.T) {
	c := newFakeClient(t)

	fakeNow := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	instance := &InstanceInfo{
		PodName:        "test-pod",
		PodNamespace:   "concourse-system",
		PodUID:         "uid-123",
		NodeName:       "node-a",
		ServiceAccount: "controller-manager",
		Image:          "concourse-op:withinstance",
		StartedAt:      fakeNow,
		Version:        "v0.0.0-test",
	}
	r, err := New(Options{
		Client:                 c,
		Type:                   "concourse",
		Identity:               testIdentity,
		SupportedWorkloadTypes: []string{},
		SupportedResourceTypes: []string{"pipelines.concourse-ci.org"},
		HeartbeatInterval:      100 * time.Millisecond,
		InstanceInfoResolver: func(context.Context) (*InstanceInfo, error) {
			return instance, nil
		},
		Log: logr.Discard(),
	})
	require.NoError(t, err)
	r.now = func() time.Time { return fakeNow }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Start(ctx) }()

	require.Eventually(t, func() bool {
		got := newLMShell(r.Name())
		if err := c.Get(context.Background(), client.ObjectKey{Name: r.Name()}, got); err != nil {
			return false
		}
		ready, _, _ := unstructured.NestedBool(got.Object, "status", "ready")
		return ready
	}, 2*time.Second, 10*time.Millisecond)

	got := newLMShell(r.Name())
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: r.Name()}, got))
	typ, _, _ := unstructured.NestedString(got.Object, "spec", "type")
	assert.Equal(t, "concourse", typ)
	resTypes, _, _ := unstructured.NestedStringSlice(got.Object, "spec", "supportedResourceTypes")
	assert.Equal(t, []string{"pipelines.concourse-ci.org"}, resTypes)
	// supportedWorkloadTypes is empty → must NOT be stamped onto the object.
	_, ok, _ := unstructured.NestedSlice(got.Object, "spec", "supportedWorkloadTypes")
	assert.False(t, ok, "empty supportedWorkloadTypes must not be persisted")

	ready, _, _ := unstructured.NestedBool(got.Object, "status", "ready")
	assert.True(t, ready)
	hb, _, _ := unstructured.NestedString(got.Object, "status", "lastHeartbeatTime")
	assert.NotEmpty(t, hb)

	// Instance identity round-trip.
	podName, _, _ := unstructured.NestedString(got.Object, "status", "instance", "podName")
	assert.Equal(t, "test-pod", podName)
	image, _, _ := unstructured.NestedString(got.Object, "status", "instance", "image")
	assert.Equal(t, "concourse-op:withinstance", image)
	sa, _, _ := unstructured.NestedString(got.Object, "status", "instance", "serviceAccount")
	assert.Equal(t, "controller-manager", sa)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

// TestRegistrar_NilResolverKeepsInstanceNil proves the (nil, nil) resolver
// contract: when nothing is knowable, status.instance stays absent rather
// than getting stamped with an empty map.
func TestRegistrar_NilResolverKeepsInstanceNil(t *testing.T) {
	c := newFakeClient(t)
	r, err := New(Options{
		Client:                 c,
		Type:                   "concourse",
		Identity:               testIdentity,
		SupportedResourceTypes: []string{},
		HeartbeatInterval:      time.Hour,
		InstanceInfoResolver: func(context.Context) (*InstanceInfo, error) {
			return nil, nil
		},
		Log: logr.Discard(),
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, r.ensureSpec(ctx))
	require.NoError(t, r.updateStatus(ctx, true))

	got := newLMShell(r.Name())
	require.NoError(t, c.Get(ctx, client.ObjectKey{Name: r.Name()}, got))
	ready, _, _ := unstructured.NestedBool(got.Object, "status", "ready")
	assert.True(t, ready)
	_, ok, _ := unstructured.NestedMap(got.Object, "status", "instance")
	assert.False(t, ok, "empty resolver must not stamp status.instance")
}

// TestRegistrar_InstancePreservedAcrossHeartbeats proves that when the
// resolver is called ONCE at Start, subsequent heartbeat ticks re-attach
// the cached InstanceInfo rather than clobbering it to nil.
func TestRegistrar_InstancePreservedAcrossHeartbeats(t *testing.T) {
	c := newFakeClient(t)

	instance := &InstanceInfo{PodName: "the-pod", PodNamespace: "concourse-system"}
	var calls int64
	r, err := New(Options{
		Client:            c,
		Type:              "concourse",
		Identity:          testIdentity,
		HeartbeatInterval: 20 * time.Millisecond,
		InstanceInfoResolver: func(context.Context) (*InstanceInfo, error) {
			atomic.AddInt64(&calls, 1)
			return instance, nil
		},
		Log: logr.Discard(),
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Start(ctx) }()

	require.Eventually(t, func() bool {
		got := newLMShell(r.Name())
		if err := c.Get(ctx, client.ObjectKey{Name: r.Name()}, got); err != nil {
			return false
		}
		hb, _, _ := unstructured.NestedString(got.Object, "status", "lastHeartbeatTime")
		ready, _, _ := unstructured.NestedBool(got.Object, "status", "ready")
		return hb != "" && ready
	}, 2*time.Second, 5*time.Millisecond)

	time.Sleep(80 * time.Millisecond) // let a few extra ticks land

	got := newLMShell(r.Name())
	require.NoError(t, c.Get(ctx, client.ObjectKey{Name: r.Name()}, got))
	podName, _, _ := unstructured.NestedString(got.Object, "status", "instance", "podName")
	assert.Equal(t, "the-pod", podName, "InstanceInfo must survive heartbeat ticks")
	assert.Equal(t, int64(1), atomic.LoadInt64(&calls),
		"resolver should be invoked exactly once, at Start")

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

// TestRegistrar_PreservesOutOfBandMetadata proves ensureSpec updates only
// spec fields, leaving labels/annotations added by other actors intact.
func TestRegistrar_PreservesOutOfBandMetadata(t *testing.T) {
	preexisting := newLMShell(DerivedName(testIdentity))
	preexisting.SetLabels(map[string]string{"user.example.com/owner": "sre"})
	preexisting.SetAnnotations(map[string]string{"user.example.com/note": "hand-tuned"})
	preexisting.Object["spec"] = map[string]any{"type": "stale-value"}

	c := newFakeClient(t, preexisting)

	r, err := New(Options{
		Client:            c,
		Type:              "concourse",
		Identity:          testIdentity,
		HeartbeatInterval: time.Hour, // won't tick within the test
		Log:               logr.Discard(),
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, r.ensureSpec(ctx))
	require.NoError(t, r.updateStatus(ctx, true))

	got := newLMShell(r.Name())
	require.NoError(t, c.Get(ctx, client.ObjectKey{Name: r.Name()}, got))
	typ, _, _ := unstructured.NestedString(got.Object, "spec", "type")
	assert.Equal(t, "concourse", typ, "spec.type should be overwritten")
	labels := got.GetLabels()
	assert.Equal(t, "sre", labels["user.example.com/owner"], "labels must survive")
	ann := got.GetAnnotations()
	assert.Equal(t, "hand-tuned", ann["user.example.com/note"], "annotations must survive")
	ready, _, _ := unstructured.NestedBool(got.Object, "status", "ready")
	assert.True(t, ready)
}

// TestRegistrar_ShutdownBestEffortClearsReady proves cancellation attempts
// to flip status.ready=false.
func TestRegistrar_ShutdownBestEffortClearsReady(t *testing.T) {
	c := newFakeClient(t)
	r, err := New(Options{
		Client:            c,
		Type:              "concourse",
		Identity:          testIdentity,
		HeartbeatInterval: time.Hour,
		Log:               logr.Discard(),
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Start(ctx) }()

	require.Eventually(t, func() bool {
		got := newLMShell(r.Name())
		if err := c.Get(context.Background(), client.ObjectKey{Name: r.Name()}, got); err != nil {
			return false
		}
		ready, _, _ := unstructured.NestedBool(got.Object, "status", "ready")
		return ready
	}, 2*time.Second, 5*time.Millisecond)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}

	got := newLMShell(r.Name())
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: r.Name()}, got))
	ready, _, _ := unstructured.NestedBool(got.Object, "status", "ready")
	assert.False(t, ready, "shutdown should flip Ready=false best-effort")
}

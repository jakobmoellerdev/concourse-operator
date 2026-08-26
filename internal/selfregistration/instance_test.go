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
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// instanceScheme registers corev1 so the fake client can serve pod lookups.
func instanceScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	return s
}

func setPodEnv(t *testing.T, ns, name, uid, node, container string) {
	t.Helper()
	t.Setenv(EnvPodNamespace, ns)
	t.Setenv(EnvPodName, name)
	t.Setenv(EnvPodUID, uid)
	t.Setenv(EnvNodeName, node)
	t.Setenv(EnvContainerName, container)
}

// TestDefaultInstanceInfoResolver_EnvPlusPod exercises the happy path: env
// vars populated + pod object visible through the client.
func TestDefaultInstanceInfoResolver_EnvPlusPod(t *testing.T) {
	setPodEnv(t, "concourse-system", "test-pod", "uid-123", "node-a", "manager")

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "concourse-system"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "sidecar", Image: "ghcr.io/side:1"},
				{Name: "manager", Image: "ghcr.io/concourse-op:withinstance"},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(instanceScheme(t)).WithObjects(pod).Build()

	started := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	resolver := DefaultInstanceInfoResolver(c,
		Identity{Namespace: "concourse-system", ServiceAccount: "controller-manager"},
		started, logr.Discard())

	got, err := resolver(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "test-pod", got.PodName)
	assert.Equal(t, "concourse-system", got.PodNamespace)
	assert.Equal(t, "uid-123", got.PodUID)
	assert.Equal(t, "node-a", got.NodeName)
	assert.Equal(t, "controller-manager", got.ServiceAccount)
	assert.Equal(t, "ghcr.io/concourse-op:withinstance", got.Image)
	assert.Equal(t, started.UTC(), got.StartedAt.UTC())
}

// TestDefaultInstanceInfoResolver_MissingContainerName falls back to the
// first container when the CONTAINER_NAME env is not set.
func TestDefaultInstanceInfoResolver_MissingContainerName(t *testing.T) {
	setPodEnv(t, "ns", "pod", "uid", "node", "" /* no container name */)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "ns"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				// No container named "manager" — resolver falls back to the
				// first entry rather than emitting an empty Image.
				{Name: "primary", Image: "primary:latest"},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(instanceScheme(t)).WithObjects(pod).Build()

	got, err := DefaultInstanceInfoResolver(c, Identity{ServiceAccount: "sa"},
		time.Now(), logr.Discard())(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "primary:latest", got.Image)
}

// TestDefaultInstanceInfoResolver_PodGetFails covers RBAC-not-granted /
// pod-not-observable: env vars carry, image stays empty, no error.
func TestDefaultInstanceInfoResolver_PodGetFails(t *testing.T) {
	setPodEnv(t, "concourse-system", "test-pod", "uid-123", "node-a", "manager")

	// No pod loaded into the fake client — Get returns NotFound.
	c := fake.NewClientBuilder().WithScheme(instanceScheme(t)).Build()
	got, err := DefaultInstanceInfoResolver(c,
		Identity{ServiceAccount: "sa"}, time.Now(), logr.Discard())(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "test-pod", got.PodName)
	assert.Empty(t, got.Image, "image must be empty when pod-get fails")
}

// TestContainerImage exercises the container-lookup helper directly so the
// selection rules are pinned independently of resolver plumbing.
func TestContainerImage(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "manager", Image: "m:1"},
			{Name: "sidecar", Image: "s:1"},
		},
	}}
	assert.Equal(t, "m:1", containerImage(pod, ""), "empty name falls back to defaultContainer")
	assert.Equal(t, "m:1", containerImage(pod, "manager"))
	assert.Equal(t, "s:1", containerImage(pod, "sidecar"))
	assert.Equal(t, "m:1", containerImage(pod, "missing"),
		"unknown container name falls back to first container")
	assert.Empty(t, containerImage(nil, "manager"))
	assert.Empty(t, containerImage(&corev1.Pod{}, "manager"))
}

// TestIsEmptyInstance keeps the empty-check honest — every field must be
// examined so an accidentally-set signal isn't dropped.
func TestIsEmptyInstance(t *testing.T) {
	assert.True(t, isEmptyInstance(nil))
	assert.True(t, isEmptyInstance(&InstanceInfo{}))
	assert.False(t, isEmptyInstance(&InstanceInfo{PodName: "x"}))
	assert.False(t, isEmptyInstance(&InstanceInfo{Version: "v1"}))
	assert.False(t, isEmptyInstance(&InstanceInfo{StartedAt: time.Now()}))
}

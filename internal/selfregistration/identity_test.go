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
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeJWT builds a JWT-shaped string with the given claim map. No signature
// is applied — we control our own token and never verify it. The signature
// segment is a dummy so the string has the required three segments.
func makeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	hb, err := json.Marshal(header)
	require.NoError(t, err)
	pb, err := json.Marshal(claims)
	require.NoError(t, err)
	enc := base64.RawURLEncoding.EncodeToString
	return enc(hb) + "." + enc(pb) + ".dummy-signature"
}

// TestFromServiceAccountMountAt_SubClaim proves the projected-token path:
// the sub claim `system:serviceaccount:<ns>:<sa>` yields the trailing SA name.
func TestFromServiceAccountMountAt_SubClaim(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "namespace"),
		[]byte("concourse-system\n"), 0o600))
	tok := makeJWT(t, map[string]any{
		"sub": "system:serviceaccount:concourse-system:concourse-operator-controller-manager",
		"iss": "https://kubernetes.default.svc",
	})
	require.NoError(t, os.WriteFile(filepath.Join(root, "token"),
		[]byte(tok), 0o600))

	got, err := FromServiceAccountMountAt(root)
	require.NoError(t, err)
	assert.Equal(t, "concourse-system", got.Namespace)
	assert.Equal(t, "concourse-operator-controller-manager", got.ServiceAccount)
}

// TestFromServiceAccountMountAt_LegacyClaim proves the fallback path used
// by legacy long-lived Secret-based SA tokens whose sub is not the k8s
// principal string.
func TestFromServiceAccountMountAt_LegacyClaim(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "namespace"),
		[]byte("kube-system"), 0o600))
	tok := makeJWT(t, map[string]any{
		"iss": "kubernetes/serviceaccount",
		"kubernetes.io/serviceaccount/service-account.name": "some-legacy-sa",
		"kubernetes.io/serviceaccount/namespace":            "kube-system",
	})
	require.NoError(t, os.WriteFile(filepath.Join(root, "token"),
		[]byte(tok), 0o600))

	got, err := FromServiceAccountMountAt(root)
	require.NoError(t, err)
	assert.Equal(t, "kube-system", got.Namespace)
	assert.Equal(t, "some-legacy-sa", got.ServiceAccount)
}

// TestFromServiceAccountMountAt_NotInCluster proves that when both files
// are absent (dev laptop) we get the well-known sentinel error so the caller
// can downgrade to a warning rather than crash-loop.
func TestFromServiceAccountMountAt_NotInCluster(t *testing.T) {
	root := t.TempDir() // empty dir — no namespace, no token
	_, err := FromServiceAccountMountAt(root)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotInCluster),
		"expected ErrNotInCluster, got %v", err)
}

// TestFromServiceAccountMountAt_MalformedToken proves that a garbage token
// yields a descriptive error, not a panic or a silent empty identity.
func TestFromServiceAccountMountAt_MalformedToken(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "namespace"),
		[]byte("ns"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "token"),
		[]byte("not-a-jwt"), 0o600))

	_, err := FromServiceAccountMountAt(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing SA token")
}

// TestFromServiceAccountMountAt_NoClaims proves that a valid JWT that
// carries neither the sub nor the legacy claim is rejected.
func TestFromServiceAccountMountAt_NoClaims(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "namespace"),
		[]byte("ns"), 0o600))
	tok := makeJWT(t, map[string]any{"iss": "somewhere"})
	require.NoError(t, os.WriteFile(filepath.Join(root, "token"),
		[]byte(tok), 0o600))

	_, err := FromServiceAccountMountAt(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sub claim")
}

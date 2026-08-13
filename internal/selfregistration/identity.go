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
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultServiceAccountMount is the standard projected ServiceAccount mount
// path inside a Kubernetes pod.
const DefaultServiceAccountMount = "/var/run/secrets/kubernetes.io/serviceaccount"

// ErrNotInCluster is returned by FromServiceAccountMount* when neither the
// projected token file nor the namespace file are readable at the given
// mount root — i.e. the process is not running inside a pod.
var ErrNotInCluster = errors.New("not running in a pod: SA mount unavailable")

// Identity is the derived pod identity used to name and describe the
// operator's self-registered LifecycleManager CR.
type Identity struct {
	// Namespace is the pod's namespace, e.g. "concourse-system".
	Namespace string
	// ServiceAccount is the mounted ServiceAccount name,
	// e.g. "concourse-operator-controller-manager".
	ServiceAccount string
}

// FromServiceAccountMount reads the pod's projected SA token + namespace
// files at the default mount path and returns the Identity. Returns
// ErrNotInCluster if the SA files aren't present (running outside a pod).
func FromServiceAccountMount() (Identity, error) {
	return FromServiceAccountMountAt(DefaultServiceAccountMount)
}

// FromServiceAccountMountAt is the mount-root-parameterised variant of
// FromServiceAccountMount, used by unit tests to point at a temp dir.
func FromServiceAccountMountAt(root string) (Identity, error) {
	nsPath := filepath.Join(root, "namespace")
	tokenPath := filepath.Join(root, "token")

	nsBytes, nsErr := os.ReadFile(nsPath)
	tokenBytes, tokenErr := os.ReadFile(tokenPath)

	// Neither file → not running in a pod at all.
	if os.IsNotExist(nsErr) && os.IsNotExist(tokenErr) {
		return Identity{}, ErrNotInCluster
	}
	if nsErr != nil {
		return Identity{}, fmt.Errorf("reading %s: %w", nsPath, nsErr)
	}
	if tokenErr != nil {
		return Identity{}, fmt.Errorf("reading %s: %w", tokenPath, tokenErr)
	}

	ns := strings.TrimSpace(string(nsBytes))
	if ns == "" {
		return Identity{}, fmt.Errorf("empty namespace file at %s", nsPath)
	}

	sa, err := serviceAccountFromJWT(string(tokenBytes))
	if err != nil {
		return Identity{}, fmt.Errorf("parsing SA token at %s: %w", tokenPath, err)
	}

	return Identity{Namespace: ns, ServiceAccount: sa}, nil
}

// serviceAccountFromJWT extracts the SA name from a Kubernetes SA JWT.
// It prefers the RFC-7519 `sub` claim (projected/bound tokens set it to
// "system:serviceaccount:<ns>:<sa>"), and falls back to the legacy
// "kubernetes.io/serviceaccount/service-account.name" claim used by
// long-lived Secret-based tokens.
//
// Signature verification is intentionally skipped: we're introspecting our
// OWN token, and any tampering would fail at the API server anyway.
func serviceAccountFromJWT(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("empty token")
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("malformed JWT: expected >=2 segments, got %d", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Some issuers pad the payload; try standard URL encoding too.
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return "", fmt.Errorf("base64-decoding JWT payload: %w", err)
		}
	}

	var claims struct {
		Sub    string `json:"sub"`
		Legacy string `json:"kubernetes.io/serviceaccount/service-account.name"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("unmarshalling JWT payload: %w", err)
	}

	if sa := saFromSubClaim(claims.Sub); sa != "" {
		return sa, nil
	}
	if strings.TrimSpace(claims.Legacy) != "" {
		return strings.TrimSpace(claims.Legacy), nil
	}
	return "", errors.New("JWT has neither a system:serviceaccount:<ns>:<sa> sub claim nor a kubernetes.io/serviceaccount/service-account.name claim")
}

// saFromSubClaim parses "system:serviceaccount:<ns>:<sa>" and returns the
// trailing SA name. Returns "" if the claim doesn't match the expected shape.
func saFromSubClaim(sub string) string {
	sub = strings.TrimSpace(sub)
	if !strings.HasPrefix(sub, "system:serviceaccount:") {
		return ""
	}
	parts := strings.Split(sub, ":")
	// Expected: ["system", "serviceaccount", "<ns>", "<sa>"]
	if len(parts) < 4 {
		return ""
	}
	name := strings.TrimSpace(parts[len(parts)-1])
	if name == "" {
		return ""
	}
	return name
}

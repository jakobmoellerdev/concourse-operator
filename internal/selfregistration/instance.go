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
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Downward-API env variable names populated by the operator Deployment.
// Kept exported so callers (tests, docs, deployment tooling) can reference
// the canonical names.
const (
	EnvPodName         = "POD_NAME"
	EnvPodNamespace    = "POD_NAMESPACE"
	EnvPodUID          = "POD_UID"
	EnvNodeName        = "NODE_NAME"
	EnvContainerName   = "CONTAINER_NAME"
	defaultContainer   = "manager"
	shortSHALen        = 12
	devVersionMarker   = "(devel)"
	vcsRevisionSetting = "vcs.revision"
)

// InstanceInfo captures identifying facts about the running operator process.
// Mirrors the shape of maestro's delivery.v1alpha1.InstanceInfo (kept as a
// plain struct so this package does not depend on maestro Go types). Fields
// are serialised into the unstructured LifecycleManager CR's
// status.instance stanza by the registrar.
type InstanceInfo struct {
	PodName        string
	PodNamespace   string
	PodUID         string
	NodeName       string
	ServiceAccount string
	Image          string
	StartedAt      time.Time
	Version        string
}

// InstanceInfoResolver computes the running-pod InstanceInfo attached to the
// LifecycleManager CR. Returning (nil, nil) is legitimate when nothing is
// knowable (e.g. running outside a pod with no build info).
type InstanceInfoResolver func(context.Context) (*InstanceInfo, error)

// DefaultInstanceInfoResolver constructs the production resolver: it reads
// Downward-API env vars, fills the SA from the passed identity, queries the
// pod object (via c) once for the manager container's image, and stamps the
// StartedAt time captured at Registrar construction. Failures to fetch the
// pod are demoted to a V(1) log — image just stays empty.
func DefaultInstanceInfoResolver(c client.Client, id Identity, startedAt time.Time, log logr.Logger) InstanceInfoResolver {
	return func(ctx context.Context) (*InstanceInfo, error) {
		info := &InstanceInfo{
			PodName:        trimEnv(EnvPodName),
			PodNamespace:   trimEnv(EnvPodNamespace),
			PodUID:         trimEnv(EnvPodUID),
			NodeName:       trimEnv(EnvNodeName),
			ServiceAccount: id.ServiceAccount,
			Version:        buildVersion(),
			StartedAt:      startedAt,
		}

		// Best-effort: fetch the pod for the manager container's image.
		if info.PodName != "" && info.PodNamespace != "" && c != nil {
			pod := &corev1.Pod{}
			key := client.ObjectKey{Namespace: info.PodNamespace, Name: info.PodName}
			if err := c.Get(ctx, key, pod); err != nil {
				log.V(1).Info("could not fetch own pod for InstanceInfo.image",
					"pod", key.String(), "err", err.Error())
			} else {
				info.Image = containerImage(pod, os.Getenv(EnvContainerName))
			}
		}

		if isEmptyInstance(info) {
			return nil, nil
		}
		return info, nil
	}
}

// containerImage returns the image reference for the named container, falling
// back to the first container when name is empty or the named one is missing.
func containerImage(pod *corev1.Pod, name string) string {
	if pod == nil || len(pod.Spec.Containers) == 0 {
		return ""
	}
	if name == "" {
		name = defaultContainer
	}
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == name {
			return pod.Spec.Containers[i].Image
		}
	}
	// Fallback: first container.
	return pod.Spec.Containers[0].Image
}

// isEmptyInstance reports whether every field on info is unset — used to
// suppress writing a status.instance stanza when nothing is known.
func isEmptyInstance(info *InstanceInfo) bool {
	if info == nil {
		return true
	}
	return info.PodName == "" &&
		info.PodNamespace == "" &&
		info.PodUID == "" &&
		info.NodeName == "" &&
		info.ServiceAccount == "" &&
		info.Image == "" &&
		info.Version == "" &&
		info.StartedAt.IsZero()
}

// trimEnv returns strings.TrimSpace(os.Getenv(name)) — keeps the resolver
// terse and centralises env access.
func trimEnv(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

// buildVersion reads the compile-time build info and returns a stable
// short identifier: prefer bi.Main.Version when it's a real semver, else
// the short (12-char) vcs.revision from -buildvcs, else "".
func buildVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	if v := bi.Main.Version; v != "" && v != devVersionMarker {
		return v
	}
	for _, s := range bi.Settings {
		if s.Key == vcsRevisionSetting && s.Value != "" {
			if len(s.Value) > shortSHALen {
				return s.Value[:shortSHALen]
			}
			return s.Value
		}
	}
	return ""
}

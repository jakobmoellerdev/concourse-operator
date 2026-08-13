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

// Package selfregistration implements the LifecycleManager self-registration
// runnable: it derives the operator's identity (namespace + service account)
// from the pod's ServiceAccount mount and keeps a maestro LifecycleManager
// CR (delivery.maestro.sap.com/v1alpha1) fresh with periodic status
// heartbeats.
//
// This package deliberately avoids importing maestro Go types — the LM CR is
// manipulated as an unstructured.Unstructured so the operator does not need
// to vendor maestro's API module.
//
// The following RBAC markers keep `make manifests` producing the necessary
// verbs on the operator's aggregate ClusterRole:
//
// +kubebuilder:rbac:groups="",resources=pods,verbs=get
// +kubebuilder:rbac:groups=delivery.maestro.sap.com,resources=lifecyclemanagers,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=delivery.maestro.sap.com,resources=lifecyclemanagers/status,verbs=get;update;patch
package selfregistration

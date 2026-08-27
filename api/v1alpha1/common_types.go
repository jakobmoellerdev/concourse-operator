/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

// LocalObjectReference names a resource. When Namespace is empty the referent
// is resolved in the referring object's namespace.
type LocalObjectReference struct {
	// Name of the referent.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace of the referent. Empty means the same namespace as the referring object.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace,omitempty"`
}

// ResolveNamespace returns the referent namespace, falling back to defaultNS.
func (r LocalObjectReference) ResolveNamespace(defaultNS string) string {
	if r.Namespace != "" {
		return r.Namespace
	}
	return defaultNS
}

// SecretKeySelector references a key in a Kubernetes Secret.
type SecretKeySelector struct {
	// Name of the Secret.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Key within the Secret.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// ConfigMapKeyRef references a key within a ConfigMap.
type ConfigMapKeyRef struct {
	// Name of the ConfigMap.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Key within the ConfigMap.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// ReclaimPolicy controls what happens to the corresponding Concourse object
// when the Kubernetes resource is deleted.
// +kubebuilder:validation:Enum=Delete;Orphan
type ReclaimPolicy string

const (
	// ReclaimDelete deletes the Concourse object when the CR is removed.
	ReclaimDelete ReclaimPolicy = "Delete"
	// ReclaimOrphan leaves the Concourse object in place when the CR is removed.
	ReclaimOrphan ReclaimPolicy = "Orphan"
)

// ContainerImageSpec defines how a resource type container image is sourced.
type ContainerImageSpec struct {
	// Repository is the image repository (e.g. ghcr.io/org/k8s-config-resource).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Repository string `json:"repository"`

	// Tag is the image tag. Defaults to latest.
	// +optional
	Tag string `json:"tag,omitempty"`
}

// K8sConfigSpec declares a Kubernetes ConfigMap or Secret to be injected into a pipeline.
// Exactly one of configMapRef or secretRef must be set.
// +kubebuilder:validation:XValidation:rule="(has(self.configMapRef) && !has(self.secretRef)) || (!has(self.configMapRef) && has(self.secretRef))",message="exactly one of configMapRef or secretRef must be set"
type K8sConfigSpec struct {
	// Name is the name of the resource in the Concourse pipeline.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=100
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`
	Name string `json:"name"`

	// ConfigMapRef references a ConfigMap in the pipeline's namespace.
	// +optional
	ConfigMapRef *LocalObjectReference `json:"configMapRef,omitempty"`

	// SecretRef references a Secret in the pipeline's namespace.
	// +optional
	SecretRef *LocalObjectReference `json:"secretRef,omitempty"`

	// Trigger configures whether changes to this ConfigMap/Secret trigger dependent jobs.
	// +optional
	// +kubebuilder:default=false
	Trigger bool `json:"trigger,omitempty"`
}

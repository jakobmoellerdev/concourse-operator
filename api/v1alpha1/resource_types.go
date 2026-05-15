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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceSpec defines the desired state of Resource.
type ResourceSpec struct {
	// PipelineRef references the Pipeline that owns this resource.
	// +kubebuilder:validation:Required
	PipelineRef LocalObjectReference `json:"pipelineRef"`

	// ResourceName is the resource name within the pipeline. Defaults to metadata.name.
	// +optional
	ResourceName string `json:"resourceName,omitempty"`

	// PinnedVersion pins the resource to a specific version.
	// When set, the controller calls PinResourceVersion in Concourse.
	// When cleared, the controller calls UnpinResource.
	// +optional
	PinnedVersion map[string]string `json:"pinnedVersion,omitempty"`

	// CheckInterval triggers a Concourse resource check at this interval.
	// When unset, no automatic checks are triggered by the operator.
	// +optional
	CheckInterval *metav1.Duration `json:"checkInterval,omitempty"`
}

// ResourceStatus defines the observed state of Resource.
type ResourceStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastChecked is the timestamp of the last resource check.
	// +optional
	LastChecked *metav1.Time `json:"lastChecked,omitempty"`

	// LatestVersion is the most recent version of the resource.
	// +optional
	LatestVersion map[string]string `json:"latestVersion,omitempty"`

	// PinnedVersionID is the Concourse resource version ID that is pinned.
	// +optional
	PinnedVersionID int `json:"pinnedVersionID,omitempty"`

	// Pinned indicates whether the resource is currently pinned.
	// +optional
	Pinned bool `json:"pinned,omitempty"`

	// ObservedGeneration is the last generation that was reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Pipeline",type=string,JSONPath=`.spec.pipelineRef.name`
// +kubebuilder:printcolumn:name="Resource",type=string,JSONPath=`.spec.resourceName`
// +kubebuilder:printcolumn:name="Pinned",type=boolean,JSONPath=`.status.pinned`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Resource manages a resource within a Concourse pipeline (pin, check).
type Resource struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec ResourceSpec `json:"spec"`

	// +optional
	Status ResourceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ResourceList contains a list of Resource.
type ResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Resource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Resource{}, &ResourceList{})
}

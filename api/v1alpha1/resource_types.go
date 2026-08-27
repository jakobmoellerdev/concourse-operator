/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/License-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceSpec defines the desired state of a pipeline resource projection.
// The CR does not define the Concourse resource (type/source live in the
// Pipeline config); it pins and checks an existing named resource.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.resourceName) || self.resourceName == oldSelf.resourceName",message="resourceName is immutable"
// +kubebuilder:validation:XValidation:rule="self.pipelineRef == oldSelf.pipelineRef",message="pipelineRef is immutable"
type ResourceSpec struct {
	// PipelineRef references the Pipeline that owns this resource.
	// +kubebuilder:validation:Required
	PipelineRef LocalObjectReference `json:"pipelineRef"`

	// ResourceName is the resource name within the pipeline. Defaults to metadata.name.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=100
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`
	ResourceName string `json:"resourceName,omitempty"`

	// PinnedVersion pins the resource to a specific version.
	// When set, the controller looks up the version ID and calls PinResourceVersion.
	// When cleared, the controller calls UnpinResource.
	// +optional
	// +kubebuilder:validation:MinProperties=1
	PinnedVersion map[string]string `json:"pinnedVersion,omitempty"`

	// CheckInterval triggers a Concourse resource check at this interval.
	// When unset, no automatic checks are triggered by the operator.
	// +optional
	CheckInterval *metav1.Duration `json:"checkInterval,omitempty"`

	// PinComment is an optional comment explaining why the version was pinned.
	// +optional
	// +kubebuilder:validation:MaxLength=512
	PinComment string `json:"pinComment,omitempty"`

	// Suspend stops all reconciliation activity on this Resource.
	// Existing Concourse state is left untouched.
	// +optional
	// +kubebuilder:default=false
	Suspend bool `json:"suspend,omitempty"`
}

// ResourceStatus defines the observed state of Resource.
type ResourceStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ResolvedName is the Concourse resource name after defaulting.
	// +optional
	ResolvedName string `json:"resolvedName,omitempty"`

	// WebURL is the Concourse UI URL for this resource.
	// +optional
	// +kubebuilder:validation:Format=uri
	WebURL string `json:"webURL,omitempty"`

	// LastChecked is the timestamp of the last resource check.
	// +optional
	LastChecked *metav1.Time `json:"lastChecked,omitempty"`

	// LatestVersion is the most recent version of the resource.
	// +optional
	LatestVersion map[string]string `json:"latestVersion,omitempty"`

	// PinnedVersionID is the Concourse resource version ID that is pinned.
	// +optional
	PinnedVersionID *int32 `json:"pinnedVersionID,omitempty"`

	// Pinned indicates whether the resource is currently pinned in Concourse.
	// +optional
	Pinned *bool `json:"pinned,omitempty"`

	// ObservedGeneration is the last generation that was reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ccr,categories=concourse
// +kubebuilder:printcolumn:name="Pipeline",type=string,JSONPath=`.spec.pipelineRef.name`
// +kubebuilder:printcolumn:name="Resource",type=string,JSONPath=`.status.resolvedName`
// +kubebuilder:printcolumn:name="Pinned",type=boolean,JSONPath=`.status.pinned`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Check",type=string,JSONPath=`.status.conditions[?(@.type=="CheckHealthy")].status`,priority=1
// +kubebuilder:printcolumn:name="WebURL",type=string,JSONPath=`.status.webURL`,priority=1
// +kubebuilder:printcolumn:name="Suspended",type=boolean,JSONPath=`.spec.suspend`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Resource is a projection of one named resource inside a Pipeline.
// It manages pin/unpin and operator-driven checks.
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

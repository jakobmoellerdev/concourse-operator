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

// PipelineConfig sources the pipeline YAML configuration.
// Exactly one of Inline or ConfigMapRef must be set.
// +kubebuilder:validation:XValidation:rule="(has(self.inline) && size(self.inline) > 0 && !has(self.configMapRef)) || ((!has(self.inline) || size(self.inline) == 0) && has(self.configMapRef))",message="exactly one of inline or configMapRef must be set"
type PipelineConfig struct {
	// Inline is the pipeline YAML embedded directly in the spec.
	// +optional
	Inline string `json:"inline,omitempty"`

	// ConfigMapRef references a ConfigMap key containing the pipeline YAML.
	// +optional
	ConfigMapRef *ConfigMapKeyRef `json:"configMapRef,omitempty"`
}

// PipelineVar defines a variable passed to fly set-pipeline -v/-l.
// Exactly one of Value or ValueFrom must be set.
// +kubebuilder:validation:XValidation:rule="(has(self.value) && size(self.value) > 0 && !has(self.valueFrom)) || ((!has(self.value) || size(self.value) == 0) && has(self.valueFrom))",message="exactly one of value or valueFrom must be set"
type PipelineVar struct {
	// Name of the variable.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Value is the plain-text value. Mutually exclusive with ValueFrom.
	// +optional
	Value string `json:"value,omitempty"`
	// ValueFrom sources the value from a Secret. Mutually exclusive with Value.
	// +optional
	ValueFrom *SecretKeySelector `json:"valueFrom,omitempty"`
}

// PipelineJobStatus is the observed state of one job in a pipeline.
type PipelineJobStatus struct {
	// Name is the Concourse job name.
	Name string `json:"name"`
	// Paused reflects the pause state in Concourse.
	// +optional
	Paused bool `json:"paused,omitempty"`
	// LastBuildStatus is the status of the last finished build, if any.
	// +optional
	LastBuildStatus BuildPhase `json:"lastBuildStatus,omitempty"`
}

// PipelineResourceStatus is the observed state of one resource in a pipeline.
type PipelineResourceStatus struct {
	// Name is the Concourse resource name.
	Name string `json:"name"`
	// Pinned indicates whether the resource is currently pinned.
	// +optional
	Pinned bool `json:"pinned,omitempty"`
	// Type is the Concourse resource type.
	// +optional
	Type string `json:"type,omitempty"`
}

// PipelineGroupStatus is the observed state of a job group config.
type PipelineGroupStatus struct {
	// Name is the group name.
	Name string `json:"name"`
	// Jobs is the list of jobs in this group.
	// +optional
	// +listType=set
	Jobs []string `json:"jobs,omitempty"`
	// Resources is the list of resources in this group.
	// +optional
	// +listType=set
	Resources []string `json:"resources,omitempty"`
}

// PipelineResourceTypeStatus is the observed custom resource type used in a pipeline.
type PipelineResourceTypeStatus struct {
	// Name of the custom resource type.
	Name string `json:"name"`
	// Type is the parent type (e.g. docker-image).
	Type string `json:"type"`
	// Privileged indicates whether this custom resource type requires privileged containers.
	// +optional
	Privileged bool `json:"privileged,omitempty"`
	// Tags are tags to select workers.
	// +optional
	// +listType=set
	Tags []string `json:"tags,omitempty"`
}

// PipelineSpec defines the desired state of Pipeline.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.pipelineName) || self.pipelineName == oldSelf.pipelineName",message="pipelineName is immutable"
// +kubebuilder:validation:XValidation:rule="self.teamRef == oldSelf.teamRef",message="teamRef is immutable"
type PipelineSpec struct {
	// TeamRef references the Team this pipeline belongs to.
	// +kubebuilder:validation:Required
	TeamRef LocalObjectReference `json:"teamRef"`

	// PipelineName is the name in Concourse. Defaults to metadata.name.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=100
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`
	PipelineName string `json:"pipelineName,omitempty"`

	// Config defines the pipeline configuration source.
	// +kubebuilder:validation:Required
	Config PipelineConfig `json:"config"`

	// Vars are pipeline variables passed at set-pipeline time.
	// +optional
	Vars []PipelineVar `json:"vars,omitempty"`

	// Paused sets the pipeline pause state in Concourse.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// Exposed makes the pipeline publicly visible without authentication.
	// +optional
	Exposed bool `json:"exposed,omitempty"`

	// ReclaimPolicy controls whether the Concourse pipeline is deleted when this
	// CR is removed. Defaults to Delete.
	// +optional
	// +kubebuilder:default=Delete
	ReclaimPolicy ReclaimPolicy `json:"reclaimPolicy,omitempty"`

	// Suspend stops all reconciliation activity on this Pipeline.
	// Existing Concourse state is left untouched.
	// +optional
	// +kubebuilder:default=false
	Suspend bool `json:"suspend,omitempty"`

	// Archived sets whether this pipeline should be archived in Concourse.
	// +optional
	// +kubebuilder:default=false
	Archived bool `json:"archived,omitempty"`

	// K8sConfigImage overrides the resource-type image used for auto-injected Kubernetes config resources.
	// +optional
	K8sConfigImage *ContainerImageSpec `json:"k8sConfigImage,omitempty"`

	// K8sConfigs defines ConfigMaps and Secrets to automatically wire as resources into the pipeline.
	// +optional
	K8sConfigs []K8sConfigSpec `json:"k8sConfigs,omitempty"`
}

// PipelineStatus defines the observed state of Pipeline.
type PipelineStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// PipelineID is the Concourse-assigned pipeline ID.
	// +optional
	PipelineID *int32 `json:"pipelineID,omitempty"`

	// ResolvedName is the Concourse pipeline name after defaulting.
	// +optional
	ResolvedName string `json:"resolvedName,omitempty"`

	// WebURL is the Concourse UI URL for this pipeline.
	// +optional
	// +kubebuilder:validation:Format=uri
	WebURL string `json:"webURL,omitempty"`

	// ConfigHash is the SHA-256 hash of the last successfully applied pipeline
	// YAML plus interpolated vars.
	// +optional
	ConfigHash string `json:"configHash,omitempty"`

	// Paused reflects the actual pause state in Concourse. Nil means not yet observed.
	// +optional
	Paused *bool `json:"paused,omitempty"`

	// Exposed reflects the actual exposed state in Concourse. Nil means not yet observed.
	// +optional
	Exposed *bool `json:"exposed,omitempty"`

	// Archived reflects whether the pipeline is archived in Concourse. Nil means not yet observed.
	// +optional
	Archived *bool `json:"archived,omitempty"`

	// PausedBy is the user who paused the pipeline, if any.
	// +optional
	PausedBy string `json:"pausedBy,omitempty"`

	// PausedAt is when the pipeline was paused, if any.
	// +optional
	PausedAt *metav1.Time `json:"pausedAt,omitempty"`

	// LastUpdated is the timestamp when Concourse last recorded a change to this pipeline.
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// GroupCount is the number of job groups defined in the pipeline.
	// +optional
	GroupCount *int32 `json:"groupCount,omitempty"`

	// Jobs lists jobs observed from the applied pipeline config.
	// +optional
	// +listType=map
	// +listMapKey=name
	Jobs []PipelineJobStatus `json:"jobs,omitempty"`

	// Resources lists resources observed from the applied pipeline config.
	// +optional
	// +listType=map
	// +listMapKey=name
	Resources []PipelineResourceStatus `json:"resources,omitempty"`

	// Groups lists job groups defined in Concourse.
	// +optional
	// +listType=map
	// +listMapKey=name
	Groups []PipelineGroupStatus `json:"groups,omitempty"`

	// ResourceTypes lists custom resource types used by this pipeline.
	// +optional
	// +listType=map
	// +listMapKey=name
	ResourceTypes []PipelineResourceTypeStatus `json:"resourceTypes,omitempty"`

	// ObservedGeneration is the last generation that was reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ccp,categories=concourse
// +kubebuilder:printcolumn:name="Team",type=string,JSONPath=`.spec.teamRef.name`
// +kubebuilder:printcolumn:name="Pipeline",type=string,JSONPath=`.status.resolvedName`
// +kubebuilder:printcolumn:name="Paused",type=boolean,JSONPath=`.status.paused`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Exposed",type=boolean,JSONPath=`.status.exposed`,priority=1
// +kubebuilder:printcolumn:name="Archived",type=boolean,JSONPath=`.status.archived`,priority=1
// +kubebuilder:printcolumn:name="Synced",type=string,JSONPath=`.status.conditions[?(@.type=="ConfigSynced")].status`,priority=1
// +kubebuilder:printcolumn:name="WebURL",type=string,JSONPath=`.status.webURL`,priority=1
// +kubebuilder:printcolumn:name="Suspended",type=boolean,JSONPath=`.spec.suspend`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Pipeline manages a Concourse pipeline configuration.
type Pipeline struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec PipelineSpec `json:"spec"`

	// +optional
	Status PipelineStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PipelineList contains a list of Pipeline.
type PipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Pipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pipeline{}, &PipelineList{})
}

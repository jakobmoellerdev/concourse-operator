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

// ConfigMapKeyRef references a key within a ConfigMap.
type ConfigMapKeyRef struct {
	// Name of the ConfigMap.
	Name string `json:"name"`
	// Key within the ConfigMap.
	Key string `json:"key"`
}

// PipelineConfig sources the pipeline YAML configuration.
// Exactly one of Inline or ConfigMapRef must be set.
type PipelineConfig struct {
	// Inline is the pipeline YAML embedded directly in the spec.
	// +optional
	Inline string `json:"inline,omitempty"`

	// ConfigMapRef references a ConfigMap key containing the pipeline YAML.
	// +optional
	ConfigMapRef *ConfigMapKeyRef `json:"configMapRef,omitempty"`
}

// PipelineVar defines a variable passed to fly set-pipeline -v/-l.
type PipelineVar struct {
	// Name of the variable.
	Name string `json:"name"`
	// Value is the plain-text value. Mutually exclusive with ValueFrom.
	// +optional
	Value string `json:"value,omitempty"`
	// ValueFrom sources the value from a Secret. Mutually exclusive with Value.
	// +optional
	ValueFrom *SecretKeySelector `json:"valueFrom,omitempty"`
}

// PipelineSpec defines the desired state of Pipeline.
type PipelineSpec struct {
	// TeamRef references the Team this pipeline belongs to.
	// +kubebuilder:validation:Required
	TeamRef LocalObjectReference `json:"teamRef"`

	// PipelineName is the name in Concourse. Defaults to metadata.name.
	// +optional
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
}

// PipelineStatus defines the observed state of Pipeline.
type PipelineStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// PipelineID is the Concourse-assigned pipeline ID.
	// +optional
	PipelineID int `json:"pipelineID,omitempty"`

	// ConfigHash is the SHA-256 hash of the last successfully applied pipeline YAML.
	// +optional
	ConfigHash string `json:"configHash,omitempty"`

	// Paused reflects the actual pause state in Concourse.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// Exposed reflects the actual exposed state in Concourse.
	// +optional
	Exposed bool `json:"exposed,omitempty"`

	// ObservedGeneration is the last generation that was reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Team",type=string,JSONPath=`.spec.teamRef.name`
// +kubebuilder:printcolumn:name="Pipeline",type=string,JSONPath=`.spec.pipelineName`
// +kubebuilder:printcolumn:name="Paused",type=boolean,JSONPath=`.status.paused`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
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

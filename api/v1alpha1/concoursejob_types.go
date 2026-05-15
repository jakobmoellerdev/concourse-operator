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

// ConcourseJobSpec defines the desired state of ConcourseJob.
type ConcourseJobSpec struct {
	// PipelineRef references the ConcoursePipeline that owns this job.
	// +kubebuilder:validation:Required
	PipelineRef LocalObjectReference `json:"pipelineRef"`

	// JobName is the job name within the pipeline. Defaults to metadata.name.
	// +optional
	JobName string `json:"jobName,omitempty"`

	// Paused sets whether the job should be paused in Concourse.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// TriggerBuild causes the controller to create a ConcourseBuild CR on
	// each spec change, triggering a new build.
	// +optional
	TriggerBuild bool `json:"triggerBuild,omitempty"`
}

// ConcourseJobStatus defines the observed state of ConcourseJob.
type ConcourseJobStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Paused reflects the actual pause state in Concourse.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// NextBuildName is the name of the next ConcourseBuild CR if one was created.
	// +optional
	NextBuildName string `json:"nextBuildName,omitempty"`

	// ObservedGeneration is the last generation that was reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Pipeline",type=string,JSONPath=`.spec.pipelineRef.name`
// +kubebuilder:printcolumn:name="Job",type=string,JSONPath=`.spec.jobName`
// +kubebuilder:printcolumn:name="Paused",type=boolean,JSONPath=`.status.paused`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ConcourseJob manages a job within a Concourse pipeline (pause/unpause, trigger builds).
type ConcourseJob struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec ConcourseJobSpec `json:"spec"`

	// +optional
	Status ConcourseJobStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ConcourseJobList contains a list of ConcourseJob.
type ConcourseJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ConcourseJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConcourseJob{}, &ConcourseJobList{})
}

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

// JobInputStatus represents the status of an input to a Concourse job.
type JobInputStatus struct {
	// Name of the input.
	Name string `json:"name"`
	// Resource used by the input.
	Resource string `json:"resource"`
	// Trigger indicates whether a new version of the resource triggers a build.
	// +optional
	Trigger bool `json:"trigger,omitempty"`
	// Passed specifies jobs that the version must have passed.
	// +optional
	// +listType=set
	Passed []string `json:"passed,omitempty"`
}

// JobOutputStatus represents the status of an output from a Concourse job.
type JobOutputStatus struct {
	// Name of the output.
	Name string `json:"name"`
	// Resource updated by the output.
	Resource string `json:"resource"`
}

// JobSpec defines the desired state of Job.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.jobName) || self.jobName == oldSelf.jobName",message="jobName is immutable"
// +kubebuilder:validation:XValidation:rule="self.pipelineRef == oldSelf.pipelineRef",message="pipelineRef is immutable"
type JobSpec struct {
	// PipelineRef references the Pipeline that owns this job.
	// +kubebuilder:validation:Required
	PipelineRef LocalObjectReference `json:"pipelineRef"`

	// JobName is the job name within the pipeline. Defaults to metadata.name.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=100
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`
	JobName string `json:"jobName,omitempty"`

	// Paused sets whether the job should be paused in Concourse.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// SuccessfulBuildsHistoryLimit is how many succeeded Build CRs to keep.
	// +optional
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=0
	SuccessfulBuildsHistoryLimit *int32 `json:"successfulBuildsHistoryLimit,omitempty"`

	// FailedBuildsHistoryLimit is how many failed/errored/aborted Build CRs to keep.
	// +optional
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=0
	FailedBuildsHistoryLimit *int32 `json:"failedBuildsHistoryLimit,omitempty"`

	// Suspend stops all reconciliation activity on this Job.
	// Existing Concourse state is left untouched.
	// +optional
	// +kubebuilder:default=false
	Suspend bool `json:"suspend,omitempty"`
}

// JobStatus defines the observed state of Job.
type JobStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ResolvedName is the Concourse job name after defaulting.
	// +optional
	ResolvedName string `json:"resolvedName,omitempty"`

	// Paused reflects the actual pause state in Concourse. Nil means not yet observed.
	// +optional
	Paused *bool `json:"paused,omitempty"`

	// WebURL is the Concourse UI URL for this job.
	// +optional
	// +kubebuilder:validation:Format=uri
	WebURL string `json:"webURL,omitempty"`

	// JobID is the Concourse-internal numeric ID of this job.
	// +optional
	JobID *int32 `json:"jobID,omitempty"`

	// HasNewInputs indicates whether this job has unbuilt inputs.
	// +optional
	HasNewInputs *bool `json:"hasNewInputs,omitempty"`

	// Groups lists job groups this job belongs to.
	// +optional
	// +listType=set
	Groups []string `json:"groups,omitempty"`

	// Inputs lists the input resources defined for this job.
	// +optional
	// +listType=map
	// +listMapKey=name
	Inputs []JobInputStatus `json:"inputs,omitempty"`

	// Outputs lists the output resources defined for this job.
	// +optional
	// +listType=map
	// +listMapKey=name
	Outputs []JobOutputStatus `json:"outputs,omitempty"`

	// NextBuildID is the numeric ID of Concourse's next build (if currently running).
	// +optional
	NextBuildID *int32 `json:"nextBuildID,omitempty"`

	// NextBuildStatus is the status of the currently running build (if any).
	// +optional
	NextBuildStatus BuildPhase `json:"nextBuildStatus,omitempty"`

	// DisableManualTrigger reflects if manual triggers are disabled.
	// +optional
	DisableManualTrigger *bool `json:"disableManualTrigger,omitempty"`

	// PausedBy is the user who paused this job.
	// +optional
	PausedBy string `json:"pausedBy,omitempty"`

	// PausedAt is when this job was paused.
	// +optional
	PausedAt *metav1.Time `json:"pausedAt,omitempty"`

	// NextBuildName is the name of the most recently created Build CR.
	// +optional
	NextBuildName string `json:"nextBuildName,omitempty"`

	// LastBuildID is the Concourse ID of the last finished build.
	// +optional
	LastBuildID *int32 `json:"lastBuildID,omitempty"`

	// LastBuildStatus is the status of the last finished build.
	// +optional
	LastBuildStatus BuildPhase `json:"lastBuildStatus,omitempty"`

	// LastBuildTime is when the last finished build ended.
	// +optional
	LastBuildTime *metav1.Time `json:"lastBuildTime,omitempty"`

	// ObservedGeneration is the last generation that was reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ccj,categories=concourse
// +kubebuilder:printcolumn:name="Pipeline",type=string,JSONPath=`.spec.pipelineRef.name`
// +kubebuilder:printcolumn:name="Job",type=string,JSONPath=`.status.resolvedName`
// +kubebuilder:printcolumn:name="Paused",type=boolean,JSONPath=`.status.paused`
// +kubebuilder:printcolumn:name="LastBuild",type=string,JSONPath=`.status.lastBuildStatus`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Next",type=string,JSONPath=`.status.nextBuildStatus`,priority=1
// +kubebuilder:printcolumn:name="WebURL",type=string,JSONPath=`.status.webURL`,priority=1
// +kubebuilder:printcolumn:name="Suspended",type=boolean,JSONPath=`.spec.suspend`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Job manages a job within a Concourse pipeline (pause/unpause).
// Create a Build CR to trigger a new build.
type Job struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec JobSpec `json:"spec"`

	// +optional
	Status JobStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// JobList contains a list of Job.
type JobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Job `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Job{}, &JobList{})
}

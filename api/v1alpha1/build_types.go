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

// BuildIO represents an input or output resource version used in a build.
type BuildIO struct {
	// Name of the input or output resource.
	Name string `json:"name"`
	// Version of the resource.
	// +optional
	Version map[string]string `json:"version,omitempty"`
	// FirstOccurrence indicates whether this build is the first time this version ran in the pipeline.
	// +optional
	FirstOccurrence *bool `json:"firstOccurrence,omitempty"`
}

// BuildPhase mirrors Concourse build status values.
// +kubebuilder:validation:Enum=pending;started;succeeded;failed;errored;aborted
type BuildPhase string

const (
	BuildPhasePending   BuildPhase = "pending"
	BuildPhaseStarted   BuildPhase = "started"
	BuildPhaseSucceeded BuildPhase = "succeeded"
	BuildPhaseFailed    BuildPhase = "failed"
	BuildPhaseErrored   BuildPhase = "errored"
	BuildPhaseAborted   BuildPhase = "aborted"
)

// BuildSpec defines the desired state of Build.
// Creating a Build with no BuildID triggers a new Concourse build once.
// Setting BuildID adopts and watches an existing Concourse build.
// Setting RerunOf creates a new build that reruns a previous build.
// +kubebuilder:validation:XValidation:rule="has(self.jobRef)",message="jobRef is required"
// +kubebuilder:validation:XValidation:rule="!(has(self.rerunOf) && has(self.buildID))",message="rerunOf and buildID are mutually exclusive"
type BuildSpec struct {
	// JobRef references the Job that this build belongs to.
	// +optional
	JobRef *LocalObjectReference `json:"jobRef,omitempty"`

	// BuildID adopts an existing Concourse build instead of creating a new one.
	// +optional
	// +kubebuilder:validation:Minimum=1
	BuildID *int32 `json:"buildID,omitempty"`

	// RerunOf is the build NAME/number (e.g. "42") of a previous build to
	// rerun. When set, the controller creates a new build as a rerun of the
	// referenced build instead of triggering a fresh one. Mutually exclusive
	// with BuildID.
	// +optional
	RerunOf string `json:"rerunOf,omitempty"`

	// Comment sets a comment on the build once it exists. Reconciled
	// idempotently: only applied when it differs from the last comment
	// recorded in status.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Comment string `json:"comment,omitempty"`

	// Canceled requests abort of a non-terminal build. Ignored once the build
	// has reached a terminal state.
	// +optional
	Canceled bool `json:"canceled,omitempty"`

	// TTLSecondsAfterFinished is how long to keep this Build after it reaches a
	// terminal state. Zero or unset means keep until history limits prune it.
	// +optional
	// +kubebuilder:validation:Minimum=0
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

	// Suspend stops all reconciliation activity on this Build.
	// Existing Concourse state is left untouched.
	// +optional
	// +kubebuilder:default=false
	Suspend bool `json:"suspend,omitempty"`
}

// BuildStatus defines the observed state of Build.
type BuildStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// BuildID is the numeric Concourse build ID.
	// +optional
	BuildID *int32 `json:"buildID,omitempty"`

	// BuildName is the display name of the build (e.g. "42").
	// +optional
	BuildName string `json:"buildName,omitempty"`

	// ConcourseStatus reflects the current Concourse build status.
	// +optional
	ConcourseStatus BuildPhase `json:"concourseStatus,omitempty"`

	// StartTime is when the build started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// EndTime is when the build completed.
	// +optional
	EndTime *metav1.Time `json:"endTime,omitempty"`

	// Duration is the elapsed time between StartTime and EndTime.
	// +optional
	Duration *metav1.Duration `json:"duration,omitempty"`

	// APIURL is the Concourse API path for this build.
	// +optional
	APIURL string `json:"apiURL,omitempty"`

	// CreatedBy is the user or system that triggered the build.
	// +optional
	CreatedBy string `json:"createdBy,omitempty"`

	// ObservedGeneration is the last generation that was reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// WebURL is the Concourse UI URL for this build.
	// +optional
	// +kubebuilder:validation:Format=uri
	WebURL string `json:"webURL,omitempty"`

	// Inputs lists the input resource versions used in this build.
	// +optional
	// +listType=map
	// +listMapKey=name
	Inputs []BuildIO `json:"inputs,omitempty"`

	// Outputs lists the output resource versions produced by this build.
	// +optional
	// +listType=map
	// +listMapKey=name
	Outputs []BuildIO `json:"outputs,omitempty"`

	// Comment is the comment last applied by the operator to this build.
	// +optional
	Comment string `json:"comment,omitempty"`

	// RerunOf reflects the build name/number this build was a rerun of, if any.
	// +optional
	RerunOf string `json:"rerunOf,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ccb,categories=concourse
// +kubebuilder:printcolumn:name="Job",type=string,JSONPath=`.spec.jobRef.name`
// +kubebuilder:printcolumn:name="BuildID",type=integer,JSONPath=`.status.buildID`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.concourseStatus`
// +kubebuilder:printcolumn:name="Complete",type=string,JSONPath=`.status.conditions[?(@.type=="Complete")].status`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,priority=1
// +kubebuilder:printcolumn:name="Duration",type=string,JSONPath=`.status.duration`,priority=1
// +kubebuilder:printcolumn:name="WebURL",type=string,JSONPath=`.status.webURL`,priority=1
// +kubebuilder:printcolumn:name="Suspended",type=boolean,JSONPath=`.spec.suspend`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Build tracks and optionally triggers a build in Concourse.
type Build struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec BuildSpec `json:"spec"`

	// +optional
	Status BuildStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// BuildList contains a list of Build.
type BuildList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Build `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Build{}, &BuildList{})
}

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

// BuildStatus mirrors Concourse build status values.
// +kubebuilder:validation:Enum=pending;started;succeeded;failed;errored;aborted
type BuildStatus string

const (
	BuildStatusPending   BuildStatus = "pending"
	BuildStatusStarted   BuildStatus = "started"
	BuildStatusSucceeded BuildStatus = "succeeded"
	BuildStatusFailed    BuildStatus = "failed"
	BuildStatusErrored   BuildStatus = "errored"
	BuildStatusAborted   BuildStatus = "aborted"
)

// ConcourseBuildSpec defines the desired state of ConcourseBuild.
type ConcourseBuildSpec struct {
	// JobRef references the ConcourseJob that triggered this build.
	// Required unless OneOff is true.
	// +optional
	JobRef *LocalObjectReference `json:"jobRef,omitempty"`

	// OneOff indicates this is a one-off build not tied to a pipeline job.
	// When true, JobRef must be unset.
	// +optional
	OneOff bool `json:"oneOff,omitempty"`

	// Abort signals the controller to abort this build if it is running.
	// +optional
	Abort bool `json:"abort,omitempty"`
}

// ConcourseBuildStatus defines the observed state of ConcourseBuild.
type ConcourseBuildStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// BuildID is the numeric Concourse build ID.
	// +optional
	BuildID int `json:"buildID,omitempty"`

	// BuildName is the display name of the build (e.g. "42").
	// +optional
	BuildName string `json:"buildName,omitempty"`

	// ConcourseStatus reflects the current Concourse build status.
	// +optional
	ConcourseStatus BuildStatus `json:"concourseStatus,omitempty"`

	// StartTime is when the build started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// EndTime is when the build completed.
	// +optional
	EndTime *metav1.Time `json:"endTime,omitempty"`

	// APIURL is the URL to the build on the Concourse web UI.
	// +optional
	APIURL string `json:"apiURL,omitempty"`

	// ObservedGeneration is the last generation that was reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Job",type=string,JSONPath=`.spec.jobRef.name`
// +kubebuilder:printcolumn:name="BuildID",type=integer,JSONPath=`.status.buildID`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.concourseStatus`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ConcourseBuild tracks and optionally triggers a build in Concourse.
type ConcourseBuild struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec ConcourseBuildSpec `json:"spec"`

	// +optional
	Status ConcourseBuildStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ConcourseBuildList contains a list of ConcourseBuild.
type ConcourseBuildList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ConcourseBuild `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConcourseBuild{}, &ConcourseBuildList{})
}

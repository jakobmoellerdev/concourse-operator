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

// WorkerDesiredState is the desired lifecycle state for a worker.
// +kubebuilder:validation:Enum=active;land;retire;prune
type WorkerDesiredState string

const (
	WorkerDesiredStateActive WorkerDesiredState = "active"
	WorkerDesiredStateLand   WorkerDesiredState = "land"
	WorkerDesiredStateRetire WorkerDesiredState = "retire"
	WorkerDesiredStatePrune  WorkerDesiredState = "prune"
)

// ConcourseWorkerSpec defines the desired state of ConcourseWorker.
type ConcourseWorkerSpec struct {
	// InstanceRef references the ConcourseInstance this worker belongs to.
	// +kubebuilder:validation:Required
	InstanceRef LocalObjectReference `json:"instanceRef"`

	// WorkerName is the unique name of the worker in Concourse.
	// +kubebuilder:validation:Required
	WorkerName string `json:"workerName"`

	// DesiredState sets the lifecycle action to perform on the worker.
	// - active: no-op (workers self-register)
	// - land: gracefully drain and land the worker
	// - retire: retire the worker from the pool
	// - prune: forcibly remove the worker
	// +optional
	// +kubebuilder:default=active
	DesiredState WorkerDesiredState `json:"desiredState,omitempty"`
}

// ConcourseWorkerStatus defines the observed state of ConcourseWorker.
type ConcourseWorkerStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ActualState is the current state of the worker as reported by Concourse.
	// +optional
	ActualState string `json:"actualState,omitempty"`

	// Platform is the worker platform (e.g. linux, darwin).
	// +optional
	Platform string `json:"platform,omitempty"`

	// Tags are the worker tags.
	// +optional
	Tags []string `json:"tags,omitempty"`

	// ActiveContainers is the number of containers running on the worker.
	// +optional
	ActiveContainers int `json:"activeContainers,omitempty"`

	// ActiveVolumes is the number of volumes on the worker.
	// +optional
	ActiveVolumes int `json:"activeVolumes,omitempty"`

	// ObservedGeneration is the last generation that was reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Instance",type=string,JSONPath=`.spec.instanceRef.name`
// +kubebuilder:printcolumn:name="Worker",type=string,JSONPath=`.spec.workerName`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.actualState`
// +kubebuilder:printcolumn:name="Platform",type=string,JSONPath=`.status.platform`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ConcourseWorker manages a worker in a Concourse instance (land, retire, prune).
type ConcourseWorker struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec ConcourseWorkerSpec `json:"spec"`

	// +optional
	Status ConcourseWorkerStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ConcourseWorkerList contains a list of ConcourseWorker.
type ConcourseWorkerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ConcourseWorker `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConcourseWorker{}, &ConcourseWorkerList{})
}

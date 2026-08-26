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

// WorkerResourceTypeStatus represents an advertised custom resource type version on the worker.
type WorkerResourceTypeStatus struct {
	// Type of the custom resource type.
	Type string `json:"type"`
	// Version of the custom resource type.
	// +optional
	Version string `json:"version,omitempty"`
	// Privileged indicates whether this custom resource type requires privileged containers.
	// +optional
	Privileged bool `json:"privileged,omitempty"`
}

// WorkerLifecycle is the desired lifecycle of a worker.
// +kubebuilder:validation:Enum=Running;Draining;Removed
type WorkerLifecycle string

const (
	// WorkerLifecycleRunning leaves the worker in the pool (no-op; workers self-register).
	WorkerLifecycleRunning WorkerLifecycle = "Running"
	// WorkerLifecycleDraining lands the worker (graceful drain).
	WorkerLifecycleDraining WorkerLifecycle = "Draining"
	// WorkerLifecycleRemoved prunes the worker from the pool.
	WorkerLifecycleRemoved WorkerLifecycle = "Removed"
)

// WorkerPhase is the observed Concourse worker state.
// +kubebuilder:validation:Enum=running;landing;landed;retiring;stalled;missing
type WorkerPhase string

const (
	WorkerPhaseRunning  WorkerPhase = "running"
	WorkerPhaseLanding  WorkerPhase = "landing"
	WorkerPhaseLanded   WorkerPhase = "landed"
	WorkerPhaseRetiring WorkerPhase = "retiring"
	WorkerPhaseStalled  WorkerPhase = "stalled"
	WorkerPhaseMissing  WorkerPhase = "missing"
)

// WorkerSpec defines the desired state of Worker.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.workerName) || self.workerName == oldSelf.workerName",message="workerName is immutable"
// +kubebuilder:validation:XValidation:rule="self.instanceRef == oldSelf.instanceRef",message="instanceRef is immutable"
type WorkerSpec struct {
	// InstanceRef references the Instance this worker belongs to.
	// +kubebuilder:validation:Required
	InstanceRef LocalObjectReference `json:"instanceRef"`

	// WorkerName is the unique name of the worker in Concourse.
	// Defaults to metadata.name.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=100
	WorkerName string `json:"workerName,omitempty"`

	// Lifecycle is the desired worker lifecycle.
	// - Running: leave the worker in the pool (default)
	// - Draining: land the worker (graceful drain)
	// - Removed: prune the worker from the pool
	// +optional
	// +kubebuilder:default=Running
	Lifecycle WorkerLifecycle `json:"lifecycle,omitempty"`

	// Suspend stops all reconciliation activity on this Worker.
	// Existing Concourse state is left untouched.
	// +optional
	// +kubebuilder:default=false
	Suspend bool `json:"suspend,omitempty"`
}

// WorkerStatus defines the observed state of Worker.
type WorkerStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Phase is the current state of the worker as reported by Concourse.
	// +optional
	Phase WorkerPhase `json:"phase,omitempty"`

	// ResolvedName is the Concourse worker name after defaulting.
	// +optional
	ResolvedName string `json:"resolvedName,omitempty"`

	// Platform is the worker platform (e.g. linux, darwin).
	// +optional
	Platform string `json:"platform,omitempty"`

	// Tags are the worker tags.
	// +optional
	Tags []string `json:"tags,omitempty"`

	// ActiveContainers is the number of containers running on the worker.
	// +optional
	ActiveContainers *int32 `json:"activeContainers,omitempty"`

	// ActiveVolumes is the number of volumes on the worker.
	// +optional
	ActiveVolumes *int32 `json:"activeVolumes,omitempty"`

	// Version is the worker binary version.
	// +optional
	Version string `json:"version,omitempty"`

	// StartTime is when the worker registered with Concourse.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// Ephemeral indicates whether the worker is ephemeral (auto-removed on disconnect).
	// +optional
	Ephemeral bool `json:"ephemeral,omitempty"`

	// Team is the Concourse team this worker is scoped to; empty means global worker.
	// +optional
	Team string `json:"team,omitempty"`

	// ActiveTasks is the number of active tasks running on this worker.
	// +optional
	ActiveTasks *int32 `json:"activeTasks,omitempty"`

	// ResourceTypes is the list of custom resource types supported by this worker.
	// +optional
	// +listType=map
	// +listMapKey=type
	ResourceTypes []WorkerResourceTypeStatus `json:"resourceTypes,omitempty"`

	// ObservedGeneration is the last generation that was reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ccw,categories=concourse
// +kubebuilder:printcolumn:name="Instance",type=string,JSONPath=`.spec.instanceRef.name`
// +kubebuilder:printcolumn:name="Worker",type=string,JSONPath=`.status.resolvedName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Platform",type=string,JSONPath=`.status.platform`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`
// +kubebuilder:printcolumn:name="Tasks",type=integer,JSONPath=`.status.activeTasks`,priority=1
// +kubebuilder:printcolumn:name="Stalled",type=string,JSONPath=`.status.conditions[?(@.type=="Stalled")].status`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Suspended",type=boolean,JSONPath=`.spec.suspend`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Worker observes and manages a worker in a Concourse instance.
type Worker struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec WorkerSpec `json:"spec"`

	// +optional
	Status WorkerStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WorkerList contains a list of Worker.
type WorkerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Worker `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Worker{}, &WorkerList{})
}

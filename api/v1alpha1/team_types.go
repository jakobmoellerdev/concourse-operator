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

// TeamRole defines a role binding within a Concourse team.
type TeamRole struct {
	// Role is the Concourse role name (owner, member, pipeline-operator, viewer).
	// +kubebuilder:validation:Enum=owner;member;pipeline-operator;viewer
	Role string `json:"role"`
	// Users is a list of user identifiers (provider:username).
	// +optional
	Users []string `json:"users,omitempty"`
	// Groups is a list of group identifiers (provider:group).
	// +optional
	Groups []string `json:"groups,omitempty"`
}

// TeamSpec defines the desired state of Team.
type TeamSpec struct {
	// InstanceRef references the Instance this team belongs to.
	// +kubebuilder:validation:Required
	InstanceRef LocalObjectReference `json:"instanceRef"`

	// TeamName is the team name in Concourse. Defaults to metadata.name.
	// +optional
	TeamName string `json:"teamName,omitempty"`

	// Roles defines auth role bindings for the team.
	// +optional
	Roles []TeamRole `json:"roles,omitempty"`
}

// TeamStatus defines the observed state of Team.
type TeamStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// TeamID is the Concourse-assigned numeric team ID.
	// +optional
	TeamID int `json:"teamID,omitempty"`

	// LastApplied is the timestamp of the last successful CreateOrUpdate.
	// +optional
	LastApplied *metav1.Time `json:"lastApplied,omitempty"`

	// ObservedGeneration is the last generation that was reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Instance",type=string,JSONPath=`.spec.instanceRef.name`
// +kubebuilder:printcolumn:name="Team",type=string,JSONPath=`.spec.teamName`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Team manages a Concourse team and its auth configuration.
type Team struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec TeamSpec `json:"spec"`

	// +optional
	Status TeamStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// TeamList contains a list of Team.
type TeamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Team `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Team{}, &TeamList{})
}

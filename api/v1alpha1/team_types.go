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

// TeamRole defines a role binding within a Concourse team.
// +kubebuilder:validation:XValidation:rule="size(self.users) > 0 || size(self.groups) > 0",message="each role must have at least one user or group"
type TeamRole struct {
	// Role is the Concourse role name (owner, member, pipeline-operator, viewer).
	// +kubebuilder:validation:Enum=owner;member;pipeline-operator;viewer
	Role string `json:"role"`
	// Users is a list of user identifiers (provider:username).
	// +optional
	// +listType=set
	Users []string `json:"users,omitempty"`
	// Groups is a list of group identifiers (provider:group).
	// +optional
	// +listType=set
	Groups []string `json:"groups,omitempty"`
}

// TeamSpec defines the desired state of Team.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.teamName) || self.teamName == oldSelf.teamName",message="teamName is immutable"
// +kubebuilder:validation:XValidation:rule="self.instanceRef == oldSelf.instanceRef",message="instanceRef is immutable"
type TeamSpec struct {
	// InstanceRef references the Instance this team belongs to.
	// +kubebuilder:validation:Required
	InstanceRef LocalObjectReference `json:"instanceRef"`

	// TeamName is the team name in Concourse. Defaults to metadata.name.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=100
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`
	TeamName string `json:"teamName,omitempty"`

	// Roles defines auth role bindings for the team.
	// +optional
	// +listType=map
	// +listMapKey=role
	Roles []TeamRole `json:"roles,omitempty"`

	// AllowDestroy permits deleting the reserved Concourse "main" team.
	// Defaults to false. Destroying main is almost always a mistake.
	// +optional
	AllowDestroy bool `json:"allowDestroy,omitempty"`

	// ReclaimPolicy controls whether the Concourse team is deleted when this
	// CR is removed. Defaults to Delete.
	// +optional
	// +kubebuilder:default=Delete
	ReclaimPolicy ReclaimPolicy `json:"reclaimPolicy,omitempty"`

	// Suspend stops all reconciliation activity on this Team.
	// Existing Concourse state is left untouched.
	// +optional
	// +kubebuilder:default=false
	Suspend bool `json:"suspend,omitempty"`
}

// TeamStatus defines the observed state of Team.
type TeamStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// TeamID is the Concourse-assigned numeric team ID.
	// +optional
	TeamID *int32 `json:"teamID,omitempty"`

	// ResolvedName is the Concourse team name after defaulting.
	// +optional
	ResolvedName string `json:"resolvedName,omitempty"`

	// WebURL is the Concourse UI URL for this team.
	// +optional
	// +kubebuilder:validation:Format=uri
	WebURL string `json:"webURL,omitempty"`

	// LastApplied is the timestamp of the last successful CreateOrUpdate.
	// +optional
	LastApplied *metav1.Time `json:"lastApplied,omitempty"`

	// ObservedGeneration is the last generation that was reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=cct,categories=concourse
// +kubebuilder:printcolumn:name="Instance",type=string,JSONPath=`.spec.instanceRef.name`
// +kubebuilder:printcolumn:name="Team",type=string,JSONPath=`.status.resolvedName`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="ID",type=integer,JSONPath=`.status.teamID`,priority=1
// +kubebuilder:printcolumn:name="WebURL",type=string,JSONPath=`.status.webURL`,priority=1
// +kubebuilder:printcolumn:name="Suspended",type=boolean,JSONPath=`.spec.suspend`,priority=1
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

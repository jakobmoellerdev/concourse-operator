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

// PasswordGrant holds local-user credentials used for an OAuth2 password grant
// against Concourse's sky/issuer token endpoint.
type PasswordGrant struct {
	// Username for the password grant.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Username string `json:"username"`
	// PasswordRef references the Secret key containing the password.
	// +kubebuilder:validation:Required
	PasswordRef SecretKeySelector `json:"passwordRef"`
	// ClientID is the OAuth2 client ID. Defaults to the well-known fly client.
	// +optional
	ClientID string `json:"clientID,omitempty"`
	// ClientSecretRef references the Secret key containing the OAuth2 client secret.
	// When unset, the well-known fly client secret is used.
	// +optional
	ClientSecretRef *SecretKeySelector `json:"clientSecretRef,omitempty"`
}

// TokenAuth holds a bearer token reference.
type TokenAuth struct {
	// TokenRef references the Secret key containing the bearer token.
	// +kubebuilder:validation:Required
	TokenRef SecretKeySelector `json:"tokenRef"`
}

// InstanceAuth configures authentication to the Concourse ATC.
// Exactly one of Password or Token must be set.
// +kubebuilder:validation:XValidation:rule="[has(self.password), has(self.token)].filter(x, x).size() == 1",message="exactly one of password or token must be set"
type InstanceAuth struct {
	// Password configures an OAuth2 password grant (local user).
	// +optional
	Password *PasswordGrant `json:"password,omitempty"`
	// Token configures bearer token authentication.
	// +optional
	Token *TokenAuth `json:"token,omitempty"`
}

// TLSConfig configures TLS for the Concourse connection.
// +kubebuilder:validation:XValidation:rule="!(has(self.caRef) && self.insecureSkipVerify)",message="caRef and insecureSkipVerify are mutually exclusive"
type TLSConfig struct {
	// CARef references a Secret key containing a PEM-encoded CA certificate.
	// +optional
	CARef *SecretKeySelector `json:"caRef,omitempty"`
	// InsecureSkipVerify disables TLS certificate verification.
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// InstanceSpec defines the desired state of Instance.
// +kubebuilder:validation:XValidation:rule="!has(self.healthProbeInterval) || (duration(self.healthProbeInterval) >= duration('30s') && duration(self.healthProbeInterval) <= duration('1h'))",message="healthProbeInterval must be between 30s and 1h"
type InstanceSpec struct {
	// URL is the base URL of the Concourse ATC, e.g. https://ci.example.com.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=uri
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^https?://.+`
	URL string `json:"url"`

	// Auth configures authentication to the ATC. Required.
	// +kubebuilder:validation:Required
	Auth InstanceAuth `json:"auth"`

	// TLS configures TLS settings for the connection.
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`

	// AllowedNamespaces lists namespaces whose Team/Worker CRs may reference this
	// Instance. Empty means only the Instance's own namespace is allowed.
	// +optional
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`

	// HealthProbeInterval is how often the controller probes this instance.
	// +optional
	// +kubebuilder:default="5m"
	HealthProbeInterval *metav1.Duration `json:"healthProbeInterval,omitempty"`

	// Suspend stops all reconciliation activity on this Instance.
	// Existing Concourse state is left untouched.
	// +optional
	// +kubebuilder:default=false
	Suspend bool `json:"suspend,omitempty"`

	// DefaultTeam is the default team name to use when not specified (defaults to "main").
	// +optional
	// +kubebuilder:default="main"
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`
	DefaultTeam string `json:"defaultTeam,omitempty"`
}

// InstanceStatus defines the observed state of Instance.
type InstanceStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Version is the Concourse server version string.
	// +optional
	Version string `json:"version,omitempty"`

	// WorkerVersion is the minimum worker binary version required by this instance.
	// +optional
	WorkerVersion string `json:"workerVersion,omitempty"`

	// ClusterName is the human-readable cluster name reported by Concourse.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// ExternalURL is the externally-visible URL reported by the Concourse server.
	// +optional
	ExternalURL string `json:"externalURL,omitempty"`

	// WorkerCount is the total number of registered workers.
	// +optional
	WorkerCount *int32 `json:"workerCount,omitempty"`

	// StalledWorkers is the number of workers in the "stalled" state.
	// +optional
	StalledWorkers *int32 `json:"stalledWorkers,omitempty"`

	// RunningWorkers is the number of workers in the "running" state.
	// +optional
	RunningWorkers *int32 `json:"runningWorkers,omitempty"`

	// LandingWorkers is the number of workers in the "landing" state.
	// +optional
	LandingWorkers *int32 `json:"landingWorkers,omitempty"`

	// WebURL is the Concourse UI URL for this instance.
	// +optional
	// +kubebuilder:validation:Format=uri
	WebURL string `json:"webURL,omitempty"`

	// FeatureFlags maps the Concourse server's enabled feature flags.
	// +optional
	FeatureFlags map[string]bool `json:"featureFlags,omitempty"`

	// AuthenticatedUser is the username currently used to connect.
	// +optional
	AuthenticatedUser string `json:"authenticatedUser,omitempty"`

	// AuthenticatedAdmin indicates whether the connected user is a Concourse cluster admin.
	// +optional
	AuthenticatedAdmin *bool `json:"authenticatedAdmin,omitempty"`

	// WallMessage is any active cluster-wide notification message.
	// +optional
	WallMessage string `json:"wallMessage,omitempty"`

	// TeamCount is the total number of teams found in this Concourse instance.
	// +optional
	TeamCount *int32 `json:"teamCount,omitempty"`

	// PipelineCount is the total number of pipelines found in this Concourse instance.
	// +optional
	PipelineCount *int32 `json:"pipelineCount,omitempty"`

	// AuthSecretGeneration is the generation of the Secret last used for auth.
	// +optional
	AuthSecretGeneration int64 `json:"authSecretGeneration,omitempty"`

	// ObservedGeneration is the last generation that was reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

const (
	// ConditionReady indicates the resource is reconciled and functional.
	ConditionReady = "Ready"
	// ConditionAuthenticated indicates authentication with Concourse succeeded.
	ConditionAuthenticated = "Authenticated"
	// ConditionWorkersHealthy indicates all workers are in the running state.
	ConditionWorkersHealthy = "WorkersHealthy"
	// ConditionConfigWarning indicates the last config apply returned warnings.
	ConditionConfigWarning = "ConfigWarning"
	// ConditionConfigSynced indicates the pipeline config is applied without error.
	ConditionConfigSynced = "ConfigSynced"
	// ConditionLastBuildSucceeded indicates the most recent finished build succeeded.
	ConditionLastBuildSucceeded = "LastBuildSucceeded"
	// ConditionCheckHealthy indicates the last resource check succeeded.
	ConditionCheckHealthy = "CheckHealthy"
	// ConditionComplete indicates the build has reached a terminal state.
	ConditionComplete = "Complete"
	// ConditionStalled indicates the worker is in the stalled state.
	ConditionStalled = "Stalled"
	// ConditionStateTransitioning indicates the worker's desired and actual states differ.
	ConditionStateTransitioning = "StateTransitioning"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=cci;concinst,categories=concourse
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`
// +kubebuilder:printcolumn:name="Workers",type=integer,JSONPath=`.status.workerCount`
// +kubebuilder:printcolumn:name="Stalled",type=integer,JSONPath=`.status.stalledWorkers`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="WebURL",type=string,JSONPath=`.status.webURL`,priority=1
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.status.clusterName`,priority=1
// +kubebuilder:printcolumn:name="Suspended",type=boolean,JSONPath=`.spec.suspend`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Instance represents a connection to a Concourse CI server.
type Instance struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec InstanceSpec `json:"spec"`

	// +optional
	Status InstanceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// InstanceList contains a list of Instance.
type InstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Instance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Instance{}, &InstanceList{})
}

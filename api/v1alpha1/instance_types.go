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

// LocalObjectReference names a resource in the same namespace.
type LocalObjectReference struct {
	Name string `json:"name"`
}

// SecretKeySelector references a key in a Kubernetes Secret.
type SecretKeySelector struct {
	// Name of the Secret.
	Name string `json:"name"`
	// Key within the Secret.
	Key string `json:"key"`
}

// BasicAuth holds credentials for HTTP basic authentication.
type BasicAuth struct {
	// Username for basic authentication.
	Username string `json:"username"`
	// PasswordRef references the Secret key containing the password.
	PasswordRef SecretKeySelector `json:"passwordRef"`
}

// TokenAuth holds a bearer token reference.
type TokenAuth struct {
	// TokenRef references the Secret key containing the bearer token.
	TokenRef SecretKeySelector `json:"tokenRef"`
}

// TLSConfig configures TLS for the Concourse connection.
type TLSConfig struct {
	// CARef references a Secret key containing a PEM-encoded CA certificate.
	// +optional
	CARef *SecretKeySelector `json:"caRef,omitempty"`
	// InsecureSkipVerify disables TLS certificate verification.
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// InstanceSpec defines the desired state of Instance.
type InstanceSpec struct {
	// URL is the base URL of the Concourse ATC, e.g. https://ci.example.com.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://`
	URL string `json:"url"`

	// BasicAuth configures username/password authentication.
	// Mutually exclusive with TokenAuth.
	// +optional
	BasicAuth *BasicAuth `json:"basicAuth,omitempty"`

	// TokenAuth configures bearer token authentication.
	// Mutually exclusive with BasicAuth.
	// +optional
	TokenAuth *TokenAuth `json:"tokenAuth,omitempty"`

	// TLS configures TLS settings for the connection.
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`

	// Interval is how often the controller reconciles this instance.
	// +optional
	// +kubebuilder:default="5m"
	Interval metav1.Duration `json:"interval,omitempty"`
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

	// WorkerCount is the number of healthy workers in this instance.
	// +optional
	WorkerCount int `json:"workerCount,omitempty"`

	// ObservedGeneration is the last generation that was reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

const (
	// ConditionReady indicates the resource is reconciled and functional.
	ConditionReady = "Ready"
	// ConditionAuthenticated indicates authentication with Concourse succeeded.
	ConditionAuthenticated = "Authenticated"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`
// +kubebuilder:printcolumn:name="Workers",type=integer,JSONPath=`.status.workerCount`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
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

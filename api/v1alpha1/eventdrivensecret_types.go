/*
Copyright 2025.

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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EventDrivenSecretSpec defines the desired state of EventDrivenSecret.
type EventDrivenSecretSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	CloudProvider        string               `json:"cloudProvider"`        // Cloud provider (aws, gcp, azure)
	CloudProviderOptions CloudProviderOptions `json:"cloudProviderOptions"` // Provider-specific settings
	SecretPath           string               `json:"secretPath"`           // Path to the secret in the cloud provider
	TargetSecretName     string               `json:"targetSecretName"`     // The Kubernetes Secret name to create/update
}

// EventDrivenSecretStatus defines the observed state of EventDrivenSecret.
type EventDrivenSecretStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	LastAppliedSecretHash string `json:"lastAppliedSecretHash,omitempty"`

	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	LastSyncedTime *metav1.Time `json:"lastSyncedTime,omitempty"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	Message string `json:"message,omitempty"`

	// +optional
	Reason string `json:"reason,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EventDrivenSecret is the Schema for the eventdrivensecrets API.
type EventDrivenSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EventDrivenSecretSpec   `json:"spec,omitempty"`
	Status EventDrivenSecretStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EventDrivenSecretList contains a list of EventDrivenSecret.
type EventDrivenSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EventDrivenSecret `json:"items"`
}

type CloudProviderOptions struct {
	Region       string `json:"region,omitempty"`
	GCPProjectID string `json:"gcpProjectID,omitempty"`
	// Add more provider-specific fields as needed
}

func init() {
	SchemeBuilder.Register(&EventDrivenSecret{}, &EventDrivenSecretList{})
}

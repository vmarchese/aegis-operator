/*
Copyright 2024.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AWSProviderSpec defines the desired state of AWSProvider
type AWSProviderSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Name           string `json:"name,omitempty"`
	Region         string `json:"region,omitempty"`
	IdentityPoolID string `json:"identityPoolID,omitempty"`
	RoleARN        string `json:"roleARN,omitempty"`
}

// AWSProviderStatus defines the observed state of AWSProvider
type AWSProviderStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// AWSProvider is the Schema for the awsproviders API
type AWSProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AWSProviderSpec   `json:"spec,omitempty"`
	Status AWSProviderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AWSProviderList contains a list of AWSProvider
type AWSProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AWSProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AWSProvider{}, &AWSProviderList{})
}

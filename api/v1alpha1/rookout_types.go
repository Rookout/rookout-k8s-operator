package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// !!!!!!!!!!
// make sure to run "make deployment_yamls" after everytime you change this file
// !!!!!!!!!!

type ContainerMatcher struct {
	Matcher string      `json:"matcher,omitempty"`
	EnvVars []v1.EnvVar `json:"env_vars,omitempty"`
}

type DeploymentMatcher struct {
	Matcher string      `json:"matcher,omitempty"`
	EnvVars []v1.EnvVar `json:"env_vars,omitempty"`
}

type LabelsMatcher struct {
	Matcher map[string]string `json:"matcher,omitempty"`
	EnvVars []v1.EnvVar       `json:"env_vars,omitempty"`
}

type Matchers struct {
	ContainerMatcher  ContainerMatcher  `json:"container_matcher,omitempty"`
	DeploymentMatcher DeploymentMatcher `json:"deployment_matcher,omitempty"`
	LabelsMatcher     LabelsMatcher     `json:"labels_matcher,omitempty"`
}

// RookoutSpec defines the desired state of Rookout
type RookoutSpec struct {
	Matchers       Matchers    `json:"matchers,omitempty"`
	RookoutEnvVars []v1.EnvVar `json:"rookout_env_vars,omitempty"`
}

// RookoutStatus defines the observed state of Rookout
type RookoutStatus struct {
	// TODO: consider using this objet to represent our operator state
	// Instead of the internal struct
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Rookout is the Schema for the rookouts API
type Rookout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RookoutSpec   `json:"spec,omitempty"`
	Status RookoutStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RookoutList contains a list of Rookout
type RookoutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rookout `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Rookout{}, &RookoutList{})
}

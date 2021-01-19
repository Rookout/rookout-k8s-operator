package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// !!!!!!!!!!
// make sure to run "make manifests" after everytime you change this file
// !!!!!!!!!!

// RookoutSpec defines the desired state of Rookout
type RookoutSpec struct {
	Token           string `json:"rookout_token,omitempty"`
	ControllerHost  string `json:"rookout_controller_host,omitempty"`
	ControllerPort  string `json:"rookout_controller_port,omitempty"`
	JavaProcMatcher string `json:"java_proc_matcher,omitempty"`
	PodsMatcher     string `json:"pods_matcher,omitempty"`
}

// RookoutStatus defines the observed state of Rookout
type RookoutStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
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

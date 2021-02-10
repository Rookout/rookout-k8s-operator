package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

// !!!!!!!!!!
// make sure to run "make deployment_yamls" after everytime you change this file
// !!!!!!!!!!

type Matcher struct {
	Container  string            `json:"container,omitempty"`
	Deployment string            `json:"deployment,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	EnvVars    []v1.EnvVar       `json:"env_vars,omitempty"`
}

type InitContainer struct {
	Image                 string        `json:"image,omitempty"`
	ImagePullPolicy       v1.PullPolicy `json:"image_pull_policy,omitempty"`
	ContainerName         string        `json:"container_name,omitempty"`
	SharedVolumeMountPath string        `json:"shared_volume_mount_path,omitempty"`
	SharedVolumeName      string        `json:"shared_volume_name,omitempty"`
}

// RookoutSpec defines the desired state of Rookout
type RookoutSpec struct {
	Matchers      []Matcher     `json:"matchers,omitempty"`
	InitContainer InitContainer `json:"init_container,omitempty"`
	RequeueAfter  time.Duration `json:"requeue_after,omitempty"`
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

/*

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

// FarmSpec defines the desired state of Farm
type FarmSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	LabelKey     string `json:"labelKey"`
	LabelValue   string `json:"labelValue"`
	MinExecutors int32  `json:"minExecutors"`
	Replicas     *int32 `json:"replicas"`
}

// FarmStatus defines the observed state of Farm
type FarmStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	AllExecutors     int32 `json:"allExecutors"`
	RunningExecutors int32 `json:"runningExecutors"`
	Replicas         int32 `json:"replicas"`
	PendingExecutors int32 `json:"pendingExecutors"`
	ErrorExecutors   int32 `json:"errorExecutors"`
	Overquota        int32 `json:"overquota"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
// Farm is the Schema for the farms API
type Farm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FarmSpec   `json:"spec,omitempty"`
	Status FarmStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FarmList contains a list of Farm
type FarmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Farm `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Farm{}, &FarmList{})
}

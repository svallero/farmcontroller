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

// FarmManagerSpec defines the desired state of FarmManager
type FarmManagerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	LabelKey   string `json:"labelKey"`
	LabelValue string `json:"labelValue"`
}

// FarmManagerStatus defines the observed state of FarmManager
type FarmManagerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Overquota      map[string]int32 `json:"overquota"`
	Underquota     map[string]int32 `json:"underquota"`
	Pending        map[string]int32 `json:"pending"`
	ToBeScaledDown map[string]int32 `json:"tobescaleddown"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=farmmanagers,scope=Cluster
// FarmManager is the Schema for the farmmanagers API
type FarmManager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FarmManagerSpec   `json:"spec,omitempty"`
	Status FarmManagerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FarmManagerList contains a list of FarmManager
type FarmManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FarmManager `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FarmManager{}, &FarmManagerList{})
}

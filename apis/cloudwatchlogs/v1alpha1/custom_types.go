/*
Copyright 2026 The Crossplane Authors.

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

// CustomLogGroupParameters contains the hand-written fields of LogGroupParameters.
type CustomLogGroupParameters struct {
	// RetentionInDays is the number of days to keep the log events of the log
	// group. 0 keeps them forever. Unset leaves the retention as it is.
	// +kubebuilder:validation:Enum=0;1;3;5;7;14;30;60;90;120;150;180;365;400;545;731;1096;1827;2192;2557;2922;3288;3653
	// +optional
	RetentionInDays *int64 `json:"retentionInDays,omitempty"`
}

// CustomLogGroupObservation contains the hand-written fields of LogGroupObservation.
type CustomLogGroupObservation struct{}

// CustomResourcePolicyParameters contains the hand-written fields of ResourcePolicyParameters.
type CustomResourcePolicyParameters struct{}

// CustomResourcePolicyObservation contains the hand-written fields of ResourcePolicyObservation.
type CustomResourcePolicyObservation struct{}

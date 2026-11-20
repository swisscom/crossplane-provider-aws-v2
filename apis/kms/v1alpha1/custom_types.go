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

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CustomAliasParameters contains the hand-written fields of AliasParameters.
type CustomAliasParameters struct {
	// TargetKeyID is the ID or ARN of the KMS key the alias points to. Set
	// it, targetKeyIDRef or targetKeyIDSelector.
	// +optional
	// +crossplane:generate:reference:type=github.com/swisscom/crossplane-provider-aws-v2/apis/kms/v1alpha1.Key
	TargetKeyID *string `json:"targetKeyID,omitempty"`

	// TargetKeyIDRef is a reference to a Key used to set TargetKeyID.
	// +optional
	TargetKeyIDRef *xpv2.NamespacedReference `json:"targetKeyIDRef,omitempty"`

	// TargetKeyIDSelector selects a reference to a Key used to set
	// TargetKeyID.
	// +optional
	TargetKeyIDSelector *xpv2.NamespacedSelector `json:"targetKeyIDSelector,omitempty"`
}

// CustomAliasObservation contains the hand-written fields of AliasObservation.
type CustomAliasObservation struct{}

// CustomGrantParameters contains the hand-written fields of GrantParameters.
type CustomGrantParameters struct {
	// KeyID is the ID or ARN of the KMS key the grant is for. Set it, keyIDRef
	// or keyIDSelector. It can't be changed after creation.
	// +optional
	// +crossplane:generate:reference:type=github.com/swisscom/crossplane-provider-aws-v2/apis/kms/v1alpha1.Key
	KeyID *string `json:"keyID,omitempty"`

	// KeyIDRef is a reference to a Key used to set KeyID.
	// +optional
	KeyIDRef *xpv2.NamespacedReference `json:"keyIDRef,omitempty"`

	// KeyIDSelector selects a reference to a Key used to set KeyID.
	// +optional
	KeyIDSelector *xpv2.NamespacedSelector `json:"keyIDSelector,omitempty"`
}

// CustomGrantObservation contains the hand-written fields of GrantObservation.
type CustomGrantObservation struct{}

// CustomKeyParameters contains the hand-written fields of KeyParameters.
type CustomKeyParameters struct {
	// Enabled sets whether the key can be used in cryptographic operations.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`

	// EnableKeyRotation turns automatic rotation of the key material on or
	// off. Only symmetric encryption keys with key material from KMS can be
	// rotated automatically. Not managed if unset.
	// +optional
	EnableKeyRotation *bool `json:"enableKeyRotation,omitempty"`

	// RotationPeriodInDays is the number of days between automatic
	// rotations while rotation is on. AWS defaults to 365.
	// +optional
	// +kubebuilder:validation:Minimum=90
	// +kubebuilder:validation:Maximum=2560
	RotationPeriodInDays *int64 `json:"rotationPeriodInDays,omitempty"`

	// PendingWindowInDays is the waiting period, in days, between deleting
	// this resource and AWS deleting the key. AWS defaults to 30.
	// +optional
	// +kubebuilder:validation:Minimum=7
	// +kubebuilder:validation:Maximum=30
	PendingWindowInDays *int64 `json:"pendingWindowInDays,omitempty"`
}

// CustomKeyObservation contains the hand-written fields of KeyObservation.
type CustomKeyObservation struct {
	// KeyRotationEnabled reports whether automatic rotation is on. Observed
	// only if enableKeyRotation or rotationPeriodInDays is set.
	KeyRotationEnabled *bool `json:"keyRotationEnabled,omitempty"`

	// RotationPeriodInDays is the number of days between automatic
	// rotations. Observed only if enableKeyRotation or rotationPeriodInDays
	// is set.
	RotationPeriodInDays *int64 `json:"rotationPeriodInDays,omitempty"`

	// NextRotationDate is the date of the next automatic rotation. Observed
	// only if enableKeyRotation or rotationPeriodInDays is set.
	NextRotationDate *metav1.Time `json:"nextRotationDate,omitempty"`
}

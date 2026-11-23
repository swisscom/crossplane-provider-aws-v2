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

import xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

// CustomKeyParameters are custom parameters for Key.
type CustomKeyParameters struct {
	// Specifies whether the CMK is enabled.
	Enabled *bool `json:"enabled,omitempty"`

	// Specifies how many days the Key is retained when scheduled for deletion. Defaults to 30 days.
	PendingWindowInDays *int64 `json:"pendingWindowInDays,omitempty"`

	// Specifies if key rotation is enabled for the corresponding key
	EnableKeyRotation *bool `json:"enableKeyRotation,omitempty"`
}

// CustomKeyObservation includes the custom status fields of Key.
type CustomKeyObservation struct{}

// CustomAliasParameters are custom parameters for Alias.
type CustomAliasParameters struct {
	// The key ID or key ARN of the customer managed KMS key the alias is
	// associated with. The key must be in the same AWS account and Region.
	//
	// TargetKeyID or one of the referencers is required.
	//
	// +crossplane:generate:reference:type=Key
	TargetKeyID *string `json:"targetKeyId,omitempty"`

	// TargetKeyIDRef is a reference to a Key used to set TargetKeyID.
	// +optional
	TargetKeyIDRef *xpv2.NamespacedReference `json:"targetKeyIdRef,omitempty"`

	// TargetKeyIDSelector selects a reference to a Key used to set
	// TargetKeyID.
	// +optional
	TargetKeyIDSelector *xpv2.NamespacedSelector `json:"targetKeyIdSelector,omitempty"`
}

// CustomAliasObservation includes the custom status fields of Alias.
type CustomAliasObservation struct{}

// CustomGrantParameters are custom parameters for Grant.
type CustomGrantParameters struct {
	// Identifies the KMS key for the grant. The grant gives principals permission
	// to use this KMS key.
	//
	// Specify the key ID or key ARN of the KMS key. To specify a KMS key in a different
	// Amazon Web Services account, you must use the key ARN.
	//
	// For example:
	//
	//    * Key ID: 1234abcd-12ab-34cd-56ef-1234567890ab
	//
	//    * Key ARN: arn:aws:kms:us-east-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab
	//
	// To get the key ID and key ARN for a KMS key, use ListKeys or DescribeKey.
	//
	// KeyID or one of the referencers is a required parameter.
	//
	// +crossplane:generate:reference:type=github.com/swisscom/crossplane-provider-aws-v2/apis/kms/v1alpha1.Key
	KeyID *string `json:"keyId,omitempty"`

	// KeyIDRef is a reference to a KeyID.
	// +optional
	KeyIDRef *xpv2.NamespacedReference `json:"keyIdRef,omitempty"`

	// KeyIDSelector selects references to a KeyID.
	// +optional
	KeyIDSelector *xpv2.NamespacedSelector `json:"keyIdSelector,omitempty"`
}

// CustomKeyObservation includes the custom status fields of Grant.
type CustomGrantObservation struct{}

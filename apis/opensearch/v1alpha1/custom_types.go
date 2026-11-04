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

// CustomDomainParameters contains the hand-written fields of DomainParameters.
type CustomDomainParameters struct {
	// MasterUserPasswordSecretRef references the password of the master user
	// of fine-grained access control
	// (advancedSecurityOptions.masterUserOptions.masterUserName), in a secret
	// in the namespace of the Domain. It is sent on create, and on update when
	// it differs from the password in the connection secret
	// (writeConnectionSecretToRef); without a connection secret, changes are
	// not detected.
	//
	// Constraints: 8 to 128 characters, with at least one uppercase letter,
	// one lowercase letter, one number and one special character.
	// +optional
	MasterUserPasswordSecretRef *xpv2.LocalSecretKeySelector `json:"masterUserPasswordSecretRef,omitempty"`
}

// CustomDomainObservation contains the hand-written fields of DomainObservation.
type CustomDomainObservation struct{}

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

// CustomSecurityGroupParameters contains the hand-written fields of
// SecurityGroupParameters.
type CustomSecurityGroupParameters struct {
	// Tags of the security group.
	// +optional
	Tags []*Tag `json:"tags,omitempty"`

	// AdoptExistingByName adopts an existing security group with the same
	// groupName (in vpcID, if set) instead of creating one, while the
	// external name is not set. More than one match is an error, and the
	// default security group is never adopted. An adopted group is managed
	// from then on: deleting this resource deletes it unless the management
	// policies exclude Delete. Prefer setting the external name to the
	// group ID to import a known group.
	// +optional
	AdoptExistingByName *bool `json:"adoptExistingByName,omitempty"`
}

// CustomSecurityGroupObservation contains the hand-written fields of
// SecurityGroupObservation.
type CustomSecurityGroupObservation struct{}

// CustomSecurityGroupIngressRuleParameters contains the hand-written fields of
// SecurityGroupIngressRuleParameters. A rule has exactly one source: cidrIPv4, cidrIPv6,
// prefixListID or referencedSecurityGroupID.
type CustomSecurityGroupIngressRuleParameters struct {
	// SecurityGroupID is the ID of the security group the rule belongs to.
	// +immutable
	// +crossplane:generate:reference:type=github.com/swisscom/crossplane-provider-aws-v2/apis/ec2/v1alpha1.SecurityGroup
	SecurityGroupID *string `json:"securityGroupID,omitempty"`

	// SecurityGroupIDRef is a reference to a SecurityGroup used to set
	// SecurityGroupID.
	// +optional
	SecurityGroupIDRef *xpv2.NamespacedReference `json:"securityGroupIDRef,omitempty"`

	// SecurityGroupIDSelector selects a reference to a SecurityGroup used to
	// set SecurityGroupID.
	// +optional
	SecurityGroupIDSelector *xpv2.NamespacedSelector `json:"securityGroupIDSelector,omitempty"`

	// CIDRIPv4 is the IPv4 CIDR range of the source.
	// +optional
	CIDRIPv4 *string `json:"cidrIPv4,omitempty"`

	// CIDRIPv6 is the IPv6 CIDR range of the source.
	// +optional
	CIDRIPv6 *string `json:"cidrIPv6,omitempty"`

	// PrefixListID is the ID of the source prefix list.
	// +optional
	PrefixListID *string `json:"prefixListID,omitempty"`

	// ReferencedSecurityGroupID is the ID of the source security group.
	// +optional
	// +crossplane:generate:reference:type=github.com/swisscom/crossplane-provider-aws-v2/apis/ec2/v1alpha1.SecurityGroup
	ReferencedSecurityGroupID *string `json:"referencedSecurityGroupID,omitempty"`

	// ReferencedSecurityGroupIDRef is a reference to a SecurityGroup used to
	// set ReferencedSecurityGroupID.
	// +optional
	ReferencedSecurityGroupIDRef *xpv2.NamespacedReference `json:"referencedSecurityGroupIDRef,omitempty"`

	// ReferencedSecurityGroupIDSelector selects a reference to a SecurityGroup
	// used to set ReferencedSecurityGroupID.
	// +optional
	ReferencedSecurityGroupIDSelector *xpv2.NamespacedSelector `json:"referencedSecurityGroupIDSelector,omitempty"`

	// Description of the rule.
	// +optional
	Description *string `json:"description,omitempty"`

	// Tags of the rule.
	// +optional
	Tags []*Tag `json:"tags,omitempty"`
}

// CustomSecurityGroupIngressRuleObservation contains the hand-written fields of
// SecurityGroupIngressRuleObservation.
type CustomSecurityGroupIngressRuleObservation struct {
	// SecurityGroupRuleID is the ID of the rule (sgr-…), also its external
	// name.
	SecurityGroupRuleID *string `json:"securityGroupRuleID,omitempty"`

	// SecurityGroupRuleARN is the ARN of the rule.
	SecurityGroupRuleARN *string `json:"securityGroupRuleARN,omitempty"`

	// GroupOwnerID is the ID of the AWS account that owns the security group.
	GroupOwnerID *string `json:"groupOwnerID,omitempty"`

	// Tags of the rule in AWS.
	Tags []*Tag `json:"tags,omitempty"`
}

// CustomSecurityGroupEgressRuleParameters contains the hand-written fields of
// SecurityGroupEgressRuleParameters. A rule has exactly one destination: cidrIPv4, cidrIPv6,
// prefixListID or referencedSecurityGroupID.
type CustomSecurityGroupEgressRuleParameters struct {
	// SecurityGroupID is the ID of the security group the rule belongs to.
	// +immutable
	// +crossplane:generate:reference:type=github.com/swisscom/crossplane-provider-aws-v2/apis/ec2/v1alpha1.SecurityGroup
	SecurityGroupID *string `json:"securityGroupID,omitempty"`

	// SecurityGroupIDRef is a reference to a SecurityGroup used to set
	// SecurityGroupID.
	// +optional
	SecurityGroupIDRef *xpv2.NamespacedReference `json:"securityGroupIDRef,omitempty"`

	// SecurityGroupIDSelector selects a reference to a SecurityGroup used to
	// set SecurityGroupID.
	// +optional
	SecurityGroupIDSelector *xpv2.NamespacedSelector `json:"securityGroupIDSelector,omitempty"`

	// CIDRIPv4 is the IPv4 CIDR range of the destination.
	// +optional
	CIDRIPv4 *string `json:"cidrIPv4,omitempty"`

	// CIDRIPv6 is the IPv6 CIDR range of the destination.
	// +optional
	CIDRIPv6 *string `json:"cidrIPv6,omitempty"`

	// PrefixListID is the ID of the destination prefix list.
	// +optional
	PrefixListID *string `json:"prefixListID,omitempty"`

	// ReferencedSecurityGroupID is the ID of the destination security group.
	// +optional
	// +crossplane:generate:reference:type=github.com/swisscom/crossplane-provider-aws-v2/apis/ec2/v1alpha1.SecurityGroup
	ReferencedSecurityGroupID *string `json:"referencedSecurityGroupID,omitempty"`

	// ReferencedSecurityGroupIDRef is a reference to a SecurityGroup used to
	// set ReferencedSecurityGroupID.
	// +optional
	ReferencedSecurityGroupIDRef *xpv2.NamespacedReference `json:"referencedSecurityGroupIDRef,omitempty"`

	// ReferencedSecurityGroupIDSelector selects a reference to a SecurityGroup
	// used to set ReferencedSecurityGroupID.
	// +optional
	ReferencedSecurityGroupIDSelector *xpv2.NamespacedSelector `json:"referencedSecurityGroupIDSelector,omitempty"`

	// Description of the rule.
	// +optional
	Description *string `json:"description,omitempty"`

	// Tags of the rule.
	// +optional
	Tags []*Tag `json:"tags,omitempty"`
}

// CustomSecurityGroupEgressRuleObservation contains the hand-written fields of
// SecurityGroupEgressRuleObservation.
type CustomSecurityGroupEgressRuleObservation struct {
	// SecurityGroupRuleID is the ID of the rule (sgr-…), also its external
	// name.
	SecurityGroupRuleID *string `json:"securityGroupRuleID,omitempty"`

	// SecurityGroupRuleARN is the ARN of the rule.
	SecurityGroupRuleARN *string `json:"securityGroupRuleARN,omitempty"`

	// GroupOwnerID is the ID of the AWS account that owns the security group.
	GroupOwnerID *string `json:"groupOwnerID,omitempty"`

	// Tags of the rule in AWS.
	Tags []*Tag `json:"tags,omitempty"`
}

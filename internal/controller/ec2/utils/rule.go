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

package utils

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"

	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

const (
	errNoGroup       = "securityGroupID is required (set it, securityGroupIDRef or securityGroupIDSelector)"
	errNoProtocol    = "ipProtocol is required"
	errSources       = "exactly one of cidrIPv4, cidrIPv6, prefixListID and referencedSecurityGroupID must be set"
	errFmtImmutable  = "%s cannot be changed: it is %q in AWS but %q in the spec; delete and recreate the rule"
	errFmtWrongKind  = "rule %s is an %s rule, but this kind manages %s rules"
	errFmtSourceType = "the source type cannot be changed: the rule has %s in AWS but %s in the spec; delete and recreate the rule"
	errFindRule      = "cannot look up the rules of the security group"
	errFmtAmbiguous  = "adoptExisting: %d %s rules of %s match; set the crossplane.io/external-name annotation"
	ruleDirIngress   = "ingress"
	ruleDirEgress    = "egress"
	fieldGroupID     = "securityGroupID"
	allPorts         = -1
)

// Rule is a single security group rule, as specified by the
// SecurityGroupIngressRule and SecurityGroupEgressRule kinds.
type Rule struct {
	GroupID                   *string
	IPProtocol                *string
	FromPort                  *int64
	ToPort                    *int64
	CIDRIPv4                  *string
	CIDRIPv6                  *string
	PrefixListID              *string
	ReferencedSecurityGroupID *string
	Description               *string
}

// Validate checks what the API server can't: AWS would otherwise accept
// some invalid combinations by creating a different rule than specified.
func (r Rule) Validate() error {
	if pointer.StringValue(r.GroupID) == "" {
		return errors.New(errNoGroup)
	}
	if pointer.StringValue(r.IPProtocol) == "" {
		return errors.New(errNoProtocol)
	}
	n := 0
	for _, s := range []*string{r.CIDRIPv4, r.CIDRIPv6, r.PrefixListID, r.ReferencedSecurityGroupID} {
		if pointer.StringValue(s) != "" {
			n++
		}
	}
	if n != 1 {
		return errors.New(errSources)
	}
	return nil
}

// IPPermission returns the rule in the form Authorize* calls take.
func (r Rule) IPPermission() svcsdktypes.IpPermission {
	p := svcsdktypes.IpPermission{
		IpProtocol: r.IPProtocol,
		FromPort:   pointer.Int32(r.FromPort),
		ToPort:     pointer.Int32(r.ToPort),
	}
	switch {
	case r.CIDRIPv4 != nil:
		p.IpRanges = []svcsdktypes.IpRange{{CidrIp: r.CIDRIPv4, Description: r.Description}}
	case r.CIDRIPv6 != nil:
		p.Ipv6Ranges = []svcsdktypes.Ipv6Range{{CidrIpv6: r.CIDRIPv6, Description: r.Description}}
	case r.PrefixListID != nil:
		p.PrefixListIds = []svcsdktypes.PrefixListId{{PrefixListId: r.PrefixListID, Description: r.Description}}
	case r.ReferencedSecurityGroupID != nil:
		p.UserIdGroupPairs = []svcsdktypes.UserIdGroupPair{{GroupId: r.ReferencedSecurityGroupID, Description: r.Description}}
	}
	return p
}

// RuleUpdate returns the ModifySecurityGroupRules entry that makes the rule
// with the given ID equal r. AWS can't change the group or the source type in
// place; Diff reports those as errors.
func (r Rule) RuleUpdate(id string) svcsdktypes.SecurityGroupRuleUpdate {
	return svcsdktypes.SecurityGroupRuleUpdate{
		SecurityGroupRuleId: aws.String(id),
		SecurityGroupRule: &svcsdktypes.SecurityGroupRuleRequest{
			IpProtocol:        r.IPProtocol,
			FromPort:          pointer.Int32(r.FromPort),
			ToPort:            pointer.Int32(r.ToPort),
			CidrIpv4:          r.CIDRIPv4,
			CidrIpv6:          r.CIDRIPv6,
			PrefixListId:      r.PrefixListID,
			ReferencedGroupId: r.ReferencedSecurityGroupID,
			Description:       r.Description,
		},
	}
}

// Diff returns the JSON names of the spec fields whose desired value differs
// from the observed rule. It returns an error if the observed rule can't be
// changed into the desired one: it belongs to another group, has the other
// direction (egress is true for SecurityGroupEgressRule), or has another
// source type (e.g. a prefix list instead of an IPv4 CIDR).
func (r Rule) Diff(observed svcsdktypes.SecurityGroupRule, egress bool) ([]string, error) {
	if pointer.BoolValue(observed.IsEgress) != egress {
		return nil, errors.Errorf(errFmtWrongKind, pointer.StringValue(observed.SecurityGroupRuleId), direction(!egress), direction(egress))
	}
	if g := pointer.StringValue(observed.GroupId); g != pointer.StringValue(r.GroupID) {
		return nil, errors.Errorf(errFmtImmutable, fieldGroupID, g, pointer.StringValue(r.GroupID))
	}

	var referenced *string
	if observed.ReferencedGroupInfo != nil {
		referenced = observed.ReferencedGroupInfo.GroupId
	}
	want := sourceType(r.CIDRIPv4, r.CIDRIPv6, r.PrefixListID, r.ReferencedSecurityGroupID)
	if got := sourceType(observed.CidrIpv4, observed.CidrIpv6, observed.PrefixListId, referenced); want != "" && got != "" && want != got {
		return nil, errors.Errorf(errFmtSourceType, got, want)
	}
	var diff []string
	add := func(name string, equal bool) {
		if !equal {
			diff = append(diff, name)
		}
	}
	add("ipProtocol", NormalizeProtocol(pointer.StringValue(r.IPProtocol)) == NormalizeProtocol(pointer.StringValue(observed.IpProtocol)))
	add("fromPort", port(r.FromPort) == port(pointer.Int64(observed.FromPort)))
	add("toPort", port(r.ToPort) == port(pointer.Int64(observed.ToPort)))
	add("cidrIPv4", pointer.StringValue(r.CIDRIPv4) == pointer.StringValue(observed.CidrIpv4))
	add("cidrIPv6", pointer.StringValue(r.CIDRIPv6) == pointer.StringValue(observed.CidrIpv6))
	add("prefixListID", pointer.StringValue(r.PrefixListID) == pointer.StringValue(observed.PrefixListId))
	add("referencedSecurityGroupID", pointer.StringValue(r.ReferencedSecurityGroupID) == pointer.StringValue(referenced))
	add("description", pointer.StringValue(r.Description) == pointer.StringValue(observed.Description))
	return diff, nil
}

// Matches reports whether observed is the rule r. Within a group, AWS allows
// only one rule with the same direction, protocol, ports and source; the
// description and tags can differ.
func (r Rule) Matches(observed svcsdktypes.SecurityGroupRule, egress bool) bool {
	diff, err := r.Diff(observed, egress)
	return err == nil && len(compare.Without(diff, "description")) == 0
}

// RuleClient is the part of the EC2 API used to look up rules.
type RuleClient interface {
	DescribeSecurityGroupRules(context.Context, *svcsdk.DescribeSecurityGroupRulesInput, ...func(*svcsdk.Options)) (*svcsdk.DescribeSecurityGroupRulesOutput, error)
}

// FindRule returns the rule of r's group that Matches r, or nil if there is
// none. r must be valid.
func FindRule(ctx context.Context, c RuleClient, r Rule, egress bool) (*svcsdktypes.SecurityGroupRule, error) {
	in := &svcsdk.DescribeSecurityGroupRulesInput{
		Filters: []svcsdktypes.Filter{{Name: aws.String("group-id"), Values: []string{pointer.StringValue(r.GroupID)}}},
	}
	var found []svcsdktypes.SecurityGroupRule
	for {
		resp, err := c.DescribeSecurityGroupRules(ctx, in)
		if err != nil {
			return nil, awsclient.Wrap(err, errFindRule)
		}
		for _, o := range resp.SecurityGroupRules {
			if r.Matches(o, egress) {
				found = append(found, o)
			}
		}
		if pointer.StringValue(resp.NextToken) == "" {
			break
		}
		in.NextToken = resp.NextToken
	}
	switch len(found) {
	case 0:
		return nil, nil
	case 1:
		return &found[0], nil
	default:
		return nil, errors.Errorf(errFmtAmbiguous, len(found), direction(egress), pointer.StringValue(r.GroupID))
	}
}

// NormalizeProtocol returns the form AWS reports a protocol in: lower case,
// with the numbers of tcp, udp, icmp and icmpv6 replaced by their names.
func NormalizeProtocol(p string) string {
	p = strings.ToLower(p)
	switch p {
	case "6":
		return "tcp"
	case "17":
		return "udp"
	case "1":
		return "icmp"
	case "58":
		return "icmpv6"
	}
	return p
}

// port returns p, treating unset as -1 (all), which AWS reports for rules
// without ports.
func port(p *int64) int64 {
	if p == nil {
		return allPorts
	}
	return *p
}

// sourceType returns the JSON name of the first set source, or "" if none is.
func sourceType(cidrIPv4, cidrIPv6, prefixListID, referencedGroupID *string) string {
	for _, s := range []struct {
		name  string
		value *string
	}{
		{"cidrIPv4", cidrIPv4},
		{"cidrIPv6", cidrIPv6},
		{"prefixListID", prefixListID},
		{"referencedSecurityGroupID", referencedGroupID},
	} {
		if pointer.StringValue(s.value) != "" {
			return s.name
		}
	}
	return ""
}

func direction(egress bool) string {
	if egress {
		return ruleDirEgress
	}
	return ruleDirIngress
}

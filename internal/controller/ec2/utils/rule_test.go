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
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/ec2/v1alpha1"
)

func TestRuleValidate(t *testing.T) {
	cases := map[string]struct {
		rule    Rule
		wantErr bool
	}{
		"Valid":        {rule: Rule{GroupID: aws.String("sg-1"), IPProtocol: aws.String("tcp"), CIDRIPv4: aws.String("10.0.0.0/8")}},
		"NoGroup":      {rule: Rule{IPProtocol: aws.String("tcp"), CIDRIPv4: aws.String("10.0.0.0/8")}, wantErr: true},
		"NoProtocol":   {rule: Rule{GroupID: aws.String("sg-1"), CIDRIPv4: aws.String("10.0.0.0/8")}, wantErr: true},
		"NoSource":     {rule: Rule{GroupID: aws.String("sg-1"), IPProtocol: aws.String("tcp")}, wantErr: true},
		"TwoSources":   {rule: Rule{GroupID: aws.String("sg-1"), IPProtocol: aws.String("tcp"), CIDRIPv4: aws.String("10.0.0.0/8"), CIDRIPv6: aws.String("::/0")}, wantErr: true},
		"EmptySource":  {rule: Rule{GroupID: aws.String("sg-1"), IPProtocol: aws.String("-1"), PrefixListID: aws.String("")}, wantErr: true},
		"GroupSource":  {rule: Rule{GroupID: aws.String("sg-1"), IPProtocol: aws.String("-1"), ReferencedSecurityGroupID: aws.String("sg-2")}},
		"PrefixSource": {rule: Rule{GroupID: aws.String("sg-1"), IPProtocol: aws.String("-1"), PrefixListID: aws.String("pl-1")}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := tc.rule.Validate(); (err != nil) != tc.wantErr {
				t.Errorf("Validate() = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestRuleIPPermission(t *testing.T) {
	desc := aws.String("d")
	base := Rule{IPProtocol: aws.String("tcp"), FromPort: aws.Int64(443), ToPort: aws.Int64(443), Description: desc}
	with := func(f func(*Rule)) Rule { r := base; f(&r); return r }
	perm := func(f func(*svcsdktypes.IpPermission)) svcsdktypes.IpPermission {
		p := svcsdktypes.IpPermission{IpProtocol: aws.String("tcp"), FromPort: aws.Int32(443), ToPort: aws.Int32(443)}
		f(&p)
		return p
	}
	cases := map[string]struct {
		rule Rule
		want svcsdktypes.IpPermission
	}{
		"IPv4": {
			rule: with(func(r *Rule) { r.CIDRIPv4 = aws.String("10.0.0.0/8") }),
			want: perm(func(p *svcsdktypes.IpPermission) {
				p.IpRanges = []svcsdktypes.IpRange{{CidrIp: aws.String("10.0.0.0/8"), Description: desc}}
			}),
		},
		"IPv6": {
			rule: with(func(r *Rule) { r.CIDRIPv6 = aws.String("::/0") }),
			want: perm(func(p *svcsdktypes.IpPermission) {
				p.Ipv6Ranges = []svcsdktypes.Ipv6Range{{CidrIpv6: aws.String("::/0"), Description: desc}}
			}),
		},
		"PrefixList": {
			rule: with(func(r *Rule) { r.PrefixListID = aws.String("pl-1") }),
			want: perm(func(p *svcsdktypes.IpPermission) {
				p.PrefixListIds = []svcsdktypes.PrefixListId{{PrefixListId: aws.String("pl-1"), Description: desc}}
			}),
		},
		"Group": {
			rule: with(func(r *Rule) { r.ReferencedSecurityGroupID = aws.String("sg-2") }),
			want: perm(func(p *svcsdktypes.IpPermission) {
				p.UserIdGroupPairs = []svcsdktypes.UserIdGroupPair{{GroupId: aws.String("sg-2"), Description: desc}}
			}),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, tc.rule.IPPermission(), cmpopts.IgnoreUnexported(svcsdktypes.IpPermission{}, svcsdktypes.IpRange{}, svcsdktypes.Ipv6Range{}, svcsdktypes.PrefixListId{}, svcsdktypes.UserIdGroupPair{})); diff != "" {
				t.Errorf("IPPermission() -want +got:\n%s", diff)
			}
		})
	}
}

func TestRuleDiff(t *testing.T) {
	desired := Rule{GroupID: aws.String("sg-1"), IPProtocol: aws.String("TCP"), FromPort: aws.Int64(22), ToPort: aws.Int64(22), CIDRIPv4: aws.String("10.0.0.0/8")}
	observed := svcsdktypes.SecurityGroupRule{
		SecurityGroupRuleId: aws.String("sgr-1"), GroupId: aws.String("sg-1"), IsEgress: aws.Bool(false),
		IpProtocol: aws.String("tcp"), FromPort: aws.Int32(22), ToPort: aws.Int32(22), CidrIpv4: aws.String("10.0.0.0/8"),
	}
	obs := func(f func(*svcsdktypes.SecurityGroupRule)) svcsdktypes.SecurityGroupRule {
		o := observed
		f(&o)
		return o
	}
	cases := map[string]struct {
		desired  Rule
		observed svcsdktypes.SecurityGroupRule
		egress   bool
		want     []string
		wantErr  bool
	}{
		"UpToDate": {desired: desired, observed: observed},
		"ProtocolNumber": {
			desired:  func() Rule { r := desired; r.IPProtocol = aws.String("6"); return r }(),
			observed: observed,
		},
		"AllProtocolsNoPorts": {
			desired:  Rule{GroupID: aws.String("sg-1"), IPProtocol: aws.String("-1"), ReferencedSecurityGroupID: aws.String("sg-2")},
			observed: svcsdktypes.SecurityGroupRule{GroupId: aws.String("sg-1"), IsEgress: aws.Bool(false), IpProtocol: aws.String("-1"), FromPort: aws.Int32(-1), ToPort: aws.Int32(-1), ReferencedGroupInfo: &svcsdktypes.ReferencedSecurityGroup{GroupId: aws.String("sg-2")}},
		},
		"Changed": {
			desired: desired,
			observed: obs(func(o *svcsdktypes.SecurityGroupRule) {
				o.IpProtocol = aws.String("udp")
				o.FromPort = aws.Int32(21)
				o.ToPort = aws.Int32(23)
				o.CidrIpv4 = aws.String("10.1.0.0/16")
				o.Description = aws.String("x")
			}),
			want: []string{"ipProtocol", "fromPort", "toPort", "cidrIPv4", "description"},
		},
		"OtherPrefixList": {
			desired: Rule{GroupID: aws.String("sg-1"), IPProtocol: aws.String("tcp"), FromPort: aws.Int64(443), ToPort: aws.Int64(443), PrefixListID: aws.String("pl-2")},
			observed: obs(func(o *svcsdktypes.SecurityGroupRule) {
				o.FromPort, o.ToPort, o.CidrIpv4, o.PrefixListId = aws.Int32(443), aws.Int32(443), nil, aws.String("pl-1")
			}),
			want: []string{"prefixListID"},
		},
		"OtherReferencedGroup": {
			desired:  Rule{GroupID: aws.String("sg-1"), IPProtocol: aws.String("-1"), ReferencedSecurityGroupID: aws.String("sg-3")},
			observed: svcsdktypes.SecurityGroupRule{GroupId: aws.String("sg-1"), IsEgress: aws.Bool(false), IpProtocol: aws.String("-1"), FromPort: aws.Int32(-1), ToPort: aws.Int32(-1), ReferencedGroupInfo: &svcsdktypes.ReferencedSecurityGroup{GroupId: aws.String("sg-2")}},
			want:     []string{"referencedSecurityGroupID"},
		},
		// AWS: "You may not specify CidrIpv6 for an existing IPv4 CIDR rule."
		"SourceTypeIPv4ToIPv6": {
			desired:  func() Rule { r := desired; r.CIDRIPv4, r.CIDRIPv6 = nil, aws.String("::/0"); return r }(),
			observed: observed,
			wantErr:  true,
		},
		"SourceTypeIPv4ToPrefixList": {
			desired:  func() Rule { r := desired; r.CIDRIPv4, r.PrefixListID = nil, aws.String("pl-1"); return r }(),
			observed: observed,
			wantErr:  true,
		},
		"SourceTypePrefixListToIPv4": {
			desired:  desired,
			observed: obs(func(o *svcsdktypes.SecurityGroupRule) { o.CidrIpv4, o.PrefixListId = nil, aws.String("pl-1") }),
			wantErr:  true,
		},
		"SourceTypeIPv4ToReferencedGroup": {
			desired:  func() Rule { r := desired; r.CIDRIPv4, r.ReferencedSecurityGroupID = nil, aws.String("sg-2"); return r }(),
			observed: observed,
			wantErr:  true,
		},
		"NoDesiredSourceIsDiffNotError": {
			desired:  func() Rule { r := desired; r.CIDRIPv4 = nil; return r }(),
			observed: observed,
			want:     []string{"cidrIPv4"},
		},
		"OtherGroup": {
			desired:  desired,
			observed: obs(func(o *svcsdktypes.SecurityGroupRule) { o.GroupId = aws.String("sg-9") }),
			wantErr:  true,
		},
		"OtherDirection": {desired: desired, observed: observed, egress: true, wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := tc.desired.Diff(tc.observed, tc.egress)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Diff() error = %v, wantErr %v", err, tc.wantErr)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Diff() -want +got:\n%s", diff)
			}
		})
	}
}

func TestDiffTags(t *testing.T) {
	current := []*svcapitypes.Tag{
		{Key: aws.String("keep"), Value: aws.String("v")},
		{Key: aws.String("change"), Value: aws.String("old")},
		{Key: aws.String("drop"), Value: aws.String("v")},
		{Key: aws.String("aws:reserved"), Value: aws.String("v")},
		{Key: aws.String("foreign"), Value: aws.String("v")},
	}
	managed := []string{"aws:reserved", "change", "drop", "gone", "keep"}
	add, remove := DiffTags(map[string]string{"keep": "v", "change": "new", "new": "v"}, current, managed)
	wantAdd := []svcsdktypes.Tag{{Key: aws.String("change"), Value: aws.String("new")}, {Key: aws.String("new"), Value: aws.String("v")}}
	wantRemove := []svcsdktypes.Tag{{Key: aws.String("drop")}}
	opt := cmpopts.IgnoreUnexported(svcsdktypes.Tag{})
	if diff := cmp.Diff(wantAdd, add, opt); diff != "" {
		t.Errorf("add -want +got:\n%s", diff)
	}
	if diff := cmp.Diff(wantRemove, remove, opt); diff != "" {
		t.Errorf("remove -want +got:\n%s", diff)
	}
}

// sdkRule is an observed ingress rule: tcp/443 from 10.0.0.0/8 in sg-1.
func sdkRule(id string, f ...func(*svcsdktypes.SecurityGroupRule)) svcsdktypes.SecurityGroupRule {
	o := svcsdktypes.SecurityGroupRule{
		SecurityGroupRuleId: aws.String(id), GroupId: aws.String("sg-1"), IsEgress: aws.Bool(false),
		IpProtocol: aws.String("tcp"), FromPort: aws.Int32(443), ToPort: aws.Int32(443), CidrIpv4: aws.String("10.0.0.0/8"),
	}
	for _, fn := range f {
		fn(&o)
	}
	return o
}

func TestRuleMatches(t *testing.T) {
	r := Rule{GroupID: aws.String("sg-1"), IPProtocol: aws.String("tcp"), FromPort: aws.Int64(443), ToPort: aws.Int64(443), CIDRIPv4: aws.String("10.0.0.0/8"), Description: aws.String("https")}
	cases := map[string]struct {
		rule     Rule
		observed svcsdktypes.SecurityGroupRule
		egress   bool
		want     bool
	}{
		"Same":               {rule: r, observed: sdkRule("sgr-1", func(o *svcsdktypes.SecurityGroupRule) { o.Description = aws.String("https") }), want: true},
		"OtherDescription":   {rule: r, observed: sdkRule("sgr-1", func(o *svcsdktypes.SecurityGroupRule) { o.Description = aws.String("old") }), want: true},
		"NoDescriptionInAWS": {rule: r, observed: sdkRule("sgr-1"), want: true},
		"ProtocolNumber":     {rule: func() Rule { x := r; x.IPProtocol = aws.String("6"); return x }(), observed: sdkRule("sgr-1"), want: true},
		"OtherPort":          {rule: r, observed: sdkRule("sgr-1", func(o *svcsdktypes.SecurityGroupRule) { o.ToPort = aws.Int32(444) }), want: false},
		"OtherProtocol":      {rule: r, observed: sdkRule("sgr-1", func(o *svcsdktypes.SecurityGroupRule) { o.IpProtocol = aws.String("udp") }), want: false},
		"OtherCIDR":          {rule: r, observed: sdkRule("sgr-1", func(o *svcsdktypes.SecurityGroupRule) { o.CidrIpv4 = aws.String("10.0.0.0/16") }), want: false},
		"OtherSourceType":    {rule: r, observed: sdkRule("sgr-1", func(o *svcsdktypes.SecurityGroupRule) { o.CidrIpv4, o.PrefixListId = nil, aws.String("pl-1") }), want: false},
		"OtherDirection":     {rule: r, observed: sdkRule("sgr-1"), egress: true, want: false},
		"OtherGroup":         {rule: r, observed: sdkRule("sgr-1", func(o *svcsdktypes.SecurityGroupRule) { o.GroupId = aws.String("sg-2") }), want: false},
		"AllTrafficUnsetPorts": {
			rule: Rule{GroupID: aws.String("sg-1"), IPProtocol: aws.String("-1"), PrefixListID: aws.String("pl-1")},
			observed: sdkRule("sgr-1", func(o *svcsdktypes.SecurityGroupRule) {
				o.IpProtocol, o.FromPort, o.ToPort, o.CidrIpv4, o.PrefixListId = aws.String("-1"), aws.Int32(-1), aws.Int32(-1), nil, aws.String("pl-1")
			}),
			want: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tc.rule.Matches(tc.observed, tc.egress); got != tc.want {
				t.Errorf("Matches() = %v, want %v", got, tc.want)
			}
		})
	}
}

// ruleClient is a fake RuleClient returning one page per call.
type ruleClient struct {
	pages  [][]svcsdktypes.SecurityGroupRule
	err    error
	inputs []*svcsdk.DescribeSecurityGroupRulesInput
}

func (c *ruleClient) DescribeSecurityGroupRules(_ context.Context, in *svcsdk.DescribeSecurityGroupRulesInput, _ ...func(*svcsdk.Options)) (*svcsdk.DescribeSecurityGroupRulesOutput, error) {
	cp := *in
	c.inputs = append(c.inputs, &cp)
	if c.err != nil {
		return nil, c.err
	}
	i := len(c.inputs) - 1
	out := &svcsdk.DescribeSecurityGroupRulesOutput{SecurityGroupRules: c.pages[i]}
	if i+1 < len(c.pages) {
		out.NextToken = aws.String(string(rune('a' + i)))
	}
	return out, nil
}

func TestFindRule(t *testing.T) {
	r := Rule{GroupID: aws.String("sg-1"), IPProtocol: aws.String("tcp"), FromPort: aws.Int64(443), ToPort: aws.Int64(443), CIDRIPv4: aws.String("10.0.0.0/8")}
	egressTwin := sdkRule("sgr-e", func(o *svcsdktypes.SecurityGroupRule) { o.IsEgress = aws.Bool(true) })
	otherPort := sdkRule("sgr-p", func(o *svcsdktypes.SecurityGroupRule) { o.FromPort, o.ToPort = aws.Int32(80), aws.Int32(80) })
	cases := map[string]struct {
		client  *ruleClient
		egress  bool
		wantID  string
		wantErr string
		calls   int
	}{
		"Match": {
			client: &ruleClient{pages: [][]svcsdktypes.SecurityGroupRule{{otherPort, egressTwin, sdkRule("sgr-1")}}},
			wantID: "sgr-1", calls: 1,
		},
		"MatchOnLaterPage": {
			client: &ruleClient{pages: [][]svcsdktypes.SecurityGroupRule{{otherPort}, {egressTwin}, {sdkRule("sgr-1")}}},
			wantID: "sgr-1", calls: 3,
		},
		"OnlyOtherDirection": {
			client: &ruleClient{pages: [][]svcsdktypes.SecurityGroupRule{{egressTwin, otherPort}}},
			calls:  1,
		},
		"EgressMatch": {
			client: &ruleClient{pages: [][]svcsdktypes.SecurityGroupRule{{sdkRule("sgr-1"), egressTwin}}},
			egress: true, wantID: "sgr-e", calls: 1,
		},
		"NoRules": {
			client: &ruleClient{pages: [][]svcsdktypes.SecurityGroupRule{nil}},
			calls:  1,
		},
		"Ambiguous": {
			client:  &ruleClient{pages: [][]svcsdktypes.SecurityGroupRule{{sdkRule("sgr-1")}, {sdkRule("sgr-2")}}},
			wantErr: "adoptExisting: 2 ingress rules of sg-1 match", calls: 2,
		},
		"Error": {
			client:  &ruleClient{err: &smithy.GenericAPIError{Code: "UnauthorizedOperation"}},
			wantErr: errFindRule, calls: 1,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := FindRule(context.Background(), tc.client, r, tc.egress)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("FindRule() error = %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("FindRule() error = %v, want containing %q", err, tc.wantErr)
			}
			gotID := ""
			if got != nil {
				gotID = aws.ToString(got.SecurityGroupRuleId)
			}
			if gotID != tc.wantID {
				t.Errorf("FindRule() = %q, want %q", gotID, tc.wantID)
			}
			if len(tc.client.inputs) != tc.calls {
				t.Fatalf("calls = %d, want %d", len(tc.client.inputs), tc.calls)
			}
			wantFilters := []svcsdktypes.Filter{{Name: aws.String("group-id"), Values: []string{"sg-1"}}}
			for i, in := range tc.client.inputs {
				if diff := cmp.Diff(wantFilters, in.Filters, cmpopts.IgnoreUnexported(svcsdktypes.Filter{})); diff != "" {
					t.Errorf("call %d filters -want +got:\n%s", i, diff)
				}
				wantToken := ""
				if i > 0 {
					wantToken = string(rune('a' + i - 1))
				}
				if aws.ToString(in.NextToken) != wantToken {
					t.Errorf("call %d NextToken = %q, want %q", i, aws.ToString(in.NextToken), wantToken)
				}
			}
		})
	}
}

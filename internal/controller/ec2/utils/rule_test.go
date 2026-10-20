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
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
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
	}
	add, remove := DiffTags(map[string]string{"keep": "v", "change": "new", "new": "v"}, current)
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

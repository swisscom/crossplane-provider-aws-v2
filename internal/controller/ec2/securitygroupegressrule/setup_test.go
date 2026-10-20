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

package securitygroupegressrule

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/ec2/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/ec2/fake"
)

const (
	testGroup = "sg-1"
	testRule  = "sgr-1"
)

func apiErr(code string) error { return &smithy.GenericAPIError{Code: code} }

func rule(f ...func(*svcapitypes.SecurityGroupEgressRule)) *svcapitypes.SecurityGroupEgressRule {
	cr := &svcapitypes.SecurityGroupEgressRule{ObjectMeta: metav1.ObjectMeta{Name: "https", Namespace: "ns"}}
	cr.SetGroupVersionKind(svcapitypes.SecurityGroupEgressRuleGroupVersionKind)
	p := &cr.Spec.ForProvider
	p.SecurityGroupID = aws.String(testGroup)
	p.IPProtocol = aws.String("tcp")
	p.FromPort = aws.Int64(443)
	p.ToPort = aws.Int64(443)
	p.CIDRIPv4 = aws.String("10.0.0.0/8")
	for _, fn := range f {
		fn(cr)
	}
	return cr
}

func withExternalName(cr *svcapitypes.SecurityGroupEgressRule) { meta.SetExternalName(cr, testRule) }

func observedRule(f ...func(*svcsdktypes.SecurityGroupRule)) func(context.Context, *svcsdk.DescribeSecurityGroupRulesInput) (*svcsdk.DescribeSecurityGroupRulesOutput, error) {
	r := svcsdktypes.SecurityGroupRule{
		SecurityGroupRuleId: aws.String(testRule), SecurityGroupRuleArn: aws.String("arn:sgr"), GroupId: aws.String(testGroup), GroupOwnerId: aws.String("123"),
		IsEgress: aws.Bool(true), IpProtocol: aws.String("tcp"), FromPort: aws.Int32(443), ToPort: aws.Int32(443), CidrIpv4: aws.String("10.0.0.0/8"),
		Tags: []svcsdktypes.Tag{
			{Key: aws.String("crossplane-kind"), Value: aws.String("securitygroupegressrule.ec2.aws2.crossplane.io")},
			{Key: aws.String("crossplane-name"), Value: aws.String("https")},
			{Key: aws.String("crossplane-namespace"), Value: aws.String("ns")},
		},
	}
	for _, fn := range f {
		fn(&r)
	}
	return func(_ context.Context, in *svcsdk.DescribeSecurityGroupRulesInput) (*svcsdk.DescribeSecurityGroupRulesOutput, error) {
		if len(in.SecurityGroupRuleIds) != 1 || in.SecurityGroupRuleIds[0] != testRule {
			return nil, apiErr("InvalidSecurityGroupRuleId.NotFound")
		}
		return &svcsdk.DescribeSecurityGroupRulesOutput{SecurityGroupRules: []svcsdktypes.SecurityGroupRule{r}}, nil
	}
}

func newTestExternal(m *fake.MockEC2Client) *external {
	return newExternal(nil, m, []option{setupExternal})
}

func TestObserve(t *testing.T) {
	cases := map[string]struct {
		cr                      *svcapitypes.SecurityGroupEgressRule
		describe                func(context.Context, *svcsdk.DescribeSecurityGroupRulesInput) (*svcsdk.DescribeSecurityGroupRulesOutput, error)
		exists, upToDate, isErr bool
	}{
		"NoExternalName": {cr: rule()},
		"NotFound": {
			cr: rule(withExternalName),
			describe: func(context.Context, *svcsdk.DescribeSecurityGroupRulesInput) (*svcsdk.DescribeSecurityGroupRulesOutput, error) {
				return nil, apiErr("InvalidSecurityGroupRuleId.NotFound")
			},
		},
		"UpToDate":     {cr: rule(withExternalName), describe: observedRule(), exists: true, upToDate: true},
		"PortsChanged": {cr: rule(withExternalName), describe: observedRule(func(r *svcsdktypes.SecurityGroupRule) { r.FromPort, r.ToPort = aws.Int32(80), aws.Int32(80) }), exists: true},
		"TagsChanged":  {cr: rule(withExternalName), describe: observedRule(func(r *svcsdktypes.SecurityGroupRule) { r.Tags = nil }), exists: true},
		"IngressRule":  {cr: rule(withExternalName), describe: observedRule(func(r *svcsdktypes.SecurityGroupRule) { r.IsEgress = aws.Bool(false) }), isErr: true},
		"PrefixListChanged": {
			cr: rule(withExternalName, func(cr *svcapitypes.SecurityGroupEgressRule) {
				cr.Spec.ForProvider.CIDRIPv4, cr.Spec.ForProvider.PrefixListID = nil, aws.String("pl-2")
			}),
			describe: observedRule(func(r *svcsdktypes.SecurityGroupRule) { r.CidrIpv4, r.PrefixListId = nil, aws.String("pl-1") }),
			exists:   true,
		},
		"SourceTypeChanged": {
			cr: rule(withExternalName, func(cr *svcapitypes.SecurityGroupEgressRule) {
				cr.Spec.ForProvider.CIDRIPv4, cr.Spec.ForProvider.PrefixListID = nil, aws.String("pl-1")
			}),
			describe: observedRule(), isErr: true,
		},
		"OtherGroup": {
			cr: rule(withExternalName, func(cr *svcapitypes.SecurityGroupEgressRule) {
				cr.Spec.ForProvider.SecurityGroupID = aws.String("sg-2")
			}),
			describe: observedRule(), isErr: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			obs, err := newTestExternal(&fake.MockEC2Client{MockDescribeSecurityGroupRules: tc.describe}).Observe(context.Background(), tc.cr)
			if (err != nil) != tc.isErr {
				t.Fatalf("Observe() error = %v, wantErr %v", err, tc.isErr)
			}
			if tc.isErr {
				return
			}
			if obs.ResourceExists != tc.exists || obs.ResourceUpToDate != tc.upToDate {
				t.Errorf("Observe() = exists %v, upToDate %v (%s); want %v, %v", obs.ResourceExists, obs.ResourceUpToDate, obs.Diff, tc.exists, tc.upToDate)
			}
			if tc.exists && aws.ToString(tc.cr.Status.AtProvider.SecurityGroupRuleARN) != "arn:sgr" {
				t.Errorf("status not filled: %+v", tc.cr.Status.AtProvider)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	cases := map[string]struct {
		cr      *svcapitypes.SecurityGroupEgressRule
		wantIn  *svcsdk.AuthorizeSecurityGroupEgressInput
		wantErr bool
	}{
		"IPv4": {
			cr: rule(),
			wantIn: &svcsdk.AuthorizeSecurityGroupEgressInput{
				GroupId: aws.String(testGroup),
				IpPermissions: []svcsdktypes.IpPermission{{
					IpProtocol: aws.String("tcp"), FromPort: aws.Int32(443), ToPort: aws.Int32(443),
					IpRanges: []svcsdktypes.IpRange{{CidrIp: aws.String("10.0.0.0/8")}},
				}},
			},
		},
		"SourceGroupAllTraffic": {
			cr: rule(func(cr *svcapitypes.SecurityGroupEgressRule) {
				p := &cr.Spec.ForProvider
				p.IPProtocol, p.FromPort, p.ToPort, p.CIDRIPv4 = aws.String("-1"), nil, nil, nil
				p.ReferencedSecurityGroupID = aws.String("sg-2")
				p.Description = aws.String("from app")
			}),
			wantIn: &svcsdk.AuthorizeSecurityGroupEgressInput{
				GroupId: aws.String(testGroup),
				IpPermissions: []svcsdktypes.IpPermission{{
					IpProtocol:       aws.String("-1"),
					UserIdGroupPairs: []svcsdktypes.UserIdGroupPair{{GroupId: aws.String("sg-2"), Description: aws.String("from app")}},
				}},
			},
		},
		"Invalid": {
			cr:      rule(func(cr *svcapitypes.SecurityGroupEgressRule) { cr.Spec.ForProvider.CIDRIPv6 = aws.String("::/0") }),
			wantErr: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m := &fake.MockEC2Client{MockAuthorizeSecurityGroupEgress: func(context.Context, *svcsdk.AuthorizeSecurityGroupEgressInput) (*svcsdk.AuthorizeSecurityGroupEgressOutput, error) {
				return &svcsdk.AuthorizeSecurityGroupEgressOutput{SecurityGroupRules: []svcsdktypes.SecurityGroupRule{{SecurityGroupRuleId: aws.String(testRule)}}}, nil
			}}
			_, err := newTestExternal(m).Create(context.Background(), tc.cr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Create() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				if len(m.Calls["AuthorizeSecurityGroupEgress"]) != 0 {
					t.Error("invalid rule must not be sent to AWS")
				}
				return
			}
			if got := meta.GetExternalName(tc.cr); got != testRule {
				t.Errorf("external name = %q, want %q", got, testRule)
			}
			in := m.Calls["AuthorizeSecurityGroupEgress"][0].(*svcsdk.AuthorizeSecurityGroupEgressInput)
			if len(in.TagSpecifications) != 1 || in.TagSpecifications[0].ResourceType != svcsdktypes.ResourceTypeSecurityGroupRule {
				t.Errorf("TagSpecifications = %+v", in.TagSpecifications)
			}
			in.TagSpecifications = nil
			opt := cmpopts.IgnoreUnexported(svcsdk.AuthorizeSecurityGroupEgressInput{}, svcsdktypes.IpPermission{}, svcsdktypes.IpRange{}, svcsdktypes.UserIdGroupPair{})
			if diff := cmp.Diff(tc.wantIn, in, opt); diff != "" {
				t.Errorf("AuthorizeSecurityGroupEgressInput -want +got:\n%s", diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	cr := rule(withExternalName, func(cr *svcapitypes.SecurityGroupEgressRule) { cr.Spec.ForProvider.ToPort = aws.Int64(444) })
	m := &fake.MockEC2Client{}
	if _, err := newTestExternal(m).Update(context.Background(), cr); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	want := &svcsdk.ModifySecurityGroupRulesInput{
		GroupId: aws.String(testGroup),
		SecurityGroupRules: []svcsdktypes.SecurityGroupRuleUpdate{{
			SecurityGroupRuleId: aws.String(testRule),
			SecurityGroupRule: &svcsdktypes.SecurityGroupRuleRequest{
				IpProtocol: aws.String("tcp"), FromPort: aws.Int32(443), ToPort: aws.Int32(444), CidrIpv4: aws.String("10.0.0.0/8"),
			},
		}},
	}
	opt := cmpopts.IgnoreUnexported(svcsdk.ModifySecurityGroupRulesInput{}, svcsdktypes.SecurityGroupRuleUpdate{}, svcsdktypes.SecurityGroupRuleRequest{})
	if diff := cmp.Diff(want, m.Calls["ModifySecurityGroupRules"][0], opt); diff != "" {
		t.Errorf("ModifySecurityGroupRulesInput -want +got:\n%s", diff)
	}
	if len(m.Calls["CreateTags"]) != 1 {
		t.Errorf("expected the Crossplane tags to be created, calls: %v", m.Calls)
	}
}

func TestDelete(t *testing.T) {
	cases := map[string]struct {
		err     error
		wantErr bool
	}{
		"Success":       {},
		"RuleNotFound":  {err: apiErr("InvalidSecurityGroupRuleId.NotFound")},
		"GroupNotFound": {err: apiErr("InvalidGroup.NotFound")},
		"OtherError":    {err: apiErr("UnauthorizedOperation"), wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m := &fake.MockEC2Client{MockRevokeSecurityGroupEgress: func(context.Context, *svcsdk.RevokeSecurityGroupEgressInput) (*svcsdk.RevokeSecurityGroupEgressOutput, error) {
				return &svcsdk.RevokeSecurityGroupEgressOutput{}, tc.err
			}}
			_, err := newTestExternal(m).Delete(context.Background(), rule(withExternalName))
			if (err != nil) != tc.wantErr {
				t.Fatalf("Delete() error = %v, wantErr %v", err, tc.wantErr)
			}
			in := m.Calls["RevokeSecurityGroupEgress"][0].(*svcsdk.RevokeSecurityGroupEgressInput)
			if aws.ToString(in.GroupId) != testGroup || len(in.SecurityGroupRuleIds) != 1 || in.SecurityGroupRuleIds[0] != testRule {
				t.Errorf("RevokeSecurityGroupEgressInput = %+v", in)
			}
		})
	}
}

// recorder records the events it receives.
type recorder struct{ events []event.Event }

func (r *recorder) Event(_ runtime.Object, e event.Event) { r.events = append(r.events, e) }

func (r *recorder) WithAnnotations(...string) event.Recorder { return r }

// groupRules is a fake DescribeSecurityGroupRules: lookups by group (Filters)
// return rules, lookups by ID (SecurityGroupRuleIds) return the rule with
// that ID among rules, or InvalidSecurityGroupRuleId.NotFound.
type groupRules struct {
	rules     []svcsdktypes.SecurityGroupRule
	lookupErr error
	byIDErr   error
	filters   [][]svcsdktypes.Filter
	byID      []string
}

func (g *groupRules) mock(_ context.Context, in *svcsdk.DescribeSecurityGroupRulesInput) (*svcsdk.DescribeSecurityGroupRulesOutput, error) {
	if len(in.Filters) > 0 {
		g.filters = append(g.filters, in.Filters)
		if g.lookupErr != nil {
			return nil, g.lookupErr
		}
		return &svcsdk.DescribeSecurityGroupRulesOutput{SecurityGroupRules: g.rules}, nil
	}
	g.byID = append(g.byID, in.SecurityGroupRuleIds...)
	if g.byIDErr != nil {
		return nil, g.byIDErr
	}
	for _, r := range g.rules {
		if len(in.SecurityGroupRuleIds) == 1 && aws.ToString(r.SecurityGroupRuleId) == in.SecurityGroupRuleIds[0] {
			return &svcsdk.DescribeSecurityGroupRulesOutput{SecurityGroupRules: []svcsdktypes.SecurityGroupRule{r}}, nil
		}
	}
	return nil, apiErr("InvalidSecurityGroupRuleId.NotFound")
}

// awsRule is a rule of testGroup in AWS with the direction of this kind and
// the content of rule(): tcp/443, 10.0.0.0/8.
func awsRule(id string, f ...func(*svcsdktypes.SecurityGroupRule)) svcsdktypes.SecurityGroupRule {
	r := svcsdktypes.SecurityGroupRule{
		SecurityGroupRuleId: aws.String(id), GroupId: aws.String(testGroup), IsEgress: aws.Bool(egress),
		IpProtocol: aws.String("tcp"), FromPort: aws.Int32(443), ToPort: aws.Int32(443), CidrIpv4: aws.String("10.0.0.0/8"),
	}
	for _, fn := range f {
		fn(&r)
	}
	return r
}

func withAdopt(cr *svcapitypes.SecurityGroupEgressRule) {
	cr.Spec.ForProvider.AdoptExisting = aws.Bool(true)
}

func TestAdoptExisting(t *testing.T) {
	groupFilter := []svcsdktypes.Filter{{Name: aws.String("group-id"), Values: []string{testGroup}}}
	otherDirection := awsRule("sgr-dir", func(r *svcsdktypes.SecurityGroupRule) { r.IsEgress = aws.Bool(!egress) })
	otherPorts := awsRule("sgr-ports", func(r *svcsdktypes.SecurityGroupRule) { r.FromPort, r.ToPort = aws.Int32(80), aws.Int32(80) })
	adopted := event.Normal(reasonAdopted, "Adopted existing security group rule sgr-1 of sg-1 with the same protocol, ports and destination")

	type want struct {
		exists, upToDate, lateInit bool
		externalName               string
		err                        string
		lookups                    int
		byID                       []string
		events                     []event.Event
	}
	cases := map[string]struct {
		cr    *svcapitypes.SecurityGroupEgressRule
		rules groupRules
		want  want
	}{
		"DisabledByDefault": {
			cr:    rule(),
			rules: groupRules{rules: []svcsdktypes.SecurityGroupRule{awsRule(testRule)}},
			want:  want{},
		},
		"DisabledExplicitly": {
			cr:    rule(func(cr *svcapitypes.SecurityGroupEgressRule) { cr.Spec.ForProvider.AdoptExisting = aws.Bool(false) }),
			rules: groupRules{rules: []svcsdktypes.SecurityGroupRule{awsRule(testRule)}},
			want:  want{},
		},
		"Adopts": {
			cr:    rule(withAdopt),
			rules: groupRules{rules: []svcsdktypes.SecurityGroupRule{otherDirection, otherPorts, awsRule(testRule)}},
			want: want{exists: true, upToDate: false, lateInit: true, externalName: testRule, lookups: 1, byID: []string{testRule},
				events: []event.Event{adopted}},
		},
		"AdoptsUpToDateWithCrossplaneTags": {
			cr: rule(withAdopt),
			rules: groupRules{rules: []svcsdktypes.SecurityGroupRule{awsRule(testRule, func(r *svcsdktypes.SecurityGroupRule) {
				r.Tags = []svcsdktypes.Tag{
					{Key: aws.String("crossplane-kind"), Value: aws.String("securitygroupegressrule.ec2.aws2.crossplane.io")},
					{Key: aws.String("crossplane-name"), Value: aws.String("https")},
					{Key: aws.String("crossplane-namespace"), Value: aws.String("ns")},
				}
			})}},
			want: want{exists: true, upToDate: true, lateInit: true, externalName: testRule, lookups: 1, byID: []string{testRule},
				events: []event.Event{adopted}},
		},
		"AdoptsWithOtherDescription": {
			cr:    rule(withAdopt, func(cr *svcapitypes.SecurityGroupEgressRule) { cr.Spec.ForProvider.Description = aws.String("new") }),
			rules: groupRules{rules: []svcsdktypes.SecurityGroupRule{awsRule(testRule, func(r *svcsdktypes.SecurityGroupRule) { r.Description = aws.String("old") })}},
			want: want{exists: true, lateInit: true, externalName: testRule, lookups: 1, byID: []string{testRule},
				events: []event.Event{adopted}},
		},
		"AdoptsProtocolNumber": {
			cr:    rule(withAdopt, func(cr *svcapitypes.SecurityGroupEgressRule) { cr.Spec.ForProvider.IPProtocol = aws.String("6") }),
			rules: groupRules{rules: []svcsdktypes.SecurityGroupRule{awsRule(testRule)}},
			want: want{exists: true, lateInit: true, externalName: testRule, lookups: 1, byID: []string{testRule},
				events: []event.Event{adopted}},
		},
		"NoMatchCreates": {
			cr:    rule(withAdopt),
			rules: groupRules{rules: []svcsdktypes.SecurityGroupRule{otherDirection, otherPorts}},
			want:  want{lookups: 1},
		},
		"InvalidRuleNotLookedUp": {
			cr:    rule(withAdopt, func(cr *svcapitypes.SecurityGroupEgressRule) { cr.Spec.ForProvider.CIDRIPv6 = aws.String("::/0") }),
			rules: groupRules{rules: []svcsdktypes.SecurityGroupRule{awsRule(testRule)}},
			want:  want{},
		},
		"NoGroupNotLookedUp": {
			cr:    rule(withAdopt, func(cr *svcapitypes.SecurityGroupEgressRule) { cr.Spec.ForProvider.SecurityGroupID = nil }),
			rules: groupRules{rules: []svcsdktypes.SecurityGroupRule{awsRule(testRule)}},
			want:  want{},
		},
		"Ambiguous": {
			cr:    rule(withAdopt),
			rules: groupRules{rules: []svcsdktypes.SecurityGroupRule{awsRule(testRule), awsRule("sgr-2")}},
			want:  want{err: "2 " + map[bool]string{false: "ingress", true: "egress"}[egress] + " rules of sg-1 match", lookups: 1},
		},
		"LookupFails": {
			cr:    rule(withAdopt),
			rules: groupRules{lookupErr: apiErr("UnauthorizedOperation")},
			want:  want{err: "UnauthorizedOperation", lookups: 1},
		},
		"ExternalNameWins": {
			cr:    rule(withAdopt, withExternalName),
			rules: groupRules{rules: []svcsdktypes.SecurityGroupRule{awsRule(testRule)}},
			want:  want{exists: true, externalName: testRule, byID: []string{testRule}},
		},
		"ExternalNameGoneCreatesWithoutLookup": {
			cr:    rule(withAdopt, func(cr *svcapitypes.SecurityGroupEgressRule) { meta.SetExternalName(cr, "sgr-gone") }),
			rules: groupRules{rules: []svcsdktypes.SecurityGroupRule{awsRule(testRule)}},
			want:  want{externalName: "sgr-gone", byID: []string{"sgr-gone"}},
		},
		"ObserveAfterLookupFails": {
			cr:    rule(withAdopt),
			rules: groupRules{rules: []svcsdktypes.SecurityGroupRule{awsRule(testRule)}, byIDErr: apiErr("RequestLimitExceeded")},
			want:  want{err: "RequestLimitExceeded", lookups: 1, byID: []string{testRule}},
		},
		"GoneAfterLookupNoEvent": {
			cr:    rule(withAdopt),
			rules: groupRules{rules: []svcsdktypes.SecurityGroupRule{awsRule(testRule)}, byIDErr: apiErr("InvalidSecurityGroupRuleId.NotFound")},
			want:  want{lateInit: true, externalName: testRule, lookups: 1, byID: []string{testRule}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			g := tc.rules
			rec := &recorder{}
			e := &adoptingExternal{external: newTestExternal(&fake.MockEC2Client{MockDescribeSecurityGroupRules: g.mock}), record: rec}

			obs, err := e.Observe(context.Background(), tc.cr)

			if tc.want.err == "" && err != nil {
				t.Fatalf("Observe() error = %v", err)
			}
			if tc.want.err != "" && (err == nil || !strings.Contains(err.Error(), tc.want.err)) {
				t.Fatalf("Observe() error = %v, want containing %q", err, tc.want.err)
			}
			if tc.want.err == "" {
				if obs.ResourceExists != tc.want.exists || obs.ResourceUpToDate != tc.want.upToDate || obs.ResourceLateInitialized != tc.want.lateInit {
					t.Errorf("Observe() = exists %v, upToDate %v (%s), lateInit %v; want %+v", obs.ResourceExists, obs.ResourceUpToDate, obs.Diff, obs.ResourceLateInitialized, tc.want)
				}
				if got := meta.GetExternalName(tc.cr); got != tc.want.externalName {
					t.Errorf("external name = %q, want %q", got, tc.want.externalName)
				}
			}
			if len(g.filters) != tc.want.lookups {
				t.Errorf("lookups = %d, want %d", len(g.filters), tc.want.lookups)
			}
			for _, f := range g.filters {
				if diff := cmp.Diff(groupFilter, f, cmpopts.IgnoreUnexported(svcsdktypes.Filter{})); diff != "" {
					t.Errorf("lookup filters -want +got:\n%s", diff)
				}
			}
			if diff := cmp.Diff(tc.want.byID, g.byID); diff != "" {
				t.Errorf("describes by ID -want +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.events, rec.events); diff != "" {
				t.Errorf("events -want +got:\n%s", diff)
			}
		})
	}
}

func TestAdoptingWrapper(t *testing.T) {
	rec := &recorder{}
	got, ok := adoptingWrapper(rec)(newTestExternal(&fake.MockEC2Client{})).(*adoptingExternal)
	if !ok {
		t.Fatalf("adoptingWrapper() returned %T, want *adoptingExternal", got)
	}
	if got.record != rec {
		t.Errorf("adoptingWrapper() recorder = %v, want %v", got.record, rec)
	}
}

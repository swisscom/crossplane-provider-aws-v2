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

package securitygroupingressrule

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/ec2/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/ec2/fake"
)

const (
	testGroup = "sg-1"
	testRule  = "sgr-1"
)

func apiErr(code string) error { return &smithy.GenericAPIError{Code: code} }

func rule(f ...func(*svcapitypes.SecurityGroupIngressRule)) *svcapitypes.SecurityGroupIngressRule {
	cr := &svcapitypes.SecurityGroupIngressRule{ObjectMeta: metav1.ObjectMeta{Name: "https", Namespace: "ns"}}
	cr.SetGroupVersionKind(svcapitypes.SecurityGroupIngressRuleGroupVersionKind)
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

func withExternalName(cr *svcapitypes.SecurityGroupIngressRule) { meta.SetExternalName(cr, testRule) }

func observedRule(f ...func(*svcsdktypes.SecurityGroupRule)) func(context.Context, *svcsdk.DescribeSecurityGroupRulesInput) (*svcsdk.DescribeSecurityGroupRulesOutput, error) {
	r := svcsdktypes.SecurityGroupRule{
		SecurityGroupRuleId: aws.String(testRule), SecurityGroupRuleArn: aws.String("arn:sgr"), GroupId: aws.String(testGroup), GroupOwnerId: aws.String("123"),
		IsEgress: aws.Bool(false), IpProtocol: aws.String("tcp"), FromPort: aws.Int32(443), ToPort: aws.Int32(443), CidrIpv4: aws.String("10.0.0.0/8"),
		Tags: []svcsdktypes.Tag{
			{Key: aws.String("crossplane-kind"), Value: aws.String("securitygroupingressrule.ec2.aws2.crossplane.io")},
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
		cr                      *svcapitypes.SecurityGroupIngressRule
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
		"EgressRule":   {cr: rule(withExternalName), describe: observedRule(func(r *svcsdktypes.SecurityGroupRule) { r.IsEgress = aws.Bool(true) }), isErr: true},
		"OtherGroup": {
			cr: rule(withExternalName, func(cr *svcapitypes.SecurityGroupIngressRule) {
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
		cr      *svcapitypes.SecurityGroupIngressRule
		wantIn  *svcsdk.AuthorizeSecurityGroupIngressInput
		wantErr bool
	}{
		"IPv4": {
			cr: rule(),
			wantIn: &svcsdk.AuthorizeSecurityGroupIngressInput{
				GroupId: aws.String(testGroup),
				IpPermissions: []svcsdktypes.IpPermission{{
					IpProtocol: aws.String("tcp"), FromPort: aws.Int32(443), ToPort: aws.Int32(443),
					IpRanges: []svcsdktypes.IpRange{{CidrIp: aws.String("10.0.0.0/8")}},
				}},
			},
		},
		"SourceGroupAllTraffic": {
			cr: rule(func(cr *svcapitypes.SecurityGroupIngressRule) {
				p := &cr.Spec.ForProvider
				p.IPProtocol, p.FromPort, p.ToPort, p.CIDRIPv4 = aws.String("-1"), nil, nil, nil
				p.ReferencedSecurityGroupID = aws.String("sg-2")
				p.Description = aws.String("from app")
			}),
			wantIn: &svcsdk.AuthorizeSecurityGroupIngressInput{
				GroupId: aws.String(testGroup),
				IpPermissions: []svcsdktypes.IpPermission{{
					IpProtocol:       aws.String("-1"),
					UserIdGroupPairs: []svcsdktypes.UserIdGroupPair{{GroupId: aws.String("sg-2"), Description: aws.String("from app")}},
				}},
			},
		},
		"Invalid": {
			cr:      rule(func(cr *svcapitypes.SecurityGroupIngressRule) { cr.Spec.ForProvider.CIDRIPv6 = aws.String("::/0") }),
			wantErr: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m := &fake.MockEC2Client{MockAuthorizeSecurityGroupIngress: func(context.Context, *svcsdk.AuthorizeSecurityGroupIngressInput) (*svcsdk.AuthorizeSecurityGroupIngressOutput, error) {
				return &svcsdk.AuthorizeSecurityGroupIngressOutput{SecurityGroupRules: []svcsdktypes.SecurityGroupRule{{SecurityGroupRuleId: aws.String(testRule)}}}, nil
			}}
			_, err := newTestExternal(m).Create(context.Background(), tc.cr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Create() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				if len(m.Calls["AuthorizeSecurityGroupIngress"]) != 0 {
					t.Error("invalid rule must not be sent to AWS")
				}
				return
			}
			if got := meta.GetExternalName(tc.cr); got != testRule {
				t.Errorf("external name = %q, want %q", got, testRule)
			}
			in := m.Calls["AuthorizeSecurityGroupIngress"][0].(*svcsdk.AuthorizeSecurityGroupIngressInput)
			if len(in.TagSpecifications) != 1 || in.TagSpecifications[0].ResourceType != svcsdktypes.ResourceTypeSecurityGroupRule {
				t.Errorf("TagSpecifications = %+v", in.TagSpecifications)
			}
			in.TagSpecifications = nil
			opt := cmpopts.IgnoreUnexported(svcsdk.AuthorizeSecurityGroupIngressInput{}, svcsdktypes.IpPermission{}, svcsdktypes.IpRange{}, svcsdktypes.UserIdGroupPair{})
			if diff := cmp.Diff(tc.wantIn, in, opt); diff != "" {
				t.Errorf("AuthorizeSecurityGroupIngressInput -want +got:\n%s", diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	cr := rule(withExternalName, func(cr *svcapitypes.SecurityGroupIngressRule) { cr.Spec.ForProvider.ToPort = aws.Int64(444) })
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
			m := &fake.MockEC2Client{MockRevokeSecurityGroupIngress: func(context.Context, *svcsdk.RevokeSecurityGroupIngressInput) (*svcsdk.RevokeSecurityGroupIngressOutput, error) {
				return &svcsdk.RevokeSecurityGroupIngressOutput{}, tc.err
			}}
			_, err := newTestExternal(m).Delete(context.Background(), rule(withExternalName))
			if (err != nil) != tc.wantErr {
				t.Fatalf("Delete() error = %v, wantErr %v", err, tc.wantErr)
			}
			in := m.Calls["RevokeSecurityGroupIngress"][0].(*svcsdk.RevokeSecurityGroupIngressInput)
			if aws.ToString(in.GroupId) != testGroup || len(in.SecurityGroupRuleIds) != 1 || in.SecurityGroupRuleIds[0] != testRule {
				t.Errorf("RevokeSecurityGroupIngressInput = %+v", in)
			}
		})
	}
}

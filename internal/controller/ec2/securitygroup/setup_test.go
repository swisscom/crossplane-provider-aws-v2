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

package securitygroup

import (
	"context"
	"testing"
	"time"

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
	testID   = "sg-123"
	testName = "web"
	testVPC  = "vpc-1"
)

func init() {
	revokeBackoff = []time.Duration{time.Millisecond, time.Millisecond}
}

func apiErr(code string) error { return &smithy.GenericAPIError{Code: code} }

func group(f ...func(*svcapitypes.SecurityGroup)) *svcapitypes.SecurityGroup {
	cr := &svcapitypes.SecurityGroup{ObjectMeta: metav1.ObjectMeta{Name: testName, Namespace: "ns"}}
	cr.SetGroupVersionKind(svcapitypes.SecurityGroupGroupVersionKind)
	cr.Spec.ForProvider.Description = aws.String("desc")
	for _, fn := range f {
		fn(cr)
	}
	return cr
}

func withExternalName(n string) func(*svcapitypes.SecurityGroup) {
	return func(cr *svcapitypes.SecurityGroup) { meta.SetExternalName(cr, n) }
}

func observed(f ...func(*svcsdktypes.SecurityGroup)) *svcsdk.DescribeSecurityGroupsOutput {
	g := svcsdktypes.SecurityGroup{GroupId: aws.String(testID), GroupName: aws.String(testName), Description: aws.String("desc"), VpcId: aws.String(testVPC)}
	for _, fn := range f {
		fn(&g)
	}
	return &svcsdk.DescribeSecurityGroupsOutput{SecurityGroups: []svcsdktypes.SecurityGroup{g}}
}

// crossplaneTags are the external tags Crossplane sets on group().
func crossplaneTags() []svcsdktypes.Tag {
	return []svcsdktypes.Tag{
		{Key: aws.String("crossplane-kind"), Value: aws.String("securitygroup.ec2.aws2.crossplane.io")},
		{Key: aws.String("crossplane-name"), Value: aws.String(testName)},
		{Key: aws.String("crossplane-namespace"), Value: aws.String("ns")},
	}
}

func newTestExternal(m *fake.MockEC2Client) *adoptingExternal {
	return &adoptingExternal{external: newExternal(nil, m, []option{setupExternal})}
}

func TestObserve(t *testing.T) {
	type want struct {
		exists, upToDate, lateInit bool
		externalName               string
		err                        bool
	}
	cases := map[string]struct {
		cr   *svcapitypes.SecurityGroup
		mock *fake.MockEC2Client
		want want
	}{
		"NoExternalName": {
			cr:   group(),
			mock: &fake.MockEC2Client{},
			want: want{},
		},
		"NotFound": {
			cr: group(withExternalName(testID)),
			mock: &fake.MockEC2Client{MockDescribeSecurityGroups: func(context.Context, *svcsdk.DescribeSecurityGroupsInput) (*svcsdk.DescribeSecurityGroupsOutput, error) {
				return nil, apiErr("InvalidGroup.NotFound")
			}},
			want: want{externalName: testID},
		},
		"LateInitAndTagsDiffer": {
			cr: group(withExternalName(testID)),
			mock: &fake.MockEC2Client{MockDescribeSecurityGroups: func(_ context.Context, in *svcsdk.DescribeSecurityGroupsInput) (*svcsdk.DescribeSecurityGroupsOutput, error) {
				if len(in.GroupIds) != 1 || in.GroupIds[0] != testID {
					t.Errorf("GroupIds = %v", in.GroupIds)
				}
				return observed(), nil
			}},
			want: want{exists: true, lateInit: true, externalName: testID},
		},
		"UpToDate": {
			cr: group(withExternalName(testID), func(cr *svcapitypes.SecurityGroup) {
				cr.Spec.ForProvider.GroupName = aws.String(testName)
				cr.Spec.ForProvider.VPCID = aws.String(testVPC)
			}),
			mock: &fake.MockEC2Client{MockDescribeSecurityGroups: func(context.Context, *svcsdk.DescribeSecurityGroupsInput) (*svcsdk.DescribeSecurityGroupsOutput, error) {
				return observed(func(g *svcsdktypes.SecurityGroup) {
					g.Tags = append(crossplaneTags(), svcsdktypes.Tag{Key: aws.String("aws:cloudformation:stack-name"), Value: aws.String("x")})
				}), nil
			}},
			want: want{exists: true, upToDate: true, externalName: testID},
		},
		"DescriptionImmutable": {
			cr: group(withExternalName(testID), func(cr *svcapitypes.SecurityGroup) { cr.Spec.ForProvider.Description = aws.String("other") }),
			mock: &fake.MockEC2Client{MockDescribeSecurityGroups: func(context.Context, *svcsdk.DescribeSecurityGroupsInput) (*svcsdk.DescribeSecurityGroupsOutput, error) {
				return observed(), nil
			}},
			want: want{externalName: testID, err: true},
		},
		"AdoptByName": {
			cr: group(func(cr *svcapitypes.SecurityGroup) {
				cr.Spec.ForProvider.AdoptExistingByName = aws.Bool(true)
				cr.Spec.ForProvider.VPCID = aws.String(testVPC)
			}),
			mock: &fake.MockEC2Client{MockDescribeSecurityGroups: func(_ context.Context, in *svcsdk.DescribeSecurityGroupsInput) (*svcsdk.DescribeSecurityGroupsOutput, error) {
				if len(in.Filters) > 0 {
					want := []svcsdktypes.Filter{
						{Name: aws.String("group-name"), Values: []string{testName}},
						{Name: aws.String("vpc-id"), Values: []string{testVPC}},
					}
					if diff := cmp.Diff(want, in.Filters, cmpopts.IgnoreUnexported(svcsdktypes.Filter{})); diff != "" {
						t.Errorf("Filters -want +got:\n%s", diff)
					}
				}
				return observed(), nil
			}},
			want: want{exists: true, lateInit: true, externalName: testID},
		},
		"AdoptNoMatch": {
			cr: group(func(cr *svcapitypes.SecurityGroup) { cr.Spec.ForProvider.AdoptExistingByName = aws.Bool(true) }),
			mock: &fake.MockEC2Client{MockDescribeSecurityGroups: func(context.Context, *svcsdk.DescribeSecurityGroupsInput) (*svcsdk.DescribeSecurityGroupsOutput, error) {
				return &svcsdk.DescribeSecurityGroupsOutput{}, nil
			}},
			want: want{},
		},
		"AdoptAmbiguous": {
			cr: group(func(cr *svcapitypes.SecurityGroup) { cr.Spec.ForProvider.AdoptExistingByName = aws.Bool(true) }),
			mock: &fake.MockEC2Client{MockDescribeSecurityGroups: func(context.Context, *svcsdk.DescribeSecurityGroupsInput) (*svcsdk.DescribeSecurityGroupsOutput, error) {
				o := observed()
				o.SecurityGroups = append(o.SecurityGroups, o.SecurityGroups[0])
				return o, nil
			}},
			want: want{err: true},
		},
		"AdoptDefault": {
			cr: group(func(cr *svcapitypes.SecurityGroup) {
				cr.Spec.ForProvider.AdoptExistingByName = aws.Bool(true)
				cr.Spec.ForProvider.GroupName = aws.String("default")
			}),
			mock: &fake.MockEC2Client{},
			want: want{err: true},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			obs, err := newTestExternal(tc.mock).Observe(context.Background(), tc.cr)
			if (err != nil) != tc.want.err {
				t.Fatalf("Observe() error = %v, wantErr %v", err, tc.want.err)
			}
			if tc.want.err {
				return
			}
			if obs.ResourceExists != tc.want.exists || obs.ResourceUpToDate != tc.want.upToDate || obs.ResourceLateInitialized != tc.want.lateInit {
				t.Errorf("Observe() = exists %v, upToDate %v (%s), lateInit %v; want %+v", obs.ResourceExists, obs.ResourceUpToDate, obs.Diff, obs.ResourceLateInitialized, tc.want)
			}
			if got := meta.GetExternalName(tc.cr); got != tc.want.externalName {
				t.Errorf("external name = %q, want %q", got, tc.want.externalName)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	created := func(context.Context, *svcsdk.CreateSecurityGroupInput) (*svcsdk.CreateSecurityGroupOutput, error) {
		return &svcsdk.CreateSecurityGroupOutput{GroupId: aws.String(testID)}, nil
	}
	cases := map[string]struct {
		mock        *fake.MockEC2Client
		wantErr     bool
		wantRevokes int
		wantDeletes int
	}{
		"Success": {
			mock:        &fake.MockEC2Client{MockCreateSecurityGroup: created},
			wantRevokes: 2,
		},
		"DefaultEgressAlreadyGone": {
			mock: &fake.MockEC2Client{MockCreateSecurityGroup: created, MockRevokeSecurityGroupEgress: func(context.Context, *svcsdk.RevokeSecurityGroupEgressInput) (*svcsdk.RevokeSecurityGroupEgressOutput, error) {
				return nil, apiErr("InvalidPermission.NotFound")
			}},
			wantRevokes: 2,
		},
		"RetryUntilGroupVisible": {
			mock: func() *fake.MockEC2Client {
				n := 0
				return &fake.MockEC2Client{MockCreateSecurityGroup: created, MockRevokeSecurityGroupEgress: func(context.Context, *svcsdk.RevokeSecurityGroupEgressInput) (*svcsdk.RevokeSecurityGroupEgressOutput, error) {
					if n++; n < 3 {
						return nil, apiErr("InvalidGroup.NotFound")
					}
					return &svcsdk.RevokeSecurityGroupEgressOutput{}, nil
				}}
			}(),
			wantRevokes: 4,
		},
		"RollbackOnRevokeFailure": {
			mock: &fake.MockEC2Client{MockCreateSecurityGroup: created, MockRevokeSecurityGroupEgress: func(context.Context, *svcsdk.RevokeSecurityGroupEgressInput) (*svcsdk.RevokeSecurityGroupEgressOutput, error) {
				return nil, apiErr("UnauthorizedOperation")
			}},
			wantErr:     true,
			wantRevokes: 1,
			wantDeletes: 1,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := group(func(cr *svcapitypes.SecurityGroup) {
				cr.Spec.ForProvider.Tags = []*svcapitypes.Tag{{Key: aws.String("team"), Value: aws.String("a")}}
			})
			_, err := newTestExternal(tc.mock).Create(context.Background(), cr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Create() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got := meta.GetExternalName(cr); got != testID {
				t.Errorf("external name = %q, want %q", got, testID)
			}
			in := tc.mock.Calls["CreateSecurityGroup"][0].(*svcsdk.CreateSecurityGroupInput)
			if aws.ToString(in.GroupName) != testName {
				t.Errorf("GroupName = %q, want metadata.name", aws.ToString(in.GroupName))
			}
			if len(in.TagSpecifications) != 1 || in.TagSpecifications[0].ResourceType != svcsdktypes.ResourceTypeSecurityGroup || len(in.TagSpecifications[0].Tags) != 4 {
				t.Errorf("TagSpecifications = %+v", in.TagSpecifications)
			}
			if got := len(tc.mock.Calls["RevokeSecurityGroupEgress"]); got != tc.wantRevokes {
				t.Errorf("RevokeSecurityGroupEgress calls = %d, want %d", got, tc.wantRevokes)
			}
			if got := len(tc.mock.Calls["DeleteSecurityGroup"]); got != tc.wantDeletes {
				t.Errorf("DeleteSecurityGroup calls = %d, want %d", got, tc.wantDeletes)
			}
		})
	}
}

func TestUpdateTags(t *testing.T) {
	cr := group(withExternalName(testID), func(cr *svcapitypes.SecurityGroup) {
		cr.Spec.ForProvider.Tags = []*svcapitypes.Tag{{Key: aws.String("team"), Value: aws.String("b")}}
		cr.Status.AtProvider.Tags = []*svcapitypes.Tag{
			{Key: aws.String("team"), Value: aws.String("a")},
			{Key: aws.String("old"), Value: aws.String("x")},
			{Key: aws.String("crossplane-kind"), Value: aws.String("securitygroup.ec2.aws2.crossplane.io")},
			{Key: aws.String("crossplane-name"), Value: aws.String(testName)},
			{Key: aws.String("crossplane-namespace"), Value: aws.String("ns")},
		}
	})
	m := &fake.MockEC2Client{}
	if _, err := newTestExternal(m).Update(context.Background(), cr); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	opt := cmpopts.IgnoreUnexported(svcsdk.CreateTagsInput{}, svcsdk.DeleteTagsInput{}, svcsdktypes.Tag{})
	wantCreate := []any{&svcsdk.CreateTagsInput{Resources: []string{testID}, Tags: []svcsdktypes.Tag{{Key: aws.String("team"), Value: aws.String("b")}}}}
	wantDelete := []any{&svcsdk.DeleteTagsInput{Resources: []string{testID}, Tags: []svcsdktypes.Tag{{Key: aws.String("old")}}}}
	if diff := cmp.Diff(wantCreate, m.Calls["CreateTags"], opt); diff != "" {
		t.Errorf("CreateTags -want +got:\n%s", diff)
	}
	if diff := cmp.Diff(wantDelete, m.Calls["DeleteTags"], opt); diff != "" {
		t.Errorf("DeleteTags -want +got:\n%s", diff)
	}
}

func TestDelete(t *testing.T) {
	cases := map[string]struct {
		err     error
		wantErr bool
	}{
		"Success":  {},
		"NotFound": {err: apiErr("InvalidGroup.NotFound")},
		"InUse":    {err: apiErr("DependencyViolation"), wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := group(withExternalName(testID), func(cr *svcapitypes.SecurityGroup) { cr.Spec.ForProvider.GroupName = aws.String(testName) })
			m := &fake.MockEC2Client{MockDeleteSecurityGroup: func(context.Context, *svcsdk.DeleteSecurityGroupInput) (*svcsdk.DeleteSecurityGroupOutput, error) {
				return &svcsdk.DeleteSecurityGroupOutput{}, tc.err
			}}
			_, err := newTestExternal(m).Delete(context.Background(), cr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Delete() error = %v, wantErr %v", err, tc.wantErr)
			}
			in := m.Calls["DeleteSecurityGroup"][0].(*svcsdk.DeleteSecurityGroupInput)
			if aws.ToString(in.GroupId) != testID || in.GroupName != nil {
				t.Errorf("DeleteSecurityGroupInput = GroupId %q, GroupName %v; want only GroupId %q", aws.ToString(in.GroupId), in.GroupName, testID)
			}
		})
	}
}

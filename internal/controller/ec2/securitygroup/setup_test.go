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
	"fmt"
	"strings"
	"testing"
	"time"

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
	return &adoptingExternal{external: newExternal(nil, m, []option{setupExternal}), record: event.NewNopRecorder()}
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

// recorder records the events it receives.
type recorder struct{ events []event.Event }

func (r *recorder) Event(_ runtime.Object, e event.Event) { r.events = append(r.events, e) }

func (r *recorder) WithAnnotations(...string) event.Recorder { return r }

// describeCalls is a fake DescribeSecurityGroups that records its inputs and
// answers lookups by name (Filters) and by ID (GroupIds) separately.
type describeCalls struct {
	inputs   []*svcsdk.DescribeSecurityGroupsInput
	byName   func() (*svcsdk.DescribeSecurityGroupsOutput, error)
	byID     func() (*svcsdk.DescribeSecurityGroupsOutput, error)
	unexpect func(string)
}

func (d *describeCalls) mock(_ context.Context, in *svcsdk.DescribeSecurityGroupsInput) (*svcsdk.DescribeSecurityGroupsOutput, error) {
	d.inputs = append(d.inputs, in)
	switch {
	case len(in.Filters) > 0 && d.byName != nil:
		return d.byName()
	case len(in.GroupIds) > 0 && d.byID != nil:
		return d.byID()
	}
	d.unexpect(fmt.Sprintf("unexpected DescribeSecurityGroups(Filters %v, GroupIds %v)", in.Filters, in.GroupIds))
	return nil, apiErr("Unexpected")
}

func nameFilter(name string) svcsdktypes.Filter {
	return svcsdktypes.Filter{Name: aws.String("group-name"), Values: []string{name}}
}

func vpcFilter(vpc string) svcsdktypes.Filter {
	return svcsdktypes.Filter{Name: aws.String("vpc-id"), Values: []string{vpc}}
}

func withAdopt(cr *svcapitypes.SecurityGroup) {
	cr.Spec.ForProvider.AdoptExistingByName = aws.Bool(true)
}

func TestAdoptExistingByName(t *testing.T) {
	found := func() (*svcsdk.DescribeSecurityGroupsOutput, error) { return observed(), nil }
	none := func() (*svcsdk.DescribeSecurityGroupsOutput, error) {
		return &svcsdk.DescribeSecurityGroupsOutput{}, nil
	}
	adopted := event.Normal(reasonAdopted, `Adopted existing security group sg-123 (name "web", VPC vpc-1) found by name`)

	type want struct {
		exists, lateInit bool
		externalName     string
		err              string
		// filters of the lookup by name; nil: no lookup.
		filters []svcsdktypes.Filter
		// describes is the number of DescribeSecurityGroups calls.
		describes int
		events    []event.Event
	}
	cases := map[string]struct {
		cr       *svcapitypes.SecurityGroup
		describe describeCalls
		want     want
	}{
		"DisabledByDefault": {
			cr:   group(),
			want: want{},
		},
		"DisabledExplicitly": {
			cr:   group(func(cr *svcapitypes.SecurityGroup) { cr.Spec.ForProvider.AdoptExistingByName = aws.Bool(false) }),
			want: want{},
		},
		"AdoptsInVPC": {
			cr:       group(withAdopt, func(cr *svcapitypes.SecurityGroup) { cr.Spec.ForProvider.VPCID = aws.String(testVPC) }),
			describe: describeCalls{byName: found, byID: found},
			want: want{
				exists: true, lateInit: true, externalName: testID,
				filters: []svcsdktypes.Filter{nameFilter(testName), vpcFilter(testVPC)}, describes: 2,
				events: []event.Event{adopted},
			},
		},
		"AdoptsInAnyVPCWithoutVPCID": {
			cr:       group(withAdopt),
			describe: describeCalls{byName: found, byID: found},
			want: want{
				exists: true, lateInit: true, externalName: testID,
				filters: []svcsdktypes.Filter{nameFilter(testName)}, describes: 2,
				events: []event.Event{adopted},
			},
		},
		"LooksUpSpecGroupName": {
			cr:       group(withAdopt, func(cr *svcapitypes.SecurityGroup) { cr.Spec.ForProvider.GroupName = aws.String("other") }),
			describe: describeCalls{byName: none},
			want:     want{filters: []svcsdktypes.Filter{nameFilter("other")}, describes: 1},
		},
		"NoMatchCreates": {
			cr:       group(withAdopt),
			describe: describeCalls{byName: none},
			want:     want{filters: []svcsdktypes.Filter{nameFilter(testName)}, describes: 1},
		},
		"Ambiguous": {
			cr: group(withAdopt),
			describe: describeCalls{byName: func() (*svcsdk.DescribeSecurityGroupsOutput, error) {
				o := observed()
				o.SecurityGroups = append(o.SecurityGroups, o.SecurityGroups[0])
				return o, nil
			}},
			want: want{err: `adoptExistingByName: 2 security groups are named "web"`, filters: []svcsdktypes.Filter{nameFilter(testName)}, describes: 1},
		},
		"LookupFails": {
			cr:       group(withAdopt),
			describe: describeCalls{byName: func() (*svcsdk.DescribeSecurityGroupsOutput, error) { return nil, apiErr("UnauthorizedOperation") }},
			want:     want{err: errAdoptDescribe, filters: []svcsdktypes.Filter{nameFilter(testName)}, describes: 1},
		},
		"RefusesDefaultInSpec": {
			cr:   group(withAdopt, func(cr *svcapitypes.SecurityGroup) { cr.Spec.ForProvider.GroupName = aws.String("default") }),
			want: want{err: errAdoptDefault},
		},
		"RefusesDefaultFromMetadataName": {
			cr:   group(withAdopt, func(cr *svcapitypes.SecurityGroup) { cr.Name = "default" }),
			want: want{err: errAdoptDefault},
		},
		"ExternalNameWins": {
			cr:       group(withAdopt, withExternalName("sg-other")),
			describe: describeCalls{byID: found},
			want:     want{exists: true, lateInit: true, externalName: "sg-other", describes: 1},
		},
		"ExternalNameNotFoundCreatesWithoutLookup": {
			cr:       group(withAdopt, withExternalName("sg-gone")),
			describe: describeCalls{byID: func() (*svcsdk.DescribeSecurityGroupsOutput, error) { return nil, apiErr("InvalidGroup.NotFound") }},
			want:     want{externalName: "sg-gone", describes: 1},
		},
		"ObserveAfterLookupFails": {
			cr: group(withAdopt),
			describe: describeCalls{byName: found, byID: func() (*svcsdk.DescribeSecurityGroupsOutput, error) {
				return nil, apiErr("RequestLimitExceeded")
			}},
			want: want{err: "RequestLimitExceeded", filters: []svcsdktypes.Filter{nameFilter(testName)}, describes: 2},
		},
		"GoneAfterLookupNoEvent": {
			cr: group(withAdopt),
			describe: describeCalls{byName: found, byID: func() (*svcsdk.DescribeSecurityGroupsOutput, error) {
				return nil, apiErr("InvalidGroup.NotFound")
			}},
			want: want{lateInit: true, externalName: testID, filters: []svcsdktypes.Filter{nameFilter(testName)}, describes: 2},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := tc.describe
			d.unexpect = func(msg string) { t.Error(msg) }
			rec := &recorder{}
			e := newTestExternal(&fake.MockEC2Client{MockDescribeSecurityGroups: d.mock})
			e.record = rec

			obs, err := e.Observe(context.Background(), tc.cr)

			if tc.want.err == "" && err != nil {
				t.Fatalf("Observe() error = %v", err)
			}
			if tc.want.err != "" && (err == nil || !strings.Contains(err.Error(), tc.want.err)) {
				t.Fatalf("Observe() error = %v, want containing %q", err, tc.want.err)
			}
			if tc.want.err == "" && (obs.ResourceExists != tc.want.exists || obs.ResourceLateInitialized != tc.want.lateInit) {
				t.Errorf("Observe() = exists %v, lateInit %v; want %v, %v", obs.ResourceExists, obs.ResourceLateInitialized, tc.want.exists, tc.want.lateInit)
			}
			if tc.want.err == "" {
				if got := meta.GetExternalName(tc.cr); got != tc.want.externalName {
					t.Errorf("external name = %q, want %q", got, tc.want.externalName)
				}
			}
			if len(d.inputs) != tc.want.describes {
				t.Errorf("DescribeSecurityGroups calls = %d, want %d", len(d.inputs), tc.want.describes)
			}
			var filters []svcsdktypes.Filter
			for _, in := range d.inputs {
				if len(in.Filters) > 0 {
					filters = in.Filters
				}
			}
			if diff := cmp.Diff(tc.want.filters, filters, cmpopts.IgnoreUnexported(svcsdktypes.Filter{})); diff != "" {
				t.Errorf("lookup filters -want +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.events, rec.events); diff != "" {
				t.Errorf("events -want +got:\n%s", diff)
			}
		})
	}
}

func TestAdoptingWrapper(t *testing.T) {
	rec := &recorder{}
	got, ok := adoptingWrapper(rec)(newExternal(nil, &fake.MockEC2Client{}, nil)).(*adoptingExternal)
	if !ok {
		t.Fatalf("adoptingWrapper() returned %T, want *adoptingExternal", got)
	}
	if got.record != rec {
		t.Errorf("adoptingWrapper() recorder = %v, want %v", got.record, rec)
	}
}

func TestCreate(t *testing.T) {
	created := func(context.Context, *svcsdk.CreateSecurityGroupInput) (*svcsdk.CreateSecurityGroupOutput, error) {
		return &svcsdk.CreateSecurityGroupOutput{GroupId: aws.String(testID)}, nil
	}
	groupNotFound := func(context.Context, *svcsdk.RevokeSecurityGroupEgressInput) (*svcsdk.RevokeSecurityGroupEgressOutput, error) {
		return nil, apiErr("InvalidGroup.NotFound")
	}
	cases := map[string]struct {
		mock        *fake.MockEC2Client
		cancelled   bool
		wantErr     bool
		wantErrText string
		wantNoName  bool
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
			wantErrText: errRevokeEgress,
			wantRevokes: 1,
			wantDeletes: 1,
		},
		"RollbackFails": {
			mock: &fake.MockEC2Client{MockCreateSecurityGroup: created,
				MockRevokeSecurityGroupEgress: func(context.Context, *svcsdk.RevokeSecurityGroupEgressInput) (*svcsdk.RevokeSecurityGroupEgressOutput, error) {
					return nil, apiErr("UnauthorizedOperation")
				},
				MockDeleteSecurityGroup: func(context.Context, *svcsdk.DeleteSecurityGroupInput) (*svcsdk.DeleteSecurityGroupOutput, error) {
					return nil, apiErr("RequestLimitExceeded")
				}},
			wantErr:     true,
			wantErrText: errRollback,
			wantRevokes: 1,
			wantDeletes: 1,
		},
		"GroupNeverVisibleRollsBack": {
			mock:        &fake.MockEC2Client{MockCreateSecurityGroup: created, MockRevokeSecurityGroupEgress: groupNotFound},
			wantErr:     true,
			wantErrText: errRevokeEgress,
			wantRevokes: 3, // first try plus one per revokeBackoff entry
			wantDeletes: 1,
		},
		"CancelledWhileRetryingRollsBack": {
			mock:        &fake.MockEC2Client{MockCreateSecurityGroup: created, MockRevokeSecurityGroupEgress: groupNotFound},
			cancelled:   true,
			wantErr:     true,
			wantErrText: "context canceled",
			wantRevokes: 1,
			wantDeletes: 1,
		},
		"CreateFails": {
			mock: &fake.MockEC2Client{MockCreateSecurityGroup: func(context.Context, *svcsdk.CreateSecurityGroupInput) (*svcsdk.CreateSecurityGroupOutput, error) {
				return nil, apiErr("InvalidGroup.Duplicate")
			}},
			wantErr:     true,
			wantErrText: "InvalidGroup.Duplicate",
			wantNoName:  true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := group(func(cr *svcapitypes.SecurityGroup) {
				cr.Spec.ForProvider.Tags = []*svcapitypes.Tag{{Key: aws.String("team"), Value: aws.String("a")}}
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			_, err := newTestExternal(tc.mock).Create(ctx, cr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Create() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), tc.wantErrText) {
				t.Errorf("Create() error = %v, want containing %q", err, tc.wantErrText)
			}
			wantName := testID
			if tc.wantNoName {
				wantName = ""
			}
			if got := meta.GetExternalName(cr); got != wantName {
				t.Errorf("external name = %q, want %q", got, wantName)
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

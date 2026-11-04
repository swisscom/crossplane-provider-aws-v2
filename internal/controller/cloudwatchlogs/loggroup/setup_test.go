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

package loggroup

import (
	"context"
	"go/token"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/smithy-go"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/cloudwatchlogs/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/cloudwatchlogs/fake"
)

const (
	testName     = "opensearch-logs"
	testGroup    = "/aws/opensearch/logs/DomainAppLogs"
	testARN      = "arn:aws:logs:eu-central-1:123456789012:log-group:" + testGroup
	testKMSKeyID = "1234abcd-12ab-34cd-56ef-1234567890ab"
	testKMSARN   = "arn:aws:kms:eu-central-1:123456789012:key/" + testKMSKeyID
)

var ignoreUnexported = cmp.FilterPath(func(p cmp.Path) bool {
	sf, ok := p.Index(-1).(cmp.StructField)
	return ok && !token.IsExported(sf.Name())
}, cmp.Ignore())

func logGroup(f ...func(*svcapitypes.LogGroup)) *svcapitypes.LogGroup {
	cr := &svcapitypes.LogGroup{ObjectMeta: metav1.ObjectMeta{Name: testName, Namespace: "ns"}}
	cr.SetGroupVersionKind(svcapitypes.LogGroupGroupVersionKind)
	meta.SetExternalName(cr, testGroup)
	cr.Spec.ForProvider.Region = "eu-central-1"
	cr.Spec.ForProvider.RetentionInDays = aws.Int64(30)
	cr.Spec.ForProvider.Tags = map[string]*string{"team": aws.String("search")}
	for _, fn := range f {
		fn(cr)
	}
	return cr
}

// observed returns the log group as AWS describes it for logGroup().
func observed(f ...func(*svcsdktypes.LogGroup)) svcsdktypes.LogGroup {
	lg := svcsdktypes.LogGroup{
		LogGroupName:    aws.String(testGroup),
		Arn:             aws.String(testARN + ":*"),
		LogGroupArn:     aws.String(testARN),
		RetentionInDays: aws.Int32(30),
		LogGroupClass:   svcsdktypes.LogGroupClassStandard,
		StoredBytes:     aws.Int64(42),
	}
	for _, fn := range f {
		fn(&lg)
	}
	return lg
}

// mock returns a client that describes the given log groups and lists the
// given tags; nil tags are the desired tags of logGroup().
func mock(tags map[string]string, groups ...svcsdktypes.LogGroup) *fake.MockClient {
	return &fake.MockClient{
		MockDescribeLogGroups: func(context.Context, *svcsdk.DescribeLogGroupsInput) (*svcsdk.DescribeLogGroupsOutput, error) {
			return &svcsdk.DescribeLogGroupsOutput{LogGroups: groups}, nil
		},
		MockListTagsForResource: func(context.Context, *svcsdk.ListTagsForResourceInput) (*svcsdk.ListTagsForResourceOutput, error) {
			if tags == nil {
				tags = desiredTags(logGroup())
			}
			return &svcsdk.ListTagsForResourceOutput{Tags: tags}, nil
		},
	}
}

func newTestExternal(m *fake.MockClient) *external {
	return newExternal(nil, m, []option{setupExternal})
}

func TestObserve(t *testing.T) {
	type want struct {
		obs managed.ExternalObservation
	}
	cases := map[string]struct {
		cr   *svcapitypes.LogGroup
		mock *fake.MockClient
		want want
	}{
		"NotFound": {
			cr:   logGroup(),
			mock: mock(nil, observed(func(lg *svcsdktypes.LogGroup) { lg.LogGroupName = aws.String(testGroup + "-other") })),
			want: want{obs: managed.ExternalObservation{}},
		},
		"UpToDate": {
			cr: logGroup(func(cr *svcapitypes.LogGroup) {
				cr.Spec.ForProvider.DeletionProtectionEnabled = aws.Bool(false)
				cr.Spec.ForProvider.KMSKeyID = aws.String(testKMSKeyID)
			}),
			mock: mock(nil, observed(func(lg *svcsdktypes.LogGroup) { lg.KmsKeyId = aws.String(testKMSARN) })),
			want: want{obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}},
		},
		"RetentionForeverAndNoneObserved": {
			cr:   logGroup(func(cr *svcapitypes.LogGroup) { cr.Spec.ForProvider.RetentionInDays = aws.Int64(0) }),
			mock: mock(nil, observed(func(lg *svcsdktypes.LogGroup) { lg.RetentionInDays = nil })),
			want: want{obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}},
		},
		"UnsetFieldsNotManaged": {
			cr: logGroup(func(cr *svcapitypes.LogGroup) { cr.Spec.ForProvider.RetentionInDays = nil }),
			mock: mock(nil, observed(func(lg *svcsdktypes.LogGroup) {
				lg.RetentionInDays = aws.Int32(7)
				lg.KmsKeyId = aws.String(testKMSARN)
				lg.DeletionProtectionEnabled = aws.Bool(true)
			})),
			want: want{obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}},
		},
		"FieldsDiffer": {
			cr: logGroup(func(cr *svcapitypes.LogGroup) {
				cr.Spec.ForProvider.DeletionProtectionEnabled = aws.Bool(true)
				cr.Spec.ForProvider.KMSKeyID = aws.String(testKMSKeyID)
			}),
			mock: mock(map[string]string{"team": "other"}, observed(func(lg *svcsdktypes.LogGroup) { lg.RetentionInDays = aws.Int32(7) })),
			want: want{obs: managed.ExternalObservation{
				ResourceExists: true,
				Diff:           "spec.forProvider fields differ from the observed state: retentionInDays, kmsKeyID, deletionProtectionEnabled, tags",
			}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			obs, err := newTestExternal(tc.mock).Observe(context.Background(), tc.cr)
			if err != nil {
				t.Fatalf("Observe(...): %v", err)
			}
			if diff := cmp.Diff(tc.want.obs, obs); diff != "" {
				t.Errorf("Observe(...): -want, +got:\n%s", diff)
			}
			in := tc.mock.Calls["DescribeLogGroups"][0].(*svcsdk.DescribeLogGroupsInput) //nolint:forcetypeassert // recorded by the mock
			if got := aws.ToString(in.LogGroupNamePrefix); got != testGroup {
				t.Errorf("LogGroupNamePrefix = %q, want %q", got, testGroup)
			}
			if !obs.ResourceExists {
				return
			}
			if got := tc.cr.GetCondition(xpv2.TypeReady).Reason; got != xpv2.ReasonAvailable {
				t.Errorf("Ready reason = %q, want %q", got, xpv2.ReasonAvailable)
			}
			if got := aws.ToString(tc.cr.Status.AtProvider.LogGroupARN); got != testARN {
				t.Errorf("status.atProvider.logGroupARN = %q, want %q", got, testARN)
			}
		})
	}
}

func TestDiffTags(t *testing.T) {
	add, remove := diffTags(
		map[string]string{"keep": "1", "change": "new", "add": "3"},
		map[string]string{"keep": "1", "change": "old", "remove": "4", "aws:cloudformation:stack-name": "s"},
	)
	if diff := cmp.Diff(map[string]string{"change": "new", "add": "3"}, add); diff != "" {
		t.Errorf("add: -want, +got:\n%s", diff)
	}
	if diff := cmp.Diff([]string{"remove"}, remove); diff != "" {
		t.Errorf("remove: -want, +got:\n%s", diff)
	}
}

func TestCreate(t *testing.T) {
	cr := logGroup(func(cr *svcapitypes.LogGroup) {
		cr.Spec.ForProvider.LogGroupClass = aws.String("INFREQUENT_ACCESS")
		cr.Spec.ForProvider.DeletionProtectionEnabled = aws.Bool(true)
	})
	m := &fake.MockClient{}
	if _, err := newTestExternal(m).Create(context.Background(), cr); err != nil {
		t.Fatalf("Create(...): %v", err)
	}
	want := &svcsdk.CreateLogGroupInput{
		LogGroupName:              aws.String(testGroup),
		LogGroupClass:             svcsdktypes.LogGroupClassInfrequentAccess,
		DeletionProtectionEnabled: aws.Bool(true),
		Tags:                      desiredTags(cr),
	}
	if diff := cmp.Diff(want, m.Calls["CreateLogGroup"][0], ignoreUnexported); diff != "" {
		t.Errorf("CreateLogGroup input: -want, +got:\n%s", diff)
	}
	got := m.Calls["CreateLogGroup"][0].(*svcsdk.CreateLogGroupInput) //nolint:forcetypeassert // recorded by the mock
	if got.Tags["crossplane-kind"] == "" || got.Tags["team"] != "search" {
		t.Errorf("CreateLogGroup tags = %v, want the spec and the external tags", got.Tags)
	}
	if n := len(m.Calls) - 1; n != 0 {
		t.Errorf("Create(...) made %d calls besides CreateLogGroup: %v", n, m.Calls)
	}
}

func TestUpdate(t *testing.T) {
	name := aws.String(testGroup)
	cases := map[string]struct {
		cr       *svcapitypes.LogGroup
		observed svcsdktypes.LogGroup
		tags     map[string]string
		want     map[string][]any
	}{
		"Retention": {
			cr:       logGroup(),
			observed: observed(func(lg *svcsdktypes.LogGroup) { lg.RetentionInDays = aws.Int32(7) }),
			want: map[string][]any{
				"PutRetentionPolicy": {&svcsdk.PutRetentionPolicyInput{LogGroupName: name, RetentionInDays: aws.Int32(30)}},
			},
		},
		"RetentionForever": {
			cr:       logGroup(func(cr *svcapitypes.LogGroup) { cr.Spec.ForProvider.RetentionInDays = aws.Int64(0) }),
			observed: observed(),
			want: map[string][]any{
				"DeleteRetentionPolicy": {&svcsdk.DeleteRetentionPolicyInput{LogGroupName: name}},
			},
		},
		"KMSKeyAndDeletionProtection": {
			cr: logGroup(func(cr *svcapitypes.LogGroup) {
				cr.Spec.ForProvider.KMSKeyID = aws.String(testKMSKeyID)
				cr.Spec.ForProvider.DeletionProtectionEnabled = aws.Bool(true)
			}),
			observed: observed(),
			want: map[string][]any{
				"AssociateKmsKey":               {&svcsdk.AssociateKmsKeyInput{LogGroupName: name, KmsKeyId: aws.String(testKMSKeyID)}},
				"PutLogGroupDeletionProtection": {&svcsdk.PutLogGroupDeletionProtectionInput{LogGroupIdentifier: name, DeletionProtectionEnabled: aws.Bool(true)}},
			},
		},
		"Tags": {
			cr:       logGroup(),
			observed: observed(),
			tags:     map[string]string{"team": "other", "old": "x"},
			want: map[string][]any{
				"UntagResource": {&svcsdk.UntagResourceInput{ResourceArn: aws.String(testARN), TagKeys: []string{"old"}}},
				// team changes, the external tags are new
				"TagResource": {&svcsdk.TagResourceInput{ResourceArn: aws.String(testARN), Tags: desiredTags(logGroup())}},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m := mock(tc.tags, tc.observed)
			e := newTestExternal(m)
			if _, err := e.Observe(context.Background(), tc.cr); err != nil {
				t.Fatalf("Observe(...): %v", err)
			}
			m.Calls = nil
			if _, err := e.Update(context.Background(), tc.cr); err != nil {
				t.Fatalf("Update(...): %v", err)
			}
			if diff := cmp.Diff(tc.want, m.Calls, ignoreUnexported); diff != "" {
				t.Errorf("Update(...) calls: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestUpdateWithoutObserve(t *testing.T) {
	if _, err := newTestExternal(&fake.MockClient{}).Update(context.Background(), logGroup()); err == nil {
		t.Error("Update(...) without Observe: want error")
	}
}

func TestDelete(t *testing.T) {
	cases := map[string]struct {
		err     error
		wantErr bool
	}{
		"Deleted":  {},
		"NotFound": {err: &smithy.GenericAPIError{Code: "ResourceNotFoundException"}},
		"Failed":   {err: &smithy.GenericAPIError{Code: "OperationAbortedException"}, wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m := &fake.MockClient{MockDeleteLogGroup: func(context.Context, *svcsdk.DeleteLogGroupInput) (*svcsdk.DeleteLogGroupOutput, error) {
				return &svcsdk.DeleteLogGroupOutput{}, tc.err
			}}
			_, err := newTestExternal(m).Delete(context.Background(), logGroup())
			if (err != nil) != tc.wantErr {
				t.Fatalf("Delete(...): err = %v, wantErr %v", err, tc.wantErr)
			}
			want := &svcsdk.DeleteLogGroupInput{LogGroupName: aws.String(testGroup)}
			if diff := cmp.Diff(want, m.Calls["DeleteLogGroup"][0], ignoreUnexported); diff != "" {
				t.Errorf("DeleteLogGroup input: -want, +got:\n%s", diff)
			}
		})
	}
}

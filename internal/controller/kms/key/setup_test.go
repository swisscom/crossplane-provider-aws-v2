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

package key

import (
	"context"
	"go/token"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/kms"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/smithy-go"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/kms/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/kms/fake"
)

const (
	opDescribeKey = "DescribeKey"

	testKeyID  = "1234abcd-12ab-34cd-56ef-1234567890ab"
	testKeyARN = "arn:aws:kms:eu-central-1:123456789012:key/" + testKeyID
	testPolicy = `{
  "Version": "2012-10-17",
  "Statement": [{"Sid": "root", "Effect": "Allow", "Principal": {"AWS": "arn:aws:iam::123456789012:root"}, "Action": "kms:*", "Resource": "*"}]
}`
	// testPolicyAWS is testPolicy as AWS returns it.
	testPolicyAWS = `{"Version":"2012-10-17","Statement":[{"Sid":"root","Effect":"Allow","Principal":{"AWS":"arn:aws:iam::123456789012:root"},"Action":"kms:*","Resource":"*"}]}`
)

var ignoreUnexported = cmp.FilterPath(func(p cmp.Path) bool {
	sf, ok := p.Index(-1).(cmp.StructField)
	return ok && !token.IsExported(sf.Name())
}, cmp.Ignore())

func key(f ...func(*svcapitypes.Key)) *svcapitypes.Key {
	cr := &svcapitypes.Key{ObjectMeta: metav1.ObjectMeta{Name: "my-key", Namespace: "ns"}}
	cr.SetGroupVersionKind(svcapitypes.KeyGroupVersionKind)
	meta.SetExternalName(cr, testKeyID)
	cr.Spec.ForProvider.Region = "eu-central-1"
	cr.Spec.ForProvider.Enabled = aws.Bool(true)
	for _, fn := range f {
		fn(cr)
	}
	return cr
}

// withAll sets every field that needs its own read operation, equal to
// what mock() returns.
func withAll(cr *svcapitypes.Key) {
	p := &cr.Spec.ForProvider
	p.Description = aws.String("app data")
	p.Policy = aws.String(testPolicy)
	p.Tags = []*svcapitypes.Tag{{TagKey: aws.String("team"), TagValue: aws.String("app")}}
	p.EnableKeyRotation = aws.Bool(true)
	p.RotationPeriodInDays = aws.Int64(180)
	p.KeySpec = aws.String("SYMMETRIC_DEFAULT")
	p.MultiRegion = aws.Bool(false)
}

func metadata(f ...func(*svcsdktypes.KeyMetadata)) *svcsdktypes.KeyMetadata {
	km := &svcsdktypes.KeyMetadata{
		KeyId:       aws.String(testKeyID),
		Arn:         aws.String(testKeyARN),
		Description: aws.String("app data"),
		Enabled:     true,
		KeyState:    svcsdktypes.KeyStateEnabled,
		KeySpec:     svcsdktypes.KeySpecSymmetricDefault,
		KeyUsage:    svcsdktypes.KeyUsageTypeEncryptDecrypt,
		Origin:      svcsdktypes.OriginTypeAwsKms,
		MultiRegion: aws.Bool(false),
	}
	for _, fn := range f {
		fn(km)
	}
	return km
}

// mock returns a client that describes km and returns the policy, tags and
// rotation of withAll().
func mock(km *svcsdktypes.KeyMetadata) *fake.MockClient {
	return &fake.MockClient{
		MockDescribeKey: func(context.Context, *svcsdk.DescribeKeyInput) (*svcsdk.DescribeKeyOutput, error) {
			return &svcsdk.DescribeKeyOutput{KeyMetadata: km}, nil
		},
		MockGetKeyPolicy: func(context.Context, *svcsdk.GetKeyPolicyInput) (*svcsdk.GetKeyPolicyOutput, error) {
			return &svcsdk.GetKeyPolicyOutput{Policy: aws.String(testPolicyAWS)}, nil
		},
		MockListResourceTags: func(context.Context, *svcsdk.ListResourceTagsInput) (*svcsdk.ListResourceTagsOutput, error) {
			return &svcsdk.ListResourceTagsOutput{Tags: []svcsdktypes.Tag{
				{TagKey: aws.String("team"), TagValue: aws.String("app")},
				{TagKey: aws.String("aws:cloudformation:stack-name"), TagValue: aws.String("s")},
			}}, nil
		},
		MockGetKeyRotationStatus: func(context.Context, *svcsdk.GetKeyRotationStatusInput) (*svcsdk.GetKeyRotationStatusOutput, error) {
			return &svcsdk.GetKeyRotationStatusOutput{KeyRotationEnabled: true, RotationPeriodInDays: aws.Int32(180)}, nil
		},
	}
}

func newTestExternal(m *fake.MockClient) *external {
	return newExternal(nil, m, []option{setupExternal})
}

func TestObserve(t *testing.T) {
	notFound := &smithy.GenericAPIError{Code: "NotFoundException"}
	type want struct {
		obs    managed.ExternalObservation
		err    string
		reason xpv2.ConditionReason
		ops    []string
	}
	cases := map[string]struct {
		cr   *svcapitypes.Key
		mock *fake.MockClient
		want want
	}{
		"NotFound": {
			cr: key(),
			mock: &fake.MockClient{MockDescribeKey: func(context.Context, *svcsdk.DescribeKeyInput) (*svcsdk.DescribeKeyOutput, error) {
				return nil, notFound
			}},
			want: want{ops: []string{opDescribeKey}},
		},
		"UpToDate": {
			cr:   key(withAll),
			mock: mock(metadata()),
			want: want{
				obs:    managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				reason: xpv2.ReasonAvailable,
				ops:    []string{opDescribeKey, "GetKeyPolicy", "ListResourceTags", "GetKeyRotationStatus"},
			},
		},
		"UnsetFieldsNotManaged": {
			cr:   key(func(cr *svcapitypes.Key) { cr.Spec.ForProvider.Enabled = nil }),
			mock: mock(metadata(func(km *svcsdktypes.KeyMetadata) { km.Enabled = false; km.KeyState = svcsdktypes.KeyStateDisabled })),
			want: want{
				obs:    managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				reason: xpv2.ReasonAvailable,
				ops:    []string{opDescribeKey},
			},
		},
		"FieldsDiffer": {
			cr: key(withAll, func(cr *svcapitypes.Key) {
				cr.Spec.ForProvider.Policy = aws.String(`{"Version":"2012-10-17","Statement":[]}`)
				cr.Spec.ForProvider.Tags = []*svcapitypes.Tag{{TagKey: aws.String("team"), TagValue: aws.String("other")}}
				cr.Spec.ForProvider.RotationPeriodInDays = aws.Int64(90)
			}),
			mock: mock(metadata(func(km *svcsdktypes.KeyMetadata) {
				km.Enabled = false
				km.KeyState = svcsdktypes.KeyStateDisabled
				km.Description = aws.String("old")
			})),
			want: want{
				obs: managed.ExternalObservation{
					ResourceExists: true,
					Diff:           "spec.forProvider fields differ from the observed state: enabled, description, policy, tags, rotationPeriodInDays",
				},
				reason: xpv2.ReasonAvailable,
				ops:    []string{opDescribeKey, "GetKeyPolicy", "ListResourceTags", "GetKeyRotationStatus"},
			},
		},
		"RotationOff": {
			cr:   key(func(cr *svcapitypes.Key) { cr.Spec.ForProvider.EnableKeyRotation = aws.Bool(false) }),
			mock: mock(metadata()),
			want: want{
				obs:    managed.ExternalObservation{ResourceExists: true, Diff: "spec.forProvider fields differ from the observed state: enableKeyRotation"},
				reason: xpv2.ReasonAvailable,
				ops:    []string{opDescribeKey, "GetKeyRotationStatus"},
			},
		},
		"ImmutableFieldDiffers": {
			cr:   key(func(cr *svcapitypes.Key) { cr.Spec.ForProvider.KeySpec = aws.String("RSA_2048") }),
			mock: mock(metadata()),
			want: want{
				err: `keySpec cannot be changed after creation: it is "SYMMETRIC_DEFAULT" in AWS but "RSA_2048" in the spec`,
				ops: []string{opDescribeKey},
			},
		},
		"PendingDeletion": {
			cr: key(),
			mock: mock(metadata(func(km *svcsdktypes.KeyMetadata) {
				km.Enabled = false
				km.KeyState = svcsdktypes.KeyStatePendingDeletion
			})),
			want: want{
				obs:    managed.ExternalObservation{ResourceExists: true, Diff: "the key is scheduled for deletion; spec.forProvider fields differ from the observed state: enabled"},
				reason: xpv2.ReasonUnavailable,
				ops:    []string{opDescribeKey},
			},
		},
		"PendingDeletionAndDeleted": {
			cr: key(func(cr *svcapitypes.Key) { cr.SetDeletionTimestamp(&metav1.Time{Time: time.Now()}) }),
			mock: mock(metadata(func(km *svcsdktypes.KeyMetadata) {
				km.Enabled = false
				km.KeyState = svcsdktypes.KeyStatePendingDeletion
			})),
			want: want{ops: []string{opDescribeKey}},
		},
		"PendingImport": {
			cr: key(),
			mock: mock(metadata(func(km *svcsdktypes.KeyMetadata) {
				km.Enabled = false
				km.KeyState = svcsdktypes.KeyStatePendingImport
			})),
			want: want{
				obs:    managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
				reason: xpv2.ReasonUnavailable,
				ops:    []string{opDescribeKey},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			obs, err := newTestExternal(tc.mock).Observe(context.Background(), tc.cr)
			if tc.want.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want.err) {
					t.Fatalf("Observe(...): error %v, want %q", err, tc.want.err)
				}
			} else if err != nil {
				t.Fatalf("Observe(...): %v", err)
			}
			if diff := cmp.Diff(tc.want.obs, obs); diff != "" {
				t.Errorf("Observe(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.ops, tc.mock.Ops); diff != "" {
				t.Errorf("operations: -want, +got:\n%s", diff)
			}
			in := tc.mock.Calls[opDescribeKey][0].(*svcsdk.DescribeKeyInput) //nolint:forcetypeassert // recorded by the mock
			if got := aws.ToString(in.KeyId); got != testKeyID {
				t.Errorf("DescribeKey KeyId = %q, want %q", got, testKeyID)
			}
			if got := tc.cr.GetCondition(xpv2.TypeReady).Reason; tc.want.reason != "" && got != tc.want.reason {
				t.Errorf("Ready reason = %q, want %q", got, tc.want.reason)
			}
		})
	}
}

func TestObserveRotationStatus(t *testing.T) {
	cr := key(withAll)
	if _, err := newTestExternal(mock(metadata())).Observe(context.Background(), cr); err != nil {
		t.Fatalf("Observe(...): %v", err)
	}
	o := cr.Status.AtProvider
	if !aws.ToBool(o.KeyRotationEnabled) || aws.ToInt64(o.RotationPeriodInDays) != 180 || aws.ToString(o.ARN) != testKeyARN {
		t.Errorf("status.atProvider = keyRotationEnabled %v, rotationPeriodInDays %v, arn %q", o.KeyRotationEnabled, o.RotationPeriodInDays, aws.ToString(o.ARN))
	}
}

func TestUpdate(t *testing.T) {
	cases := map[string]struct {
		cr   *svcapitypes.Key
		km   *svcsdktypes.KeyMetadata
		want []string
	}{
		"RestoreAndEnableFirst": {
			cr: key(withAll, func(cr *svcapitypes.Key) { cr.Spec.ForProvider.Description = aws.String("new") }),
			km: metadata(func(km *svcsdktypes.KeyMetadata) {
				km.Enabled = false
				km.KeyState = svcsdktypes.KeyStatePendingDeletion
			}),
			want: []string{"CancelKeyDeletion", "EnableKey", "UpdateKeyDescription"},
		},
		"DisableLast": {
			cr: key(withAll, func(cr *svcapitypes.Key) {
				cr.Spec.ForProvider.Enabled = aws.Bool(false)
				cr.Spec.ForProvider.RotationPeriodInDays = aws.Int64(365)
			}),
			km:   metadata(),
			want: []string{"EnableKeyRotation", "DisableKey"},
		},
		"PolicyAndTags": {
			cr: key(withAll, func(cr *svcapitypes.Key) {
				cr.Spec.ForProvider.Policy = aws.String(`{"Version":"2012-10-17","Statement":[]}`)
				cr.Spec.ForProvider.Tags = []*svcapitypes.Tag{{TagKey: aws.String("env"), TagValue: aws.String("prod")}}
			}),
			km:   metadata(),
			want: []string{"PutKeyPolicy", "UntagResource", "TagResource"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m := mock(tc.km)
			e := newTestExternal(m)
			if _, err := e.Observe(context.Background(), tc.cr); err != nil {
				t.Fatalf("Observe(...): %v", err)
			}
			m.Ops = nil
			if _, err := e.Update(context.Background(), tc.cr); err != nil {
				t.Fatalf("Update(...): %v", err)
			}
			if diff := cmp.Diff(tc.want, m.Ops); diff != "" {
				t.Errorf("operations: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestUpdateInputs(t *testing.T) {
	cr := key(withAll, func(cr *svcapitypes.Key) {
		cr.Spec.ForProvider.Policy = aws.String(`{"Version":"2012-10-17","Statement":[]}`)
		cr.Spec.ForProvider.BypassPolicyLockoutSafetyCheck = aws.Bool(true)
		cr.Spec.ForProvider.Tags = []*svcapitypes.Tag{{TagKey: aws.String("env"), TagValue: aws.String("prod")}}
		cr.Spec.ForProvider.RotationPeriodInDays = aws.Int64(365)
	})
	m := mock(metadata())
	e := newTestExternal(m)
	if _, err := e.Observe(context.Background(), cr); err != nil {
		t.Fatalf("Observe(...): %v", err)
	}
	if _, err := e.Update(context.Background(), cr); err != nil {
		t.Fatalf("Update(...): %v", err)
	}
	want := map[string][]any{
		"PutKeyPolicy": {&svcsdk.PutKeyPolicyInput{
			KeyId: aws.String(testKeyID), PolicyName: aws.String("default"),
			Policy: cr.Spec.ForProvider.Policy, BypassPolicyLockoutSafetyCheck: true,
		}},
		"UntagResource":     {&svcsdk.UntagResourceInput{KeyId: aws.String(testKeyID), TagKeys: []string{"team"}}},
		"TagResource":       {&svcsdk.TagResourceInput{KeyId: aws.String(testKeyID), Tags: []svcsdktypes.Tag{{TagKey: aws.String("env"), TagValue: aws.String("prod")}}}},
		"EnableKeyRotation": {&svcsdk.EnableKeyRotationInput{KeyId: aws.String(testKeyID), RotationPeriodInDays: aws.Int32(365)}},
	}
	for op, in := range want {
		if diff := cmp.Diff(in, m.Calls[op], ignoreUnexported); diff != "" {
			t.Errorf("%s: -want, +got:\n%s", op, diff)
		}
	}
}

func TestUpdateNotObserved(t *testing.T) {
	if _, err := newTestExternal(&fake.MockClient{}).Update(context.Background(), key()); err == nil {
		t.Error("Update(...) without Observe: want error")
	}
}

func TestCreate(t *testing.T) {
	cr := key(withAll)
	meta.SetExternalName(cr, "")
	m := &fake.MockClient{MockCreateKey: func(context.Context, *svcsdk.CreateKeyInput) (*svcsdk.CreateKeyOutput, error) {
		return &svcsdk.CreateKeyOutput{KeyMetadata: metadata()}, nil
	}}
	if _, err := newTestExternal(m).Create(context.Background(), cr); err != nil {
		t.Fatalf("Create(...): %v", err)
	}
	if got := meta.GetExternalName(cr); got != testKeyID {
		t.Errorf("external name = %q, want %q", got, testKeyID)
	}
	want := &svcsdk.CreateKeyInput{
		Description: aws.String("app data"),
		KeySpec:     svcsdktypes.KeySpecSymmetricDefault,
		MultiRegion: aws.Bool(false),
		Policy:      aws.String(testPolicy),
		Tags:        []svcsdktypes.Tag{{TagKey: aws.String("team"), TagValue: aws.String("app")}},
	}
	if diff := cmp.Diff([]any{want}, m.Calls["CreateKey"], ignoreUnexported); diff != "" {
		t.Errorf("CreateKey: -want, +got:\n%s", diff)
	}
}

func TestDelete(t *testing.T) {
	cr := key(func(cr *svcapitypes.Key) { cr.Spec.ForProvider.PendingWindowInDays = aws.Int64(7) })
	m := &fake.MockClient{}
	if _, err := newTestExternal(m).Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete(...): %v", err)
	}
	want := &svcsdk.ScheduleKeyDeletionInput{KeyId: aws.String(testKeyID), PendingWindowInDays: aws.Int32(7)}
	if diff := cmp.Diff([]any{want}, m.Calls["ScheduleKeyDeletion"], ignoreUnexported); diff != "" {
		t.Errorf("ScheduleKeyDeletion: -want, +got:\n%s", diff)
	}
}

func TestDiffTags(t *testing.T) {
	add, remove := diffTags(
		[]*svcapitypes.Tag{
			{TagKey: aws.String("keep"), TagValue: aws.String("1")},
			{TagKey: aws.String("change"), TagValue: aws.String("new")},
			{TagKey: aws.String("add"), TagValue: aws.String("3")},
		},
		[]svcsdktypes.Tag{
			{TagKey: aws.String("keep"), TagValue: aws.String("1")},
			{TagKey: aws.String("change"), TagValue: aws.String("old")},
			{TagKey: aws.String("remove"), TagValue: aws.String("4")},
			{TagKey: aws.String("aws:cloudformation:stack-name"), TagValue: aws.String("s")},
		},
	)
	wantAdd := []svcsdktypes.Tag{
		{TagKey: aws.String("add"), TagValue: aws.String("3")},
		{TagKey: aws.String("change"), TagValue: aws.String("new")},
	}
	if diff := cmp.Diff(wantAdd, add, ignoreUnexported); diff != "" {
		t.Errorf("add: -want, +got:\n%s", diff)
	}
	if diff := cmp.Diff([]string{"remove"}, remove); diff != "" {
		t.Errorf("remove: -want, +got:\n%s", diff)
	}
}

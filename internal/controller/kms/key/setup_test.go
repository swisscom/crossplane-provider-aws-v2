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
	"sort"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/kms"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/kms/v1alpha1"
)

const (
	testKeyID  = "1234abcd-12ab-34cd-56ef-1234567890ab"
	testPolicy = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::111122223333:root"},"Action":"kms:*","Resource":"*"}]}`
)

var ignoreUnexported = cmpopts.IgnoreUnexported(svcsdktypes.Tag{})

// fakeClient panics on operations a test doesn't expect.
type fakeClient struct {
	Client
	description string
	policy      string
	rotation    bool
	tags        []svcsdktypes.Tag

	calls             []string
	tagged            []svcsdktypes.Tag
	untagged          []string
	scheduledDeletion *svcsdk.ScheduleKeyDeletionInput
}

func (c *fakeClient) DescribeKey(context.Context, *svcsdk.DescribeKeyInput, ...func(*svcsdk.Options)) (*svcsdk.DescribeKeyOutput, error) {
	return &svcsdk.DescribeKeyOutput{KeyMetadata: &svcsdktypes.KeyMetadata{Description: aws.String(c.description)}}, nil
}

func (c *fakeClient) GetKeyPolicy(context.Context, *svcsdk.GetKeyPolicyInput, ...func(*svcsdk.Options)) (*svcsdk.GetKeyPolicyOutput, error) {
	return &svcsdk.GetKeyPolicyOutput{Policy: aws.String(c.policy)}, nil
}

func (c *fakeClient) GetKeyRotationStatus(context.Context, *svcsdk.GetKeyRotationStatusInput, ...func(*svcsdk.Options)) (*svcsdk.GetKeyRotationStatusOutput, error) {
	return &svcsdk.GetKeyRotationStatusOutput{KeyRotationEnabled: c.rotation}, nil
}

func (c *fakeClient) ListResourceTags(context.Context, *svcsdk.ListResourceTagsInput, ...func(*svcsdk.Options)) (*svcsdk.ListResourceTagsOutput, error) {
	return &svcsdk.ListResourceTagsOutput{Tags: c.tags}, nil
}

func (c *fakeClient) UpdateKeyDescription(context.Context, *svcsdk.UpdateKeyDescriptionInput, ...func(*svcsdk.Options)) (*svcsdk.UpdateKeyDescriptionOutput, error) {
	c.calls = append(c.calls, "UpdateKeyDescription")
	return &svcsdk.UpdateKeyDescriptionOutput{}, nil
}

func (c *fakeClient) PutKeyPolicy(context.Context, *svcsdk.PutKeyPolicyInput, ...func(*svcsdk.Options)) (*svcsdk.PutKeyPolicyOutput, error) {
	c.calls = append(c.calls, "PutKeyPolicy")
	return &svcsdk.PutKeyPolicyOutput{}, nil
}

func (c *fakeClient) EnableKeyRotation(context.Context, *svcsdk.EnableKeyRotationInput, ...func(*svcsdk.Options)) (*svcsdk.EnableKeyRotationOutput, error) {
	c.calls = append(c.calls, "EnableKeyRotation")
	return &svcsdk.EnableKeyRotationOutput{}, nil
}

func (c *fakeClient) DisableKeyRotation(context.Context, *svcsdk.DisableKeyRotationInput, ...func(*svcsdk.Options)) (*svcsdk.DisableKeyRotationOutput, error) {
	c.calls = append(c.calls, "DisableKeyRotation")
	return &svcsdk.DisableKeyRotationOutput{}, nil
}

func (c *fakeClient) TagResource(_ context.Context, in *svcsdk.TagResourceInput, _ ...func(*svcsdk.Options)) (*svcsdk.TagResourceOutput, error) {
	c.calls = append(c.calls, "TagResource")
	c.tagged = in.Tags
	return &svcsdk.TagResourceOutput{}, nil
}

func (c *fakeClient) UntagResource(_ context.Context, in *svcsdk.UntagResourceInput, _ ...func(*svcsdk.Options)) (*svcsdk.UntagResourceOutput, error) {
	c.calls = append(c.calls, "UntagResource")
	c.untagged = in.TagKeys
	return &svcsdk.UntagResourceOutput{}, nil
}

func (c *fakeClient) EnableKey(context.Context, *svcsdk.EnableKeyInput, ...func(*svcsdk.Options)) (*svcsdk.EnableKeyOutput, error) {
	c.calls = append(c.calls, "EnableKey")
	return &svcsdk.EnableKeyOutput{}, nil
}

func (c *fakeClient) DisableKey(context.Context, *svcsdk.DisableKeyInput, ...func(*svcsdk.Options)) (*svcsdk.DisableKeyOutput, error) {
	c.calls = append(c.calls, "DisableKey")
	return &svcsdk.DisableKeyOutput{}, nil
}

func (c *fakeClient) ScheduleKeyDeletion(_ context.Context, in *svcsdk.ScheduleKeyDeletionInput, _ ...func(*svcsdk.Options)) (*svcsdk.ScheduleKeyDeletionOutput, error) {
	c.scheduledDeletion = in
	return &svcsdk.ScheduleKeyDeletionOutput{}, nil
}

func tag(k, v string) svcsdktypes.Tag {
	return svcsdktypes.Tag{TagKey: aws.String(k), TagValue: aws.String(v)}
}

func specTag(k, v string) *svcapitypes.Tag {
	return &svcapitypes.Tag{TagKey: aws.String(k), TagValue: aws.String(v)}
}

type keyOption func(*svcapitypes.Key)

func key(opts ...keyOption) *svcapitypes.Key {
	cr := &svcapitypes.Key{}
	meta.SetExternalName(cr, testKeyID)
	cr.Spec.ForProvider.Policy = aws.String(testPolicy)
	cr.Spec.ForProvider.Enabled = aws.Bool(true)
	cr.Status.AtProvider.Enabled = aws.Bool(true)
	for _, o := range opts {
		o(cr)
	}
	return cr
}

func TestDiffTags(t *testing.T) {
	cases := map[string]struct {
		spec       []*svcapitypes.Tag
		current    []svcsdktypes.Tag
		wantAdd    []svcsdktypes.Tag
		wantRemove []string
	}{
		"UpToDate": {
			spec:    []*svcapitypes.Tag{specTag("a", "1")},
			current: []svcsdktypes.Tag{tag("a", "1")},
		},
		"AddMissing": {
			spec:    []*svcapitypes.Tag{specTag("a", "1"), specTag("b", "2")},
			current: []svcsdktypes.Tag{tag("a", "1")},
			wantAdd: []svcsdktypes.Tag{tag("b", "2")},
		},
		"ChangedValueIsOnlyAdded": {
			spec:    []*svcapitypes.Tag{specTag("a", "new")},
			current: []svcsdktypes.Tag{tag("a", "old")},
			wantAdd: []svcsdktypes.Tag{tag("a", "new")},
		},
		"RemoveNotInSpec": {
			current:    []svcsdktypes.Tag{tag("a", "1")},
			wantRemove: []string{"a"},
		},
		"IgnoreAWSTags": {
			spec:    []*svcapitypes.Tag{specTag("aws:x", "1")},
			current: []svcsdktypes.Tag{tag("AWS:cloudformation:stack-name", "s")},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			add, remove := diffTags(tc.spec, tc.current)
			if diff := cmp.Diff(tc.wantAdd, add, ignoreUnexported, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("add: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantRemove, remove, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("remove: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestPostObserve(t *testing.T) {
	cases := map[string]struct {
		state      svcsdktypes.KeyState
		wantExists bool
		wantCond   xpv2.Condition
	}{
		"Enabled":         {state: svcsdktypes.KeyStateEnabled, wantExists: true, wantCond: xpv2.Available()},
		"Disabled":        {state: svcsdktypes.KeyStateDisabled, wantExists: true, wantCond: xpv2.Unavailable()},
		"PendingDeletion": {state: svcsdktypes.KeyStatePendingDeletion, wantExists: false, wantCond: xpv2.Deleting()},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := key()
			obs, err := postObserve(context.Background(), cr,
				&svcsdk.DescribeKeyOutput{KeyMetadata: &svcsdktypes.KeyMetadata{KeyState: tc.state}},
				managed.ExternalObservation{ResourceExists: true}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if obs.ResourceExists != tc.wantExists {
				t.Errorf("ResourceExists: want %v, got %v", tc.wantExists, obs.ResourceExists)
			}
			if diff := cmp.Diff(tc.wantCond, cr.GetCondition(tc.wantCond.Type), cmpopts.IgnoreFields(xpv2.Condition{}, "LastTransitionTime")); diff != "" {
				t.Errorf("condition: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestIsUpToDate(t *testing.T) {
	cases := map[string]struct {
		cr     *svcapitypes.Key
		client *fakeClient
		want   bool
	}{
		"UpToDate": {
			cr:     key(func(cr *svcapitypes.Key) { cr.Spec.ForProvider.Tags = []*svcapitypes.Tag{specTag("a", "1")} }),
			client: &fakeClient{policy: testPolicy, tags: []svcsdktypes.Tag{tag("a", "1"), tag("aws:x", "y")}},
			want:   true,
		},
		"PolicyDiffers": {
			cr:     key(),
			client: &fakeClient{policy: `{"Version":"2012-10-17","Statement":[]}`},
		},
		"RotationDiffers": {
			cr:     key(func(cr *svcapitypes.Key) { cr.Spec.ForProvider.EnableKeyRotation = aws.Bool(true) }),
			client: &fakeClient{policy: testPolicy},
		},
		"TagsDiffer": {
			cr:     key(),
			client: &fakeClient{policy: testPolicy, tags: []svcsdktypes.Tag{tag("a", "1")}},
		},
		"EnabledDiffers": {
			cr:     key(func(cr *svcapitypes.Key) { cr.Status.AtProvider.Enabled = aws.Bool(false) }),
			client: &fakeClient{policy: testPolicy},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := &hooks{client: tc.client}
			got, _, err := h.isUpToDate(context.Background(), tc.cr, &svcsdk.DescribeKeyOutput{KeyMetadata: &svcsdktypes.KeyMetadata{}})
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	cases := map[string]struct {
		cr         *svcapitypes.Key
		client     *fakeClient
		wantCalls  []string
		wantTagged []svcsdktypes.Tag
	}{
		"OnlyChangedFields": {
			cr: key(func(cr *svcapitypes.Key) {
				cr.Spec.ForProvider.Description = aws.String("same")
				cr.Spec.ForProvider.Tags = []*svcapitypes.Tag{specTag("a", "new")}
				cr.Status.AtProvider.Enabled = aws.Bool(false)
			}),
			client:     &fakeClient{description: "same", policy: testPolicy, tags: []svcsdktypes.Tag{tag("a", "old"), tag("aws:x", "y")}},
			wantCalls:  []string{"TagResource", "EnableKey"},
			wantTagged: []svcsdktypes.Tag{tag("a", "new")},
		},
		"DescriptionPolicyRotation": {
			cr: key(func(cr *svcapitypes.Key) {
				cr.Spec.ForProvider.Description = aws.String("new")
				cr.Spec.ForProvider.EnableKeyRotation = aws.Bool(true)
			}),
			client:    &fakeClient{description: "old", policy: `{"Version":"2012-10-17","Statement":[]}`},
			wantCalls: []string{"UpdateKeyDescription", "PutKeyPolicy", "EnableKeyRotation"},
		},
		"NothingChangedCallsNoUpdate": {
			cr:     key(func(cr *svcapitypes.Key) { cr.Spec.ForProvider.Description = aws.String("same") }),
			client: &fakeClient{description: "same", policy: testPolicy},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := (&hooks{client: tc.client}).update(context.Background(), tc.cr); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.wantCalls, tc.client.calls, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("calls: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantTagged, tc.client.tagged, ignoreUnexported, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("tagged: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestUpdateRemovesTags(t *testing.T) {
	c := &fakeClient{tags: []svcsdktypes.Tag{tag("b", "1"), tag("a", "1")}}
	if err := (&hooks{client: c}).updateTags(context.Background(), key()); err != nil {
		t.Fatal(err)
	}
	sort.Strings(c.untagged)
	if diff := cmp.Diff([]string{"a", "b"}, c.untagged); diff != "" {
		t.Errorf("untagged: -want, +got:\n%s", diff)
	}
}

func TestDelete(t *testing.T) {
	cases := map[string]struct {
		cr   *svcapitypes.Key
		want *svcsdk.ScheduleKeyDeletionInput
	}{
		"Schedule": {
			cr:   key(func(cr *svcapitypes.Key) { cr.Spec.ForProvider.PendingWindowInDays = aws.Int64(7) }),
			want: &svcsdk.ScheduleKeyDeletionInput{KeyId: aws.String(testKeyID), PendingWindowInDays: aws.Int32(7)},
		},
		"AlreadyScheduledIsNotCalled": {
			cr: key(func(cr *svcapitypes.Key) { cr.Status.AtProvider.DeletionDate = &metav1.Time{} }),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := &fakeClient{}
			if _, err := (&hooks{client: c}).delete(context.Background(), tc.cr); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, c.scheduledDeletion, cmpopts.IgnoreUnexported(svcsdk.ScheduleKeyDeletionInput{})); diff != "" {
				t.Errorf("-want, +got:\n%s", diff)
			}
		})
	}
}

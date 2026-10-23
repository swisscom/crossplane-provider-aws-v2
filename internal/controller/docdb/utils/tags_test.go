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
	"testing"

	svcsdk "github.com/aws/aws-sdk-go-v2/service/docdb"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/aws/smithy-go"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"k8s.io/utils/ptr"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	fake "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/docdb/fake"
)

func specTags(kv ...string) []*svcapitypes.Tag {
	var out []*svcapitypes.Tag
	for i := 0; i < len(kv); i += 2 {
		out = append(out, &svcapitypes.Tag{Key: ptr.To(kv[i]), Value: ptr.To(kv[i+1])})
	}
	return out
}

func awsTags(kv ...string) []svcsdktypes.Tag {
	var out []svcsdktypes.Tag
	for i := 0; i < len(kv); i += 2 {
		out = append(out, svcsdktypes.Tag{Key: ptr.To(kv[i]), Value: ptr.To(kv[i+1])})
	}
	return out
}

func TestDiffTags(t *testing.T) {
	cases := map[string]struct {
		spec       []*svcapitypes.Tag
		current    []svcsdktypes.Tag
		managed    []string
		wantAdd    []svcsdktypes.Tag
		wantRemove []string
	}{
		"NilSpecRemovesNothingUnmanaged": {current: awsTags("cost-center", "42", "backup-plan", "daily")},
		"NewTag":                         {spec: specTags("a", "1"), wantAdd: awsTags("a", "1")},
		"ValueChangeIsAddOnly":           {spec: specTags("a", "2"), current: awsTags("a", "1"), managed: []string{"a"}, wantAdd: awsTags("a", "2")},
		"UpToDate":                       {spec: specTags("a", "1"), current: awsTags("a", "1", "other", "x"), managed: []string{"a"}},
		"DroppedManagedKey":              {spec: specTags("a", "1"), current: awsTags("a", "1", "b", "2"), managed: []string{"a", "b"}, wantRemove: []string{"b"}},
		"NilSpecRemovesManaged":          {current: awsTags("b", "2", "backup-plan", "daily"), managed: []string{"b"}, wantRemove: []string{"b"}},
		"ManagedKeyAlreadyGone":          {current: awsTags("x", "1"), managed: []string{"b"}},
		"AWSKeysNeverRemoved":            {current: awsTags("aws:cloudformation:stack", "s"), managed: []string{"aws:cloudformation:stack"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			add, remove := DiffTags(tc.spec, tc.current, tc.managed)
			if diff := cmp.Diff(tc.wantAdd, add, cmpopts.EquateEmpty(), cmpopts.IgnoreUnexported(svcsdktypes.Tag{})); diff != "" {
				t.Errorf("add: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantRemove, remove, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("remove: -want, +got:\n%s", diff)
			}
		})
	}
}

func listing(kv ...string) *fake.MockDocDBClient {
	return &fake.MockDocDBClient{
		MockListTagsForResource: func(context.Context, *svcsdk.ListTagsForResourceInput, ...func(*svcsdk.Options)) (*svcsdk.ListTagsForResourceOutput, error) {
			return &svcsdk.ListTagsForResourceOutput{TagList: awsTags(kv...)}, nil
		},
	}
}

func cluster(recorded string, policies ...xpv2.ManagementAction) *svcapitypes.DBCluster {
	cr := &svcapitypes.DBCluster{}
	if recorded != "" {
		cr.SetAnnotations(map[string]string{AnnotationManagedTagKeys: recorded})
	}
	cr.SetManagementPolicies(policies)
	return cr
}

func TestObserveTags(t *testing.T) {
	cases := map[string]struct {
		cr           *svcapitypes.DBCluster
		spec         []*svcapitypes.Tag
		aws          []string
		wantUpToDate bool
		wantChanged  bool
		wantRecord   string
	}{
		"RecordsSpecKeysOnly": {
			cr: cluster(""), spec: specTags("a", "1"), aws: []string{"a", "1", "aws:x", "y", "backup-plan", "d"},
			wantUpToDate: true, wantChanged: true, wantRecord: "a",
		},
		"RecordsBeforeAdding": {
			cr: cluster(""), spec: specTags("a", "1", "b", "2"), aws: []string{"a", "1"},
			wantChanged: true, wantRecord: "a,b",
		},
		"KeepsDroppedKeyUntilRemoved": {
			cr: cluster("a,b"), spec: specTags("a", "1"), aws: []string{"a", "1", "b", "2"},
			wantRecord: "a,b",
		},
		"PrunesKeyGoneFromAWS": {
			cr: cluster("a,b"), spec: specTags("a", "1"), aws: []string{"a", "1"},
			wantUpToDate: true, wantChanged: true, wantRecord: "a",
		},
		"NilSpecForgetsRemovedKeys": {
			cr: cluster("b"), aws: []string{"other", "x"},
			wantUpToDate: true, wantChanged: true,
		},
		"AWSKeyInSpecNotRecorded": {
			cr: cluster(""), spec: specTags("aws:x", "y"), aws: []string{"aws:x", "y"},
			wantUpToDate: true,
		},
		"NoLateInitNoRecord": {
			cr: cluster("", xpv2.ManagementActionObserve, xpv2.ManagementActionUpdate), spec: specTags("a", "1"), aws: []string{"a", "1"},
			wantUpToDate: true,
		},
		"NoLateInitNoRemoval": {
			cr: cluster("a,b", xpv2.ManagementActionObserve, xpv2.ManagementActionUpdate), spec: specTags("a", "1"), aws: []string{"a", "1", "b", "2"},
			wantUpToDate: true, wantRecord: "a,b",
		},
		"LateInitPolicyRecords": {
			cr: cluster("", xpv2.ManagementActionObserve, xpv2.ManagementActionLateInitialize), spec: specTags("a", "1"), aws: []string{"a", "1"},
			wantUpToDate: true, wantChanged: true, wantRecord: "a",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			upToDate, changed, err := ObserveTags(context.Background(), listing(tc.aws...), tc.cr, tc.spec, ptr.To("arn"))
			if err != nil {
				t.Fatal(err)
			}
			if upToDate != tc.wantUpToDate || changed != tc.wantChanged {
				t.Errorf("ObserveTags(...) = %v, %v; want %v, %v", upToDate, changed, tc.wantUpToDate, tc.wantChanged)
			}
			if got := tc.cr.GetAnnotations()[AnnotationManagedTagKeys]; got != tc.wantRecord {
				t.Errorf("record = %q, want %q", got, tc.wantRecord)
			}
		})
	}
}

func TestUpdateTagsForResource(t *testing.T) {
	c := listing("a", "1", "b", "2", "backup-plan", "d")
	cr := cluster("a,b")

	if err := UpdateTagsForResource(context.Background(), c, cr, specTags("a", "9", "c", "3"), ptr.To("arn")); err != nil {
		t.Fatal(err)
	}

	wantAdd := &svcsdk.AddTagsToResourceInput{ResourceName: ptr.To("arn"), Tags: awsTags("a", "9", "c", "3")}
	wantRemove := &svcsdk.RemoveTagsFromResourceInput{ResourceName: ptr.To("arn"), TagKeys: []string{"b"}}
	opts := []cmp.Option{cmpopts.IgnoreUnexported(svcsdk.AddTagsToResourceInput{}, svcsdk.RemoveTagsFromResourceInput{}, svcsdktypes.Tag{})}
	if len(c.Called.AddTagsToResource) != 1 || len(c.Called.RemoveTagsFromResource) != 1 {
		t.Fatalf("calls = %+v, want one add and one remove", c.Called)
	}
	if diff := cmp.Diff(wantAdd, c.Called.AddTagsToResource[0].I, opts...); diff != "" {
		t.Errorf("add: -want, +got:\n%s", diff)
	}
	if diff := cmp.Diff(wantRemove, c.Called.RemoveTagsFromResource[0].I, opts...); diff != "" {
		t.Errorf("remove: -want, +got:\n%s", diff)
	}
}

func TestTagErrorsAreWrapped(t *testing.T) {
	apiErr := &smithy.GenericAPIError{Code: "AccessDenied", Message: "no"}
	fail := &fake.MockDocDBClient{
		MockListTagsForResource: func(context.Context, *svcsdk.ListTagsForResourceInput, ...func(*svcsdk.Options)) (*svcsdk.ListTagsForResourceOutput, error) {
			return nil, apiErr
		},
	}
	_, _, err := ObserveTags(context.Background(), fail, cluster(""), nil, nil)
	if want := "cannot list tags: AccessDenied: no"; err == nil || err.Error() != want {
		t.Errorf("ObserveTags(...) error = %v, want %q", err, want)
	}

	c := listing("b", "2")
	c.MockAddTagsToResource = func(context.Context, *svcsdk.AddTagsToResourceInput, ...func(*svcsdk.Options)) (*svcsdk.AddTagsToResourceOutput, error) {
		return nil, apiErr
	}
	c.MockRemoveTagsFromResource = func(context.Context, *svcsdk.RemoveTagsFromResourceInput, ...func(*svcsdk.Options)) (*svcsdk.RemoveTagsFromResourceOutput, error) {
		return nil, apiErr
	}
	err = UpdateTagsForResource(context.Background(), c, cluster("b"), specTags("a", "1"), nil)
	if want := "cannot create tags: AccessDenied: no"; err == nil || err.Error() != want {
		t.Errorf("UpdateTagsForResource(...) add error = %v, want %q", err, want)
	}
	err = UpdateTagsForResource(context.Background(), c, cluster("b"), nil, nil)
	if want := "cannot remove tags: AccessDenied: no"; err == nil || err.Error() != want {
		t.Errorf("UpdateTagsForResource(...) remove error = %v, want %q", err, want)
	}
	if err := UpdateTagsForResource(context.Background(), fail, cluster(""), nil, nil); err == nil {
		t.Error("UpdateTagsForResource(...) ignored a list error")
	}
}

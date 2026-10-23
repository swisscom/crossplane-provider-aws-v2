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

package dbcluster

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/docdb"
	docdbtypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	fake "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/docdb/fake"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
)

const managedTagKeys = "docdb.aws2.crossplane.io/managed-tag-keys"

// tagClient serves an available cluster with ARN "arn" and the given tags.
func tagClient(tags map[string]string) *fake.MockDocDBClient {
	c := updateClient()
	c.MockListTagsForResource = func(context.Context, *docdb.ListTagsForResourceInput, ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
		out := &docdb.ListTagsForResourceOutput{}
		for k, v := range tags {
			out.TagList = append(out.TagList, docdbtypes.Tag{Key: ptr.To(k), Value: ptr.To(v)})
		}
		return out, nil
	}
	return c
}

func tag(k, v string) *svcapitypes.Tag { return &svcapitypes.Tag{Key: ptr.To(k), Value: ptr.To(v)} }

// Tags from tag policies, AWS Backup or the console are not ours to remove.
func TestObserveRecordsOnlySpecTags(t *testing.T) {
	c := tagClient(map[string]string{"a": "1", "aws:x": "y", "backup-plan": "daily"})
	e := newExternal(nil, c, []option{setupExternal})
	cr := instance(withExternalName(testDBClusterIdentifier), withTags(tag("a", "1")))

	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatalf("Observe(...): %v", err)
	}

	if !obs.ResourceUpToDate || !obs.ResourceLateInitialized {
		t.Errorf("Observe(...) = up to date %v, late-initialized %v, diff %q; want both true", obs.ResourceUpToDate, obs.ResourceLateInitialized, obs.Diff)
	}
	if got := cr.GetAnnotations()[managedTagKeys]; got != "a" {
		t.Errorf("managed tag keys = %q, want %q", got, "a")
	}
	if len(c.Called.RemoveTagsFromResource) != 0 {
		t.Errorf("Observe(...) removed tags")
	}
}

func TestUpdateRemovesOnlyRecordedKeys(t *testing.T) {
	c := tagClient(map[string]string{"a": "1", "b": "2", "backup-plan": "daily"})
	c.MockModifyDBCluster = func(context.Context, *docdb.ModifyDBClusterInput, ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error) {
		return &docdb.ModifyDBClusterOutput{}, nil
	}
	e := newExternal(nil, c, []option{setupExternal})
	cr := instance(withExternalName(testDBClusterIdentifier), withTags(tag("b", "2")), withManagedTagKeys("a,b"))

	obs, err := e.Observe(context.Background(), cr)
	if err != nil || obs.ResourceUpToDate || obs.Diff != compare.Message([]string{"tags"}) {
		t.Fatalf("Observe(...) = up to date %v, diff %q, %v; want the tags diff", obs.ResourceUpToDate, obs.Diff, err)
	}
	// The late-init update replaces the in-memory status with the stored one.
	cr.Status.AtProvider = svcapitypes.DBClusterObservation{}

	if _, err := e.Update(context.Background(), cr); err != nil {
		t.Fatalf("Update(...): %v", err)
	}
	want := []*docdb.RemoveTagsFromResourceInput{{ResourceName: ptr.To(testDBClusterArn), TagKeys: []string{"a"}}}
	got := make([]*docdb.RemoveTagsFromResourceInput, 0, len(c.Called.RemoveTagsFromResource))
	for _, call := range c.Called.RemoveTagsFromResource {
		got = append(got, call.I)
	}
	if diff := cmp.Diff(want, got, ignoreUnexported); diff != "" {
		t.Errorf("RemoveTagsFromResource: -want, +got:\n%s", diff)
	}
	if len(c.Called.AddTagsToResource) != 0 {
		t.Errorf("Update(...) added tags that were already set")
	}
}

// Without LateInitialize the runtime never persists the record, so it
// cannot be trusted and nothing is removed.
func TestNoLateInitPolicyNeverRemovesTags(t *testing.T) {
	c := tagClient(map[string]string{"a": "1", "b": "2"})
	e := newExternal(nil, c, []option{setupExternal})
	cr := instance(withExternalName(testDBClusterIdentifier), withTags(tag("a", "1")), withManagedTagKeys("a,b"))
	cr.SetManagementPolicies(xpv2.ManagementPolicies{xpv2.ManagementActionObserve, xpv2.ManagementActionCreate, xpv2.ManagementActionUpdate, xpv2.ManagementActionDelete})

	obs, err := e.Observe(context.Background(), cr)
	if err != nil || !obs.ResourceUpToDate {
		t.Fatalf("Observe(...) = up to date %v, diff %q, %v; want up to date", obs.ResourceUpToDate, obs.Diff, err)
	}
	if _, err := e.Update(context.Background(), cr); err != nil {
		t.Fatalf("Update(...): %v", err)
	}
	if len(c.Called.RemoveTagsFromResource) != 0 {
		t.Errorf("Update(...) removed tags")
	}
}

// The runtime persists the record only if Observe reports a late-init, even
// when nothing else in the spec changed.
func TestObserveRequestsPersistingTagRecord(t *testing.T) {
	c := tagClient(map[string]string{"a": "1"})
	e := newExternal(nil, c, []option{setupExternal})
	cr := instance(withExternalName(testDBClusterIdentifier), withTags(tag("a", "1")))
	cr.Spec.ForProvider.VPCSecurityGroupIDs = []*string{}

	obs, err := e.Observe(context.Background(), cr)
	if err != nil || !obs.ResourceUpToDate {
		t.Fatalf("Observe(...) = %+v, %v; want up to date", obs, err)
	}
	if !obs.ResourceLateInitialized {
		t.Error("Observe(...) did not ask to persist the new tag record")
	}

	// Once recorded, nothing is left to persist.
	obs, err = e.Observe(context.Background(), cr)
	if err != nil || obs.ResourceLateInitialized {
		t.Errorf("Observe(...) again = late-initialized %v, %v; want false", obs.ResourceLateInitialized, err)
	}
}

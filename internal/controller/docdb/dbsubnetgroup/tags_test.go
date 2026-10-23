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

package dbsubnetgroup

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/docdb"
	docdbtypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	fake "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/docdb/fake"
	svcutils "github.com/swisscom/crossplane-provider-aws-v2/internal/controller/docdb/utils"
)

func TestTagsOwnership(t *testing.T) {
	c := &fake.MockDocDBClient{
		MockDescribeDBSubnetGroups: func(context.Context, *docdb.DescribeDBSubnetGroupsInput, ...func(*docdb.Options)) (*docdb.DescribeDBSubnetGroupsOutput, error) {
			return &docdb.DescribeDBSubnetGroupsOutput{DBSubnetGroups: []docdbtypes.DBSubnetGroup{{DBSubnetGroupName: ptr.To("subnets"), DBSubnetGroupArn: ptr.To("arn"), SubnetGroupStatus: ptr.To("Complete")}}}, nil
		},
		MockListTagsForResource: func(context.Context, *docdb.ListTagsForResourceInput, ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
			return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{
				{Key: ptr.To("a"), Value: ptr.To("1")}, {Key: ptr.To("b"), Value: ptr.To("2")}, {Key: ptr.To("backup-plan"), Value: ptr.To("daily")},
			}}, nil
		},
	}
	e := newExternal(nil, c, []option{setupExternal})
	cr := &svcapitypes.DBSubnetGroup{}
	meta.SetExternalName(cr, "subnets")
	cr.Spec.ForProvider.Tags = []*svcapitypes.Tag{{Key: ptr.To("a"), Value: ptr.To("1")}}

	// The first Observe records only the spec key and asks for it to be persisted.
	obs, err := e.Observe(context.Background(), cr)
	if err != nil || !obs.ResourceUpToDate || !obs.ResourceLateInitialized {
		t.Fatalf("Observe(...) = %+v, %v; want up to date and late-initialized", obs, err)
	}
	if got := cr.GetAnnotations()[svcutils.AnnotationManagedTagKeys]; got != "a" {
		t.Fatalf("managed tag keys = %q, want %q", got, "a")
	}

	// Dropping a recorded key removes exactly that key.
	meta.AddAnnotations(cr, map[string]string{svcutils.AnnotationManagedTagKeys: "a,b"})
	cr.Spec.ForProvider.Tags = []*svcapitypes.Tag{{Key: ptr.To("a"), Value: ptr.To("1")}}
	if obs, err := e.Observe(context.Background(), cr); err != nil || obs.ResourceUpToDate {
		t.Fatalf("Observe(...) = %+v, %v; want the tags diff", obs, err)
	}
	// The late-init update replaces the in-memory status with the stored one.
	cr.Status.AtProvider = svcapitypes.DBSubnetGroupObservation{}
	if _, err := e.Update(context.Background(), cr); err != nil {
		t.Fatalf("Update(...): %v", err)
	}
	want := []*docdb.RemoveTagsFromResourceInput{{ResourceName: ptr.To("arn"), TagKeys: []string{"b"}}}
	got := make([]*docdb.RemoveTagsFromResourceInput, 0, len(c.Called.RemoveTagsFromResource))
	for _, call := range c.Called.RemoveTagsFromResource {
		got = append(got, call.I)
	}
	if diff := cmp.Diff(want, got, ignoreUnexported); diff != "" {
		t.Errorf("RemoveTagsFromResource: -want, +got:\n%s", diff)
	}
}

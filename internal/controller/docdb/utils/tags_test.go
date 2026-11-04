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
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
)

func TestDiffTags(t *testing.T) {
	spec := []*svcapitypes.Tag{
		{Key: aws.String("keep"), Value: aws.String("1")},
		{Key: aws.String("change"), Value: aws.String("new")},
		{Key: aws.String("add"), Value: aws.String("3")},
	}
	current := []svcsdktypes.Tag{
		{Key: aws.String("keep"), Value: aws.String("1")},
		{Key: aws.String("change"), Value: aws.String("old")},
		{Key: aws.String("remove"), Value: aws.String("4")},
		{Key: aws.String("aws:cloudformation:stack-name"), Value: aws.String("s")},
		{Key: aws.String("AWS:other"), Value: aws.String("s")},
	}
	add, remove := DiffTags(spec, current)

	wantAdd := []svcsdktypes.Tag{
		{Key: aws.String("change"), Value: aws.String("new")},
		{Key: aws.String("add"), Value: aws.String("3")},
	}
	opts := []cmp.Option{cmpopts.IgnoreUnexported(svcsdktypes.Tag{}), cmpopts.SortSlices(func(a, b string) bool { return a < b })}
	if diff := cmp.Diff(wantAdd, add, opts...); diff != "" {
		t.Errorf("add: -want, +got:\n%s", diff)
	}
	// AWS tags are neither removed nor drift.
	if diff := cmp.Diff([]string{"change", "remove"}, remove, opts...); diff != "" {
		t.Errorf("remove: -want, +got:\n%s", diff)
	}

	add, remove = DiffTags(nil, current[3:])
	if len(add)+len(remove) != 0 {
		t.Errorf("DiffTags(nil, AWS tags) = %v, %v; want no changes", add, remove)
	}
}

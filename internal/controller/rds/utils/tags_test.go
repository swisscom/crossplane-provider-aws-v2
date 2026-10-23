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
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
)

func specTags(kv ...string) []*svcapitypes.Tag {
	var out []*svcapitypes.Tag
	for i := 0; i < len(kv); i += 2 {
		out = append(out, &svcapitypes.Tag{Key: aws.String(kv[i]), Value: aws.String(kv[i+1])})
	}
	return out
}

func awsTags(kv ...string) []svcsdktypes.Tag {
	var out []svcsdktypes.Tag
	for i := 0; i < len(kv); i += 2 {
		out = append(out, svcsdktypes.Tag{Key: aws.String(kv[i]), Value: aws.String(kv[i+1])})
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
		"New":                 {spec: specTags("a", "1"), wantAdd: awsTags("a", "1")},
		"Changed":             {spec: specTags("a", "2"), current: awsTags("a", "1"), managed: []string{"a"}, wantAdd: awsTags("a", "2")},
		"RecordedRemoved":     {current: awsTags("a", "1"), managed: []string{"a"}, wantRemove: []string{"a"}},
		"ForeignKept":         {current: awsTags("backup-plan", "daily")},
		"RecordedAlreadyGone": {current: awsTags("x", "1"), managed: []string{"a"}},
		"AWSKeyNeverRemoved":  {current: awsTags("aws:x", "1"), managed: []string{"aws:x"}},
		"Unchanged":           {spec: specTags("a", "1"), current: awsTags("a", "1"), managed: []string{"a"}},
		"Mixed": {
			spec:       specTags("keep", "1", "change", "new", "add", "x"),
			current:    awsTags("keep", "1", "change", "old", "drop", "y", "foreign", "z"),
			managed:    []string{"add", "change", "drop", "keep"},
			wantAdd:    awsTags("change", "new", "add", "x"),
			wantRemove: []string{"drop"},
		},
		"DuplicateSpecKeyLastWins": {
			spec:    specTags("a", "2", "a", "3"),
			current: awsTags("a", "1"),
			wantAdd: awsTags("a", "3"),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			add, remove := DiffTags(tc.spec, tc.current, tc.managed)
			sortTags := cmpopts.SortSlices(func(a, b svcsdktypes.Tag) bool { return aws.ToString(a.Key) < aws.ToString(b.Key) })
			if diff := cmp.Diff(tc.wantAdd, add, cmpopts.EquateEmpty(), sortTags, cmpopts.IgnoreUnexported(svcsdktypes.Tag{})); diff != "" {
				t.Errorf("add: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantRemove, remove, cmpopts.EquateEmpty(), cmpopts.SortSlices(func(a, b string) bool { return a < b })); diff != "" {
				t.Errorf("remove: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestShouldIgnore(t *testing.T) {
	rules := []string{"", "*", "c7n:*", "exact"}
	cases := map[string]bool{"c7n:policy": true, "exact": true, "exactly": false, "other": false}
	for key, want := range cases {
		if got := ShouldIgnore(key, rules); got != want {
			t.Errorf("ShouldIgnore(%q) = %v, want %v", key, got, want)
		}
	}
}

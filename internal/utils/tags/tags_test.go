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

package tags

import (
	"testing"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
)

const ann = "example.aws2.crossplane.io/managed-tag-keys"

func mr(recorded string, policies ...xpv2.ManagementAction) *svcapitypes.DBCluster {
	cr := &svcapitypes.DBCluster{}
	if recorded != "" {
		cr.SetAnnotations(map[string]string{ann: recorded})
	}
	cr.SetManagementPolicies(policies)
	return cr
}

func TestRecord(t *testing.T) {
	cases := map[string]struct {
		mr            *svcapitypes.DBCluster
		desired       []string
		current       []string
		wantChanged   bool
		wantRecord    string
		wantRemovable []string
	}{
		"FirstObserveRecordsDesiredOnly": {
			mr: mr(""), desired: []string{"a"}, current: []string{"a", "foreign", "aws:x"},
			wantChanged: true, wantRecord: "a", wantRemovable: []string{"a"},
		},
		"RecordsBeforeAdding": {
			mr: mr(""), desired: []string{"b", "a"}, current: nil,
			wantChanged: true, wantRecord: "a,b", wantRemovable: []string{"a", "b"},
		},
		"KeepsDroppedKeyUntilGone": {
			mr: mr("a,b"), desired: []string{"a"}, current: []string{"a", "b"},
			wantRecord: "a,b", wantRemovable: []string{"a", "b"},
		},
		"PrunesKeyGoneFromAWS": {
			mr: mr("a,b"), desired: []string{"a"}, current: []string{"a"},
			wantChanged: true, wantRecord: "a", wantRemovable: []string{"a"},
		},
		"EmptyRecordDropsAnnotation": {
			mr: mr("b"), current: []string{"other"}, wantChanged: true,
		},
		"AWSAndEmptyKeysNeverRecorded": {
			mr: mr(""), desired: []string{"aws:x", ""}, current: []string{"aws:x"},
		},
		"NoLateInitNoRecord": {
			mr: mr("", xpv2.ManagementActionObserve, xpv2.ManagementActionUpdate), desired: []string{"a"},
		},
		"NoLateInitNothingRemovable": {
			mr: mr("a,b", xpv2.ManagementActionObserve, xpv2.ManagementActionUpdate), desired: []string{"a"}, current: []string{"a", "b"},
			wantRecord: "a,b",
		},
		"LateInitPolicyRecords": {
			mr: mr("", xpv2.ManagementActionObserve, xpv2.ManagementActionLateInitialize), desired: []string{"a"},
			wantChanged: true, wantRecord: "a", wantRemovable: []string{"a"},
		},
		"AllPolicyRecords": {
			mr: mr("", xpv2.ManagementActionAll), desired: []string{"a"},
			wantChanged: true, wantRecord: "a", wantRemovable: []string{"a"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Record(tc.mr, ann, tc.desired, tc.current); got != tc.wantChanged {
				t.Errorf("Record(...) = %v, want %v", got, tc.wantChanged)
			}
			if got := tc.mr.GetAnnotations()[ann]; got != tc.wantRecord {
				t.Errorf("record = %q, want %q", got, tc.wantRecord)
			}
			if diff := cmp.Diff(tc.wantRemovable, Removable(tc.mr, ann), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("Removable: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestIsAWS(t *testing.T) {
	for key, want := range map[string]bool{"aws:cloudformation:stack": true, "aws": false, "team": false, "": false} {
		if got := IsAWS(key); got != want {
			t.Errorf("IsAWS(%q) = %v, want %v", key, got, want)
		}
	}
}

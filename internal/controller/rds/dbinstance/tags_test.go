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

package dbinstance

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds/fake"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/controller/rds/utils"
)

const (
	tagTeam        = "team"
	tagBackup      = "backup-plan"
	recordGoneTeam = "gone,team"
	daily          = "daily"
)

// tagStore is a fake AWS tag store for one instance.
type tagStore struct {
	tags    map[string]string
	adds    [][]svcsdktypes.Tag
	removed []string
}

func (s *tagStore) tagList() []svcsdktypes.Tag {
	out := make([]svcsdktypes.Tag, 0, len(s.tags))
	for k, v := range s.tags {
		out = append(out, svcsdktypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return out
}

func (s *tagStore) client(modifyOut *svcsdk.ModifyDBInstanceOutput) *fake.MockRDSClient {
	return &fake.MockRDSClient{
		MockDescribeDBInstances: func(context.Context, *svcsdk.DescribeDBInstancesInput, ...func(*svcsdk.Options)) (*svcsdk.DescribeDBInstancesOutput, error) {
			return describeOut("available", func(db *svcsdktypes.DBInstance) { db.TagList = s.tagList() }), nil
		},
		MockModifyDBInstance: func(context.Context, *svcsdk.ModifyDBInstanceInput, ...func(*svcsdk.Options)) (*svcsdk.ModifyDBInstanceOutput, error) {
			return modifyOut, nil
		},
		MockAddTagsToResource: func(_ context.Context, in *svcsdk.AddTagsToResourceInput, _ ...func(*svcsdk.Options)) (*svcsdk.AddTagsToResourceOutput, error) {
			s.adds = append(s.adds, in.Tags)
			for _, tg := range in.Tags {
				s.tags[aws.ToString(tg.Key)] = aws.ToString(tg.Value)
			}
			return &svcsdk.AddTagsToResourceOutput{}, nil
		},
		MockRemoveTagsFromResource: func(_ context.Context, in *svcsdk.RemoveTagsFromResourceInput, _ ...func(*svcsdk.Options)) (*svcsdk.RemoveTagsFromResourceOutput, error) {
			s.removed = append(s.removed, in.TagKeys...)
			for _, k := range in.TagKeys {
				delete(s.tags, k)
			}
			return &svcsdk.RemoveTagsFromResourceOutput{}, nil
		},
	}
}

func specTags(kv ...string) []*svcapitypes.Tag {
	var out []*svcapitypes.Tag
	for i := 0; i < len(kv); i += 2 {
		out = append(out, &svcapitypes.Tag{Key: aws.String(kv[i]), Value: aws.String(kv[i+1])})
	}
	return out
}

func TestTagOwnership(t *testing.T) {
	cases := map[string]struct {
		recorded   string
		policies   []xpv2.ManagementAction
		spec       []*svcapitypes.Tag
		aws        map[string]string
		wantTags   map[string]string
		wantAdds   int
		wantRemove []string
		wantRecord string
		// wantUpToDate is about the tags only.
		wantUpToDate bool
	}{
		// The first Observe after the upgrade: no record yet.
		"UpgradeRecordsSpecKeysOnly": {
			spec:         specTags(tagTeam, "a"),
			aws:          map[string]string{tagTeam: "a", tagBackup: daily, "dropped-before-upgrade": "x", "aws:cloudformation:stack": "s"},
			wantTags:     map[string]string{tagTeam: "a", tagBackup: daily, "dropped-before-upgrade": "x", "aws:cloudformation:stack": "s"},
			wantUpToDate: true,
			wantRecord:   tagTeam,
		},
		"ForeignTagSurvivesNewKey": {
			recorded:   tagTeam,
			spec:       specTags(tagTeam, "a", "env", "prod"),
			aws:        map[string]string{tagTeam: "a", tagBackup: daily},
			wantTags:   map[string]string{tagTeam: "a", "env": "prod", tagBackup: daily},
			wantAdds:   1,
			wantRecord: "env,team",
		},
		"RecordedKeyDroppedFromSpecIsRemoved": {
			recorded:   recordGoneTeam,
			spec:       specTags(tagTeam, "a"),
			aws:        map[string]string{tagTeam: "a", "gone": "x", tagBackup: daily},
			wantTags:   map[string]string{tagTeam: "a", tagBackup: daily},
			wantRemove: []string{"gone"},
			wantRecord: recordGoneTeam,
		},
		"ValueChangeIsOneAddNoRemove": {
			recorded:   tagTeam,
			spec:       specTags(tagTeam, "new"),
			aws:        map[string]string{tagTeam: oldValue},
			wantTags:   map[string]string{tagTeam: "new"},
			wantAdds:   1,
			wantRecord: tagTeam,
		},
		"UnsetTagsRemoveRecordedKeys": {
			recorded:   tagTeam,
			aws:        map[string]string{tagTeam: "a", tagBackup: daily},
			wantTags:   map[string]string{tagBackup: daily},
			wantRemove: []string{tagTeam},
			wantRecord: tagTeam,
		},
		"NoLateInitPolicyRemovesNothing": {
			recorded:   recordGoneTeam,
			policies:   []xpv2.ManagementAction{xpv2.ManagementActionObserve, xpv2.ManagementActionUpdate},
			spec:       specTags(tagTeam, "a", "env", "prod"),
			aws:        map[string]string{tagTeam: "a", "gone": "x"},
			wantTags:   map[string]string{tagTeam: "a", "env": "prod", "gone": "x"},
			wantAdds:   1,
			wantRecord: recordGoneTeam,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := &tagStore{tags: tc.aws}
			cr := newTestCR(func(cr *svcapitypes.DBInstance) {
				cr.Spec.ForProvider.Tags = tc.spec
				cr.SetManagementPolicies(tc.policies)
				if tc.recorded != "" {
					meta.AddAnnotations(cr, map[string]string{utils.AnnotationManagedTagKeys: tc.recorded})
				}
			})
			e := newCustomExternal(newTestKube(t), store.client(&svcsdk.ModifyDBInstanceOutput{}))
			obs, err := e.Observe(context.Background(), cr)
			if err != nil {
				t.Fatal(err)
			}
			if got := !strings.Contains(obs.Diff, "tag(s)"); got != tc.wantUpToDate {
				t.Errorf("Observe() tags up to date = %v, want %v; diff: %s", got, tc.wantUpToDate, obs.Diff)
			}
			if got := cr.GetAnnotations()[utils.AnnotationManagedTagKeys]; got != tc.wantRecord {
				t.Errorf("record after Observe = %q, want %q", got, tc.wantRecord)
			}
			if _, err := e.Update(context.Background(), cr); err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(store.tags, tc.wantTags) {
				t.Errorf("tags after Update = %v, want %v", store.tags, tc.wantTags)
			}
			if len(store.adds) != tc.wantAdds {
				t.Errorf("AddTagsToResource calls = %d, want %d", len(store.adds), tc.wantAdds)
			}
			if !slices.Equal(store.removed, tc.wantRemove) {
				t.Errorf("removed tag keys = %v, want %v", store.removed, tc.wantRemove)
			}
		})
	}
}

// A changed record must be reported as a late-init, so that the runtime
// persists it before Update adds the tag.
func TestTagRecordChangeLateInitializes(t *testing.T) {
	store := &tagStore{tags: map[string]string{tagTeam: "a"}}
	cr := newTestCR(func(cr *svcapitypes.DBInstance) { cr.Spec.ForProvider.Tags = specTags(tagTeam, "a") })
	e := newCustomExternal(newTestKube(t), store.client(&svcsdk.ModifyDBInstanceOutput{}))
	observe := func() bool {
		t.Helper()
		obs, err := e.Observe(context.Background(), cr)
		if err != nil {
			t.Fatal(err)
		}
		return obs.ResourceLateInitialized
	}
	observe() // late-inits the spec

	if observe() {
		t.Fatal("Observe with an unchanged record: want no late-init")
	}
	cr.Spec.ForProvider.Tags = specTags(tagTeam, "a", "env", "prod")
	if !observe() {
		t.Error("Observe of a new spec key: want late-init so the record persists")
	}
	if got := cr.GetAnnotations()[utils.AnnotationManagedTagKeys]; got != "env,team" {
		t.Errorf("record = %q, want %q", got, "env,team")
	}
}

func TestUpdateTagsARNAndErrors(t *testing.T) {
	errBoom := errors.New("boom")
	cases := map[string]struct {
		describeARN *string
		modifyOut   *svcsdk.ModifyDBInstanceOutput
		addErr      error
		removeErr   error
		wantARN     string
		wantErr     string
	}{
		"ARNFromModifyOutput": {
			modifyOut: &svcsdk.ModifyDBInstanceOutput{DBInstance: &svcsdktypes.DBInstance{DBInstanceArn: aws.String("arn:from-modify")}},
			wantARN:   "arn:from-modify",
		},
		"ARNFallsBackToStatus": {
			describeARN: aws.String(testARN),
			modifyOut:   &svcsdk.ModifyDBInstanceOutput{},
			wantARN:     testARN,
		},
		"NoARNAnywhere": {
			modifyOut: &svcsdk.ModifyDBInstanceOutput{},
			wantErr:   errNoARNForTags,
		},
		"AddFails": {
			describeARN: aws.String(testARN),
			modifyOut:   &svcsdk.ModifyDBInstanceOutput{},
			addErr:      errBoom,
			wantARN:     testARN,
			wantErr:     errAddTags + ": boom",
		},
		"RemoveFails": {
			describeARN: aws.String(testARN),
			modifyOut:   &svcsdk.ModifyDBInstanceOutput{},
			removeErr:   errBoom,
			wantARN:     testARN,
			wantErr:     errRemoveTags + ": boom",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var gotARNs []string
			rdsc := &fake.MockRDSClient{
				MockDescribeDBInstances: describeFn(describeOut("available", func(db *svcsdktypes.DBInstance) {
					db.DBInstanceArn = tc.describeARN
					db.TagList = awsTagList(tagTeam, oldValue, "gone", "x")
				}), nil),
				MockModifyDBInstance: func(context.Context, *svcsdk.ModifyDBInstanceInput, ...func(*svcsdk.Options)) (*svcsdk.ModifyDBInstanceOutput, error) {
					return tc.modifyOut, nil
				},
				MockAddTagsToResource: func(_ context.Context, in *svcsdk.AddTagsToResourceInput, _ ...func(*svcsdk.Options)) (*svcsdk.AddTagsToResourceOutput, error) {
					gotARNs = append(gotARNs, aws.ToString(in.ResourceName))
					return &svcsdk.AddTagsToResourceOutput{}, tc.addErr
				},
				MockRemoveTagsFromResource: func(_ context.Context, in *svcsdk.RemoveTagsFromResourceInput, _ ...func(*svcsdk.Options)) (*svcsdk.RemoveTagsFromResourceOutput, error) {
					gotARNs = append(gotARNs, aws.ToString(in.ResourceName))
					return &svcsdk.RemoveTagsFromResourceOutput{}, tc.removeErr
				},
			}
			cr := newTestCR(func(cr *svcapitypes.DBInstance) {
				cr.Spec.ForProvider.Tags = []*svcapitypes.Tag{{Key: aws.String(tagTeam), Value: aws.String("new")}}
				meta.AddAnnotations(cr, map[string]string{utils.AnnotationManagedTagKeys: recordGoneTeam})
			})
			e := newCustomExternal(newTestKube(t), rdsc)
			if _, err := e.Observe(context.Background(), cr); err != nil {
				t.Fatal(err)
			}
			_, err := e.Update(context.Background(), cr)
			if got := errString(err); got != tc.wantErr {
				t.Fatalf("Update error = %q, want %q", got, tc.wantErr)
			}
			for _, arn := range gotARNs {
				if arn != tc.wantARN {
					t.Errorf("tag call ResourceName = %q, want %q", arn, tc.wantARN)
				}
			}
		})
	}
}

func awsTagList(kv ...string) []svcsdktypes.Tag {
	var out []svcsdktypes.Tag
	for i := 0; i < len(kv); i += 2 {
		out = append(out, svcsdktypes.Tag{Key: aws.String(kv[i]), Value: aws.String(kv[i+1])})
	}
	return out
}

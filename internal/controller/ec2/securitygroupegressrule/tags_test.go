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

package securitygroupegressrule

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/ec2/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/ec2/fake"
	svcutils "github.com/swisscom/crossplane-provider-aws-v2/internal/controller/ec2/utils"
)

// tagStore is a fake AWS tag store for one rule.
type tagStore struct {
	tags    map[string]string
	creates int
	deleted []string
}

func (s *tagStore) client() *fake.MockEC2Client {
	return &fake.MockEC2Client{
		MockDescribeSecurityGroupRules: func(ctx context.Context, in *svcsdk.DescribeSecurityGroupRulesInput) (*svcsdk.DescribeSecurityGroupRulesOutput, error) {
			return observedRule(func(r *svcsdktypes.SecurityGroupRule) {
				r.Tags = nil
				for k, v := range s.tags {
					r.Tags = append(r.Tags, svcsdktypes.Tag{Key: aws.String(k), Value: aws.String(v)})
				}
			})(ctx, in)
		},
		MockCreateTags: func(_ context.Context, in *svcsdk.CreateTagsInput) (*svcsdk.CreateTagsOutput, error) {
			s.creates++
			for _, tg := range in.Tags {
				s.tags[aws.ToString(tg.Key)] = aws.ToString(tg.Value)
			}
			return &svcsdk.CreateTagsOutput{}, nil
		},
		MockDeleteTags: func(_ context.Context, in *svcsdk.DeleteTagsInput) (*svcsdk.DeleteTagsOutput, error) {
			for _, tg := range in.Tags {
				s.deleted = append(s.deleted, aws.ToString(tg.Key))
				delete(s.tags, aws.ToString(tg.Key))
			}
			return &svcsdk.DeleteTagsOutput{}, nil
		},
	}
}

// crossplaneTags are the tags Crossplane sets on rule().
func crossplaneTags() map[string]string {
	out, _ := observedRule()(context.Background(), &svcsdk.DescribeSecurityGroupRulesInput{SecurityGroupRuleIds: []string{testRule}})
	m := map[string]string{}
	for _, t := range out.SecurityGroupRules[0].Tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return m
}

// awsTags returns the Crossplane tags of rule() plus kv.
func awsTags(kv ...string) map[string]string {
	m := crossplaneTags()
	for i := 0; i < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return m
}

// record returns the record of rule() with the Crossplane tags plus keys.
func record(keys ...string) string {
	keys = append(keys, slices.Collect(maps.Keys(crossplaneTags()))...)
	slices.Sort(keys)
	return strings.Join(keys, ",")
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
		recorded     string
		policies     []xpv2.ManagementAction
		spec         []*svcapitypes.Tag
		aws          map[string]string
		wantTags     map[string]string
		wantCreates  int
		wantDeleted  []string
		wantRecord   string
		wantLateInit bool
		wantUpToDate bool
	}{
		// The first Observe after the upgrade: no record yet.
		"UpgradeRecordsDesiredKeysOnly": {
			spec:         specTags("team", "a"),
			aws:          awsTags("team", "a", "backup-plan", "daily", "dropped-before-upgrade", "x", "aws:cloudformation:stack-name", "s"),
			wantTags:     awsTags("team", "a", "backup-plan", "daily", "dropped-before-upgrade", "x", "aws:cloudformation:stack-name", "s"),
			wantUpToDate: true,
			wantRecord:   record("team"),
			wantLateInit: true,
		},
		"ForeignTagSurvivesNewKey": {
			recorded:     record("team"),
			spec:         specTags("team", "a", "env", "prod"),
			aws:          awsTags("team", "a", "backup-plan", "daily"),
			wantTags:     awsTags("team", "a", "env", "prod", "backup-plan", "daily"),
			wantCreates:  1,
			wantRecord:   record("env", "team"),
			wantLateInit: true,
		},
		"RecordedKeyDroppedFromSpecIsRemoved": {
			recorded:    record("gone", "team"),
			spec:        specTags("team", "a"),
			aws:         awsTags("team", "a", "gone", "x", "backup-plan", "daily"),
			wantTags:    awsTags("team", "a", "backup-plan", "daily"),
			wantDeleted: []string{"gone"},
			wantRecord:  record("gone", "team"),
		},
		"ValueChangeIsOneCreateNoDelete": {
			recorded:    record("team"),
			spec:        specTags("team", "new"),
			aws:         awsTags("team", "old"),
			wantTags:    awsTags("team", "new"),
			wantCreates: 1,
			wantRecord:  record("team"),
		},
		"UnsetTagsRemoveRecordedKeys": {
			recorded:    record("team"),
			aws:         awsTags("team", "a", "backup-plan", "daily"),
			wantTags:    awsTags("backup-plan", "daily"),
			wantDeleted: []string{"team"},
			wantRecord:  record("team"),
		},
		"NoLateInitPolicyRemovesNothing": {
			recorded:    record("gone", "team"),
			policies:    []xpv2.ManagementAction{xpv2.ManagementActionObserve, xpv2.ManagementActionUpdate},
			spec:        specTags("team", "a", "env", "prod"),
			aws:         awsTags("team", "a", "gone", "x"),
			wantTags:    awsTags("team", "a", "env", "prod", "gone", "x"),
			wantCreates: 1,
			wantRecord:  record("gone", "team"),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := &tagStore{tags: tc.aws}
			cr := rule(withExternalName, func(cr *svcapitypes.SecurityGroupEgressRule) {
				cr.Spec.ForProvider.Tags = tc.spec
				cr.SetManagementPolicies(tc.policies)
				if tc.recorded != "" {
					meta.AddAnnotations(cr, map[string]string{svcutils.AnnotationManagedTagKeys: tc.recorded})
				}
			})
			e := newTestExternal(store.client())
			obs, err := e.Observe(context.Background(), cr)
			if err != nil {
				t.Fatal(err)
			}
			if obs.ResourceUpToDate != tc.wantUpToDate {
				t.Errorf("Observe() upToDate = %v, want %v (%s)", obs.ResourceUpToDate, tc.wantUpToDate, obs.Diff)
			}
			if obs.ResourceLateInitialized != tc.wantLateInit {
				t.Errorf("Observe() lateInit = %v, want %v", obs.ResourceLateInitialized, tc.wantLateInit)
			}
			if got := cr.GetAnnotations()[svcutils.AnnotationManagedTagKeys]; got != tc.wantRecord {
				t.Errorf("record after Observe = %q, want %q", got, tc.wantRecord)
			}
			// The late-init update resets the status to the stored one.
			cr.Status.AtProvider.Tags = nil
			if _, err := e.Update(context.Background(), cr); err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(store.tags, tc.wantTags) {
				t.Errorf("tags after Update = %v, want %v", store.tags, tc.wantTags)
			}
			if store.creates != tc.wantCreates {
				t.Errorf("CreateTags calls = %d, want %d", store.creates, tc.wantCreates)
			}
			if !slices.Equal(store.deleted, tc.wantDeleted) {
				t.Errorf("deleted tag keys = %v, want %v", store.deleted, tc.wantDeleted)
			}
		})
	}
}

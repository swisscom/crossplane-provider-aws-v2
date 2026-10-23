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

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/smithy-go"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/ec2/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/ec2/fake"
)

func apiTags(kv ...string) []*svcapitypes.Tag {
	var out []*svcapitypes.Tag
	for i := 0; i < len(kv); i += 2 {
		out = append(out, &svcapitypes.Tag{Key: aws.String(kv[i]), Value: aws.String(kv[i+1])})
	}
	return out
}

func withRecord(keys string) *svcapitypes.SecurityGroup {
	cr := &svcapitypes.SecurityGroup{}
	if keys != "" {
		meta.AddAnnotations(cr, map[string]string{AnnotationManagedTagKeys: keys})
	}
	return cr
}

func TestObserveTags(t *testing.T) {
	cases := map[string]struct {
		cr           *svcapitypes.SecurityGroup
		desired      map[string]string
		current      []*svcapitypes.Tag
		wantUpToDate bool
		wantChanged  bool
		wantRecord   string
	}{
		"ForeignTagIsUpToDate": {
			cr: withRecord(""), desired: map[string]string{"a": "1"}, current: apiTags("a", "1", "foreign", "x"),
			wantUpToDate: true, wantChanged: true, wantRecord: "a",
		},
		"RecordedKeyToRemove": {
			cr: withRecord("a,b"), desired: map[string]string{"a": "1"}, current: apiTags("a", "1", "b", "2"),
			wantRecord: "a,b",
		},
		"MissingKey": {
			cr: withRecord("a"), desired: map[string]string{"a": "1", "c": "3"}, current: apiTags("a", "1"),
			wantChanged: true, wantRecord: "a,c",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			upToDate, changed := ObserveTags(tc.cr, tc.desired, tc.current)
			if upToDate != tc.wantUpToDate || changed != tc.wantChanged {
				t.Errorf("ObserveTags(...) = %v, %v; want %v, %v", upToDate, changed, tc.wantUpToDate, tc.wantChanged)
			}
			if got := tc.cr.GetAnnotations()[AnnotationManagedTagKeys]; got != tc.wantRecord {
				t.Errorf("record = %q, want %q", got, tc.wantRecord)
			}
		})
	}
}

func TestUpdateTagsErrors(t *testing.T) {
	boom := &smithy.GenericAPIError{Code: "UnauthorizedOperation", Message: "no"}
	cases := map[string]struct {
		mock *fake.MockEC2Client
		want string
	}{
		"Delete": {
			mock: &fake.MockEC2Client{MockDeleteTags: func(context.Context, *svcsdk.DeleteTagsInput) (*svcsdk.DeleteTagsOutput, error) { return nil, boom }},
			want: "cannot delete tags: UnauthorizedOperation: no",
		},
		"Create": {
			mock: &fake.MockEC2Client{MockCreateTags: func(context.Context, *svcsdk.CreateTagsInput) (*svcsdk.CreateTagsOutput, error) { return nil, boom }},
			want: "cannot create tags: UnauthorizedOperation: no",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := UpdateTags(context.Background(), tc.mock, withRecord("b"), "sg-1", map[string]string{"a": "1"}, apiTags("b", "2"))
			if err == nil || err.Error() != tc.want {
				t.Errorf("UpdateTags(...) error = %v, want %q", err, tc.want)
			}
		})
	}
}

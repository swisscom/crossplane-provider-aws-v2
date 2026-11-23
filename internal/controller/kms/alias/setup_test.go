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

package alias

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/kms"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/google/go-cmp/cmp"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/kms/v1alpha1"
)

const testKeyID = "1234abcd-12ab-34cd-56ef-1234567890ab"

type pagedClient struct {
	Client
	pages  [][]string
	inputs []*svcsdk.ListAliasesInput
}

func (c *pagedClient) ListAliases(_ context.Context, in *svcsdk.ListAliasesInput, _ ...func(*svcsdk.Options)) (*svcsdk.ListAliasesOutput, error) {
	c.inputs = append(c.inputs, in)
	i := len(c.inputs) - 1
	out := &svcsdk.ListAliasesOutput{}
	for _, n := range c.pages[i] {
		out.Aliases = append(out.Aliases, svcsdktypes.AliasListEntry{AliasName: aws.String(n)})
	}
	if i < len(c.pages)-1 {
		out.Truncated = true
		out.NextMarker = aws.String("next")
	}
	return out, nil
}

func names(out *svcsdk.ListAliasesOutput) []string {
	r := make([]string, 0, len(out.Aliases))
	for _, a := range out.Aliases {
		r = append(r, aws.ToString(a.AliasName))
	}
	return r
}

func alias(name string, target *string) *svcapitypes.Alias {
	cr := &svcapitypes.Alias{}
	meta.SetExternalName(cr, name)
	cr.Spec.ForProvider.TargetKeyID = target
	return cr
}

func TestListAllAliases(t *testing.T) {
	c := &pagedClient{pages: [][]string{{"alias/a", "alias/b"}, {"alias/c"}}}
	out, err := (&listAllAliases{Client: c}).ListAliases(context.Background(), &svcsdk.ListAliasesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"alias/a", "alias/b", "alias/c"}, names(out)); diff != "" {
		t.Errorf("-want, +got:\n%s", diff)
	}
	if got := aws.ToString(c.inputs[1].Marker); got != "next" {
		t.Errorf("second page marker: want next, got %q", got)
	}
}

func TestFilterList(t *testing.T) {
	list := &svcsdk.ListAliasesOutput{Aliases: []svcsdktypes.AliasListEntry{
		{AliasName: aws.String("alias/other")},
		{AliasName: aws.String("alias/mine")},
	}}
	if diff := cmp.Diff([]string{"alias/mine"}, names(filterList(alias("mine", nil), list))); diff != "" {
		t.Errorf("-want, +got:\n%s", diff)
	}
	if got := names(filterList(alias("missing", nil), list)); len(got) != 0 {
		t.Errorf("want no alias, got %v", got)
	}
}

func TestIsUpToDate(t *testing.T) {
	cases := map[string]struct {
		target *string
		want   bool
	}{
		"SameKeyID":  {target: aws.String(testKeyID), want: true},
		"SameKeyARN": {target: aws.String("arn:aws:kms:eu-central-1:111122223333:key/" + testKeyID), want: true},
		"OtherKey":   {target: aws.String("0987dcba-09fe-87dc-65ba-ab0987654321")},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resp := &svcsdk.ListAliasesOutput{Aliases: []svcsdktypes.AliasListEntry{
				{AliasName: aws.String("alias/a"), TargetKeyId: aws.String(testKeyID)},
			}}
			got, diff, err := isUpToDate(context.Background(), alias("a", tc.target), resp)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("want %v, got %v (%s)", tc.want, got, diff)
			}
		})
	}
}

func TestPreCreate(t *testing.T) {
	in := &svcsdk.CreateAliasInput{}
	if err := preCreate(context.Background(), alias("a", aws.String(testKeyID)), in); err != nil {
		t.Fatal(err)
	}
	if aws.ToString(in.AliasName) != "alias/a" || aws.ToString(in.TargetKeyId) != testKeyID {
		t.Errorf("unexpected input: %s, %s", aws.ToString(in.AliasName), aws.ToString(in.TargetKeyId))
	}

	if err := preCreate(context.Background(), alias("aws/a", aws.String(testKeyID)), &svcsdk.CreateAliasInput{}); err == nil {
		t.Error("want an error for a reserved alias name")
	}
}

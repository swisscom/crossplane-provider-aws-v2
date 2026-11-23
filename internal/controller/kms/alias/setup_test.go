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
	"go/token"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/kms"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/kms/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/kms/fake"
)

const (
	testAlias  = "alias/my-app"
	testKeyID  = "1234abcd-12ab-34cd-56ef-1234567890ab"
	testKeyARN = "arn:aws:kms:eu-central-1:123456789012:key/" + testKeyID
	testOther  = "0987dcba-09fe-87dc-65ba-ab0987654321"
)

var ignoreUnexported = cmp.FilterPath(func(p cmp.Path) bool {
	sf, ok := p.Index(-1).(cmp.StructField)
	return ok && !token.IsExported(sf.Name())
}, cmp.Ignore())

func alias(f ...func(*svcapitypes.Alias)) *svcapitypes.Alias {
	cr := &svcapitypes.Alias{ObjectMeta: metav1.ObjectMeta{Name: "my-app", Namespace: "ns"}}
	cr.SetGroupVersionKind(svcapitypes.AliasGroupVersionKind)
	meta.SetExternalName(cr, "my-app")
	cr.Spec.ForProvider.Region = "eu-central-1"
	cr.Spec.ForProvider.TargetKeyID = aws.String(testKeyARN)
	for _, fn := range f {
		fn(cr)
	}
	return cr
}

func entry(name, target string) svcsdktypes.AliasListEntry {
	return svcsdktypes.AliasListEntry{
		AliasName:   aws.String(name),
		AliasArn:    aws.String("arn:aws:kms:eu-central-1:123456789012:" + name),
		TargetKeyId: aws.String(target),
	}
}

// pages returns a client whose ListAliases returns one page per element of
// pages, linked by markers.
func pages(pages ...[]svcsdktypes.AliasListEntry) *fake.MockClient {
	return &fake.MockClient{MockListAliases: func(_ context.Context, in *svcsdk.ListAliasesInput) (*svcsdk.ListAliasesOutput, error) {
		i := 0
		if in.Marker != nil {
			i = int(aws.ToString(in.Marker)[0] - '0')
		}
		out := &svcsdk.ListAliasesOutput{Aliases: pages[i]}
		if i+1 < len(pages) {
			out.Truncated = true
			out.NextMarker = aws.String(string(rune('0' + i + 1)))
		}
		return out, nil
	}}
}

func newTestExternal(m *fake.MockClient) *external {
	return newExternal(nil, m, []option{setupExternal})
}

func TestObserve(t *testing.T) {
	cases := map[string]struct {
		cr   *svcapitypes.Alias
		mock *fake.MockClient
		want managed.ExternalObservation
	}{
		"NotFound": {
			cr:   alias(),
			mock: pages([]svcsdktypes.AliasListEntry{entry("alias/aws/ebs", "x"), entry("alias/my-app-2", testKeyID)}),
			want: managed.ExternalObservation{},
		},
		"UpToDateOnSecondPage": {
			cr:   alias(),
			mock: pages([]svcsdktypes.AliasListEntry{entry("alias/aws/ebs", "x")}, []svcsdktypes.AliasListEntry{entry(testAlias, testKeyID)}),
			want: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
		},
		"ExternalNameWithPrefix": {
			cr:   alias(func(cr *svcapitypes.Alias) { meta.SetExternalName(cr, testAlias) }),
			mock: pages([]svcsdktypes.AliasListEntry{entry(testAlias, testKeyID)}),
			want: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
		},
		"TargetDiffers": {
			cr:   alias(),
			mock: pages([]svcsdktypes.AliasListEntry{entry(testAlias, testOther)}),
			want: managed.ExternalObservation{ResourceExists: true, Diff: "spec.forProvider fields differ from the observed state: targetKeyID"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			obs, err := newTestExternal(tc.mock).Observe(context.Background(), tc.cr)
			if err != nil {
				t.Fatalf("Observe(...): %v", err)
			}
			if diff := cmp.Diff(tc.want, obs); diff != "" {
				t.Errorf("Observe(...): -want, +got:\n%s", diff)
			}
			in := tc.mock.Calls["ListAliases"][0].(*svcsdk.ListAliasesInput) //nolint:forcetypeassert // recorded by the mock
			if got := aws.ToInt32(in.Limit); got != maxAliases {
				t.Errorf("ListAliases Limit = %d, want %d", got, maxAliases)
			}
			if !obs.ResourceExists {
				return
			}
			if got := tc.cr.GetCondition(xpv2.TypeReady).Reason; got != xpv2.ReasonAvailable {
				t.Errorf("Ready reason = %q, want %q", got, xpv2.ReasonAvailable)
			}
			if got := aws.ToString(tc.cr.Status.AtProvider.AliasARN); got != "arn:aws:kms:eu-central-1:123456789012:"+testAlias {
				t.Errorf("status.atProvider.aliasARN = %q", got)
			}
		})
	}
}

func TestCreateUpdateDelete(t *testing.T) {
	cr := alias()
	m := pages([]svcsdktypes.AliasListEntry{entry(testAlias, testOther)})
	e := newTestExternal(m)
	ctx := context.Background()
	if _, err := e.Create(ctx, cr); err != nil {
		t.Fatalf("Create(...): %v", err)
	}
	if _, err := e.Update(ctx, cr); err != nil {
		t.Fatalf("Update(...): %v", err)
	}
	if _, err := e.Delete(ctx, cr); err != nil {
		t.Fatalf("Delete(...): %v", err)
	}
	want := map[string][]any{
		"CreateAlias": {&svcsdk.CreateAliasInput{AliasName: aws.String(testAlias), TargetKeyId: aws.String(testKeyARN)}},
		"UpdateAlias": {&svcsdk.UpdateAliasInput{AliasName: aws.String(testAlias), TargetKeyId: aws.String(testKeyARN)}},
		"DeleteAlias": {&svcsdk.DeleteAliasInput{AliasName: aws.String(testAlias)}},
	}
	for op, in := range want {
		if diff := cmp.Diff(in, m.Calls[op], ignoreUnexported); diff != "" {
			t.Errorf("%s: -want, +got:\n%s", op, diff)
		}
	}
}

func TestCreateWithoutTarget(t *testing.T) {
	m := &fake.MockClient{}
	_, err := newTestExternal(m).Create(context.Background(), alias(func(cr *svcapitypes.Alias) { cr.Spec.ForProvider.TargetKeyID = nil }))
	if err == nil {
		t.Fatal("Create(...): want error")
	}
	if len(m.Calls["CreateAlias"]) != 0 {
		t.Error("CreateAlias called without target key")
	}
}

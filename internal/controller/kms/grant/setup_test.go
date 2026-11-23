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

package grant

import (
	"context"
	"go/token"
	"strings"
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
	testGrantID = "0c237476b39f8bc44e45212e08498fbe3151305030726c0590dd8d3e9f3d6a60"
	testKeyID   = "1234abcd-12ab-34cd-56ef-1234567890ab"
	testKeyARN  = "arn:aws:kms:eu-central-1:123456789012:key/" + testKeyID
	testRole    = "arn:aws:iam::123456789012:role/app"
)

var ignoreUnexported = cmp.FilterPath(func(p cmp.Path) bool {
	sf, ok := p.Index(-1).(cmp.StructField)
	return ok && !token.IsExported(sf.Name())
}, cmp.Ignore())

func grant(f ...func(*svcapitypes.Grant)) *svcapitypes.Grant {
	cr := &svcapitypes.Grant{ObjectMeta: metav1.ObjectMeta{Name: "app-decrypt", Namespace: "ns"}}
	cr.SetGroupVersionKind(svcapitypes.GrantGroupVersionKind)
	meta.SetExternalName(cr, testGrantID)
	p := &cr.Spec.ForProvider
	p.Region = "eu-central-1"
	p.KeyID = aws.String(testKeyID)
	p.GranteePrincipal = aws.String(testRole)
	p.Operations = []*string{aws.String("Decrypt"), aws.String("DescribeKey")}
	p.Constraints = &svcapitypes.GrantConstraints{EncryptionContextEquals: map[string]*string{"app": aws.String("x")}}
	for _, fn := range f {
		fn(cr)
	}
	return cr
}

func observed(f ...func(*svcsdktypes.GrantListEntry)) svcsdktypes.GrantListEntry {
	g := svcsdktypes.GrantListEntry{
		GrantId:          aws.String(testGrantID),
		KeyId:            aws.String(testKeyARN),
		Name:             aws.String("app-decrypt"),
		GranteePrincipal: aws.String(testRole),
		IssuingAccount:   aws.String("arn:aws:iam::123456789012:root"),
		Operations:       []svcsdktypes.GrantOperation{svcsdktypes.GrantOperationDescribeKey, svcsdktypes.GrantOperationDecrypt},
		Constraints:      &svcsdktypes.GrantConstraints{EncryptionContextEquals: map[string]string{"app": "x"}},
	}
	for _, fn := range f {
		fn(&g)
	}
	return g
}

func mock(grants ...svcsdktypes.GrantListEntry) *fake.MockClient {
	return &fake.MockClient{MockListGrants: func(context.Context, *svcsdk.ListGrantsInput) (*svcsdk.ListGrantsOutput, error) {
		return &svcsdk.ListGrantsOutput{Grants: grants}, nil
	}}
}

func newTestExternal(m *fake.MockClient) *external {
	return newExternal(nil, m, []option{setupExternal})
}

func TestObserve(t *testing.T) {
	type want struct {
		obs   managed.ExternalObservation
		err   string
		keyID string
	}
	cases := map[string]struct {
		cr   *svcapitypes.Grant
		mock *fake.MockClient
		want want
	}{
		"NotFound": {
			cr:   grant(),
			mock: mock(),
			want: want{keyID: testKeyID},
		},
		"UpToDate": {
			cr:   grant(),
			mock: mock(observed()),
			want: want{obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, keyID: testKeyID},
		},
		"ServicePrincipalReturned": {
			cr:   grant(),
			mock: mock(observed(func(g *svcsdktypes.GrantListEntry) { g.GranteePrincipal = aws.String("rds.eu-central-1.amazonaws.com") })),
			want: want{obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, keyID: testKeyID},
		},
		"ObservedKeyIsUsed": {
			cr:   grant(func(cr *svcapitypes.Grant) { cr.Status.AtProvider.KeyID = aws.String(testKeyARN) }),
			mock: mock(observed()),
			want: want{obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, keyID: testKeyARN},
		},
		"ChangedFieldsAreAnError": {
			cr: grant(func(cr *svcapitypes.Grant) {
				cr.Status.AtProvider.KeyID = aws.String(testKeyARN)
				cr.Spec.ForProvider.KeyID = aws.String("other")
				cr.Spec.ForProvider.Operations = []*string{aws.String("Encrypt")}
			}),
			mock: mock(observed()),
			want: want{err: "these fields differ from the grant in AWS: keyID, operations", keyID: testKeyARN},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			obs, err := newTestExternal(tc.mock).Observe(context.Background(), tc.cr)
			if tc.want.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want.err) {
					t.Fatalf("Observe(...): error %v, want %q", err, tc.want.err)
				}
			} else if err != nil {
				t.Fatalf("Observe(...): %v", err)
			}
			if diff := cmp.Diff(tc.want.obs, obs); diff != "" {
				t.Errorf("Observe(...): -want, +got:\n%s", diff)
			}
			in := tc.mock.Calls["ListGrants"][0].(*svcsdk.ListGrantsInput) //nolint:forcetypeassert // recorded by the mock
			if aws.ToString(in.KeyId) != tc.want.keyID || aws.ToString(in.GrantId) != testGrantID {
				t.Errorf("ListGrants KeyId = %q, GrantId = %q", aws.ToString(in.KeyId), aws.ToString(in.GrantId))
			}
			if !obs.ResourceExists {
				return
			}
			if got := tc.cr.GetCondition(xpv2.TypeReady).Reason; got != xpv2.ReasonAvailable {
				t.Errorf("Ready reason = %q, want %q", got, xpv2.ReasonAvailable)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	cr := grant()
	meta.SetExternalName(cr, "")
	m := &fake.MockClient{MockCreateGrant: func(context.Context, *svcsdk.CreateGrantInput) (*svcsdk.CreateGrantOutput, error) {
		return &svcsdk.CreateGrantOutput{GrantId: aws.String(testGrantID), GrantToken: aws.String("token")}, nil
	}}
	cre, err := newTestExternal(m).Create(context.Background(), cr)
	if err != nil {
		t.Fatalf("Create(...): %v", err)
	}
	if got := meta.GetExternalName(cr); got != testGrantID {
		t.Errorf("external name = %q, want %q", got, testGrantID)
	}
	wantConn := managed.ConnectionDetails{"grantID": []byte(testGrantID), "grantToken": []byte("token")}
	if diff := cmp.Diff(wantConn, cre.ConnectionDetails); diff != "" {
		t.Errorf("connection details: -want, +got:\n%s", diff)
	}
	want := &svcsdk.CreateGrantInput{
		KeyId:            aws.String(testKeyID),
		Name:             aws.String("app-decrypt"),
		GranteePrincipal: aws.String(testRole),
		Operations:       []svcsdktypes.GrantOperation{svcsdktypes.GrantOperationDecrypt, svcsdktypes.GrantOperationDescribeKey},
		Constraints:      &svcsdktypes.GrantConstraints{EncryptionContextEquals: map[string]string{"app": "x"}},
	}
	if diff := cmp.Diff([]any{want}, m.Calls["CreateGrant"], ignoreUnexported); diff != "" {
		t.Errorf("CreateGrant: -want, +got:\n%s", diff)
	}
}

func TestCreateWithoutKey(t *testing.T) {
	m := &fake.MockClient{}
	if _, err := newTestExternal(m).Create(context.Background(), grant(func(cr *svcapitypes.Grant) { cr.Spec.ForProvider.KeyID = nil })); err == nil {
		t.Fatal("Create(...): want error")
	}
	if len(m.Calls["CreateGrant"]) != 0 {
		t.Error("CreateGrant called without key")
	}
}

func TestDelete(t *testing.T) {
	cr := grant(func(cr *svcapitypes.Grant) { cr.Status.AtProvider.KeyID = aws.String(testKeyARN) })
	m := &fake.MockClient{}
	if _, err := newTestExternal(m).Delete(context.Background(), cr); err != nil {
		t.Fatalf("Delete(...): %v", err)
	}
	want := &svcsdk.RevokeGrantInput{KeyId: aws.String(testKeyARN), GrantId: aws.String(testGrantID)}
	if diff := cmp.Diff([]any{want}, m.Calls["RevokeGrant"], ignoreUnexported); diff != "" {
		t.Errorf("RevokeGrant: -want, +got:\n%s", diff)
	}
}

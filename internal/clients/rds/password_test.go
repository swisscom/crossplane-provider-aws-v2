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
	"testing"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
)

func cacheSecret(data map[string]string) client.Object { return secret(ns, "dbinstance."+uid, data) }

func TestPasswordUpToDateAfterRestoreUsesCachedPassword(t *testing.T) {
	kube := newKube(t, cacheSecret(map[string]string{PasswordCacheKey: "generated", RestoreFlagCacheKay: string(RestoreStateRestored)}))

	upToDate, desired, err := PasswordUpToDate(context.Background(), kube, instance(nil))
	if err != nil {
		t.Fatal(err)
	}
	if upToDate || desired != "generated" {
		t.Errorf("PasswordUpToDate() = %t, %q; want false, %q", upToDate, desired, "generated")
	}
}

func TestPasswordUpToDateMatrix(t *testing.T) {
	ref := &xpv2.LocalSecretKeySelector{LocalSecretReference: xpv2.LocalSecretReference{Name: "creds"}, Key: passwordKey}
	type in struct {
		restored bool
		ref      *string // nil: no masterUserPasswordSecretRef
		cache    string
	}
	cases := map[string]struct {
		in
		wantUpToDate bool
		wantDesired  string
	}{
		"NoRefNoCache":           {in: in{}, wantUpToDate: true},
		"NoRefCache":             {in: in{cache: "c"}, wantUpToDate: true, wantDesired: "c"},
		"EmptyRefCache":          {in: in{ref: strPtr(""), cache: "c"}, wantUpToDate: true, wantDesired: "c"},
		"RefEqualsCache":         {in: in{ref: strPtr("c"), cache: "c"}, wantUpToDate: true, wantDesired: "c"},
		"RefDiffers":             {in: in{ref: strPtr("n"), cache: "c"}, wantDesired: "n"},
		"RefNoCache":             {in: in{ref: strPtr("n")}, wantDesired: "n"},
		"RestoredNothingToApply": {in: in{restored: true}, wantUpToDate: true},
		"RestoredEmptyRef":       {in: in{restored: true, ref: strPtr("")}, wantUpToDate: true},
		"RestoredCache":          {in: in{restored: true, cache: "c"}, wantDesired: "c"},
		"RestoredRefEqualsCache": {in: in{restored: true, ref: strPtr("c"), cache: "c"}, wantDesired: "c"},
		"RestoredRefDiffers":     {in: in{restored: true, ref: strPtr("n"), cache: "c"}, wantDesired: "n"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			data := map[string]string{PasswordCacheKey: tc.cache}
			if tc.restored {
				data[RestoreFlagCacheKay] = string(RestoreStateRestored)
			}
			objs := []client.Object{cacheSecret(data)}
			var r *xpv2.LocalSecretKeySelector
			if tc.ref != nil {
				r = ref
				objs = append(objs, secret(ns, "creds", map[string]string{passwordKey: *tc.ref}))
			}
			upToDate, desired, err := PasswordUpToDate(context.Background(), newKube(t, objs...), instance(r))
			if err != nil || upToDate != tc.wantUpToDate || desired != tc.wantDesired {
				t.Errorf("PasswordUpToDate() = %t, %q, %v; want %t, %q", upToDate, desired, err, tc.wantUpToDate, tc.wantDesired)
			}
		})
	}
}

func strPtr(s string) *string { return &s }

func TestCacheKeepsPasswordAndOwner(t *testing.T) {
	ctx := context.Background()
	kube := newKube(t)
	cr := instance(nil)
	cr.SetName("db")

	if _, err := Cache(ctx, kube, cr, map[string]string{PasswordCacheKey: "pw", RestoreFlagCacheKay: string(RestoreStateRestored)}); err != nil {
		t.Fatal(err)
	}
	// A later write with an empty password must not lose the stored one, but
	// it does reset the restore flag.
	if _, err := Cache(ctx, kube, cr, map[string]string{PasswordCacheKey: "", RestoreFlagCacheKay: ""}); err != nil {
		t.Fatal(err)
	}

	got := &corev1.Secret{}
	if err := kube.Get(ctx, types.NamespacedName{Namespace: ns, Name: "dbinstance." + uid}, got); err != nil {
		t.Fatal(err)
	}
	if pw := string(got.Data[PasswordCacheKey]); pw != "pw" {
		t.Errorf("cached password = %q, want pw", pw)
	}
	if flag := string(got.Data[RestoreFlagCacheKay]); flag != "" {
		t.Errorf("restore flag = %q, want it reset", flag)
	}
	want := []metav1.OwnerReference{{
		APIVersion:         v1alpha1.DBInstanceGroupVersionKind.GroupVersion().String(),
		Kind:               v1alpha1.DBInstanceKind,
		Name:               "db",
		UID:                uid,
		Controller:         ptrBool(true),
		BlockOwnerDeletion: ptrBool(false),
	}}
	if diff := cmp.Diff(want, got.OwnerReferences); diff != "" {
		t.Errorf("ownerReferences: -want, +got:\n%s", diff)
	}
}

func ptrBool(b bool) *bool { return &b }

func TestAppliedPassword(t *testing.T) {
	cases := map[string]struct {
		objs []client.Object
		want string
	}{
		"Cached":         {objs: []client.Object{cacheSecret(map[string]string{PasswordCacheKey: "pw"})}, want: "pw"},
		"RestorePending": {objs: []client.Object{cacheSecret(map[string]string{PasswordCacheKey: "pw", RestoreFlagCacheKay: string(RestoreStateRestored)})}},
		"NoCache":        {},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := AppliedPassword(context.Background(), newKube(t, tc.objs...), instance(nil))
			if err != nil || got != tc.want {
				t.Errorf("AppliedPassword() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

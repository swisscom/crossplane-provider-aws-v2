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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/teeverr/provider-aws-v2/apis/rds/v1alpha1"
)

const (
	ns  = "team-a"
	uid = "1234"
)

func newKube(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

func instance(ref *xpv2.LocalSecretKeySelector) *v1alpha1.DBInstance {
	cr := &v1alpha1.DBInstance{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: ns, UID: uid}}
	cr.SetGroupVersionKind(v1alpha1.DBInstanceGroupVersionKind)
	cr.Spec.ForProvider.MasterUserPasswordSecretRef = ref
	return cr
}

func secret(namespace, name string, data map[string]string) *corev1.Secret {
	s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}, Data: map[string][]byte{}}
	for k, v := range data {
		s.Data[k] = []byte(v)
	}
	return s
}

func TestCacheLivesInManagedResourceNamespace(t *testing.T) {
	kube := newKube(t)
	cr := instance(nil)
	ctx := context.Background()

	if _, err := Cache(ctx, kube, cr, map[string]string{PasswordCacheKey: "pw"}); err != nil {
		t.Fatal(err)
	}
	got := &corev1.Secret{}
	if err := kube.Get(ctx, types.NamespacedName{Namespace: ns, Name: "dbinstance." + uid}, got); err != nil {
		t.Fatalf("cache secret not in the MR namespace: %v", err)
	}
	if pw, err := GetCachedPassword(ctx, kube, cr); err != nil || pw != "pw" {
		t.Errorf("GetCachedPassword() = %q, %v; want pw", pw, err)
	}
	if err := DeleteCache(ctx, kube, cr); err != nil {
		t.Fatal(err)
	}
	if pw, err := GetCachedPassword(ctx, kube, cr); err != nil || pw != "" {
		t.Errorf("GetCachedPassword() after DeleteCache = %q, %v; want empty", pw, err)
	}
	if err := DeleteCache(ctx, kube, cr); err != nil {
		t.Errorf("DeleteCache() of a missing cache: %v", err)
	}
}

func TestGetSecretValue(t *testing.T) {
	ref := &xpv2.LocalSecretKeySelector{LocalSecretReference: xpv2.LocalSecretReference{Name: "creds"}, Key: "password"}
	cases := map[string]struct {
		objs []client.Object
		want string
	}{
		"SameNamespace":  {objs: []client.Object{secret(ns, "creds", map[string]string{"password": "s3cret"})}, want: "s3cret"},
		"OtherNamespace": {objs: []client.Object{secret("other", "creds", map[string]string{"password": "s3cret"})}},
		"MissingKey":     {objs: []client.Object{secret(ns, "creds", nil)}},
		"MissingSecret":  {},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := GetSecretValue(context.Background(), newKube(t, tc.objs...), ns, ref)
			if err != nil || got != tc.want {
				t.Errorf("GetSecretValue() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestPasswords(t *testing.T) {
	ref := &xpv2.LocalSecretKeySelector{LocalSecretReference: xpv2.LocalSecretReference{Name: "creds"}, Key: "password"}
	cache := func(data map[string]string) client.Object { return secret(ns, "dbinstance."+uid, data) }
	cases := map[string]struct {
		ref          *xpv2.LocalSecretKeySelector
		objs         []client.Object
		wantDesired  string
		wantUpToDate bool
	}{
		"CachedOnly": {
			objs:         []client.Object{cache(map[string]string{PasswordCacheKey: "cached"})},
			wantDesired:  "cached",
			wantUpToDate: true,
		},
		"SecretMatchesCache": {
			ref:          ref,
			objs:         []client.Object{cache(map[string]string{PasswordCacheKey: "pw"}), secret(ns, "creds", map[string]string{"password": "pw"})},
			wantDesired:  "pw",
			wantUpToDate: true,
		},
		"SecretChanged": {
			ref:         ref,
			objs:        []client.Object{cache(map[string]string{PasswordCacheKey: "old"}), secret(ns, "creds", map[string]string{"password": "new"})},
			wantDesired: "new",
		},
		"Restored": {
			objs:        []client.Object{cache(map[string]string{PasswordCacheKey: "pw", RestoreFlagCacheKay: string(RestoreStateRestored)})},
			wantDesired: "pw",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			kube := newKube(t, tc.objs...)
			cr := instance(tc.ref)
			desired, err := GetDesiredPassword(context.Background(), kube, cr)
			if err != nil || desired != tc.wantDesired {
				t.Errorf("GetDesiredPassword() = %q, %v; want %q", desired, err, tc.wantDesired)
			}
			upToDate, _, err := PasswordUpToDate(context.Background(), kube, cr)
			if err != nil || upToDate != tc.wantUpToDate {
				t.Errorf("PasswordUpToDate() = %t, %v; want %t", upToDate, err, tc.wantUpToDate)
			}
		})
	}
}

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

package aws

import (
	"context"
	"strings"
	"testing"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/teeverr/provider-aws-v2/apis/v1alpha1"
)

const nsA = "team-a"

const credsINI = `[default]
aws_access_key_id = AKIDEXAMPLE
aws_secret_access_key = secret
`

func newKube(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

func credsSecret(ns string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-creds", Namespace: ns},
		Data:       map[string][]byte{"credentials": []byte(credsINI)},
	}
}

func secretSpec(ns string) *v1alpha1.ProviderConfigSpec {
	return &v1alpha1.ProviderConfigSpec{Credentials: v1alpha1.ProviderCredentials{
		Source: v1alpha1.CredentialsSourceSecret,
		SecretRef: &xpv2.SecretKeySelector{
			SecretReference: xpv2.SecretReference{Name: "aws-creds", Namespace: ns},
			Key:             "credentials",
		},
	}}
}

func TestParseCredentials(t *testing.T) {
	cases := map[string]struct {
		data    string
		wantID  string
		wantErr bool
	}{
		"Valid":          {data: credsINI, wantID: "AKIDEXAMPLE"},
		"MissingSecret":  {data: "[default]\naws_access_key_id = x\n", wantErr: true},
		"MissingProfile": {data: "[other]\naws_access_key_id = x\naws_secret_access_key = y\n", wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseCredentials([]byte(tc.data), DefaultProfile)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseCredentials() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got.AccessKeyID != tc.wantID {
				t.Errorf("AccessKeyID = %q, want %q", got.AccessKeyID, tc.wantID)
			}
		})
	}
}

func TestNewConfig(t *testing.T) {
	ctx := context.Background()
	cases := map[string]struct {
		kube        client.Client
		spec        *v1alpha1.ProviderConfigSpec
		secretNS    string
		wantErr     string
		wantKeyID   string
		wantRegion  string
		wantBaseURL string
	}{
		"SecretSameNamespace": {
			kube:       newKube(t, credsSecret(nsA)),
			spec:       secretSpec(nsA),
			secretNS:   nsA,
			wantKeyID:  "AKIDEXAMPLE",
			wantRegion: "eu-central-1",
		},
		"SecretOtherNamespaceRejectedForProviderConfig": {
			kube:     newKube(t, credsSecret("team-b")),
			spec:     secretSpec("team-b"),
			secretNS: nsA,
			wantErr:  errSecretOtherNS,
		},
		"SecretAnyNamespaceForClusterProviderConfig": {
			kube:       newKube(t, credsSecret("crossplane-system")),
			spec:       secretSpec("crossplane-system"),
			wantKeyID:  "AKIDEXAMPLE",
			wantRegion: "eu-central-1",
		},
		"SecretWithEndpoint": {
			kube: newKube(t, credsSecret(nsA)),
			spec: func() *v1alpha1.ProviderConfigSpec {
				s := secretSpec(nsA)
				s.Endpoint = &v1alpha1.EndpointConfig{URL: "http://localstack:4566"}
				return s
			}(),
			secretNS:    nsA,
			wantKeyID:   "AKIDEXAMPLE",
			wantRegion:  "eu-central-1",
			wantBaseURL: "http://localstack:4566",
		},
		"SecretMissingRef": {
			kube:    newKube(t),
			spec:    &v1alpha1.ProviderConfigSpec{Credentials: v1alpha1.ProviderCredentials{Source: v1alpha1.CredentialsSourceSecret}},
			wantErr: errNoSecretRef,
		},
		"WebIdentityMissingOptions": {
			kube:    newKube(t),
			spec:    &v1alpha1.ProviderConfigSpec{Credentials: v1alpha1.ProviderCredentials{Source: v1alpha1.CredentialsSourceWebIdentity}},
			wantErr: errNoWebIdentity,
		},
		"UnknownSource": {
			kube:    newKube(t),
			spec:    &v1alpha1.ProviderConfigSpec{Credentials: v1alpha1.ProviderCredentials{Source: "Nope"}},
			wantErr: errUnknownSource,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, err := NewConfig(ctx, tc.kube, tc.spec, tc.secretNS, "eu-central-1")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("NewConfig() error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewConfig() unexpected error: %v", err)
			}
			if cfg.Region != tc.wantRegion {
				t.Errorf("Region = %q, want %q", cfg.Region, tc.wantRegion)
			}
			creds, err := cfg.Credentials.Retrieve(ctx)
			if err != nil {
				t.Fatalf("Retrieve() error: %v", err)
			}
			if creds.AccessKeyID != tc.wantKeyID {
				t.Errorf("AccessKeyID = %q, want %q", creds.AccessKeyID, tc.wantKeyID)
			}
			got := ""
			if cfg.BaseEndpoint != nil {
				got = *cfg.BaseEndpoint
			}
			if got != tc.wantBaseURL {
				t.Errorf("BaseEndpoint = %q, want %q", got, tc.wantBaseURL)
			}
		})
	}
}

func TestGetProviderConfigSpec(t *testing.T) {
	ctx := context.Background()
	pc := &v1alpha1.ProviderConfig{ObjectMeta: metav1.ObjectMeta{Name: "pc", Namespace: nsA}, Spec: *secretSpec(nsA)}
	cpc := &v1alpha1.ClusterProviderConfig{ObjectMeta: metav1.ObjectMeta{Name: "cpc"}, Spec: v1alpha1.ProviderConfigSpec{
		Credentials: v1alpha1.ProviderCredentials{Source: v1alpha1.CredentialsSourceIRSA},
	}}
	kube := newKube(t, pc, cpc)

	cases := map[string]struct {
		ref        ProviderConfigRef
		wantSource v1alpha1.CredentialsSource
		wantErr    bool
	}{
		"ProviderConfig":               {ref: ProviderConfigRef{Kind: "ProviderConfig", Name: "pc", Namespace: nsA}, wantSource: v1alpha1.CredentialsSourceSecret},
		"ProviderConfigOtherNamespace": {ref: ProviderConfigRef{Kind: "ProviderConfig", Name: "pc", Namespace: "team-b"}, wantErr: true},
		"ClusterProviderConfig":        {ref: ProviderConfigRef{Kind: "ClusterProviderConfig", Name: "cpc", Namespace: nsA}, wantSource: v1alpha1.CredentialsSourceIRSA},
		"UnknownKind":                  {ref: ProviderConfigRef{Kind: "Foo", Name: "pc"}, wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			spec, err := GetProviderConfigSpec(ctx, kube, tc.ref)
			if (err != nil) != tc.wantErr {
				t.Fatalf("GetProviderConfigSpec() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && spec.Credentials.Source != tc.wantSource {
				t.Errorf("Source = %q, want %q", spec.Credentials.Source, tc.wantSource)
			}
		})
	}
}

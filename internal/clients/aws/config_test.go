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
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpfake "github.com/crossplane/crossplane-runtime/v2/pkg/resource/fake"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/swisscom/crossplane-provider-aws-v2/apis/v1alpha1"
)

const (
	nsA        = "team-a"
	secretAKID = "AKIDEXAMPLE"
)

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
		"Valid":          {data: credsINI, wantID: secretAKID},
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
			wantKeyID:  secretAKID,
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
			wantKeyID:  secretAKID,
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
			wantKeyID:   secretAKID,
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

type trackerFunc func() error

func (f trackerFunc) Track(context.Context, resource.ModernManaged) error { return f() }

// mrIn returns a managed resource in ns that references the given config.
func mrIn(ns, kind, name string) *xpfake.ModernManaged {
	mr := &xpfake.ModernManaged{ObjectMeta: metav1.ObjectMeta{Name: "mr", Namespace: ns}}
	mr.SetProviderConfigReference(&xpv2.ProviderConfigReference{Kind: kind, Name: name})
	return mr
}

// podIdentityEnv makes the SDK default chain return static "pod" credentials
// and never reach IMDS or local AWS files.
func podIdentityEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "PODAKID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "pod-secret")
	t.Setenv("AWS_SESSION_TOKEN", "POD-SESSION-TOKEN")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_CONFIG_FILE", "/dev/null")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/dev/null")
	resetDefaultConfig()
	t.Cleanup(resetDefaultConfig)
}

func resetDefaultConfig() {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultCfg = nil
}

func TestGetConfig(t *testing.T) {
	pc := func(name, ns string, spec *v1alpha1.ProviderConfigSpec) *v1alpha1.ProviderConfig {
		return &v1alpha1.ProviderConfig{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Spec: *spec}
	}
	cpc := func(name string, spec *v1alpha1.ProviderConfigSpec) *v1alpha1.ClusterProviderConfig {
		return &v1alpha1.ClusterProviderConfig{ObjectMeta: metav1.ObjectMeta{Name: name}, Spec: *spec}
	}
	source := func(src v1alpha1.CredentialsSource) *v1alpha1.ProviderConfigSpec {
		return &v1alpha1.ProviderConfigSpec{Credentials: v1alpha1.ProviderCredentials{Source: src}}
	}
	noRef := &xpfake.ModernManaged{ObjectMeta: metav1.ObjectMeta{Name: "mr", Namespace: nsA}}

	cases := map[string]struct {
		objs      []client.Object
		tracker   Tracker
		mr        resource.ModernManaged
		wantErr   string
		wantKeyID string
	}{
		"LookupUsesTheMRNamespace": {
			objs:    []client.Object{pc("x", nsA, secretSpec(nsA)), credsSecret(nsA)},
			mr:      mrIn("team-b", v1alpha1.ProviderConfigKind, "x"),
			wantErr: errGetPC,
		},
		"SecretWithoutNamespaceRejected": {
			objs:    []client.Object{pc("pc", nsA, secretSpec("")), credsSecret(nsA)},
			mr:      mrIn(nsA, v1alpha1.ProviderConfigKind, "pc"),
			wantErr: errSecretOtherNS,
		},
		"SecretInForeignNamespaceRejected": {
			objs:    []client.Object{pc("pc", nsA, secretSpec("team-b")), credsSecret("team-b")},
			mr:      mrIn(nsA, v1alpha1.ProviderConfigKind, "pc"),
			wantErr: errSecretOtherNS,
		},
		"SecretInOwnNamespace": {
			objs:      []client.Object{pc("pc", nsA, secretSpec(nsA)), credsSecret(nsA)},
			mr:        mrIn(nsA, v1alpha1.ProviderConfigKind, "pc"),
			wantKeyID: secretAKID,
		},
		"NamespacedIRSA": {
			objs:      []client.Object{pc("pc", nsA, source(v1alpha1.CredentialsSourceIRSA))},
			mr:        mrIn(nsA, v1alpha1.ProviderConfigKind, "pc"),
			wantKeyID: "PODAKID",
		},
		"NamespacedPodIdentity": {
			objs:      []client.Object{pc("pc", nsA, source(v1alpha1.CredentialsSourcePodIdentity))},
			mr:        mrIn(nsA, v1alpha1.ProviderConfigKind, "pc"),
			wantKeyID: "PODAKID",
		},
		"ClusterProviderConfigSecretAnyNamespace": {
			objs:      []client.Object{cpc("cpc", secretSpec("crossplane-system")), credsSecret("crossplane-system")},
			mr:        mrIn(nsA, v1alpha1.ClusterProviderConfigKind, "cpc"),
			wantKeyID: secretAKID,
		},
		"ClusterProviderConfigIRSA": {
			objs:      []client.Object{cpc("cpc", source(v1alpha1.CredentialsSourceIRSA))},
			mr:        mrIn(nsA, v1alpha1.ClusterProviderConfigKind, "cpc"),
			wantKeyID: "PODAKID",
		},
		"ClusterProviderConfigNotFound": {
			mr:      mrIn(nsA, v1alpha1.ClusterProviderConfigKind, "missing"),
			wantErr: errGetCPC,
		},
		"NoProviderConfigRef": {
			mr:      noRef,
			wantErr: errNoPCRef,
		},
		"TrackerError": {
			tracker: trackerFunc(func() error { return fmt.Errorf("boom") }),
			mr:      mrIn(nsA, v1alpha1.ProviderConfigKind, "pc"),
			wantErr: errTrackUsage,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			podIdentityEnv(t)
			tr := tc.tracker
			if tr == nil {
				tr = trackerFunc(func() error { return nil })
			}
			cfg, err := GetConfig(context.Background(), newKube(t, tc.objs...), tr, tc.mr, "eu-central-1")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("GetConfig() error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetConfig() unexpected error: %v", err)
			}
			creds, err := cfg.Credentials.Retrieve(context.Background())
			if err != nil {
				t.Fatalf("Retrieve() error: %v", err)
			}
			if creds.AccessKeyID != tc.wantKeyID {
				t.Errorf("AccessKeyID = %q, want %q", creds.AccessKeyID, tc.wantKeyID)
			}
		})
	}
}

// stsServer is a fake STS that records every request it gets.
type stsServer struct {
	mu   sync.Mutex
	reqs []url.Values
}

func (s *stsServer) requests() []url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]url.Values(nil), s.reqs...)
}

const stsCredsXML = `<Credentials><AccessKeyId>ASIAFAKE</AccessKeyId><SecretAccessKey>fake-secret</SecretAccessKey>` +
	`<SessionToken>fake-token</SessionToken><Expiration>2099-01-01T00:00:00Z</Expiration></Credentials>` +
	`<AssumedRoleUser><Arn>arn:aws:sts::111111111111:assumed-role/r/s</Arn><AssumedRoleId>AROAFAKE:s</AssumedRoleId></AssumedRoleUser>`

// fakeSTS starts a fake STS and points the SDK at it through AWS_ENDPOINT_URL.
// AWS_ENDPOINT_URL_STS is not used: it would override cfg.BaseEndpoint and
// hide an STS client that uses endpoint.url.
func fakeSTS(t *testing.T) *stsServer {
	t.Helper()
	s := &stsServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.reqs = append(s.reqs, r.PostForm)
		s.mu.Unlock()
		action := r.PostForm.Get("Action")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprintf(w, `<%[1]sResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><%[1]sResult>%[2]s</%[1]sResult>`+
			`<ResponseMetadata><RequestId>req</RequestId></ResponseMetadata></%[1]sResponse>`, action, stsCredsXML)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AWS_ENDPOINT_URL", srv.URL)
	resetDefaultConfig()
	return s
}

// countingServer stands in for endpoint.url and counts what reaches it.
type countingServer struct {
	URL string

	mu sync.Mutex
	n  int
}

func (s *countingServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

func newCountingServer(t *testing.T) *countingServer {
	t.Helper()
	s := &countingServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		s.n++
		s.mu.Unlock()
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)
	s.URL = srv.URL
	return s
}

const roleARN = "arn:aws:iam::222222222222:role/team-a"

// assumeRole and the web identity exchange pass the user's fields to STS as
// they are, for ProviderConfigs and ClusterProviderConfigs alike.
func TestNewConfigSTSRequests(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("POD-SA-TOKEN"), 0o600); err != nil {
		t.Fatal(err)
	}
	ext, session := "vendor-given-id", "my-session"
	assumeRole := func(s *v1alpha1.ProviderConfigSpec) *v1alpha1.ProviderConfigSpec {
		s.AssumeRole = &v1alpha1.AssumeRoleOptions{
			RoleARN:           roleARN,
			ExternalID:        &ext,
			RoleSessionName:   &session,
			Tags:              []v1alpha1.Tag{{Key: "team", Value: "a"}},
			TransitiveTagKeys: []string{"team"},
		}
		return s
	}
	wantAssumeRole := map[string]string{
		"Action":                     "AssumeRole",
		"RoleArn":                    roleARN,
		"ExternalId":                 ext,
		"RoleSessionName":            session,
		"Tags.member.1.Key":          "team",
		"Tags.member.1.Value":        "a",
		"TransitiveTagKeys.member.1": "team",
	}
	webIdentity := &v1alpha1.ProviderConfigSpec{
		Credentials:               v1alpha1.ProviderCredentials{Source: v1alpha1.CredentialsSourceWebIdentity},
		AssumeRoleWithWebIdentity: &v1alpha1.AssumeRoleWithWebIdentityOptions{RoleARN: roleARN, TokenFile: &tokenFile, RoleSessionName: &session},
	}
	cases := map[string]struct {
		spec     *v1alpha1.ProviderConfigSpec
		secretNS string
		want     map[string]string
	}{
		"NamespacedSecretAssumeRole": {
			spec:     assumeRole(secretSpec(nsA)),
			secretNS: nsA,
			want:     wantAssumeRole,
		},
		"NamespacedIRSAAssumeRole": {
			spec:     assumeRole(&v1alpha1.ProviderConfigSpec{Credentials: v1alpha1.ProviderCredentials{Source: v1alpha1.CredentialsSourceIRSA}}),
			secretNS: nsA,
			want:     wantAssumeRole,
		},
		"ClusterSecretAssumeRole": {
			spec: assumeRole(secretSpec(nsA)),
			want: wantAssumeRole,
		},
		"WebIdentity": {
			spec: webIdentity,
			want: map[string]string{
				"Action":           "AssumeRoleWithWebIdentity",
				"RoleArn":          roleARN,
				"RoleSessionName":  session,
				"WebIdentityToken": "POD-SA-TOKEN",
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			podIdentityEnv(t)
			sts := fakeSTS(t)
			// STS must never use endpoint.url: it would send signed requests
			// or the pod's token to that host.
			ep := newCountingServer(t)
			tc.spec.Endpoint = &v1alpha1.EndpointConfig{URL: ep.URL}
			cfg, err := NewConfig(context.Background(), newKube(t, credsSecret(nsA)), tc.spec, tc.secretNS, "eu-central-1")
			if err != nil {
				t.Fatalf("NewConfig() error: %v", err)
			}
			creds, err := cfg.Credentials.Retrieve(context.Background())
			if err != nil {
				t.Fatalf("Retrieve() error: %v", err)
			}
			if creds.AccessKeyID != "ASIAFAKE" {
				t.Errorf("AccessKeyID = %q, want ASIAFAKE", creds.AccessKeyID)
			}
			reqs := sts.requests()
			if len(reqs) != 1 {
				t.Fatalf("STS got %d requests, want 1", len(reqs))
			}
			for k, v := range tc.want {
				if got := reqs[0].Get(k); got != v {
					t.Errorf("%s = %q, want %q", k, got, v)
				}
			}
			if n := ep.count(); n != 0 {
				t.Errorf("endpoint.url got %d requests, want 0", n)
			}
			if cfg.BaseEndpoint == nil || *cfg.BaseEndpoint != ep.URL {
				t.Errorf("BaseEndpoint = %v, want %q", cfg.BaseEndpoint, ep.URL)
			}
		})
	}
}

func TestNewConfigEndpointURL(t *testing.T) {
	cases := map[string]struct {
		url   string
		valid bool
	}{
		"LocalStack":        {url: "http://localstack:4566", valid: true},
		"VPCEndpoint":       {url: "https://vpce-1.rds.eu-central-1.vpce.amazonaws.com", valid: true},
		"IPv6":              {url: "http://[::1]:4566", valid: true},
		"WithPath":          {url: "https://example.com/aws/", valid: true},
		"Underscore":        {url: "http://localstack_main:4566", valid: true},
		"ServiceDNS":        {url: "http://localstack.localstack.svc:4566", valid: true},
		"MaxLength":         {url: "https://x/" + strings.Repeat("a", 2038), valid: true},
		"OtherScheme":       {url: "ftp://x"},
		"UserInfo":          {url: "http://user:pw@x"},
		"Query":             {url: "http://x?a=b"},
		"Fragment":          {url: "http://x#f"},
		"EmptyHost":         {url: "http://"},
		"EmptyHostPath":     {url: "http:///path"},
		"NotAURL":           {url: "not a url"},
		"EmptyHostWithPort": {url: "http://:4566"},
		"DotHost":           {url: "http://."},
		"PercentHost":       {url: "https://%00"},
		"PercentInHost":     {url: "http://a%00"},
		"NonNumericPort":    {url: "http://x:port"},
		"Whitespace":        {url: "http://x y"},
		"UppercaseScheme":   {url: "HTTP://x"},
		"TooLong":           {url: "https://x/" + strings.Repeat("a", 2039)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			spec := secretSpec(nsA)
			spec.Endpoint = &v1alpha1.EndpointConfig{URL: tc.url}
			cfg, err := NewConfig(context.Background(), newKube(t, credsSecret(nsA)), spec, nsA, "eu-central-1")
			if !tc.valid {
				if err == nil || err.Error() != errEndpointURL {
					t.Fatalf("NewConfig() error = %v, want %q", err, errEndpointURL)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewConfig() error: %v", err)
			}
			if cfg.BaseEndpoint == nil || *cfg.BaseEndpoint != tc.url {
				t.Errorf("BaseEndpoint = %v, want %q", cfg.BaseEndpoint, tc.url)
			}
		})
	}
}

func TestParseCredentialsErrorsHideSecretContent(t *testing.T) {
	cases := map[string]struct {
		profile string
		data    string
		want    string
		secret  []string
	}{
		"KeyLineWithoutEquals": {
			data:   "[default]\nwJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\n",
			want:   errParseCredsFile,
			secret: []string{"EXAMPLEKEY", "wJalrXUtnFEMI"},
		},
		"UnclosedSection": {
			data:   "[default\naws_access_key_id=SECRETLINE",
			want:   errParseCredsFile,
			secret: []string{"SECRETLINE", "[default"},
		},
		// ini always has an implicit "default" section, so a missing
		// [default] reads as a profile without keys.
		"MissingDefaultProfile": {
			data:   "[other]\naws_access_key_id = SECRETLINE\naws_secret_access_key = y\n",
			want:   errMissingAccessKeyIDs,
			secret: []string{"SECRETLINE"},
		},
		"MissingNamedProfile": {
			profile: "prod",
			data:    "[other]\naws_access_key_id = SECRETLINE\naws_secret_access_key = y\n",
			want:    fmt.Sprintf(errProfileNotFound, "prod"),
			secret:  []string{"SECRETLINE", "other"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			profile := tc.profile
			if profile == "" {
				profile = DefaultProfile
			}
			_, err := ParseCredentials([]byte(tc.data), profile)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("ParseCredentials() error = %v, want %q", err, tc.want)
			}
			for _, s := range tc.secret {
				if strings.Contains(err.Error(), s) {
					t.Errorf("error %q leaks %q", err, s)
				}
			}
		})
	}
}

func TestNewConfigMalformedSecretHidesContent(t *testing.T) {
	s := credsSecret(nsA)
	s.Data["credentials"] = []byte("[default]\nwJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\n")
	_, err := NewConfig(context.Background(), newKube(t, s), secretSpec(nsA), nsA, "eu-central-1")
	want := `cannot parse key "credentials" of the credentials secret: ` + errParseCredsFile
	if err == nil || err.Error() != want {
		t.Fatalf("NewConfig() error = %v, want %q", err, want)
	}
	if strings.Contains(err.Error(), "EXAMPLEKEY") {
		t.Errorf("error leaks the Secret: %q", err)
	}
}

// The Go check and the CRD pattern must stay the same regex.
func TestEndpointURLMatchesCRD(t *testing.T) {
	for _, f := range []string{"aws2.crossplane.io_providerconfigs.yaml", "aws2.crossplane.io_clusterproviderconfigs.yaml"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "..", "package", "crds", f))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "pattern: "+endpointURL.String()+"\n") {
			t.Errorf("%s: endpoint.url pattern differs from %q", f, endpointURL.String())
		}
		if !strings.Contains(string(b), fmt.Sprintf("maxLength: %d\n", maxEndpointURL)) {
			t.Errorf("%s: endpoint.url maxLength differs from %d", f, maxEndpointURL)
		}
	}
}

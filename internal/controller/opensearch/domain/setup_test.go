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

package domain

import (
	"context"
	"go/token"
	"sort"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/opensearch"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/aws/smithy-go"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kubefake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/opensearch/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/opensearch/fake"
)

const (
	keyPassword = "password"
	keyEndpoint = "endpoint"
	keyPort     = "port"
	keyUsername = "username"

	testName      = "logs"
	testNamespace = "ns"
	testARN       = "arn:aws:es:eu-central-1:123456789012:domain/logs"
	testEndpoint  = "search-logs-abc.eu-central-1.es.amazonaws.com"
	testVersion   = "OpenSearch_2.13"
	testPassword  = "Secret-1"
	testKMSKeyID  = "1234abcd-12ab-34cd-56ef-1234567890ab"
	testPolicy    = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"es:*","Resource":"` + testARN + `/*"}]}`
	// testPolicyAWS is testPolicy as AWS returns it: other formatting, and
	// lists with one element replaced by the element.
	testPolicyAWS = `{"Statement": {"Action": ["es:*"], "Effect": "Allow", "Principal": {"AWS": "*"}, "Resource": "` + testARN + `/*"}, "Version": "2012-10-17"}`
	// testPolicyMultiLine is testPolicy as users write it in YAML.
	testPolicyMultiLine = `{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {"AWS": "*"},
    "Action": "es:*",
    "Resource": "` + testARN + `/*"
  }]
}
`
)

var ignoreUnexported = cmp.FilterPath(func(p cmp.Path) bool {
	sf, ok := p.Index(-1).(cmp.StructField)
	return ok && !token.IsExported(sf.Name())
}, cmp.Ignore())

func domain(f ...func(*svcapitypes.Domain)) *svcapitypes.Domain {
	cr := &svcapitypes.Domain{ObjectMeta: metav1.ObjectMeta{Name: testName, Namespace: testNamespace}}
	cr.SetGroupVersionKind(svcapitypes.DomainGroupVersionKind)
	meta.SetExternalName(cr, testName)
	p := &cr.Spec.ForProvider
	p.Region = "eu-central-1"
	p.EngineVersion = aws.String(testVersion)
	p.ClusterConfig = &svcapitypes.ClusterConfig{InstanceType: aws.String("r6g.large.search"), InstanceCount: aws.Int64(2)}
	p.AccessPolicies = aws.String(testPolicy)
	p.Tags = []*svcapitypes.Tag{{Key: aws.String("team"), Value: aws.String("search")}}
	for _, fn := range f {
		fn(cr)
	}
	return cr
}

// withMasterUser enables fine-grained access control with an internal
// master user, its password in secret "pw" and connection secret "conn".
func withMasterUser(cr *svcapitypes.Domain) {
	cr.Spec.ForProvider.AdvancedSecurityOptions = &svcapitypes.AdvancedSecurityOptionsInput{
		Enabled:                     aws.Bool(true),
		InternalUserDatabaseEnabled: aws.Bool(true),
		MasterUserOptions:           &svcapitypes.MasterUserOptions{MasterUserName: aws.String("admin")},
	}
	cr.Spec.ForProvider.MasterUserPasswordSecretRef = &xpv2.LocalSecretKeySelector{
		LocalSecretReference: xpv2.LocalSecretReference{Name: "pw"}, Key: keyPassword,
	}
	cr.Spec.WriteConnectionSecretToReference = &xpv2.LocalSecretReference{Name: "conn"}
}

// status returns a created domain that matches domain().
func status(f ...func(*svcsdktypes.DomainStatus)) *svcsdktypes.DomainStatus {
	ds := &svcsdktypes.DomainStatus{
		ARN:             aws.String(testARN),
		DomainId:        aws.String("123456789012/" + testName),
		DomainName:      aws.String(testName),
		Created:         aws.Bool(true),
		Endpoint:        aws.String(testEndpoint),
		EngineVersion:   aws.String(testVersion),
		ClusterConfig:   &svcsdktypes.ClusterConfig{InstanceType: "r6g.large.search", InstanceCount: aws.Int32(2), DedicatedMasterEnabled: aws.Bool(false)},
		AccessPolicies:  aws.String(testPolicyAWS),
		AdvancedOptions: map[string]string{"rest.action.multi.allow_explicit_index": "true"},
	}
	for _, fn := range f {
		fn(ds)
	}
	return ds
}

func withMasterUserStatus(ds *svcsdktypes.DomainStatus) {
	ds.AdvancedSecurityOptions = &svcsdktypes.AdvancedSecurityOptions{Enabled: aws.Bool(true), InternalUserDatabaseEnabled: aws.Bool(true)}
}

// tags returns the tags of a domain created from cr: its spec tags.
func tags(cr *svcapitypes.Domain, extra ...svcsdktypes.Tag) []svcsdktypes.Tag {
	return append(sdkTags(desiredTags(cr)), extra...)
}

func secret(name string, data map[string]string) *corev1.Secret {
	s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace}, Data: map[string][]byte{}}
	for k, v := range data {
		s.Data[k] = []byte(v)
	}
	return s
}

func kube(objs ...client.Object) client.Client {
	return kubefake.NewClientBuilder().WithObjects(objs...).Build()
}

func newTestExternal(k client.Client, m *fake.MockOpenSearchClient) *domainExternal {
	return wrap(newExternal(k, m, nil)).(*domainExternal) //nolint:forcetypeassert // wrap always returns a *domainExternal
}

func describe(ds *svcsdktypes.DomainStatus) func(context.Context, *svcsdk.DescribeDomainInput) (*svcsdk.DescribeDomainOutput, error) {
	return func(context.Context, *svcsdk.DescribeDomainInput) (*svcsdk.DescribeDomainOutput, error) {
		return &svcsdk.DescribeDomainOutput{DomainStatus: ds}, nil
	}
}

func listTags(t []svcsdktypes.Tag) func(context.Context, *svcsdk.ListTagsInput) (*svcsdk.ListTagsOutput, error) {
	return func(context.Context, *svcsdk.ListTagsInput) (*svcsdk.ListTagsOutput, error) {
		return &svcsdk.ListTagsOutput{TagList: t}, nil
	}
}

func TestObserve(t *testing.T) {
	type want struct {
		obs       managed.ExternalObservation
		err       bool
		condition xpv2.Condition
		endpoint  string
	}
	cases := map[string]struct {
		cr   *svcapitypes.Domain
		ds   *svcsdktypes.DomainStatus
		err  error
		kube client.Client
		want want
	}{
		"NotFound": {
			cr:   domain(),
			err:  &smithy.GenericAPIError{Code: "ResourceNotFoundException"},
			want: want{obs: managed.ExternalObservation{ResourceExists: false}},
		},
		"OtherError": {
			cr:   domain(),
			err:  &smithy.GenericAPIError{Code: "AccessDenied"},
			want: want{err: true},
		},
		"Creating": {
			cr: domain(),
			ds: status(func(ds *svcsdktypes.DomainStatus) { ds.Created = aws.Bool(false); ds.Endpoint = nil }),
			want: want{
				obs:       managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, ConnectionDetails: managed.ConnectionDetails{keyEndpoint: []byte(""), keyPort: []byte("443")}},
				condition: xpv2.Creating(),
			},
		},
		"CreatedWithoutEndpointYet": {
			cr: domain(),
			ds: status(func(ds *svcsdktypes.DomainStatus) { ds.Endpoint = nil }),
			want: want{
				obs:       managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, ConnectionDetails: managed.ConnectionDetails{keyEndpoint: []byte(""), keyPort: []byte("443")}},
				condition: xpv2.Creating(),
			},
		},
		"Available": {
			cr: domain(withMasterUser),
			ds: status(withMasterUserStatus),
			kube: kube(secret("pw", map[string]string{keyPassword: testPassword}),
				secret("conn", map[string]string{keyPassword: testPassword})),
			want: want{
				obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, ConnectionDetails: managed.ConnectionDetails{
					keyEndpoint: []byte(testEndpoint), keyPort: []byte("443"), keyUsername: []byte("admin"), keyPassword: []byte(testPassword),
				}},
				condition: xpv2.Available(),
				endpoint:  testEndpoint,
			},
		},
		"VPCEndpoint": {
			cr: domain(),
			ds: status(func(ds *svcsdktypes.DomainStatus) {
				ds.Endpoint = nil
				ds.Endpoints = map[string]string{"vpc": "vpc-logs-abc.eu-central-1.es.amazonaws.com"}
			}),
			want: want{
				obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, ConnectionDetails: managed.ConnectionDetails{
					keyEndpoint: []byte("vpc-logs-abc.eu-central-1.es.amazonaws.com"), keyPort: []byte("443"),
				}},
				condition: xpv2.Available(),
				endpoint:  "vpc-logs-abc.eu-central-1.es.amazonaws.com",
			},
		},
		"ProcessingIsUpToDate": {
			cr: domain(func(cr *svcapitypes.Domain) { cr.Spec.ForProvider.ClusterConfig.InstanceCount = aws.Int64(3) }),
			ds: status(func(ds *svcsdktypes.DomainStatus) { ds.Processing = aws.Bool(true) }),
			want: want{
				obs:       managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, ConnectionDetails: managed.ConnectionDetails{keyEndpoint: []byte(testEndpoint), keyPort: []byte("443")}},
				condition: xpv2.Available(),
				endpoint:  testEndpoint,
			},
		},
		"Deleted": {
			cr: domain(),
			ds: status(func(ds *svcsdktypes.DomainStatus) { ds.Deleted = aws.Bool(true) }),
			want: want{
				obs:       managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true, ConnectionDetails: managed.ConnectionDetails{keyEndpoint: []byte(testEndpoint), keyPort: []byte("443")}},
				condition: xpv2.Deleting(),
				endpoint:  testEndpoint,
			},
		},
		"ChangedPasswordIsNotPublished": {
			cr: domain(withMasterUser),
			ds: status(withMasterUserStatus),
			kube: kube(secret("pw", map[string]string{keyPassword: "New-Secret-2"}),
				secret("conn", map[string]string{keyPassword: testPassword})),
			want: want{
				obs: managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: false,
					Diff: "spec.forProvider fields differ from the observed state: masterUserPasswordSecretRef",
					ConnectionDetails: managed.ConnectionDetails{
						keyEndpoint: []byte(testEndpoint), keyPort: []byte("443"), keyUsername: []byte("admin"),
					}},
				condition: xpv2.Available(),
				endpoint:  testEndpoint,
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m := &fake.MockOpenSearchClient{
				MockDescribeDomain: func(context.Context, *svcsdk.DescribeDomainInput) (*svcsdk.DescribeDomainOutput, error) {
					if tc.err != nil {
						return nil, tc.err
					}
					return &svcsdk.DescribeDomainOutput{DomainStatus: tc.ds}, nil
				},
				MockListTags: listTags(tags(tc.cr)),
			}
			k := tc.kube
			if k == nil {
				k = kube()
			}
			obs, err := newTestExternal(k, m).Observe(context.Background(), tc.cr)
			if (err != nil) != tc.want.err {
				t.Fatalf("Observe(...): err = %v, want error %v", err, tc.want.err)
			}
			if diff := cmp.Diff(tc.want.obs, obs); diff != "" {
				t.Errorf("Observe(...): -want, +got:\n%s", diff)
			}
			if tc.want.condition.Type != "" {
				if diff := cmp.Diff(tc.want.condition, tc.cr.GetCondition(xpv2.TypeReady), cmp.FilterPath(func(p cmp.Path) bool {
					return p.Last().String() == ".LastTransitionTime"
				}, cmp.Ignore())); diff != "" {
					t.Errorf("Ready condition: -want, +got:\n%s", diff)
				}
			}
			if got := aws.ToString(tc.cr.Status.AtProvider.Endpoint); got != tc.want.endpoint {
				t.Errorf("status.atProvider.endpoint = %q, want %q", got, tc.want.endpoint)
			}
			if in := m.Calls["DescribeDomain"]; len(in) > 0 && aws.ToString(in[0].(*svcsdk.DescribeDomainInput).DomainName) != testName { //nolint:forcetypeassert // recorded by the mock
				t.Errorf("DescribeDomain called with domain name %v, want %q", in[0], testName)
			}
		})
	}
}

func TestIsUpToDate(t *testing.T) {
	pwSecrets := []client.Object{secret("pw", map[string]string{keyPassword: testPassword}), secret("conn", map[string]string{keyPassword: testPassword})}
	cases := map[string]struct {
		cr   *svcapitypes.Domain
		ds   *svcsdktypes.DomainStatus
		tags []svcsdktypes.Tag
		kube []client.Object
		want []string
		err  bool
	}{
		"UpToDate": {
			cr: domain(), ds: status(),
		},
		"AWSTagsAndDefaultAdvancedOptionsIgnored": {
			cr: domain(func(cr *svcapitypes.Domain) {
				cr.Spec.ForProvider.AdvancedOptions = map[string]*string{"indices.fielddata.cache.size": aws.String("20")}
			}),
			ds: status(func(ds *svcsdktypes.DomainStatus) {
				ds.AdvancedOptions["indices.fielddata.cache.size"] = "20"
			}),
			tags: []svcsdktypes.Tag{{Key: aws.String("aws:cloudformation:stack-name"), Value: aws.String("x")}},
		},
		"ClusterConfig": {
			cr:   domain(func(cr *svcapitypes.Domain) { cr.Spec.ForProvider.ClusterConfig.InstanceCount = aws.Int64(3) }),
			ds:   status(),
			want: []string{"clusterConfig"},
		},
		"AccessPolicies": {
			cr: domain(func(cr *svcapitypes.Domain) {
				cr.Spec.ForProvider.AccessPolicies = aws.String(`{"Version":"2012-10-17","Statement":[]}`)
			}),
			ds:   status(),
			want: []string{"accessPolicies"},
		},
		"MultiLineAccessPoliciesUpToDate": {
			cr: domain(func(cr *svcapitypes.Domain) { cr.Spec.ForProvider.AccessPolicies = aws.String(testPolicyMultiLine) }),
			ds: status(),
		},
		"AdvancedOptions": {
			cr: domain(func(cr *svcapitypes.Domain) {
				cr.Spec.ForProvider.AdvancedOptions = map[string]*string{"rest.action.multi.allow_explicit_index": aws.String("false")}
			}),
			ds:   status(),
			want: []string{"advancedOptions"},
		},
		"AdvancedSecurityWithoutMasterUser": {
			cr:   domain(withMasterUser),
			ds:   status(withMasterUserStatus),
			kube: pwSecrets,
		},
		"AdvancedSecurity": {
			cr:   domain(withMasterUser),
			ds:   status(),
			kube: pwSecrets,
			want: []string{"advancedSecurityOptions"},
		},
		"PasswordChanged": {
			cr: domain(withMasterUser),
			ds: status(withMasterUserStatus),
			kube: []client.Object{secret("pw", map[string]string{keyPassword: "New-Secret-2"}),
				secret("conn", map[string]string{keyPassword: testPassword})},
			want: []string{"masterUserPasswordSecretRef"},
		},
		"PasswordWithoutConnectionSecretRefNotDetected": {
			cr: domain(withMasterUser, func(cr *svcapitypes.Domain) { cr.Spec.WriteConnectionSecretToReference = nil }),
			ds: status(withMasterUserStatus),
			kube: []client.Object{secret("pw", map[string]string{keyPassword: "New-Secret-2"}),
				secret("conn", map[string]string{keyPassword: testPassword})},
		},
		"PasswordWithoutConnectionSecretYet": {
			cr:   domain(withMasterUser),
			ds:   status(withMasterUserStatus),
			kube: []client.Object{secret("pw", map[string]string{keyPassword: testPassword})},
			want: []string{"masterUserPasswordSecretRef"},
		},
		"PasswordSecretMissing": {
			cr:  domain(withMasterUser),
			ds:  status(withMasterUserStatus),
			err: true,
		},
		"PasswordEmpty": {
			cr:   domain(withMasterUser),
			ds:   status(withMasterUserStatus),
			kube: []client.Object{secret("pw", map[string]string{keyPassword: ""})},
			err:  true,
		},
		"AutoTuneEnabled": {
			cr: domain(func(cr *svcapitypes.Domain) {
				cr.Spec.ForProvider.AutoTuneOptions = &svcapitypes.AutoTuneOptions{DesiredState: aws.String("ENABLED"), RollbackOnDisable: aws.String("NO_ROLLBACK")}
			}),
			ds: status(func(ds *svcsdktypes.DomainStatus) {
				ds.AutoTuneOptions = &svcsdktypes.AutoTuneOptionsOutput{State: svcsdktypes.AutoTuneStateEnableInProgress}
			}),
		},
		"AutoTuneDisabled": {
			cr: domain(func(cr *svcapitypes.Domain) {
				cr.Spec.ForProvider.AutoTuneOptions = &svcapitypes.AutoTuneOptions{DesiredState: aws.String("ENABLED")}
			}),
			ds: status(func(ds *svcsdktypes.DomainStatus) {
				ds.AutoTuneOptions = &svcsdktypes.AutoTuneOptionsOutput{State: svcsdktypes.AutoTuneStateDisabledAndRollbackComplete}
			}),
			want: []string{"autoTuneOptions"},
		},
		"AutoTuneDisabledWithoutStatus": {
			cr: domain(func(cr *svcapitypes.Domain) {
				cr.Spec.ForProvider.AutoTuneOptions = &svcapitypes.AutoTuneOptions{DesiredState: aws.String("DISABLED"), RollbackOnDisable: aws.String("NO_ROLLBACK")}
			}),
			ds: status(),
		},
		"AutoTuneEnabledWithoutStatus": {
			cr: domain(func(cr *svcapitypes.Domain) {
				cr.Spec.ForProvider.AutoTuneOptions = &svcapitypes.AutoTuneOptions{DesiredState: aws.String("ENABLED")}
			}),
			ds:   status(),
			want: []string{"autoTuneOptions"},
		},
		"AutoTuneError": {
			cr: domain(func(cr *svcapitypes.Domain) {
				cr.Spec.ForProvider.AutoTuneOptions = &svcapitypes.AutoTuneOptions{DesiredState: aws.String("ENABLED")}
			}),
			ds: status(func(ds *svcsdktypes.DomainStatus) {
				ds.AutoTuneOptions = &svcsdktypes.AutoTuneOptionsOutput{State: svcsdktypes.AutoTuneStateError}
			}),
		},
		"KMSKeyIDMatchesARN": {
			cr: domain(func(cr *svcapitypes.Domain) {
				cr.Spec.ForProvider.EncryptionAtRestOptions = &svcapitypes.EncryptionAtRestOptions{Enabled: aws.Bool(true), KMSKeyID: aws.String(testKMSKeyID)}
			}),
			ds: status(func(ds *svcsdktypes.DomainStatus) {
				ds.EncryptionAtRestOptions = &svcsdktypes.EncryptionAtRestOptions{Enabled: aws.Bool(true), KmsKeyId: aws.String("arn:aws:kms:eu-central-1:123456789012:key/" + testKMSKeyID)}
			}),
		},
		"KMSKeyID": {
			cr: domain(func(cr *svcapitypes.Domain) {
				cr.Spec.ForProvider.EncryptionAtRestOptions = &svcapitypes.EncryptionAtRestOptions{Enabled: aws.Bool(true), KMSKeyID: aws.String("other")}
			}),
			ds: status(func(ds *svcsdktypes.DomainStatus) {
				ds.EncryptionAtRestOptions = &svcsdktypes.EncryptionAtRestOptions{Enabled: aws.Bool(true), KmsKeyId: aws.String("arn:aws:kms:eu-central-1:123456789012:key/" + testKMSKeyID)}
			}),
			want: []string{"encryptionAtRestOptions"},
		},
		"LogPublishingSubset": {
			cr: domain(func(cr *svcapitypes.Domain) {
				cr.Spec.ForProvider.LogPublishingOptions = map[string]*svcapitypes.LogPublishingOption{
					"ES_APPLICATION_LOGS": {Enabled: aws.Bool(true), CloudWatchLogsLogGroupARN: aws.String("arn:lg")},
				}
			}),
			ds: status(func(ds *svcsdktypes.DomainStatus) {
				ds.LogPublishingOptions = map[string]svcsdktypes.LogPublishingOption{
					"ES_APPLICATION_LOGS": {Enabled: aws.Bool(true), CloudWatchLogsLogGroupArn: aws.String("arn:lg")},
					"AUDIT_LOGS":          {Enabled: aws.Bool(false)},
				}
			}),
		},
		"LogPublishing": {
			cr: domain(func(cr *svcapitypes.Domain) {
				cr.Spec.ForProvider.LogPublishingOptions = map[string]*svcapitypes.LogPublishingOption{
					"ES_APPLICATION_LOGS": {Enabled: aws.Bool(true), CloudWatchLogsLogGroupARN: aws.String("arn:lg")},
				}
			}),
			ds:   status(),
			want: []string{"logPublishingOptions"},
		},
		"EngineVersion": {
			cr:   domain(func(cr *svcapitypes.Domain) { cr.Spec.ForProvider.EngineVersion = aws.String("OpenSearch_2.15") }),
			ds:   status(),
			want: []string{"engineVersion"},
		},
		"Tags": {
			cr:   domain(func(cr *svcapitypes.Domain) { cr.Spec.ForProvider.Tags[0].Value = aws.String("other") }),
			ds:   status(),
			want: []string{"tags"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m := &fake.MockOpenSearchClient{MockListTags: listTags(tags(domain(), tc.tags...))}
			e := newTestExternal(kube(tc.kube...), m)
			upToDate, _, err := e.hooks.isUpToDate(context.Background(), tc.cr, &svcsdk.DescribeDomainOutput{DomainStatus: tc.ds})
			if (err != nil) != tc.err {
				t.Fatalf("isUpToDate(...): err = %v, want error %v", err, tc.err)
			}
			if tc.err {
				return
			}
			got := e.hooks.diff
			sort.Strings(got)
			if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("isUpToDate(...): differing fields -want, +got:\n%s", diff)
			}
			if upToDate != (len(tc.want) == 0) {
				t.Errorf("isUpToDate(...) = %v, want %v", upToDate, len(tc.want) == 0)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	cr := domain(withMasterUser, func(cr *svcapitypes.Domain) {
		cr.Spec.ForProvider.AutoTuneOptions = &svcapitypes.AutoTuneOptions{DesiredState: aws.String("ENABLED"), UseOffPeakWindow: aws.Bool(true)}
		cr.Spec.ForProvider.AccessPolicies = aws.String(testPolicyMultiLine)
	})
	want := cr.DeepCopy()
	m := &fake.MockOpenSearchClient{
		MockCreateDomain: func(context.Context, *svcsdk.CreateDomainInput) (*svcsdk.CreateDomainOutput, error) {
			return &svcsdk.CreateDomainOutput{DomainStatus: status(func(ds *svcsdktypes.DomainStatus) {
				ds.Created, ds.Endpoint, ds.AccessPolicies = aws.Bool(false), nil, nil
			})}, nil
		},
	}
	e := newTestExternal(kube(secret("pw", map[string]string{keyPassword: testPassword})), m)
	cre, err := e.Create(context.Background(), cr)
	if err != nil {
		t.Fatalf("Create(...): %v", err)
	}

	in := m.Calls["CreateDomain"][0].(*svcsdk.CreateDomainInput) //nolint:forcetypeassert // recorded by the mock
	if got := aws.ToString(in.DomainName); got != testName {
		t.Errorf("CreateDomainInput.DomainName = %q, want %q", got, testName)
	}
	// Only the spec tags.
	wantTags := []svcsdktypes.Tag{{Key: aws.String("team"), Value: aws.String("search")}}
	if diff := cmp.Diff(wantTags, in.TagList, ignoreUnexported); diff != "" {
		t.Errorf("CreateDomainInput.TagList: -want, +got:\n%s", diff)
	}
	wantAutoTune := &svcsdktypes.AutoTuneOptionsInput{DesiredState: svcsdktypes.AutoTuneDesiredStateEnabled, UseOffPeakWindow: aws.Bool(true)}
	if diff := cmp.Diff(wantAutoTune, in.AutoTuneOptions, ignoreUnexported); diff != "" {
		t.Errorf("CreateDomainInput.AutoTuneOptions: -want, +got:\n%s", diff)
	}
	wantMaster := &svcsdktypes.MasterUserOptions{MasterUserName: aws.String("admin"), MasterUserPassword: aws.String(testPassword)}
	if diff := cmp.Diff(wantMaster, in.AdvancedSecurityOptions.MasterUserOptions, ignoreUnexported); diff != "" {
		t.Errorf("CreateDomainInput.AdvancedSecurityOptions.MasterUserOptions: -want, +got:\n%s", diff)
	}
	// Compacted: AWS rejects line breaks.
	if aws.ToString(in.AccessPolicies) != testPolicy {
		t.Errorf("CreateDomainInput.AccessPolicies = %q, want %q", aws.ToString(in.AccessPolicies), testPolicy)
	}

	wantCD := managed.ConnectionDetails{keyEndpoint: []byte(""), keyPort: []byte("443"), keyUsername: []byte("admin"), keyPassword: []byte(testPassword)}
	if diff := cmp.Diff(wantCD, cre.ConnectionDetails); diff != "" {
		t.Errorf("Create(...): connection details -want, +got:\n%s", diff)
	}
	if diff := cmp.Diff(want.Spec, cr.Spec); diff != "" {
		t.Errorf("Create(...) changed the spec: -want, +got:\n%s", diff)
	}
	if got := aws.ToString(cr.Status.AtProvider.ARN); got != testARN {
		t.Errorf("status.atProvider.arn = %q, want %q", got, testARN)
	}
}

func TestUpdate(t *testing.T) {
	pwSecrets := []client.Object{secret("pw", map[string]string{keyPassword: testPassword}), secret("conn", map[string]string{keyPassword: testPassword})}
	type want struct {
		calls  []string
		update *svcsdk.UpdateDomainConfigInput
		add    []svcsdktypes.Tag
		remove []string
		cd     managed.ConnectionDetails
	}
	cases := map[string]struct {
		cr   *svcapitypes.Domain
		ds   *svcsdktypes.DomainStatus
		tags []svcsdktypes.Tag
		kube []client.Object
		want want
	}{
		"OnlyDifferingFields": {
			cr: domain(withMasterUser, func(cr *svcapitypes.Domain) {
				cr.Spec.ForProvider.ClusterConfig.InstanceCount = aws.Int64(3)
				cr.Spec.ForProvider.AdvancedSecurityOptions.AnonymousAuthEnabled = aws.Bool(true)
			}),
			ds:   status(withMasterUserStatus),
			kube: pwSecrets,
			want: want{
				calls: []string{"UpdateDomainConfig"},
				update: &svcsdk.UpdateDomainConfigInput{
					DomainName:    aws.String(testName),
					ClusterConfig: &svcsdktypes.ClusterConfig{InstanceType: "r6g.large.search", InstanceCount: aws.Int32(3)},
					// Without the master user: it can only be sent with its password.
					AdvancedSecurityOptions: &svcsdktypes.AdvancedSecurityOptionsInput{
						Enabled: aws.Bool(true), InternalUserDatabaseEnabled: aws.Bool(true), AnonymousAuthEnabled: aws.Bool(true),
					},
				},
			},
		},
		"Password": {
			cr: domain(withMasterUser),
			ds: status(withMasterUserStatus),
			kube: []client.Object{secret("pw", map[string]string{keyPassword: "New-Secret-2"}),
				secret("conn", map[string]string{keyPassword: testPassword})},
			want: want{
				calls: []string{"UpdateDomainConfig"},
				update: &svcsdk.UpdateDomainConfigInput{
					DomainName: aws.String(testName),
					AdvancedSecurityOptions: &svcsdktypes.AdvancedSecurityOptionsInput{
						Enabled: aws.Bool(true), InternalUserDatabaseEnabled: aws.Bool(true),
						MasterUserOptions: &svcsdktypes.MasterUserOptions{MasterUserName: aws.String("admin"), MasterUserPassword: aws.String("New-Secret-2")},
					},
				},
				cd: managed.ConnectionDetails{keyEndpoint: []byte(testEndpoint), keyPort: []byte("443"), keyUsername: []byte("admin"), keyPassword: []byte("New-Secret-2")},
			},
		},
		"TagsOnly": {
			cr: domain(func(cr *svcapitypes.Domain) {
				cr.Spec.ForProvider.Tags = []*svcapitypes.Tag{{Key: aws.String("team"), Value: aws.String("other")}}
			}),
			ds: status(),
			tags: append(tags(domain()),
				svcsdktypes.Tag{Key: aws.String("old"), Value: aws.String("x")},
				// added by earlier provider versions
				svcsdktypes.Tag{Key: aws.String("crossplane-kind"), Value: aws.String("domain.opensearch.aws2.crossplane.io")},
				svcsdktypes.Tag{Key: aws.String("aws:cloudformation:stack-name"), Value: aws.String("x")}),
			want: want{
				calls:  []string{"RemoveTags", "AddTags"},
				add:    []svcsdktypes.Tag{{Key: aws.String("team"), Value: aws.String("other")}},
				remove: []string{"crossplane-kind", "old"},
			},
		},
		"ConfigBeforeUpgrade": {
			cr: domain(func(cr *svcapitypes.Domain) {
				cr.Spec.ForProvider.EngineVersion = aws.String("OpenSearch_2.15")
				cr.Spec.ForProvider.AccessPolicies = aws.String("{\n  \"Version\": \"2012-10-17\",\n  \"Statement\": []\n}\n")
			}),
			ds: status(),
			want: want{
				calls: []string{"UpdateDomainConfig"},
				update: &svcsdk.UpdateDomainConfigInput{
					DomainName: aws.String(testName),
					// Compacted: AWS rejects line breaks.
					AccessPolicies: aws.String(`{"Version":"2012-10-17","Statement":[]}`),
				},
			},
		},
		"Upgrade": {
			cr:   domain(func(cr *svcapitypes.Domain) { cr.Spec.ForProvider.EngineVersion = aws.String("OpenSearch_2.15") }),
			ds:   status(),
			want: want{calls: []string{"UpgradeDomain"}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			current := tc.tags
			if current == nil {
				current = tags(domain())
			}
			m := &fake.MockOpenSearchClient{MockDescribeDomain: describe(tc.ds), MockListTags: listTags(current)}
			e := newTestExternal(kube(tc.kube...), m)
			ctx := context.Background()
			obs, err := e.Observe(ctx, tc.cr)
			if err != nil {
				t.Fatalf("Observe(...): %v", err)
			}
			if obs.ResourceUpToDate {
				t.Fatal("Observe(...): up to date, want an update")
			}
			m.Calls = nil
			upd, err := e.Update(ctx, tc.cr)
			if err != nil {
				t.Fatalf("Update(...): %v", err)
			}

			var calls []string
			for _, op := range []string{"RemoveTags", "AddTags", "UpdateDomainConfig", "UpgradeDomain"} {
				if len(m.Calls[op]) > 0 {
					calls = append(calls, op)
				}
			}
			if diff := cmp.Diff(tc.want.calls, calls); diff != "" {
				t.Errorf("Update(...): calls -want, +got:\n%s", diff)
			}
			if tc.want.update != nil {
				if diff := cmp.Diff(tc.want.update, m.Calls["UpdateDomainConfig"][0], ignoreUnexported); diff != "" {
					t.Errorf("UpdateDomainConfigInput: -want, +got:\n%s", diff)
				}
			}
			if tc.want.add != nil {
				want := &svcsdk.AddTagsInput{ARN: aws.String(testARN), TagList: tc.want.add}
				if diff := cmp.Diff(want, m.Calls["AddTags"][0], ignoreUnexported); diff != "" {
					t.Errorf("AddTagsInput: -want, +got:\n%s", diff)
				}
			}
			if tc.want.remove != nil {
				want := &svcsdk.RemoveTagsInput{ARN: aws.String(testARN), TagKeys: tc.want.remove}
				if diff := cmp.Diff(want, m.Calls["RemoveTags"][0], ignoreUnexported); diff != "" {
					t.Errorf("RemoveTagsInput: -want, +got:\n%s", diff)
				}
			}
			if in := m.Calls["UpgradeDomain"]; len(in) > 0 {
				want := &svcsdk.UpgradeDomainInput{DomainName: aws.String(testName), TargetVersion: aws.String("OpenSearch_2.15")}
				if diff := cmp.Diff(want, in[0], ignoreUnexported); diff != "" {
					t.Errorf("UpgradeDomainInput: -want, +got:\n%s", diff)
				}
			}
			if diff := cmp.Diff(tc.want.cd, upd.ConnectionDetails); diff != "" {
				t.Errorf("Update(...): connection details -want, +got:\n%s", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	cases := map[string]struct {
		deleted bool
		calls   int
	}{
		"Delete":          {calls: 1},
		"AlreadyDeleting": {deleted: true, calls: 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := domain(func(cr *svcapitypes.Domain) { cr.Status.AtProvider.Deleted = aws.Bool(tc.deleted) })
			m := &fake.MockOpenSearchClient{}
			if _, err := newTestExternal(kube(), m).Delete(context.Background(), cr); err != nil {
				t.Fatalf("Delete(...): %v", err)
			}
			calls := m.Calls["DeleteDomain"]
			if len(calls) != tc.calls {
				t.Fatalf("DeleteDomain called %d times, want %d", len(calls), tc.calls)
			}
			if tc.calls > 0 {
				if got := aws.ToString(calls[0].(*svcsdk.DeleteDomainInput).DomainName); got != testName { //nolint:forcetypeassert // recorded by the mock
					t.Errorf("DeleteDomainInput.DomainName = %q, want %q", got, testName)
				}
			}
		})
	}
}

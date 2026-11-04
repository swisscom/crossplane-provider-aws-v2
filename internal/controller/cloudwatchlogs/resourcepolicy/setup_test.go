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

package resourcepolicy

import (
	"context"
	"go/token"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/smithy-go"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/cloudwatchlogs/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/cloudwatchlogs/fake"
)

const (
	testName   = "opensearch-log-policy"
	testPolicy = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"es.amazonaws.com"},"Action":["logs:PutLogEvents","logs:CreateLogStream"],"Resource":"arn:aws:logs:eu-central-1:123456789012:log-group:/aws/opensearch/*"}]}`
	// testPolicyAWS is testPolicy as AWS may return it.
	testPolicyAWS = `{"Statement": {"Action": ["logs:PutLogEvents", "logs:CreateLogStream"], "Effect": "Allow", "Principal": {"Service": "es.amazonaws.com"}, "Resource": "arn:aws:logs:eu-central-1:123456789012:log-group:/aws/opensearch/*"}, "Version": "2012-10-17"}`
)

var ignoreUnexported = cmp.FilterPath(func(p cmp.Path) bool {
	sf, ok := p.Index(-1).(cmp.StructField)
	return ok && !token.IsExported(sf.Name())
}, cmp.Ignore())

func resourcePolicy() *svcapitypes.ResourcePolicy {
	cr := &svcapitypes.ResourcePolicy{ObjectMeta: metav1.ObjectMeta{Name: "policy", Namespace: "ns"}}
	meta.SetExternalName(cr, testName)
	cr.Spec.ForProvider.Region = "eu-central-1"
	cr.Spec.ForProvider.PolicyDocument = aws.String(testPolicy)
	return cr
}

func policy(name, doc string) svcsdktypes.ResourcePolicy {
	return svcsdktypes.ResourcePolicy{
		PolicyName:      aws.String(name),
		PolicyDocument:  aws.String(doc),
		PolicyScope:     svcsdktypes.PolicyScopeAccount,
		LastUpdatedTime: aws.Int64(1700000000000),
	}
}

func newTestExternal(m *fake.MockClient) *external {
	return newExternal(nil, m, []option{setupExternal})
}

func TestObserve(t *testing.T) {
	cases := map[string]struct {
		policies []svcsdktypes.ResourcePolicy
		want     managed.ExternalObservation
	}{
		"NotFound": {
			policies: []svcsdktypes.ResourcePolicy{policy("other", testPolicy)},
		},
		"UpToDate": {
			policies: []svcsdktypes.ResourcePolicy{policy("other", "{}"), policy(testName, testPolicyAWS)},
			want:     managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true},
		},
		"PolicyDiffers": {
			policies: []svcsdktypes.ResourcePolicy{policy(testName, `{"Version":"2012-10-17","Statement":[]}`)},
			want: managed.ExternalObservation{
				ResourceExists: true,
				Diff:           "spec.forProvider fields differ from the observed state: policyDocument",
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m := &fake.MockClient{MockDescribeResourcePolicies: func(context.Context, *svcsdk.DescribeResourcePoliciesInput) (*svcsdk.DescribeResourcePoliciesOutput, error) {
				return &svcsdk.DescribeResourcePoliciesOutput{ResourcePolicies: tc.policies}, nil
			}}
			cr := resourcePolicy()
			obs, err := newTestExternal(m).Observe(context.Background(), cr)
			if err != nil {
				t.Fatalf("Observe(...): %v", err)
			}
			if diff := cmp.Diff(tc.want, obs); diff != "" {
				t.Errorf("Observe(...): -want, +got:\n%s", diff)
			}
			want := &svcsdk.DescribeResourcePoliciesInput{Limit: aws.Int32(maxPolicies)}
			if diff := cmp.Diff(want, m.Calls["DescribeResourcePolicies"][0], ignoreUnexported); diff != "" {
				t.Errorf("DescribeResourcePolicies input: -want, +got:\n%s", diff)
			}
			if !obs.ResourceExists {
				return
			}
			if got := cr.GetCondition(xpv2.TypeReady).Reason; got != xpv2.ReasonAvailable {
				t.Errorf("Ready reason = %q, want %q", got, xpv2.ReasonAvailable)
			}
			if got := aws.ToString(cr.Status.AtProvider.PolicyScope); got != "ACCOUNT" {
				t.Errorf("status.atProvider.policyScope = %q, want ACCOUNT", got)
			}
			if got := aws.ToString(cr.Spec.ForProvider.PolicyDocument); got != testPolicy {
				t.Errorf("Observe(...) changed spec.forProvider.policyDocument to %q", got)
			}
		})
	}
}

func TestCreateAndUpdate(t *testing.T) {
	want := &svcsdk.PutResourcePolicyInput{PolicyName: aws.String(testName), PolicyDocument: aws.String(testPolicy)}
	m := &fake.MockClient{}
	e := newTestExternal(m)
	if _, err := e.Create(context.Background(), resourcePolicy()); err != nil {
		t.Fatalf("Create(...): %v", err)
	}
	if _, err := e.Update(context.Background(), resourcePolicy()); err != nil {
		t.Fatalf("Update(...): %v", err)
	}
	if diff := cmp.Diff([]any{want, want}, m.Calls["PutResourcePolicy"], ignoreUnexported); diff != "" {
		t.Errorf("PutResourcePolicy inputs: -want, +got:\n%s", diff)
	}
}

func TestDelete(t *testing.T) {
	cases := map[string]struct {
		err     error
		wantErr bool
	}{
		"Deleted":  {},
		"NotFound": {err: &smithy.GenericAPIError{Code: "ResourceNotFoundException"}},
		"Failed":   {err: &smithy.GenericAPIError{Code: "ServiceUnavailableException"}, wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m := &fake.MockClient{MockDeleteResourcePolicy: func(context.Context, *svcsdk.DeleteResourcePolicyInput) (*svcsdk.DeleteResourcePolicyOutput, error) {
				return &svcsdk.DeleteResourcePolicyOutput{}, tc.err
			}}
			_, err := newTestExternal(m).Delete(context.Background(), resourcePolicy())
			if (err != nil) != tc.wantErr {
				t.Fatalf("Delete(...): err = %v, wantErr %v", err, tc.wantErr)
			}
			want := &svcsdk.DeleteResourcePolicyInput{PolicyName: aws.String(testName)}
			if diff := cmp.Diff(want, m.Calls["DeleteResourcePolicy"][0], ignoreUnexported); diff != "" {
				t.Errorf("DeleteResourcePolicy input: -want, +got:\n%s", diff)
			}
		})
	}
}

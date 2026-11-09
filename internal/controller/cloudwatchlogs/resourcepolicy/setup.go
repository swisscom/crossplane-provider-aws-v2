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

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/cloudwatchlogs/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

const (
	// maxPolicies is the largest page DescribeResourcePolicies returns. An
	// account has at most 10 resource policies per region, so one page has
	// all of them.
	maxPolicies = 50

	fieldPolicyDocument = "policyDocument"
)

func preObserve(_ context.Context, _ *svcapitypes.ResourcePolicy, obj *svcsdk.DescribeResourcePoliciesInput) error {
	obj.Limit = aws.Int32(maxPolicies)
	return nil
}

// filterList keeps the policy whose name is the external name.
func filterList(cr *svcapitypes.ResourcePolicy, list *svcsdk.DescribeResourcePoliciesOutput) *svcsdk.DescribeResourcePoliciesOutput {
	name := meta.GetExternalName(cr)
	for _, p := range list.ResourcePolicies {
		if pointer.StringValue(p.PolicyName) == name {
			return &svcsdk.DescribeResourcePoliciesOutput{ResourcePolicies: []svcsdktypes.ResourcePolicy{p}}
		}
	}
	return &svcsdk.DescribeResourcePoliciesOutput{}
}

func postObserve(_ context.Context, cr *svcapitypes.ResourcePolicy, _ *svcsdk.DescribeResourcePoliciesOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return obs, err
	}
	cr.SetConditions(xpv2.Available())
	return obs, nil
}

// isUpToDate compares the policy documents as JSON: AWS may return them
// formatted differently.
func isUpToDate(_ context.Context, cr *svcapitypes.ResourcePolicy, resp *svcsdk.DescribeResourcePoliciesOutput) (bool, string, error) {
	var diff []string
	if !compare.PolicyEqual(cr.Spec.ForProvider.PolicyDocument, resp.ResourcePolicies[0].PolicyDocument) {
		diff = append(diff, fieldPolicyDocument)
	}
	return len(diff) == 0, compare.Message(diff), nil
}

// prePut sets the name for PutResourcePolicy, which creates and updates.
func prePut(_ context.Context, cr *svcapitypes.ResourcePolicy, obj *svcsdk.PutResourcePolicyInput) error {
	obj.PolicyName = aws.String(meta.GetExternalName(cr))
	return nil
}

func preDelete(_ context.Context, cr *svcapitypes.ResourcePolicy, obj *svcsdk.DeleteResourcePolicyInput) (bool, error) {
	obj.PolicyName = aws.String(meta.GetExternalName(cr))
	return false, nil
}

func setupExternal(e *external) {
	e.preObserve = preObserve
	e.filterList = filterList
	e.postObserve = postObserve
	e.isUpToDate = isUpToDate
	e.preCreate = prePut
	e.preUpdate = prePut
	e.preDelete = preDelete
}

func init() {
	configure = func(ctrl.Manager, controller.Options) ([]option, []managed.ReconcilerOption, error) {
		return []option{setupExternal}, nil, nil
	}
}

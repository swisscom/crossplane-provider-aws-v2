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

package securitygroupingressrule

import (
	"context"

	svcsdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/ec2/v1alpha1"
	svcutils "github.com/swisscom/crossplane-provider-aws-v2/internal/controller/ec2/utils"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
)

const (
	errNoRuleCreated = "AWS returned no security group rule"
	errInvalidRule   = "invalid rule"
)

type hooks struct {
	client Client
}

func ruleOf(cr *svcapitypes.SecurityGroupIngressRule) svcutils.Rule {
	p := cr.Spec.ForProvider
	return svcutils.Rule{
		GroupID:                   p.SecurityGroupID,
		IPProtocol:                p.IPProtocol,
		FromPort:                  p.FromPort,
		ToPort:                    p.ToPort,
		CIDRIPv4:                  p.CIDRIPv4,
		CIDRIPv6:                  p.CIDRIPv6,
		PrefixListID:              p.PrefixListID,
		ReferencedSecurityGroupID: p.ReferencedSecurityGroupID,
		Description:               p.Description,
	}
}

func preObserve(_ context.Context, cr *svcapitypes.SecurityGroupIngressRule, obj *svcsdk.DescribeSecurityGroupRulesInput) error {
	obj.SecurityGroupRuleIds = []string{meta.GetExternalName(cr)}
	return nil
}

func postObserve(_ context.Context, cr *svcapitypes.SecurityGroupIngressRule, resp *svcsdk.DescribeSecurityGroupRulesOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return obs, err
	}
	r := resp.SecurityGroupRules[0]
	cr.Status.AtProvider.SecurityGroupRuleID = r.SecurityGroupRuleId
	cr.Status.AtProvider.SecurityGroupRuleARN = r.SecurityGroupRuleArn
	cr.Status.AtProvider.GroupOwnerID = r.GroupOwnerId
	cr.Status.AtProvider.Tags = svcutils.FromSDK(r.Tags)
	cr.SetConditions(xpv2.Available())
	return obs, nil
}

func isUpToDate(_ context.Context, cr *svcapitypes.SecurityGroupIngressRule, resp *svcsdk.DescribeSecurityGroupRulesOutput) (bool, string, error) {
	r := resp.SecurityGroupRules[0]
	diff, err := ruleOf(cr).Diff(r, false)
	if err != nil {
		return false, "", err
	}
	add, remove := svcutils.DiffTags(svcutils.DesiredTags(cr, cr.Spec.ForProvider.Tags), svcutils.FromSDK(r.Tags))
	if len(add)+len(remove) > 0 {
		diff = append(diff, "tags")
	}
	return len(diff) == 0, compare.Message(diff), nil
}

func preCreate(_ context.Context, cr *svcapitypes.SecurityGroupIngressRule, obj *svcsdk.AuthorizeSecurityGroupIngressInput) error {
	r := ruleOf(cr)
	if err := r.Validate(); err != nil {
		return errors.Wrap(err, errInvalidRule)
	}
	// The generated input uses the legacy top-level fields, which can't carry
	// IPv6, prefix list or group sources; use IpPermissions instead.
	obj.IpProtocol, obj.FromPort, obj.ToPort = nil, nil, nil
	obj.GroupId = r.GroupID
	obj.IpPermissions = []svcsdktypes.IpPermission{r.IPPermission()}
	obj.TagSpecifications = svcutils.TagSpecifications(svcsdktypes.ResourceTypeSecurityGroupRule, svcutils.DesiredTags(cr, cr.Spec.ForProvider.Tags))
	return nil
}

func postCreate(_ context.Context, cr *svcapitypes.SecurityGroupIngressRule, resp *svcsdk.AuthorizeSecurityGroupIngressOutput, cre managed.ExternalCreation, err error) (managed.ExternalCreation, error) {
	if err != nil {
		return cre, err
	}
	if len(resp.SecurityGroupRules) == 0 {
		return cre, errors.New(errNoRuleCreated)
	}
	meta.SetExternalName(cr, *resp.SecurityGroupRules[0].SecurityGroupRuleId)
	return cre, nil
}

func preUpdate(_ context.Context, cr *svcapitypes.SecurityGroupIngressRule, obj *svcsdk.ModifySecurityGroupRulesInput) error {
	r := ruleOf(cr)
	if err := r.Validate(); err != nil {
		return errors.Wrap(err, errInvalidRule)
	}
	obj.GroupId = r.GroupID
	obj.SecurityGroupRules = []svcsdktypes.SecurityGroupRuleUpdate{r.RuleUpdate(meta.GetExternalName(cr))}
	return nil
}

func (h *hooks) postUpdate(ctx context.Context, cr *svcapitypes.SecurityGroupIngressRule, _ *svcsdk.ModifySecurityGroupRulesOutput, upd managed.ExternalUpdate, err error) (managed.ExternalUpdate, error) {
	if err != nil {
		return upd, err
	}
	// Observe ran in the same reconcile and stored the current tags.
	return upd, svcutils.UpdateTags(ctx, h.client, meta.GetExternalName(cr), svcutils.DesiredTags(cr, cr.Spec.ForProvider.Tags), cr.Status.AtProvider.Tags)
}

func preDelete(_ context.Context, cr *svcapitypes.SecurityGroupIngressRule, obj *svcsdk.RevokeSecurityGroupIngressInput) (bool, error) {
	obj.GroupId = cr.Spec.ForProvider.SecurityGroupID
	obj.SecurityGroupRuleIds = []string{meta.GetExternalName(cr)}
	return false, nil
}

// postDelete treats a deleted group as a deleted rule: AWS deletes the rules
// of a group with it.
func postDelete(_ context.Context, _ *svcapitypes.SecurityGroupIngressRule, _ *svcsdk.RevokeSecurityGroupIngressOutput, err error) (managed.ExternalDelete, error) {
	if svcutils.IsGroupNotFound(err) {
		return managed.ExternalDelete{}, nil
	}
	return managed.ExternalDelete{}, err
}

func setupExternal(e *external) {
	h := &hooks{client: e.client}
	e.preObserve = preObserve
	e.postObserve = postObserve
	e.isUpToDate = isUpToDate
	e.preCreate = preCreate
	e.postCreate = postCreate
	e.preUpdate = preUpdate
	e.postUpdate = h.postUpdate
	e.preDelete = preDelete
	e.postDelete = postDelete
}

func configureSecurityGroupIngressRule(mgr ctrl.Manager, _ controller.Options) ([]option, []managed.ReconcilerOption, error) {
	return []option{setupExternal}, []managed.ReconcilerOption{
		managed.WithReferenceResolver(managed.NewAPISimpleReferenceResolver(mgr.GetClient())),
		// AWS assigns the rule ID; the default would use metadata.name.
		managed.WithInitializers(),
	}, nil
}

func init() {
	configure = configureSecurityGroupIngressRule
}

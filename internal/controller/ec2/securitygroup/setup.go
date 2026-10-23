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

package securitygroup

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/ec2/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	svcutils "github.com/swisscom/crossplane-provider-aws-v2/internal/controller/ec2/utils"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

const (
	defaultGroupName = "default"

	errFmtImmutable      = "%s cannot be changed: it is %q in AWS but %q in the spec; delete and recreate the security group"
	errRevokeEgress      = "cannot revoke a default egress rule"
	errNoGroupID         = "AWS returned no security group ID"
	errRollback          = "cannot delete the security group after failing to revoke its default egress rules"
	errAdoptDescribe     = "cannot look up the security group by name"
	errFmtAdoptAmbiguous = "adoptExistingByName: %d security groups are named %q; set vpcID or the crossplane.io/external-name annotation"
	errAdoptDefault      = "adoptExistingByName: the default security group can't be adopted"
	errInUse             = "security group is still in use (by network interfaces or rules of other groups)"

	msgFmtAdopted = "Adopted existing security group %s (name %q, VPC %s) found by name"
)

const reasonAdopted event.Reason = "AdoptedExistingSecurityGroup"

// revokeBackoff is the wait before each retry of revoking the default egress
// rule of a new group, which AWS may not know yet (eventual consistency).
var revokeBackoff = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

type hooks struct {
	client Client
	// tags are the tags Observe saw: the late-init update replaces the
	// status before Update runs.
	tags     []*svcapitypes.Tag
	observed bool
	// tagKeysChanged makes Observe report a late-init so that the runtime
	// persists the managed tag keys.
	tagKeysChanged bool
}

// groupName returns the name of the group: spec.forProvider.groupName,
// default metadata.name.
func groupName(cr *svcapitypes.SecurityGroup) string {
	if n := pointer.StringValue(cr.Spec.ForProvider.GroupName); n != "" {
		return n
	}
	return cr.GetName()
}

func preObserve(_ context.Context, cr *svcapitypes.SecurityGroup, obj *svcsdk.DescribeSecurityGroupsInput) error {
	obj.GroupIds = []string{meta.GetExternalName(cr)}
	return nil
}

func (h *hooks) postObserve(_ context.Context, cr *svcapitypes.SecurityGroup, _ *svcsdk.DescribeSecurityGroupsOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return obs, err
	}
	obs.ResourceLateInitialized = obs.ResourceLateInitialized || h.tagKeysChanged
	cr.SetConditions(xpv2.Available())
	return obs, nil
}

func lateInitialize(spec *svcapitypes.SecurityGroupParameters, resp *svcsdk.DescribeSecurityGroupsOutput) error {
	g := resp.SecurityGroups[0]
	spec.GroupName = pointer.LateInitialize(spec.GroupName, g.GroupName)
	spec.VPCID = pointer.LateInitialize(spec.VPCID, g.VpcId)
	return nil
}

// isUpToDate returns an error for differences AWS can't change in place: the
// name, description and VPC of a group are fixed. Only tags are updated.
func (h *hooks) isUpToDate(_ context.Context, cr *svcapitypes.SecurityGroup, resp *svcsdk.DescribeSecurityGroupsOutput) (bool, string, error) {
	g := resp.SecurityGroups[0]
	p := cr.Spec.ForProvider
	for _, f := range []struct {
		name            string
		desired, actual *string
	}{
		{"groupName", p.GroupName, g.GroupName},
		{"description", p.Description, g.Description},
		{"vpcID", p.VPCID, g.VpcId},
	} {
		if f.desired != nil && *f.desired != pointer.StringValue(f.actual) {
			return false, "", errors.Errorf(errFmtImmutable, f.name, pointer.StringValue(f.actual), *f.desired)
		}
	}
	h.tags, h.observed = svcutils.FromSDK(g.Tags), true
	upToDate, changed := svcutils.ObserveTags(cr, svcutils.DesiredTags(cr, p.Tags), h.tags)
	h.tagKeysChanged = changed
	if !upToDate {
		return false, compare.Message([]string{"tags"}), nil
	}
	return true, "", nil
}

func (h *hooks) update(ctx context.Context, cr *svcapitypes.SecurityGroup) (managed.ExternalUpdate, error) {
	current := cr.Status.AtProvider.Tags
	if h.observed {
		current = h.tags
	}
	return managed.ExternalUpdate{}, svcutils.UpdateTags(ctx, h.client, cr, meta.GetExternalName(cr), svcutils.DesiredTags(cr, cr.Spec.ForProvider.Tags), current)
}

func preCreate(_ context.Context, cr *svcapitypes.SecurityGroup, obj *svcsdk.CreateSecurityGroupInput) error {
	obj.GroupName = aws.String(groupName(cr))
	obj.TagSpecifications = svcutils.TagSpecifications(svcsdktypes.ResourceTypeSecurityGroup, svcutils.DesiredTags(cr, cr.Spec.ForProvider.Tags))
	return nil
}

// postCreate revokes the rules allowing all egress that AWS adds to every new
// group, so that egress is only what SecurityGroupEgressRules allow. If that
// fails, it deletes the group again: a later reconcile would not retry the
// revoke, but it recreates a group that is gone.
func (h *hooks) postCreate(ctx context.Context, cr *svcapitypes.SecurityGroup, resp *svcsdk.CreateSecurityGroupOutput, cre managed.ExternalCreation, err error) (managed.ExternalCreation, error) {
	if err != nil {
		return cre, err
	}
	// Without the AWS-assigned ID the group can't be found again.
	if resp == nil || resp.GroupId == nil {
		return cre, errors.New(errNoGroupID)
	}
	id := *resp.GroupId
	meta.SetExternalName(cr, id)

	if err := h.revokeDefaultEgress(ctx, id); err != nil {
		if _, derr := h.client.DeleteSecurityGroup(ctx, &svcsdk.DeleteSecurityGroupInput{GroupId: aws.String(id)}); derr != nil {
			return cre, errors.Wrap(awsclient.Wrap(derr, errRollback), err.Error())
		}
		return cre, err
	}
	return cre, nil
}

// defaultEgress are the rules AWS adds to a new group: all IPv4 egress, and
// all IPv6 egress if the VPC has an IPv6 CIDR.
var defaultEgress = []svcsdktypes.IpPermission{
	{IpProtocol: aws.String("-1"), IpRanges: []svcsdktypes.IpRange{{CidrIp: aws.String("0.0.0.0/0")}}},
	{IpProtocol: aws.String("-1"), Ipv6Ranges: []svcsdktypes.Ipv6Range{{CidrIpv6: aws.String("::/0")}}},
}

// revokeDefaultEgress revokes each default rule in its own call, so that a
// missing one doesn't keep the other.
func (h *hooks) revokeDefaultEgress(ctx context.Context, id string) error {
	for _, p := range defaultEgress {
		if err := h.revokeEgress(ctx, id, p); err != nil {
			return err
		}
	}
	return nil
}

func (h *hooks) revokeEgress(ctx context.Context, id string, p svcsdktypes.IpPermission) error {
	in := &svcsdk.RevokeSecurityGroupEgressInput{GroupId: aws.String(id), IpPermissions: []svcsdktypes.IpPermission{p}}
	for i := 0; ; i++ {
		_, err := h.client.RevokeSecurityGroupEgress(ctx, in)
		switch {
		case err == nil, svcutils.ErrorCode(err) == svcutils.CodePermissionNotFound:
			return nil
		case !svcutils.IsGroupNotFound(err) || i == len(revokeBackoff):
			return awsclient.Wrap(err, errRevokeEgress)
		}
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), errRevokeEgress)
		case <-time.After(revokeBackoff[i]):
		}
	}
}

func preDelete(_ context.Context, cr *svcapitypes.SecurityGroup, obj *svcsdk.DeleteSecurityGroupInput) (bool, error) {
	obj.GroupName = nil
	obj.GroupId = aws.String(meta.GetExternalName(cr))
	return false, nil
}

func postDelete(_ context.Context, _ *svcapitypes.SecurityGroup, _ *svcsdk.DeleteSecurityGroupOutput, err error) (managed.ExternalDelete, error) {
	if svcutils.ErrorCode(err) == svcutils.CodeDependency {
		return managed.ExternalDelete{}, errors.Wrap(err, errInUse)
	}
	return managed.ExternalDelete{}, err
}

// adoptingExternal adds spec.forProvider.adoptExistingByName to the
// generated client: without an external name, it looks the group up by name.
type adoptingExternal struct {
	*external
	record event.Recorder
}

func (e *adoptingExternal) Observe(ctx context.Context, cr *svcapitypes.SecurityGroup) (managed.ExternalObservation, error) {
	if meta.GetExternalName(cr) != "" || !pointer.BoolValue(cr.Spec.ForProvider.AdoptExistingByName) {
		return e.external.Observe(ctx, cr)
	}
	g, err := e.findByName(ctx, cr)
	if err != nil || g == nil {
		return managed.ExternalObservation{}, err
	}
	id := pointer.StringValue(g.GroupId)
	meta.SetExternalName(cr, id)
	obs, err := e.external.Observe(ctx, cr)
	if err == nil && obs.ResourceExists {
		e.record.Event(cr, event.Normal(reasonAdopted, fmt.Sprintf(msgFmtAdopted, id, pointer.StringValue(g.GroupName), pointer.StringValue(g.VpcId))))
	}
	// Have the runtime save the external name.
	obs.ResourceLateInitialized = true
	return obs, err
}

// findByName returns the group with the name of cr, in the VPC of cr if
// set, or nil if there is none.
func (e *adoptingExternal) findByName(ctx context.Context, cr *svcapitypes.SecurityGroup) (*svcsdktypes.SecurityGroup, error) {
	name := groupName(cr)
	if name == defaultGroupName {
		return nil, errors.New(errAdoptDefault)
	}
	filters := []svcsdktypes.Filter{{Name: aws.String("group-name"), Values: []string{name}}}
	if vpc := pointer.StringValue(cr.Spec.ForProvider.VPCID); vpc != "" {
		filters = append(filters, svcsdktypes.Filter{Name: aws.String("vpc-id"), Values: []string{vpc}})
	}
	resp, err := e.client.DescribeSecurityGroups(ctx, &svcsdk.DescribeSecurityGroupsInput{Filters: filters})
	if err != nil {
		return nil, awsclient.Wrap(err, errAdoptDescribe)
	}
	switch n := len(resp.SecurityGroups); n {
	case 0:
		return nil, nil
	case 1:
		return &resp.SecurityGroups[0], nil
	default:
		return nil, errors.Errorf(errFmtAdoptAmbiguous, n, name)
	}
}

// adoptingWrapper returns a wrapExternal that emits adoption events to rec.
func adoptingWrapper(rec event.Recorder) func(*external) managed.TypedExternalClient[*svcapitypes.SecurityGroup] {
	return func(e *external) managed.TypedExternalClient[*svcapitypes.SecurityGroup] {
		return &adoptingExternal{external: e, record: rec}
	}
}

func setupExternal(e *external) {
	h := &hooks{client: e.client}
	e.preObserve = preObserve
	e.postObserve = h.postObserve
	e.lateInitialize = lateInitialize
	e.isUpToDate = h.isUpToDate
	e.update = h.update
	e.preCreate = preCreate
	e.postCreate = h.postCreate
	e.preDelete = preDelete
	e.postDelete = postDelete
}

func configureSecurityGroup(mgr ctrl.Manager, _ controller.Options) ([]option, []managed.ReconcilerOption, error) {
	// Setup reads wrapExternal only after configure, in Connect.
	wrapExternal = adoptingWrapper(event.NewAPIRecorder(mgr.GetEventRecorderFor(managed.ControllerName(svcapitypes.SecurityGroupGroupKind)))) //nolint:staticcheck // same recorder as the generated controller
	return []option{setupExternal}, []managed.ReconcilerOption{
		managed.WithReferenceResolver(managed.NewAPISimpleReferenceResolver(mgr.GetClient())),
		// AWS assigns the group ID; the default would use metadata.name.
		managed.WithInitializers(),
	}, nil
}

func init() {
	configure = configureSecurityGroup
	wrapExternal = adoptingWrapper(event.NewNopRecorder())
}

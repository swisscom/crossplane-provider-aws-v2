/*
Copyright 2021 The Crossplane Authors.

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

package dbsubnetgroup

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/docdb"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	cpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	svcutils "github.com/swisscom/crossplane-provider-aws-v2/internal/controller/docdb/utils"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

type hooks struct {
	client Client
	kube   client.Client
	// arn is cached by Observe: the late-init update replaces the status
	// before Update runs.
	arn *string
	// tagKeysChanged makes Observe report a late-init so that the runtime
	// persists the managed tag keys.
	tagKeysChanged bool
}

// observeTags records the ARN and the managed tag keys for Update.
func (e *hooks) observeTags(ctx context.Context, mg cpresource.Managed, spec []*svcapitypes.Tag, arn *string) (bool, error) {
	e.arn = arn
	upToDate, changed, err := svcutils.ObserveTags(ctx, e.client, mg, spec, arn)
	e.tagKeysChanged = changed
	return upToDate, err
}

// tagARN prefers the ARN of this reconcile's Observe.
func (e *hooks) tagARN(status *string) *string {
	if e.arn != nil {
		return e.arn
	}
	return status
}

func preObserve(_ context.Context, cr *svcapitypes.DBSubnetGroup, obj *svcsdk.DescribeDBSubnetGroupsInput) error {
	obj.DBSubnetGroupName = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	return nil
}

func (e *hooks) postObserve(_ context.Context, cr *svcapitypes.DBSubnetGroup, _ *svcsdk.DescribeDBSubnetGroupsOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return obs, err
	}
	obs.ResourceLateInitialized = obs.ResourceLateInitialized || e.tagKeysChanged

	cr.SetConditions(xpv2.Available())
	return obs, nil
}

func (e *hooks) isUpToDate(ctx context.Context, cr *svcapitypes.DBSubnetGroup, resp *svcsdk.DescribeDBSubnetGroupsOutput) (bool, string, error) {
	group := resp.DBSubnetGroups[0]

	diff := DiffUpdatableParameters(&cr.Spec.ForProvider, &GenerateDBSubnetGroup(resp).Spec.ForProvider)
	if !areSubnetsEqual(cr.Spec.ForProvider.SubnetIDs, group.Subnets) {
		diff = append(diff, "subnetIDs")
	}
	// Always, so new spec keys are recorded before Update adds them.
	tagsUpToDate, err := e.observeTags(ctx, cr, cr.Spec.ForProvider.Tags, group.DBSubnetGroupArn)
	if err != nil {
		return false, "", err
	}
	if !tagsUpToDate {
		diff = append(diff, "tags")
	}
	return len(diff) == 0, compare.Message(diff), nil
}

// areSubnetsEqual reports whether the subnet group consists of exactly the
// subnets of the spec, in any order.
func areSubnetsEqual(specSubnetIds []*string, current []svcsdktypes.Subnet) bool {
	if len(specSubnetIds) != len(current) {
		return false
	}

	currentMap := make(map[string]struct{}, len(current))
	for _, s := range current {
		currentMap[pointer.StringValue(s.SubnetIdentifier)] = struct{}{}
	}

	for _, spec := range specSubnetIds {
		if _, exists := currentMap[pointer.StringValue(spec)]; !exists {
			return false
		}
	}

	return true
}

func preUpdate(_ context.Context, cr *svcapitypes.DBSubnetGroup, obj *svcsdk.ModifyDBSubnetGroupInput) error {
	obj.DBSubnetGroupName = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	obj.SubnetIds = pointer.SlicePtrToValue(cr.Spec.ForProvider.SubnetIDs)
	return nil
}

func (e *hooks) postUpdate(ctx context.Context, cr *svcapitypes.DBSubnetGroup, resp *svcsdk.ModifyDBSubnetGroupOutput, upd managed.ExternalUpdate, err error) (managed.ExternalUpdate, error) {
	if err != nil {
		return upd, err
	}

	cr.Status.SetConditions(xpv2.Available())
	var arn *string
	if resp != nil && resp.DBSubnetGroup != nil {
		arn = resp.DBSubnetGroup.DBSubnetGroupArn
	}
	return upd, svcutils.UpdateTagsForResource(ctx, e.client, cr, cr.Spec.ForProvider.Tags, e.tagARN(arn))
}

func preCreate(_ context.Context, cr *svcapitypes.DBSubnetGroup, obj *svcsdk.CreateDBSubnetGroupInput) error {
	obj.DBSubnetGroupName = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	obj.SubnetIds = pointer.SlicePtrToValue(cr.Spec.ForProvider.SubnetIDs)
	return nil
}

func preDelete(_ context.Context, cr *svcapitypes.DBSubnetGroup, obj *svcsdk.DeleteDBSubnetGroupInput) (bool, error) {
	obj.DBSubnetGroupName = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	return false, nil
}

func filterList(cr *svcapitypes.DBSubnetGroup, list *svcsdk.DescribeDBSubnetGroupsOutput) *svcsdk.DescribeDBSubnetGroupsOutput {
	name := meta.GetExternalName(cr)
	for _, group := range list.DBSubnetGroups {
		if compare.IdentifierEqual(aws.ToString(group.DBSubnetGroupName), name) {
			return &svcsdk.DescribeDBSubnetGroupsOutput{
				Marker:         list.Marker,
				DBSubnetGroups: []svcsdktypes.DBSubnetGroup{group},
			}
		}
	}

	return &svcsdk.DescribeDBSubnetGroupsOutput{
		Marker:         list.Marker,
		DBSubnetGroups: []svcsdktypes.DBSubnetGroup{},
	}
}

func setupExternal(e *external) {
	h := &hooks{client: e.client, kube: e.kube}
	e.preObserve = preObserve
	e.postObserve = h.postObserve
	e.isUpToDate = h.isUpToDate
	e.preUpdate = preUpdate
	e.postUpdate = h.postUpdate
	e.preCreate = preCreate
	e.preDelete = preDelete
	e.filterList = filterList
}

func configureDBSubnetGroup(mgr ctrl.Manager, _ controller.Options) ([]option, []managed.ReconcilerOption, error) {
	return []option{setupExternal}, []managed.ReconcilerOption{
		managed.WithReferenceResolver(managed.NewAPISimpleReferenceResolver(mgr.GetClient())),
		managed.WithInitializers(managed.NewNameAsExternalName(mgr.GetClient())),
	}, nil
}

func init() {
	configure = configureDBSubnetGroup
}

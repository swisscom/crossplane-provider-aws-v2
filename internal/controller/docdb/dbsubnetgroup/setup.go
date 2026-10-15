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

	svcsdk "github.com/aws/aws-sdk-go-v2/service/docdb"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
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
}

func preObserve(_ context.Context, cr *svcapitypes.DBSubnetGroup, obj *svcsdk.DescribeDBSubnetGroupsInput) error {
	obj.DBSubnetGroupName = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	return nil
}

func postObserve(_ context.Context, cr *svcapitypes.DBSubnetGroup, _ *svcsdk.DescribeDBSubnetGroupsOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return obs, err
	}

	cr.SetConditions(xpv2.Available())
	return obs, nil
}

func (e *hooks) isUpToDate(ctx context.Context, cr *svcapitypes.DBSubnetGroup, resp *svcsdk.DescribeDBSubnetGroupsOutput) (bool, string, error) {
	group := resp.DBSubnetGroups[0]

	diff := DiffUpdatableParameters(&cr.Spec.ForProvider, &GenerateDBSubnetGroup(resp).Spec.ForProvider)
	if !areSubnetsEqual(cr.Spec.ForProvider.SubnetIDs, group.Subnets) {
		diff = append(diff, "subnetIDs")
	}
	if len(diff) > 0 {
		return false, compare.Message(diff), nil
	}

	areTagsUpToDate, err := svcutils.AreTagsUpToDate(ctx, e.client, cr.Spec.ForProvider.Tags, group.DBSubnetGroupArn)
	if err != nil || !areTagsUpToDate {
		return false, compare.Message([]string{"tags"}), err
	}
	return true, "", nil
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
	return upd, svcutils.UpdateTagsForResource(ctx, e.client, cr.Spec.ForProvider.Tags, resp.DBSubnetGroup.DBSubnetGroupArn)
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
		if pointer.StringValue(group.DBSubnetGroupName) == name {
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
	e.postObserve = postObserve
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

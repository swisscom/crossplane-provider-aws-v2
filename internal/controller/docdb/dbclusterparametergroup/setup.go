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

package dbclusterparametergroup

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/docdb"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
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

const (
	errModifyFamily       = "cannot modify DBParameterGroupFamily of an existing DBClusterParameterGroup"
	errModifyDescription  = "cannot modify Description of an existing DBClusterParameterGroup"
	errDescribeParameters = "cannot describe parameters for DBClusterParameterGroup"
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

func preObserve(_ context.Context, cr *svcapitypes.DBClusterParameterGroup, obj *svcsdk.DescribeDBClusterParameterGroupsInput) error {
	obj.DBClusterParameterGroupName = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	return nil
}

func (e *hooks) postObserve(_ context.Context, cr *svcapitypes.DBClusterParameterGroup, _ *svcsdk.DescribeDBClusterParameterGroupsOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return obs, err
	}
	obs.ResourceLateInitialized = obs.ResourceLateInitialized || e.tagKeysChanged

	cr.SetConditions(xpv2.Available())
	return obs, err
}

func preCreate(_ context.Context, cr *svcapitypes.DBClusterParameterGroup, obj *svcsdk.CreateDBClusterParameterGroupInput) error {
	obj.DBClusterParameterGroupName = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	// CreateDBClusterParameterGroup does not create the parameters themselves. Parameters are added during update.
	return nil
}

func preDelete(_ context.Context, cr *svcapitypes.DBClusterParameterGroup, obj *svcsdk.DeleteDBClusterParameterGroupInput) (bool, error) {
	obj.DBClusterParameterGroupName = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	return false, nil
}

// isUpToDate compares the generated updatable fields, the parameters and
// the tags. AWS can't change the family or the description of an existing
// group; a change of them is reported as an error.
func (e *hooks) isUpToDate(ctx context.Context, cr *svcapitypes.DBClusterParameterGroup, resp *svcsdk.DescribeDBClusterParameterGroupsOutput) (bool, string, error) {
	group := resp.DBClusterParameterGroups[0]

	if pointer.StringValue(cr.Spec.ForProvider.DBParameterGroupFamily) != pointer.StringValue(group.DBParameterGroupFamily) {
		return false, "", errors.New(errModifyFamily)
	}

	if pointer.StringValue(cr.Spec.ForProvider.Description) != pointer.StringValue(group.Description) {
		return false, "", errors.New(errModifyDescription)
	}

	diff := DiffUpdatableParameters(&cr.Spec.ForProvider, &GenerateDBClusterParameterGroup(resp).Spec.ForProvider)

	params, err := e.describeParameters(ctx, group.DBClusterParameterGroupName)
	if err != nil {
		return false, "", err
	}
	if len(changedParameters(cr.Spec.ForProvider.Parameters, params)) > 0 {
		diff = append(diff, "parameters")
	}
	// Always, so new spec keys are recorded before Update adds them.
	tagsUpToDate, err := e.observeTags(ctx, cr, cr.Spec.ForProvider.Tags, group.DBClusterParameterGroupArn)
	if err != nil {
		return false, "", err
	}
	if !tagsUpToDate {
		diff = append(diff, "tags")
	}
	return len(diff) == 0, compare.Message(diff), nil
}

func (e *hooks) postUpdate(ctx context.Context, cr *svcapitypes.DBClusterParameterGroup, _ *svcsdk.ModifyDBClusterParameterGroupOutput, upd managed.ExternalUpdate, err error) (managed.ExternalUpdate, error) {
	if err != nil {
		return upd, err
	}
	cr.Status.SetConditions(xpv2.Available())
	return upd, svcutils.UpdateTagsForResource(ctx, e.client, cr, cr.Spec.ForProvider.Tags, e.tagARN(cr.Status.AtProvider.DBClusterParameterGroupARN))
}

func filterList(cr *svcapitypes.DBClusterParameterGroup, list *svcsdk.DescribeDBClusterParameterGroupsOutput) *svcsdk.DescribeDBClusterParameterGroupsOutput {
	id := meta.GetExternalName(cr)
	for _, group := range list.DBClusterParameterGroups {
		if !compare.IdentifierEqual(aws.ToString(group.DBClusterParameterGroupName), id) {
			continue
		}
		// A deleted MR lets go of a foreign group without touching it.
		if meta.WasDeleted(cr) && svcutils.CheckParameterGroupFamily(id, group.DBParameterGroupFamily) != nil {
			break
		}
		return &svcsdk.DescribeDBClusterParameterGroupsOutput{
			Marker:                   list.Marker,
			DBClusterParameterGroups: []svcsdktypes.DBClusterParameterGroup{group},
		}
	}

	return &svcsdk.DescribeDBClusterParameterGroupsOutput{
		Marker:                   list.Marker,
		DBClusterParameterGroups: []svcsdktypes.DBClusterParameterGroup{},
	}
}

func setupExternal(e *external) {
	h := &hooks{client: e.client, kube: e.kube}
	e.preObserve = preObserve
	e.postObserve = h.postObserve
	e.isUpToDate = h.isUpToDate
	e.postUpdate = h.postUpdate
	e.preCreate = preCreate
	e.preDelete = preDelete
	e.filterList = filterList
	e.lateInitialize = h.lateInitialize
}

func (e *hooks) lateInitialize(cr *svcapitypes.DBClusterParameterGroupParameters, resp *svcsdk.DescribeDBClusterParameterGroupsOutput) error {
	group := resp.DBClusterParameterGroups[0]
	// First, so a foreign group is neither read nor copied into the spec.
	if err := svcutils.CheckParameterGroupFamily(aws.ToString(group.DBClusterParameterGroupName), group.DBParameterGroupFamily); err != nil {
		return err
	}

	// The lateInitialize hook has no context.
	params, err := e.describeParameters(context.TODO(), group.DBClusterParameterGroupName)
	if err != nil {
		return err
	}

	cr.Parameters = lateInitializeParameters(cr.Parameters, params)
	return nil
}

func configureDBClusterParameterGroup(mgr ctrl.Manager, _ controller.Options) ([]option, []managed.ReconcilerOption, error) {
	return []option{setupExternal}, []managed.ReconcilerOption{
		managed.WithReferenceResolver(managed.NewAPISimpleReferenceResolver(mgr.GetClient())),
		managed.WithInitializers(managed.NewNameAsExternalName(mgr.GetClient())),
	}, nil
}

func init() {
	configure = configureDBClusterParameterGroup
	wrapExternal = wrap
}

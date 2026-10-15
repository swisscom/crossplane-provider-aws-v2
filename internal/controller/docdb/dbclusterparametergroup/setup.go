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
}

func preObserve(_ context.Context, cr *svcapitypes.DBClusterParameterGroup, obj *svcsdk.DescribeDBClusterParameterGroupsInput) error {
	obj.DBClusterParameterGroupName = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	return nil
}

func postObserve(_ context.Context, cr *svcapitypes.DBClusterParameterGroup, _ *svcsdk.DescribeDBClusterParameterGroupsOutput, obs managed.ExternalObservation, err error) (managed.ExternalObservation, error) {
	if err != nil {
		return obs, err
	}

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
	if !areParametersEqual(cr.Spec.ForProvider.Parameters, params) {
		diff = append(diff, "parameters")
	}
	if len(diff) > 0 {
		return false, compare.Message(diff), nil
	}

	areTagsUpToDate, err := svcutils.AreTagsUpToDate(ctx, e.client, cr.Spec.ForProvider.Tags, group.DBClusterParameterGroupArn)
	if err != nil || !areTagsUpToDate {
		return false, compare.Message([]string{"tags"}), err
	}
	return true, "", nil
}

// describeParameters returns all parameters of the group, following the
// pagination markers.
func (e *hooks) describeParameters(ctx context.Context, name *string) ([]svcsdktypes.Parameter, error) {
	var params []svcsdktypes.Parameter
	in := &svcsdk.DescribeDBClusterParametersInput{DBClusterParameterGroupName: name}
	for {
		out, err := e.client.DescribeDBClusterParameters(ctx, in)
		if err != nil {
			return nil, errors.Wrap(err, errDescribeParameters)
		}
		params = append(params, out.Parameters...)
		if aws.ToString(out.Marker) == "" {
			return params, nil
		}
		in = &svcsdk.DescribeDBClusterParametersInput{DBClusterParameterGroupName: name, Marker: out.Marker}
	}
}

func preUpdate(_ context.Context, cr *svcapitypes.DBClusterParameterGroup, obj *svcsdk.ModifyDBClusterParameterGroupInput) error {
	obj.DBClusterParameterGroupName = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	obj.Parameters = generateSdkParameters(cr.Spec.ForProvider.Parameters)
	return nil
}

func (e *hooks) postUpdate(ctx context.Context, cr *svcapitypes.DBClusterParameterGroup, _ *svcsdk.ModifyDBClusterParameterGroupOutput, upd managed.ExternalUpdate, err error) (managed.ExternalUpdate, error) {
	if err != nil {
		return upd, err
	}
	cr.Status.SetConditions(xpv2.Available())
	return upd, svcutils.UpdateTagsForResource(ctx, e.client, cr.Spec.ForProvider.Tags, cr.Status.AtProvider.DBClusterParameterGroupARN)
}

func filterList(cr *svcapitypes.DBClusterParameterGroup, list *svcsdk.DescribeDBClusterParameterGroupsOutput) *svcsdk.DescribeDBClusterParameterGroupsOutput {
	id := meta.GetExternalName(cr)
	for _, group := range list.DBClusterParameterGroups {
		if pointer.StringValue(group.DBClusterParameterGroupName) == id {
			return &svcsdk.DescribeDBClusterParameterGroupsOutput{
				Marker:                   list.Marker,
				DBClusterParameterGroups: []svcsdktypes.DBClusterParameterGroup{group},
			}
		}
	}

	return &svcsdk.DescribeDBClusterParameterGroupsOutput{
		Marker:                   list.Marker,
		DBClusterParameterGroups: []svcsdktypes.DBClusterParameterGroup{},
	}
}

func generateSdkParameters(params []*svcapitypes.CustomParameter) []svcsdktypes.Parameter {
	sdkParams := make([]svcsdktypes.Parameter, len(params))
	for i, p := range params {
		sdkParams[i] = svcsdktypes.Parameter{
			ApplyMethod:    svcsdktypes.ApplyMethod(pointer.StringValue(p.ApplyMethod)),
			ParameterName:  p.ParameterName,
			ParameterValue: p.ParameterValue,
		}
	}

	return sdkParams
}

func generateAPIParameter(p *svcsdktypes.Parameter, o *svcapitypes.CustomParameter) *svcapitypes.CustomParameter {
	o.ApplyMethod = pointer.ToOrNilIfZeroValue(string(p.ApplyMethod))
	o.ParameterName = p.ParameterName
	o.ParameterValue = p.ParameterValue

	return o
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
	e.lateInitialize = h.lateInitialize
}

func (e *hooks) lateInitialize(cr *svcapitypes.DBClusterParameterGroupParameters, resp *svcsdk.DescribeDBClusterParameterGroupsOutput) error {
	group := resp.DBClusterParameterGroups[0]

	// The lateInitialize hook has no context.
	params, err := e.describeParameters(context.TODO(), group.DBClusterParameterGroupName)
	if err != nil {
		return err
	}

	cr.Parameters = lateInitializeParameters(cr.Parameters, params)
	return nil
}

// lateInitializeParameters adds the parameters of the group that the spec
// does not list.
func lateInitializeParameters(in []*svcapitypes.CustomParameter, from []svcsdktypes.Parameter) []*svcapitypes.CustomParameter {
	out := in
	if out == nil {
		out = []*svcapitypes.CustomParameter{}
	}

	apiParams := make(map[string]*svcapitypes.CustomParameter, len(out))
	for _, p := range out {
		apiParams[pointer.StringValue(p.ParameterName)] = p
	}

	for i := range from {
		if _, exists := apiParams[pointer.StringValue(from[i].ParameterName)]; !exists {
			out = append(out, generateAPIParameter(&from[i], &svcapitypes.CustomParameter{}))
		}
	}

	return out
}

// areParametersEqual reports whether every parameter of the spec has the
// same value in AWS. The apply method is not compared: AWS only changes it
// together with the value.
func areParametersEqual(spec []*svcapitypes.CustomParameter, current []svcsdktypes.Parameter) bool {
	currentMap := make(map[string]string, len(current))
	for _, currentParam := range current {
		currentMap[pointer.StringValue(currentParam.ParameterName)] = pointer.StringValue(currentParam.ParameterValue)
	}

	for _, specParam := range spec {
		currentValue, exists := currentMap[pointer.StringValue(specParam.ParameterName)]
		if !exists || pointer.StringValue(specParam.ParameterValue) != currentValue {
			return false
		}
	}

	return true
}

func configureDBClusterParameterGroup(mgr ctrl.Manager, _ controller.Options) ([]option, []managed.ReconcilerOption, error) {
	return []option{setupExternal}, []managed.ReconcilerOption{
		managed.WithReferenceResolver(managed.NewAPISimpleReferenceResolver(mgr.GetClient())),
		managed.WithInitializers(managed.NewNameAsExternalName(mgr.GetClient())),
	}, nil
}

func init() {
	configure = configureDBClusterParameterGroup
}

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

package dbclusterparametergroup

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/docdb"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

// maxParametersPerModify is AWS's limit for ModifyDBClusterParameterGroup.
const maxParametersPerModify = 20

// customExternal sends only the changed parameters, in batches AWS accepts.
type customExternal struct {
	*external
}

func wrap(e *external) managed.TypedExternalClient[*svcapitypes.DBClusterParameterGroup] {
	return &customExternal{external: e}
}

func (c *customExternal) Update(ctx context.Context, cr *svcapitypes.DBClusterParameterGroup) (managed.ExternalUpdate, error) {
	h := &hooks{client: c.client}
	name := pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
	current, err := h.describeParameters(ctx, name)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	changed := changedParameters(cr.Spec.ForProvider.Parameters, current)
	for len(changed) > 0 {
		n := min(len(changed), maxParametersPerModify)
		in := &svcsdk.ModifyDBClusterParameterGroupInput{DBClusterParameterGroupName: name, Parameters: changed[:n]}
		if _, err := c.client.ModifyDBClusterParameterGroup(ctx, in); err != nil {
			return managed.ExternalUpdate{}, awsclient.Wrap(err, errUpdate)
		}
		changed = changed[n:]
	}
	return c.postUpdate(ctx, cr, nil, managed.ExternalUpdate{}, nil)
}

// describeParameters returns all parameters of the group, following the
// pagination markers.
func (e *hooks) describeParameters(ctx context.Context, name *string) ([]svcsdktypes.Parameter, error) {
	var params []svcsdktypes.Parameter
	in := &svcsdk.DescribeDBClusterParametersInput{DBClusterParameterGroupName: name}
	for {
		out, err := e.client.DescribeDBClusterParameters(ctx, in)
		if err != nil {
			return nil, awsclient.Wrap(err, errDescribeParameters)
		}
		params = append(params, out.Parameters...)
		if aws.ToString(out.Marker) == "" {
			return params, nil
		}
		in = &svcsdk.DescribeDBClusterParametersInput{DBClusterParameterGroupName: name, Marker: out.Marker}
	}
}

// changedParameters returns the spec parameters whose value AWS does not
// have. The apply method is not compared: AWS only changes it together with
// the value.
func changedParameters(spec []*svcapitypes.CustomParameter, current []svcsdktypes.Parameter) []svcsdktypes.Parameter {
	currentMap := make(map[string]string, len(current))
	for _, p := range current {
		currentMap[pointer.StringValue(p.ParameterName)] = pointer.StringValue(p.ParameterValue)
	}
	var changed []svcsdktypes.Parameter
	for _, p := range spec {
		if v, ok := currentMap[pointer.StringValue(p.ParameterName)]; !ok || v != pointer.StringValue(p.ParameterValue) {
			changed = append(changed, generateSdkParameter(p))
		}
	}
	return changed
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
		// The CRD requires a name and a value; a nil one would fail the update.
		if from[i].ParameterName == nil || from[i].ParameterValue == nil {
			continue
		}
		if _, exists := apiParams[pointer.StringValue(from[i].ParameterName)]; !exists {
			out = append(out, generateAPIParameter(&from[i], &svcapitypes.CustomParameter{}))
		}
	}

	return out
}

// applyMethodOrDefault falls back to pending-reboot, which AWS accepts for
// static and dynamic parameters.
func applyMethodOrDefault(m string) svcsdktypes.ApplyMethod {
	if m == "" {
		return svcsdktypes.ApplyMethodPendingReboot
	}
	return svcsdktypes.ApplyMethod(m)
}

func generateSdkParameter(p *svcapitypes.CustomParameter) svcsdktypes.Parameter {
	return svcsdktypes.Parameter{
		ApplyMethod:    applyMethodOrDefault(pointer.StringValue(p.ApplyMethod)),
		ParameterName:  p.ParameterName,
		ParameterValue: p.ParameterValue,
	}
}

func generateAPIParameter(p *svcsdktypes.Parameter, o *svcapitypes.CustomParameter) *svcapitypes.CustomParameter {
	o.ApplyMethod = aws.String(string(applyMethodOrDefault(string(p.ApplyMethod))))
	o.ParameterName = p.ParameterName
	o.ParameterValue = p.ParameterValue

	return o
}

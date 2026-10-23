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
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/docdb"
	docdbtypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/aws/smithy-go"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	fake "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/docdb/fake"
)

func param(name, value string) *svcapitypes.CustomParameter {
	return &svcapitypes.CustomParameter{ParameterName: ptr.To(name), ParameterValue: ptr.To(value), ApplyMethod: ptr.To("pending-reboot")}
}

// groupClient serves a docdb group whose parameters p<i> have value "old".
func groupClient(n int, tags map[string]string) *fake.MockDocDBClient {
	current := make([]docdbtypes.Parameter, 0, n)
	for i := range n {
		current = append(current, docdbtypes.Parameter{ParameterName: ptr.To(fmt.Sprintf("p%02d", i)), ParameterValue: ptr.To("old"), ApplyMethod: docdbtypes.ApplyMethodPendingReboot})
	}
	return &fake.MockDocDBClient{
		MockDescribeDBClusterParameters: func(context.Context, *docdb.DescribeDBClusterParametersInput, ...func(*docdb.Options)) (*docdb.DescribeDBClusterParametersOutput, error) {
			return &docdb.DescribeDBClusterParametersOutput{Parameters: current}, nil
		},
		MockListTagsForResource: func(context.Context, *docdb.ListTagsForResourceInput, ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
			out := &docdb.ListTagsForResourceOutput{}
			for k, v := range tags {
				out.TagList = append(out.TagList, docdbtypes.Tag{Key: ptr.To(k), Value: ptr.To(v)})
			}
			return out, nil
		},
	}
}

func groupWith(params ...*svcapitypes.CustomParameter) *svcapitypes.DBClusterParameterGroup {
	cr := &svcapitypes.DBClusterParameterGroup{}
	meta.SetExternalName(cr, "params")
	cr.Spec.ForProvider.DBParameterGroupFamily = ptr.To("docdb5.0")
	cr.Spec.ForProvider.Parameters = params
	return cr
}

// AWS accepts at most 20 parameters per ModifyDBClusterParameterGroup call.
func TestUpdateSendsChangedParametersInBatches(t *testing.T) {
	c := groupClient(30, nil)
	params := make([]*svcapitypes.CustomParameter, 0, 30)
	for i := range 30 {
		v := "old"
		if i < 25 {
			v = "new"
		}
		params = append(params, param(fmt.Sprintf("p%02d", i), v))
	}

	if _, err := wrap(newExternal(nil, c, []option{setupExternal})).Update(context.Background(), groupWith(params...)); err != nil {
		t.Fatalf("Update(...): %v", err)
	}

	sizes := make([]int, 0, len(c.Called.ModifyDBClusterParameterGroup))
	for _, call := range c.Called.ModifyDBClusterParameterGroup {
		sizes = append(sizes, len(call.I.Parameters))
		for _, p := range call.I.Parameters {
			if ptr.Deref(p.ParameterValue, "") != "new" {
				t.Errorf("unchanged parameter %s was sent", ptr.Deref(p.ParameterName, ""))
			}
		}
		if ptr.Deref(call.I.DBClusterParameterGroupName, "") != "params" {
			t.Errorf("Modify sent to group %v", call.I.DBClusterParameterGroupName)
		}
	}
	if fmt.Sprint(sizes) != "[20 5]" {
		t.Errorf("parameters per Modify call = %v, want [20 5]", sizes)
	}
}

func TestUpdateTagsOnlyDoesNotModifyParameters(t *testing.T) {
	c := groupClient(3, nil)
	cr := groupWith(param("p00", "old"))
	cr.Spec.ForProvider.Tags = []*svcapitypes.Tag{{Key: ptr.To("team"), Value: ptr.To("a")}}

	if _, err := wrap(newExternal(nil, c, []option{setupExternal})).Update(context.Background(), cr); err != nil {
		t.Fatalf("Update(...): %v", err)
	}

	if n := len(c.Called.ModifyDBClusterParameterGroup); n != 0 {
		t.Errorf("ModifyDBClusterParameterGroup calls = %d, want 0", n)
	}
	if n := len(c.Called.AddTagsToResource); n != 1 {
		t.Errorf("AddTagsToResource calls = %d, want 1", n)
	}
}

func TestUpdateWrapsModifyErrorOfLaterBatch(t *testing.T) {
	c := groupClient(25, nil)
	c.MockModifyDBClusterParameterGroup = func(_ context.Context, in *docdb.ModifyDBClusterParameterGroupInput, _ ...func(*docdb.Options)) (*docdb.ModifyDBClusterParameterGroupOutput, error) {
		if len(in.Parameters) < 20 {
			return nil, &smithy.GenericAPIError{Code: "InvalidParameterValue", Message: "bad"}
		}
		return &docdb.ModifyDBClusterParameterGroupOutput{}, nil
	}
	params := make([]*svcapitypes.CustomParameter, 0, 25)
	for i := range 25 {
		params = append(params, param(fmt.Sprintf("p%02d", i), "new"))
	}

	_, err := wrap(newExternal(nil, c, []option{setupExternal})).Update(context.Background(), groupWith(params...))

	if want := "cannot update DBClusterParameterGroup in AWS: InvalidParameterValue: bad"; err == nil || err.Error() != want {
		t.Errorf("Update(...) error = %v, want %q", err, want)
	}
	if len(c.Called.ListTagsForResource) != 0 {
		t.Error("Update(...) went on to the tags after a failed Modify")
	}
}

// The CRD requires applyMethod and parameterValue; a nil one would make the
// late-init update fail and the MR stuck.
func TestLateInitParametersFitTheCRD(t *testing.T) {
	c := describeGroups(docdbtypes.DBClusterParameterGroup{DBClusterParameterGroupName: ptr.To("params"), DBParameterGroupFamily: ptr.To("docdb5.0")})
	c.MockDescribeDBClusterParameters = func(context.Context, *docdb.DescribeDBClusterParametersInput, ...func(*docdb.Options)) (*docdb.DescribeDBClusterParametersOutput, error) {
		return &docdb.DescribeDBClusterParametersOutput{Parameters: []docdbtypes.Parameter{
			{ParameterName: ptr.To("tls"), ParameterValue: ptr.To("enabled")},
			{ParameterName: ptr.To("audit_logs"), ApplyMethod: docdbtypes.ApplyMethodImmediate},
		}}, nil
	}
	cr := groupWith()

	if _, err := newExternal(nil, c, []option{setupExternal}).Observe(context.Background(), cr); err != nil {
		t.Fatalf("Observe(...): %v", err)
	}

	want := []*svcapitypes.CustomParameter{{ParameterName: ptr.To("tls"), ParameterValue: ptr.To("enabled"), ApplyMethod: ptr.To("pending-reboot")}}
	if diff := cmp.Diff(want, cr.Spec.ForProvider.Parameters); diff != "" {
		t.Errorf("late-initialized parameters: -want, +got:\n%s", diff)
	}
}

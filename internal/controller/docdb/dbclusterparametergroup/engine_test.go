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
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/docdb"
	docdbtypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/aws/smithy-go"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	fake "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/docdb/fake"
)

// describeGroups returns a fake whose DescribeDBClusterParameterGroups answers with g.
func describeGroups(g ...docdbtypes.DBClusterParameterGroup) *fake.MockDocDBClient {
	return &fake.MockDocDBClient{
		MockDescribeDBClusterParameterGroups: func(context.Context, *docdb.DescribeDBClusterParameterGroupsInput, ...func(*docdb.Options)) (*docdb.DescribeDBClusterParameterGroupsOutput, error) {
			return &docdb.DescribeDBClusterParameterGroupsOutput{DBClusterParameterGroups: g}, nil
		},
	}
}

func TestObserveRefusesForeignFamily(t *testing.T) {
	cases := map[string]*string{
		"Aurora":  ptr.To("aurora-mysql8.0"),
		"Neptune": ptr.To("neptune1.3"),
		"Unset":   nil,
	}
	for name, family := range cases {
		t.Run(name, func(t *testing.T) {
			cr := instance(withExternalName("params"), withDBParameterGroupFamily("docdb5.0"))
			want := cr.DeepCopy()
			c := describeGroups(docdbtypes.DBClusterParameterGroup{
				DBClusterParameterGroupName: ptr.To("params"),
				DBParameterGroupFamily:      family,
			})
			e := newExternal(nil, c, []option{setupExternal})

			obs, err := e.Observe(context.Background(), cr)

			if err == nil {
				t.Fatalf("Observe(...): want error, got %+v", obs)
			}
			wantErr := `late-init failed: DBClusterParameterGroup "params" has family "` + ptr.Deref(family, "") + `", not a DocumentDB (docdb*) family; refusing to manage it`
			if diff := cmp.Diff(wantErr, err.Error()); diff != "" {
				t.Errorf("Observe(...) error: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(want, cr, test.EquateConditions()); diff != "" {
				t.Errorf("Observe(...) changed the CR: -want, +got:\n%s", diff)
			}
			if n := len(c.Called.DescribeDBClusterParameters); n != 0 {
				t.Errorf("DescribeDBClusterParameters calls = %d, want 0", n)
			}
		})
	}
}

func TestObserveDeletedForeignFamilyDoesNotExist(t *testing.T) {
	cr := instance(withExternalName("params"))
	now := metav1.Now()
	cr.SetDeletionTimestamp(&now)
	c := describeGroups(docdbtypes.DBClusterParameterGroup{DBClusterParameterGroupName: ptr.To("params"), DBParameterGroupFamily: ptr.To("aurora-mysql8.0")})
	e := newExternal(nil, c, []option{setupExternal})

	obs, err := e.Observe(context.Background(), cr)

	if err != nil || obs.ResourceExists {
		t.Errorf("Observe(...) = %+v, %v; want not existing, nil", obs, err)
	}
	if len(c.Called.DescribeDBClusterParameters)+len(c.Called.DeleteDBClusterParameterGroup) != 0 {
		t.Errorf("Observe(...) calls = %+v, want Describe only", c.Called)
	}
}

func TestObserveMatchesExternalNameIgnoringCase(t *testing.T) {
	c := describeGroups(docdbtypes.DBClusterParameterGroup{DBClusterParameterGroupName: ptr.To("params"), DBParameterGroupFamily: ptr.To("docdb5.0")})
	e := newExternal(nil, c, []option{setupExternal})

	obs, err := e.Observe(context.Background(), instance(withExternalName("Params"), withDBParameterGroupFamily("docdb5.0")))

	if err != nil || !obs.ResourceExists {
		t.Errorf("Observe(...) = %+v, %v; want existing, nil", obs, err)
	}
}

func TestTypedNotFound(t *testing.T) {
	notFound := &smithy.GenericAPIError{Code: "DBParameterGroupNotFound", Message: "gone"}
	c := &fake.MockDocDBClient{
		MockDescribeDBClusterParameterGroups: func(context.Context, *docdb.DescribeDBClusterParameterGroupsInput, ...func(*docdb.Options)) (*docdb.DescribeDBClusterParameterGroupsOutput, error) {
			return nil, notFound
		},
		MockDeleteDBClusterParameterGroup: func(context.Context, *docdb.DeleteDBClusterParameterGroupInput, ...func(*docdb.Options)) (*docdb.DeleteDBClusterParameterGroupOutput, error) {
			return nil, notFound
		},
	}
	e := newExternal(nil, c, []option{setupExternal})
	cr := instance(withExternalName("params"))

	obs, err := e.Observe(context.Background(), cr)
	if err != nil || obs.ResourceExists {
		t.Errorf("Observe(...) = %+v, %v; want not existing, nil", obs, err)
	}
	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Errorf("Delete(...): %v, want nil", err)
	}
}

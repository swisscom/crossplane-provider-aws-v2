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

package dbinstance

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

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	fake "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/docdb/fake"
)

// describeInstances returns a fake whose DescribeDBInstances answers with i.
func describeInstances(i ...docdbtypes.DBInstance) *fake.MockDocDBClient {
	return &fake.MockDocDBClient{
		MockDescribeDBInstances: func(context.Context, *docdb.DescribeDBInstancesInput, ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
			return &docdb.DescribeDBInstancesOutput{DBInstances: i}, nil
		},
	}
}

func TestObserveRefusesForeignEngine(t *testing.T) {
	cases := map[string]*string{
		"Postgres": ptr.To("postgres"),
		"Aurora":   ptr.To("aurora-mysql"),
		"Unset":    nil,
	}
	for name, engine := range cases {
		t.Run(name, func(t *testing.T) {
			cr := instance(withExternalName("orders-1"), withDBInstanceClass("db.r6g.large"))
			want := cr.DeepCopy()
			c := describeInstances(docdbtypes.DBInstance{
				DBInstanceIdentifier: ptr.To("orders-1"),
				Engine:               engine,
				DBInstanceArn:        ptr.To("arn"),
				DBInstanceStatus:     ptr.To("available"),
				DBInstanceClass:      ptr.To("db.m5.large"),
				AvailabilityZone:     ptr.To("eu-central-1a"),
			})
			e := newExternal(nil, c, []option{setupExternal})

			obs, err := e.Observe(context.Background(), cr)

			if err == nil {
				t.Fatalf("Observe(...): want error, got %+v", obs)
			}
			wantErr := `late-init failed: DBInstance "orders-1" has engine "` + ptr.Deref(engine, "") + `", not "docdb"; refusing to manage it`
			if diff := cmp.Diff(wantErr, err.Error()); diff != "" {
				t.Errorf("Observe(...) error: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(want, cr, test.EquateConditions()); diff != "" {
				t.Errorf("Observe(...) changed the CR: -want, +got:\n%s", diff)
			}
			if n := len(c.Called.ModifyDBInstance) + len(c.Called.ListTagsForResource); n != 0 {
				t.Errorf("Observe(...) made %d calls after Describe, want 0", n)
			}
		})
	}
}

func TestObserveDeletedForeignEngineDoesNotExist(t *testing.T) {
	cr := instance(withExternalName("orders-1"))
	now := metav1.Now()
	cr.SetDeletionTimestamp(&now)
	c := describeInstances(docdbtypes.DBInstance{DBInstanceIdentifier: ptr.To("orders-1"), Engine: ptr.To("postgres")})
	e := newExternal(nil, c, []option{setupExternal})

	obs, err := e.Observe(context.Background(), cr)

	if err != nil || obs.ResourceExists {
		t.Errorf("Observe(...) = %+v, %v; want not existing, nil", obs, err)
	}
	if len(c.Called.DescribeDBInstances) != 1 || len(c.Called.DeleteDBInstance) != 0 {
		t.Errorf("Observe(...) calls = %+v, want one Describe only", c.Called)
	}
}

// AWS documents no engine filter for DescribeDBInstances, so none is sent.
func TestObserveSendsNoFilter(t *testing.T) {
	c := describeInstances()
	e := newExternal(nil, c, []option{setupExternal})

	if _, err := e.Observe(context.Background(), instance(withExternalName("orders-1"))); err != nil {
		t.Fatalf("Observe(...): %v", err)
	}
	want := &docdb.DescribeDBInstancesInput{DBInstanceIdentifier: ptr.To("orders-1")}
	if diff := cmp.Diff(want, c.Called.DescribeDBInstances[0].I, ignoreUnexported); diff != "" {
		t.Errorf("DescribeDBInstances input: -want, +got:\n%s", diff)
	}
}

func TestObserveMatchesExternalNameIgnoringCase(t *testing.T) {
	c := describeInstances(docdbtypes.DBInstance{
		DBInstanceIdentifier: ptr.To("orders-1"),
		Engine:               ptr.To(svcapitypes.EngineDocDB),
		DBInstanceStatus:     ptr.To("available"),
	})
	e := newExternal(nil, c, []option{setupExternal})

	obs, err := e.Observe(context.Background(), instance(withExternalName("Orders-1")))

	if err != nil || !obs.ResourceExists {
		t.Errorf("Observe(...) = %+v, %v; want existing, nil", obs, err)
	}
}

func TestTypedNotFound(t *testing.T) {
	notFound := &smithy.GenericAPIError{Code: "DBInstanceNotFound", Message: "gone"}
	c := &fake.MockDocDBClient{
		MockDescribeDBInstances: func(context.Context, *docdb.DescribeDBInstancesInput, ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
			return nil, notFound
		},
		MockDeleteDBInstance: func(context.Context, *docdb.DeleteDBInstanceInput, ...func(*docdb.Options)) (*docdb.DeleteDBInstanceOutput, error) {
			return nil, notFound
		},
	}
	e := newExternal(nil, c, []option{setupExternal})
	cr := instance(withExternalName("orders-1"))

	obs, err := e.Observe(context.Background(), cr)
	if err != nil || obs.ResourceExists {
		t.Errorf("Observe(...) = %+v, %v; want not existing, nil", obs, err)
	}
	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Errorf("Delete(...): %v, want nil", err)
	}
}

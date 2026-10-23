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

package dbcluster

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

// describeClusters returns a fake whose DescribeDBClusters answers with c.
func describeClusters(c ...docdbtypes.DBCluster) *fake.MockDocDBClient {
	return &fake.MockDocDBClient{
		MockDescribeDBClusters: func(context.Context, *docdb.DescribeDBClustersInput, ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
			return &docdb.DescribeDBClustersOutput{DBClusters: c}, nil
		},
	}
}

func TestObserveRefusesForeignEngine(t *testing.T) {
	cases := map[string]*string{
		"Aurora":  ptr.To("aurora-mysql"),
		"Neptune": ptr.To("neptune"),
		"Unset":   nil,
	}
	for name, engine := range cases {
		t.Run(name, func(t *testing.T) {
			cr := instance(withExternalName("orders"), withSkipFinalSnapshot(true), withPort(testPort))
			want := cr.DeepCopy()
			c := describeClusters(docdbtypes.DBCluster{
				DBClusterIdentifier:   ptr.To("orders"),
				Engine:                engine,
				Status:                ptr.To("available"),
				DBClusterArn:          ptr.To("arn:aws:rds:x:1:cluster:orders"),
				Endpoint:              ptr.To("orders.cluster-x.rds.amazonaws.com"),
				Port:                  ptr.To[int32](3306),
				BackupRetentionPeriod: ptr.To[int32](1),
			})
			e := newExternal(nil, c, []option{setupExternal})

			obs, err := e.Observe(context.Background(), cr)

			if err == nil {
				t.Fatalf("Observe(...): want error, got %+v", obs)
			}
			wantErr := `DBCluster "orders" has engine "` + ptr.Deref(engine, "") + `", not "docdb"; refusing to manage it`
			if diff := cmp.Diff(wantErr, errCause(err)); diff != "" {
				t.Errorf("Observe(...) error: -want, +got:\n%s", diff)
			}
			if obs.ResourceExists {
				t.Errorf("Observe(...) ResourceExists = true, want false")
			}
			if diff := cmp.Diff(want, cr, test.EquateConditions()); diff != "" {
				t.Errorf("Observe(...) changed the CR: -want, +got:\n%s", diff)
			}
			if n := len(c.Called.ModifyDBCluster) + len(c.Called.ListTagsForResource); n != 0 {
				t.Errorf("Observe(...) made %d calls after Describe, want 0", n)
			}
		})
	}
}

// A deleted MR must let go of a foreign cluster without touching it.
func TestObserveDeletedForeignEngineDoesNotExist(t *testing.T) {
	cr := instance(withExternalName("orders"), withSkipFinalSnapshot(true), withDeletionTimestamp(&metav1.Time{Time: timeNow}))
	c := describeClusters(docdbtypes.DBCluster{
		DBClusterIdentifier: ptr.To("orders"),
		Engine:              ptr.To("aurora-postgresql"),
		Status:              ptr.To("available"),
	})
	e := newExternal(nil, c, []option{setupExternal})

	obs, err := e.Observe(context.Background(), cr)

	if err != nil {
		t.Fatalf("Observe(...): %v", err)
	}
	if obs.ResourceExists {
		t.Errorf("Observe(...) ResourceExists = true, want false")
	}
	if cr.Status.AtProvider.Status != nil {
		t.Errorf("Observe(...) copied the foreign status %q", *cr.Status.AtProvider.Status)
	}
	if len(c.Called.DescribeDBClusters) != 1 || len(c.Called.DeleteDBCluster) != 0 || len(c.Called.ModifyDBCluster) != 0 {
		t.Errorf("Observe(...) calls = %+v, want one Describe only", c.Called)
	}
}

// AWS documents the engine filter for exactly this purpose, so a foreign
// cluster should not even be returned.
func TestObserveSendsEngineFilter(t *testing.T) {
	c := describeClusters()
	e := newExternal(nil, c, []option{setupExternal})

	if _, err := e.Observe(context.Background(), instance(withExternalName("orders"))); err != nil {
		t.Fatalf("Observe(...): %v", err)
	}

	want := &docdb.DescribeDBClustersInput{
		DBClusterIdentifier: ptr.To("orders"),
		Filters:             []docdbtypes.Filter{{Name: ptr.To("engine"), Values: []string{"docdb"}}},
	}
	if len(c.Called.DescribeDBClusters) != 1 {
		t.Fatalf("DescribeDBClusters calls = %d, want 1", len(c.Called.DescribeDBClusters))
	}
	if diff := cmp.Diff(want, c.Called.DescribeDBClusters[0].I, ignoreUnexported); diff != "" {
		t.Errorf("DescribeDBClusters input: -want, +got:\n%s", diff)
	}
}

// AWS stores identifiers in lowercase.
func TestObserveMatchesExternalNameIgnoringCase(t *testing.T) {
	c := describeClusters(docdbtypes.DBCluster{
		DBClusterIdentifier: ptr.To("mydb"),
		Engine:              ptr.To(svcapitypes.EngineDocDB),
		Status:              ptr.To("available"),
	})
	e := newExternal(nil, c, []option{setupExternal})

	obs, err := e.Observe(context.Background(), instance(withExternalName("MyDb")))

	if err != nil {
		t.Fatalf("Observe(...): %v", err)
	}
	if !obs.ResourceExists {
		t.Errorf("Observe(...) ResourceExists = false, want true")
	}
}

func TestTypedNotFound(t *testing.T) {
	notFound := &smithy.GenericAPIError{Code: "DBClusterNotFoundFault", Message: "gone"}
	c := &fake.MockDocDBClient{
		MockDescribeDBClusters: func(context.Context, *docdb.DescribeDBClustersInput, ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
			return nil, notFound
		},
		MockDeleteDBCluster: func(context.Context, *docdb.DeleteDBClusterInput, ...func(*docdb.Options)) (*docdb.DeleteDBClusterOutput, error) {
			return nil, notFound
		},
	}
	e := newExternal(nil, c, []option{setupExternal})
	cr := instance(withExternalName("orders"))

	obs, err := e.Observe(context.Background(), cr)
	if err != nil || obs.ResourceExists {
		t.Errorf("Observe(...) = %+v, %v; want not existing, nil", obs, err)
	}
	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Errorf("Delete(...): %v, want nil", err)
	}
}

// errCause strips the generated "late-init failed: " prefix.
func errCause(err error) string {
	const prefix = "late-init failed: "
	s := err.Error()
	if len(s) > len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

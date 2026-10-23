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

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds/fake"
)

// foreignClient serves an instance with the given engine and fails the test on
// any mutating call.
func foreignClient(t *testing.T, engine string, cluster *string) *fake.MockRDSClient {
	t.Helper()
	return &fake.MockRDSClient{
		MockDescribeDBInstances: describeFn(describeOut("available", func(db *svcsdktypes.DBInstance) {
			db.Engine = aws.String(engine)
			db.DBClusterIdentifier = cluster
		}), nil),
		MockModifyDBInstance: func(context.Context, *svcsdk.ModifyDBInstanceInput, ...func(*svcsdk.Options)) (*svcsdk.ModifyDBInstanceOutput, error) {
			t.Error("ModifyDBInstance must not be called")
			return &svcsdk.ModifyDBInstanceOutput{}, nil
		},
		MockDeleteDBInstance: func(context.Context, *svcsdk.DeleteDBInstanceInput, ...func(*svcsdk.Options)) (*svcsdk.DeleteDBInstanceOutput, error) {
			t.Error("DeleteDBInstance must not be called")
			return &svcsdk.DeleteDBInstanceOutput{}, nil
		},
		MockDescribeDBClusters: func(context.Context, *svcsdk.DescribeDBClustersInput, ...func(*svcsdk.Options)) (*svcsdk.DescribeDBClustersOutput, error) {
			return &svcsdk.DescribeDBClustersOutput{DBClusters: []svcsdktypes.DBCluster{{DBClusterMembers: []svcsdktypes.DBClusterMember{
				{DBInstanceIdentifier: aws.String(testName), IsClusterWriter: aws.Bool(true)},
			}}}}, nil
		},
	}
}

func TestObserveRefusesForeignEngine(t *testing.T) {
	for _, engine := range []string{"docdb", "neptune", "DocDB", "NEPTUNE"} {
		t.Run(engine, func(t *testing.T) {
			cr := newTestCR()
			before := cr.DeepCopy()
			_, err := newCustomExternal(newTestKube(t), foreignClient(t, engine, nil)).Observe(context.Background(), cr)
			want := `late-init failed: DBInstance "db1" has engine "` + engine + `", which is not managed by rds/DBInstance; refusing to manage it`
			if got := errString(err); got != want {
				t.Fatalf("Observe error = %q, want %q", got, want)
			}
			if diff := cmp.Diff(before, cr); diff != "" {
				t.Errorf("Observe changed the CR: -before, +after:\n%s", diff)
			}
		})
	}
}

func TestObserveDeletedForeignEngineIsGone(t *testing.T) {
	for _, engine := range []string{"docdb", "neptune"} {
		t.Run(engine, func(t *testing.T) {
			cr := newTestCR()
			now := metav1.Now()
			cr.SetDeletionTimestamp(&now)
			e := newCustomExternal(newTestKube(t), foreignClient(t, engine, nil))
			obs, err := e.Observe(context.Background(), cr)
			if err != nil {
				t.Fatal(err)
			}
			if obs.ResourceExists {
				t.Error("a deleted MR whose name matches a foreign instance must see it as gone")
			}
		})
	}
}

func TestObserveRDSEnginesUnaffected(t *testing.T) {
	cases := map[string]struct {
		engine  string
		cluster *string
	}{
		"Postgres":            {engine: "postgres"},
		"AuroraClusterMember": {engine: "aurora-postgresql", cluster: aws.String("c1")},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := newTestCR(func(cr *svcapitypes.DBInstance) { cr.Spec.ForProvider.Engine = aws.String(tc.engine) })
			obs, err := newCustomExternal(newTestKube(t), foreignClient(t, tc.engine, tc.cluster)).Observe(context.Background(), cr)
			if err != nil {
				t.Fatal(err)
			}
			if !obs.ResourceExists {
				t.Error("want the instance to exist")
			}
		})
	}
}

func TestObserveMixedCaseExternalName(t *testing.T) {
	cr := newTestCR()
	meta.SetExternalName(cr, "MyDB")
	rdsc := &fake.MockRDSClient{MockDescribeDBInstances: describeFn(describeOut("available", func(db *svcsdktypes.DBInstance) {
		db.DBInstanceIdentifier = aws.String("mydb") // AWS stores identifiers in lowercase
	}), nil)}
	obs, err := newCustomExternal(newTestKube(t), rdsc).Observe(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}
	if !obs.ResourceExists {
		t.Error("want MyDB to match the instance mydb")
	}
}

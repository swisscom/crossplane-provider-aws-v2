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
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	rds "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds/fake"
)

func replicaOf(db *svcsdktypes.DBInstance) {
	db.ReadReplicaSourceDBInstanceIdentifier = aws.String("primary")
}

func TestObserveReadReplicaPassword(t *testing.T) {
	cases := map[string]struct {
		opts         []func(*svcapitypes.DBInstance)
		objs         []client.Object
		cache        map[string]string
		wantUpToDate bool
	}{
		"NoPasswordSource": {wantUpToDate: true},
		"RotatedRef": {
			opts:  []func(*svcapitypes.DBInstance){withPasswordRef},
			objs:  []client.Object{pwSecret("new")},
			cache: map[string]string{rds.PasswordCacheKey: oldValue},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := newTestCR(append(tc.opts, func(cr *svcapitypes.DBInstance) {
				cr.Spec.ForProvider.SourceDBInstanceID = aws.String("primary")
				cr.Status.AtProvider.DatabaseRole = aws.String(databaseRoleReadReplica) // from the previous poll
			})...)
			kube := newTestKube(t, tc.objs...)
			if tc.cache != nil {
				seedCache(t, kube, cr, tc.cache)
			}
			obs, err := newCustomExternal(kube, &fake.MockRDSClient{MockDescribeDBInstances: describeFn(describeOut("available", replicaOf), nil)}).Observe(context.Background(), cr)
			if err != nil {
				t.Fatal(err)
			}
			if obs.ResourceUpToDate != tc.wantUpToDate {
				t.Errorf("upToDate = %v, want %v; diff: %s", obs.ResourceUpToDate, tc.wantUpToDate, obs.Diff)
			}
			if got := aws.ToString(cr.Status.AtProvider.DatabaseRole); got != databaseRoleReadReplica {
				t.Errorf("databaseRole = %q, want %q", got, databaseRoleReadReplica)
			}
		})
	}
}

var errCacheRead = errors.New("cache read failed")

// cacheGetFails fails every read of the password cache Secret.
type cacheGetFails struct{ client.Client }

func (k cacheGetFails) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if strings.HasPrefix(key.Name, "dbinstance.") {
		return errCacheRead
	}
	return k.Client.Get(ctx, key, obj, opts...)
}

// A cache that can't be read is an error, not a replica without a password.
func TestObserveReadReplicaCacheError(t *testing.T) {
	cr := newTestCR(func(cr *svcapitypes.DBInstance) {
		cr.Spec.ForProvider.SourceDBInstanceID = aws.String("primary")
		cr.Status.AtProvider.DatabaseRole = aws.String(databaseRoleReadReplica)
	})
	kube := cacheGetFails{newTestKube(t)}
	_, err := newCustomExternal(kube, &fake.MockRDSClient{MockDescribeDBInstances: describeFn(describeOut("available", replicaOf), nil)}).Observe(context.Background(), cr)
	if !errors.Is(err, errCacheRead) || !strings.Contains(err.Error(), "isUpToDate") {
		t.Fatalf("Observe() error = %v, want the cache error from the up-to-date check", err)
	}
}

func TestObserveClusterMemberRole(t *testing.T) {
	cases := map[string]struct {
		members  []svcsdktypes.DBClusterMember
		wantRole *string
	}{
		"Writer":    {members: []svcsdktypes.DBClusterMember{{DBInstanceIdentifier: aws.String(testName), IsClusterWriter: aws.Bool(true)}}, wantRole: aws.String(databaseRoleClusterWriter)},
		"Reader":    {members: []svcsdktypes.DBClusterMember{{DBInstanceIdentifier: aws.String(testName), IsClusterWriter: aws.Bool(false)}}, wantRole: aws.String(databaseRoleClusterReader)},
		"NotMember": {members: []svcsdktypes.DBClusterMember{{DBInstanceIdentifier: aws.String("other"), IsClusterWriter: aws.Bool(true)}}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := newTestCR()
			cr.Status.AtProvider.DatabaseRole = aws.String(databaseRoleClusterWriter) // from the previous poll
			rdsc := &fake.MockRDSClient{
				MockDescribeDBInstances: describeFn(describeOut("available", func(db *svcsdktypes.DBInstance) { db.DBClusterIdentifier = aws.String("c1") }), nil),
				MockDescribeDBClusters: func(context.Context, *svcsdk.DescribeDBClustersInput, ...func(*svcsdk.Options)) (*svcsdk.DescribeDBClustersOutput, error) {
					return &svcsdk.DescribeDBClustersOutput{DBClusters: []svcsdktypes.DBCluster{{DBClusterMembers: tc.members}}}, nil
				},
			}
			if _, err := newCustomExternal(newTestKube(t), rdsc).Observe(context.Background(), cr); err != nil {
				t.Fatal(err)
			}
			if got := cr.Status.AtProvider.DatabaseRole; aws.ToString(got) != aws.ToString(tc.wantRole) || (got == nil) != (tc.wantRole == nil) {
				t.Errorf("databaseRole = %v, want %v", aws.ToString(got), aws.ToString(tc.wantRole))
			}
		})
	}
}

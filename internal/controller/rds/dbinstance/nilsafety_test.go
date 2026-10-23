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
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds/fake"
)

func TestObserveNilMaintenanceWindow(t *testing.T) {
	cr := newTestCR(func(cr *svcapitypes.DBInstance) {
		cr.Spec.ForProvider.PreferredMaintenanceWindow = aws.String("sun:03:00-sun:04:00")
	})
	rdsc := &fake.MockRDSClient{MockDescribeDBInstances: describeFn(describeOut("available"), nil)}

	obs, err := newCustomExternal(newTestKube(t), rdsc).Observe(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}
	if obs.ResourceUpToDate {
		t.Error("want not up to date: AWS has no maintenance window")
	}
	if strings.Contains(obs.Diff, "<nil>") || !strings.Contains(obs.Diff, "desired maintenanceWindow: sun:03:00-sun:04:00") {
		t.Errorf("diff = %q", obs.Diff)
	}
}

func TestObserveDoesNotSortSpecSecurityGroups(t *testing.T) {
	cr := newTestCR(func(cr *svcapitypes.DBInstance) {
		cr.Spec.ForProvider.VPCSecurityGroupIDs = []string{"sg-b", sgA}
	})
	rdsc := &fake.MockRDSClient{MockDescribeDBInstances: describeFn(describeOut("available", func(db *svcsdktypes.DBInstance) {
		db.VpcSecurityGroups = []svcsdktypes.VpcSecurityGroupMembership{{VpcSecurityGroupId: aws.String(sgA)}, {VpcSecurityGroupId: aws.String("sg-b")}}
	}), nil)}
	e := newCustomExternal(newTestKube(t), rdsc)

	if _, err := e.Observe(context.Background(), cr); err != nil { // late-inits other fields
		t.Fatal(err)
	}
	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}
	if got := cr.Spec.ForProvider.VPCSecurityGroupIDs; got[0] != "sg-b" || got[1] != sgA {
		t.Errorf("spec vpcSecurityGroupIDs = %v, want the user's order [sg-b sg-a]", got)
	}
	if obs.ResourceLateInitialized {
		t.Error("an order-only difference must not count as late initialization")
	}
	if !obs.ResourceUpToDate {
		t.Errorf("same groups in another order must be up to date, diff: %s", obs.Diff)
	}
}

func TestObserveNilSites(t *testing.T) {
	cases := map[string]struct {
		spec     func(*svcapitypes.DBInstance)
		db       func(*svcsdktypes.DBInstance)
		wantDiff string
	}{
		"NoBackupWindow": {
			spec: func(cr *svcapitypes.DBInstance) {
				cr.Spec.ForProvider.PreferredBackupWindow = aws.String("03:00-04:00")
			},
			wantDiff: "desired backupWindow: 03:00-04:00",
		},
		"SecurityGroupWithoutID": {
			spec:     func(cr *svcapitypes.DBInstance) { cr.Spec.ForProvider.VPCSecurityGroupIDs = []string{sgA} },
			db:       func(db *svcsdktypes.DBInstance) { db.VpcSecurityGroups = []svcsdktypes.VpcSecurityGroupMembership{{}} },
			wantDiff: "desired vpcSecurityGroupIDs: [sg-a]",
		},
		"NoParameterGroup": {
			spec:     func(cr *svcapitypes.DBInstance) { cr.Spec.ForProvider.DBParameterGroupName = aws.String("pg") },
			wantDiff: "desired dbParameterGroupName: pg",
		},
		"NoOptionGroup": {
			spec:     func(cr *svcapitypes.DBInstance) { cr.Spec.ForProvider.OptionGroupName = aws.String("og") },
			wantDiff: "desired optionGroupName: og",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := newTestCR(tc.spec)
			out := describeOut("available")
			if tc.db != nil {
				tc.db(&out.DBInstances[0])
			}
			rdsc := &fake.MockRDSClient{MockDescribeDBInstances: describeFn(out, nil)}
			obs, err := newCustomExternal(newTestKube(t), rdsc).Observe(context.Background(), cr)
			if err != nil {
				t.Fatal(err)
			}
			if obs.ResourceUpToDate || !strings.Contains(obs.Diff, tc.wantDiff) || strings.Contains(obs.Diff, "<nil>") {
				t.Errorf("upToDate = %v, diff = %q; want a diff with %q and no <nil>", obs.ResourceUpToDate, obs.Diff, tc.wantDiff)
			}
		})
	}
}

func TestObserveClusterMemberWithoutCluster(t *testing.T) {
	cr := newTestCR()
	cr.Status.AtProvider.DatabaseRole = aws.String(databaseRoleClusterWriter) // from the previous poll
	rdsc := &fake.MockRDSClient{
		MockDescribeDBInstances: describeFn(describeOut("available", func(db *svcsdktypes.DBInstance) { db.DBClusterIdentifier = aws.String("c1") }), nil),
		MockDescribeDBClusters: func(context.Context, *svcsdk.DescribeDBClustersInput, ...func(*svcsdk.Options)) (*svcsdk.DescribeDBClustersOutput, error) {
			return &svcsdk.DescribeDBClustersOutput{}, nil
		},
	}
	if _, err := newCustomExternal(newTestKube(t), rdsc).Observe(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	if cr.Status.AtProvider.DatabaseRole != nil {
		t.Errorf("databaseRole = %q, want unset when the cluster is not found", *cr.Status.AtProvider.DatabaseRole)
	}
}

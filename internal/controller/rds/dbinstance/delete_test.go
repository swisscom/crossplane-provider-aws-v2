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
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/smithy-go"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	rds "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds/fake"
)

// deleteCalls records the mutating calls of a Delete round.
type deleteCalls struct {
	modify []*svcsdk.ModifyDBInstanceInput
	delete []*svcsdk.DeleteDBInstanceInput
}

func (c *deleteCalls) client(out *svcsdk.DescribeDBInstancesOutput, modifyErr, deleteErr error) *fake.MockRDSClient {
	return &fake.MockRDSClient{
		MockDescribeDBInstances: describeFn(out, nil),
		MockModifyDBInstance: func(_ context.Context, in *svcsdk.ModifyDBInstanceInput, _ ...func(*svcsdk.Options)) (*svcsdk.ModifyDBInstanceOutput, error) {
			c.modify = append(c.modify, in)
			return &svcsdk.ModifyDBInstanceOutput{}, modifyErr
		},
		MockDeleteDBInstance: func(_ context.Context, in *svcsdk.DeleteDBInstanceInput, _ ...func(*svcsdk.Options)) (*svcsdk.DeleteDBInstanceOutput, error) {
			c.delete = append(c.delete, in)
			return &svcsdk.DeleteDBInstanceOutput{}, deleteErr
		},
		MockDescribeDBClusters: func(context.Context, *svcsdk.DescribeDBClustersInput, ...func(*svcsdk.Options)) (*svcsdk.DescribeDBClustersOutput, error) {
			return &svcsdk.DescribeDBClustersOutput{}, nil
		},
	}
}

func deletedCR(opts ...func(*svcapitypes.DBInstance)) *svcapitypes.DBInstance {
	cr := newTestCR(opts...)
	now := metav1.Now()
	cr.SetDeletionTimestamp(&now)
	return cr
}

func TestDeleteLiftsDeletionProtectionOnly(t *testing.T) {
	cr := deletedCR(func(cr *svcapitypes.DBInstance) {
		cr.Spec.ForProvider.EngineVersion = aws.String("16.3") // AWS auto-upgraded to 16.4
		cr.Spec.ForProvider.DeletionProtection = aws.Bool(false)
		cr.Spec.ForProvider.SkipFinalSnapshot = true
	})
	calls := &deleteCalls{}
	e := newCustomExternal(newTestKube(t), calls.client(describeOut("available", func(db *svcsdktypes.DBInstance) {
		db.DeletionProtection = aws.Bool(true)
	}), nil, nil))

	if _, err := e.Observe(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	want := []*svcsdk.ModifyDBInstanceInput{{
		DBInstanceIdentifier: aws.String(testName),
		DeletionProtection:   aws.Bool(false),
		ApplyImmediately:     aws.Bool(true),
	}}
	if diff := cmp.Diff(want, calls.modify, cmpopts.IgnoreUnexported(svcsdk.ModifyDBInstanceInput{})); diff != "" {
		t.Errorf("ModifyDBInstance calls: -want, +got:\n%s", diff)
	}
	if len(calls.delete) != 0 {
		t.Errorf("DeleteDBInstance must wait for the next round, got %d calls", len(calls.delete))
	}
}

func TestDeleteDeletionProtectionCases(t *testing.T) {
	notFound := &smithy.GenericAPIError{Code: "DBInstanceNotFound"}
	cases := map[string]struct {
		spec        func(*svcapitypes.DBInstance)
		observed    *bool
		cluster     *string
		skipObserve bool
		modifyErr   error
		wantModify  int
		wantDelete  int
		wantErr     string
	}{
		"SpecUnset":      {observed: aws.Bool(true), wantDelete: 1},
		"SpecTrue":       {spec: func(cr *svcapitypes.DBInstance) { cr.Spec.ForProvider.DeletionProtection = aws.Bool(true) }, observed: aws.Bool(true), wantDelete: 1},
		"ObservedOff":    {spec: protectionOff, observed: aws.Bool(false), wantDelete: 1},
		"ClusterMember":  {spec: protectionOff, observed: aws.Bool(true), cluster: aws.String("c1"), wantDelete: 1},
		"NoObserveLifts": {spec: protectionOff, skipObserve: true, wantModify: 1},
		"ModifyNotFound": {spec: protectionOff, observed: aws.Bool(true), modifyErr: notFound, wantModify: 1, wantDelete: 1},
		"ModifyFails": {
			spec: protectionOff, observed: aws.Bool(true), modifyErr: errors.New("boom"), wantModify: 1,
			wantErr: "pre-delete failed: " + errLiftDeletionProtection + ": boom",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := deletedCR(func(cr *svcapitypes.DBInstance) {
				if tc.spec != nil {
					tc.spec(cr)
				}
			})
			calls := &deleteCalls{}
			e := newCustomExternal(newTestKube(t), calls.client(describeOut("available", func(db *svcsdktypes.DBInstance) {
				db.DeletionProtection = tc.observed
				db.DBClusterIdentifier = tc.cluster
			}), tc.modifyErr, nil))
			if !tc.skipObserve {
				if _, err := e.Observe(context.Background(), cr); err != nil {
					t.Fatal(err)
				}
			}
			_, err := e.Delete(context.Background(), cr)
			if got := errString(err); got != tc.wantErr {
				t.Errorf("Delete error = %q, want %q", got, tc.wantErr)
			}
			if len(calls.modify) != tc.wantModify || len(calls.delete) != tc.wantDelete {
				t.Errorf("calls: modify %d, delete %d; want %d, %d", len(calls.modify), len(calls.delete), tc.wantModify, tc.wantDelete)
			}
		})
	}
}

func protectionOff(cr *svcapitypes.DBInstance) {
	cr.Spec.ForProvider.DeletionProtection = aws.Bool(false)
}

func TestDeleteRound(t *testing.T) {
	notFound := &smithy.GenericAPIError{Code: "DBInstanceNotFound"}
	cases := map[string]struct {
		status     *string
		deleteErr  error
		wantDelete bool
		wantErr    string
		wantCache  bool
	}{
		"AlreadyDeleting": {status: aws.String("deleting"), wantCache: true},
		"NilStatus":       {wantDelete: true},
		"Deletes":         {status: aws.String("available"), wantDelete: true},
		"DeleteNotFound":  {status: aws.String("available"), deleteErr: notFound, wantDelete: true},
		"DeleteFails":     {status: aws.String("available"), deleteErr: errors.New("boom"), wantDelete: true, wantErr: errDelete + ": boom", wantCache: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := deletedCR(func(cr *svcapitypes.DBInstance) {
				cr.Spec.ForProvider.FinalDBSnapshotIdentifier = "final"
				cr.Spec.ForProvider.DeleteAutomatedBackups = aws.Bool(false)
				cr.Status.AtProvider.DBInstanceStatus = tc.status
			})
			kube := newTestKube(t)
			seedCache(t, kube, cr, map[string]string{rds.PasswordCacheKey: "pw"})
			calls := &deleteCalls{}
			_, err := newCustomExternal(kube, calls.client(nil, nil, tc.deleteErr)).Delete(context.Background(), cr)
			if got := errString(err); got != tc.wantErr {
				t.Fatalf("Delete error = %q, want %q", got, tc.wantErr)
			}
			if !tc.wantDelete {
				if len(calls.delete) != 0 {
					t.Fatalf("DeleteDBInstance called %d times, want none", len(calls.delete))
				}
			} else {
				want := []*svcsdk.DeleteDBInstanceInput{{
					DBInstanceIdentifier:      aws.String(testName),
					FinalDBSnapshotIdentifier: aws.String("final"),
					DeleteAutomatedBackups:    aws.Bool(false),
				}}
				if diff := cmp.Diff(want, calls.delete, cmpopts.IgnoreUnexported(svcsdk.DeleteDBInstanceInput{})); diff != "" {
					t.Errorf("DeleteDBInstance: -want, +got:\n%s", diff)
				}
			}
			if got := cacheData(t, kube, cr) != nil; got != tc.wantCache {
				t.Errorf("cache Secret exists = %v, want %v", got, tc.wantCache)
			}
		})
	}
}

func TestDeleteSkipFinalSnapshot(t *testing.T) {
	cr := deletedCR(func(cr *svcapitypes.DBInstance) { cr.Spec.ForProvider.SkipFinalSnapshot = true })
	calls := &deleteCalls{}
	if _, err := newCustomExternal(newTestKube(t), calls.client(nil, nil, nil)).Delete(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	if len(calls.delete) != 1 || !aws.ToBool(calls.delete[0].SkipFinalSnapshot) || calls.delete[0].FinalDBSnapshotIdentifier != nil {
		t.Errorf("DeleteDBInstance input = %+v, want SkipFinalSnapshot and no snapshot id", calls.delete)
	}
}

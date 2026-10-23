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

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	rds "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds/fake"
)

func fromSnapshot(cr *svcapitypes.DBInstance) {
	cr.Spec.ForProvider.RestoreFrom = &svcapitypes.RestoreDBInstanceBackupConfiguration{
		Source:   aws.String("Snapshot"),
		Snapshot: &svcapitypes.SnapshotRestoreBackupConfiguration{SnapshotIdentifier: aws.String("snap")},
	}
}

// restoreAWS is a fake AWS for a restored instance whose status the test sets.
type restoreAWS struct {
	status   string
	creates  int
	restores int
	modifies []*svcsdk.ModifyDBInstanceInput
}

func (a *restoreAWS) client() *fake.MockRDSClient {
	return &fake.MockRDSClient{
		MockCreateDBInstance: func(context.Context, *svcsdk.CreateDBInstanceInput, ...func(*svcsdk.Options)) (*svcsdk.CreateDBInstanceOutput, error) {
			a.creates++
			return &svcsdk.CreateDBInstanceOutput{}, nil
		},
		MockRestoreDBInstanceFromDBSnapshot: func(context.Context, *svcsdk.RestoreDBInstanceFromDBSnapshotInput, ...func(*svcsdk.Options)) (*svcsdk.RestoreDBInstanceFromDBSnapshotOutput, error) {
			a.restores++
			a.status = statusCreating
			return &svcsdk.RestoreDBInstanceFromDBSnapshotOutput{}, nil
		},
		MockDescribeDBInstances: func(context.Context, *svcsdk.DescribeDBInstancesInput, ...func(*svcsdk.Options)) (*svcsdk.DescribeDBInstancesOutput, error) {
			return describeOut(a.status), nil
		},
		MockModifyDBInstance: func(_ context.Context, in *svcsdk.ModifyDBInstanceInput, _ ...func(*svcsdk.Options)) (*svcsdk.ModifyDBInstanceOutput, error) {
			a.modifies = append(a.modifies, in)
			return &svcsdk.ModifyDBInstanceOutput{DBInstance: &svcsdktypes.DBInstance{DBInstanceArn: aws.String(testARN)}}, nil
		},
	}
}

func TestRestoreAutogeneratePasswordFlow(t *testing.T) {
	ctx := context.Background()
	kube := newTestKube(t)
	cr := newTestCR(withAutogenerate, fromSnapshot)
	a := &restoreAWS{}

	// Reconcile 1: create restores and never calls CreateDBInstance.
	if _, err := newCustomExternal(kube, a.client()).Create(ctx, cr); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.restores != 1 || a.creates != 0 {
		t.Fatalf("restores %d, creates %d; want 1, 0", a.restores, a.creates)
	}
	generated := cacheData(t, kube, cr)[rds.PasswordCacheKey]
	if generated == "" {
		t.Fatal("no generated password cached")
	}

	// Reconcile 2: while creating, the snapshot's password is still active.
	obs, err := newCustomExternal(kube, a.client()).Observe(ctx, cr)
	if err != nil {
		t.Fatal(err)
	}
	if pw, ok := obs.ConnectionDetails[passwordKey]; ok {
		t.Errorf("published %q while the restore is pending, want no password", pw)
	}

	// Reconcile 3: available, the generated password is applied.
	a.status = "available"
	e := newCustomExternal(kube, a.client())
	if obs, err = e.Observe(ctx, cr); err != nil {
		t.Fatal(err)
	}
	if obs.ResourceUpToDate {
		t.Fatal("want not up to date: the generated password is not applied yet")
	}
	upd, err := e.Update(ctx, cr)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.modifies) != 1 || aws.ToString(a.modifies[0].MasterUserPassword) != generated {
		t.Fatalf("ModifyDBInstance did not send the generated password")
	}
	if got := string(upd.ConnectionDetails[passwordKey]); got != generated {
		t.Errorf("Update published %q, want the generated password", got)
	}
	if c := cacheData(t, kube, cr); c[rds.PasswordCacheKey] != generated || c[rds.RestoreFlagCacheKay] != "" {
		t.Errorf("cache = %v, want the generated password and no restore flag", c)
	}

	// Reconcile 4: up to date, and the password stays published.
	if obs, err = newCustomExternal(kube, a.client()).Observe(ctx, cr); err != nil {
		t.Fatal(err)
	}
	if !obs.ResourceUpToDate || string(obs.ConnectionDetails[passwordKey]) != generated {
		t.Errorf("upToDate %v, password %q; want true and the generated password", obs.ResourceUpToDate, obs.ConnectionDetails[passwordKey])
	}
}

// restoreRecorder fakes the three restore calls and records what they got.
type restoreRecorder struct {
	t         *testing.T
	kube      *testKube
	err       error
	nilOutput bool
	calls     []string
	passwords []*string
	cacheNow  []map[string]string
	creates   int
	replicas  int
}

func (r *restoreRecorder) seen(ctx context.Context, call string, pw *string) {
	r.calls = append(r.calls, call)
	r.passwords = append(r.passwords, pw)
	r.cacheNow = append(r.cacheNow, cacheDataCtx(ctx, r.t, r.kube, newTestCR()))
}

func (r *restoreRecorder) db() *svcsdktypes.DBInstance {
	if r.nilOutput {
		return nil
	}
	return &svcsdktypes.DBInstance{DBInstanceStatus: aws.String("creating"), DBInstanceArn: aws.String(testARN)}
}

func (r *restoreRecorder) client() *fake.MockRDSClient {
	return &fake.MockRDSClient{
		MockRestoreDBInstanceFromS3: func(ctx context.Context, in *svcsdk.RestoreDBInstanceFromS3Input, _ ...func(*svcsdk.Options)) (*svcsdk.RestoreDBInstanceFromS3Output, error) {
			r.seen(ctx, "S3:"+aws.ToString(in.DBInstanceIdentifier), in.MasterUserPassword)
			if r.nilOutput {
				return nil, r.err
			}
			return &svcsdk.RestoreDBInstanceFromS3Output{DBInstance: r.db()}, r.err
		},
		MockRestoreDBInstanceFromDBSnapshot: func(ctx context.Context, in *svcsdk.RestoreDBInstanceFromDBSnapshotInput, _ ...func(*svcsdk.Options)) (*svcsdk.RestoreDBInstanceFromDBSnapshotOutput, error) {
			r.seen(ctx, "Snapshot:"+aws.ToString(in.DBInstanceIdentifier)+":"+aws.ToString(in.DBSnapshotIdentifier), nil)
			return &svcsdk.RestoreDBInstanceFromDBSnapshotOutput{DBInstance: r.db()}, r.err
		},
		MockRestoreDBInstanceToPointInTime: func(ctx context.Context, in *svcsdk.RestoreDBInstanceToPointInTimeInput, _ ...func(*svcsdk.Options)) (*svcsdk.RestoreDBInstanceToPointInTimeOutput, error) {
			r.seen(ctx, "PointInTime:"+aws.ToString(in.TargetDBInstanceIdentifier)+":"+aws.ToString(in.SourceDBInstanceIdentifier), nil)
			return &svcsdk.RestoreDBInstanceToPointInTimeOutput{DBInstance: r.db()}, r.err
		},
		MockCreateDBInstance: func(context.Context, *svcsdk.CreateDBInstanceInput, ...func(*svcsdk.Options)) (*svcsdk.CreateDBInstanceOutput, error) {
			r.creates++
			return &svcsdk.CreateDBInstanceOutput{}, nil
		},
		MockCreateDBInstanceReadReplica: func(context.Context, *svcsdk.CreateDBInstanceReadReplicaInput, ...func(*svcsdk.Options)) (*svcsdk.CreateDBInstanceReadReplicaOutput, error) {
			r.replicas++
			return &svcsdk.CreateDBInstanceReadReplicaOutput{}, nil
		},
	}
}

func fromS3(cr *svcapitypes.DBInstance) {
	cr.Spec.ForProvider.RestoreFrom = &svcapitypes.RestoreDBInstanceBackupConfiguration{
		Source: aws.String("S3"),
		S3:     &svcapitypes.S3RestoreBackupConfiguration{BucketName: aws.String("b"), SourceEngine: aws.String("mysql")},
	}
}

func fromPointInTime(cr *svcapitypes.DBInstance) {
	cr.Spec.ForProvider.RestoreFrom = &svcapitypes.RestoreDBInstanceBackupConfiguration{
		Source:      aws.String("PointInTime"),
		PointInTime: &svcapitypes.PointInTimeRestoreBackupConfiguration{SourceDBInstanceIdentifier: aws.String("src"), UseLatestRestorableTime: true},
	}
}

func TestRestorePerSource(t *testing.T) {
	cases := map[string]struct {
		source       func(*svcapitypes.DBInstance)
		wantCall     string
		wantSentPW   bool
		wantRestored string
	}{
		"S3":          {source: fromS3, wantCall: "S3:db1", wantSentPW: true, wantRestored: ""},
		"Snapshot":    {source: fromSnapshot, wantCall: "Snapshot:db1:snap", wantRestored: "RestoreStateRestored"},
		"PointInTime": {source: fromPointInTime, wantCall: "PointInTime:db1:src", wantRestored: "RestoreStateRestored"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			kube := newTestKube(t, pwSecret(refPassword))
			rec := &restoreRecorder{t: t, kube: kube}
			cr := newTestCR(withPasswordRef, tc.source)

			cre, err := newCustomExternal(kube, rec.client()).Create(context.Background(), cr)
			if err != nil {
				t.Fatal(err)
			}
			if len(rec.calls) != 1 || rec.calls[0] != tc.wantCall || rec.creates != 0 {
				t.Fatalf("calls = %v, creates = %d; want [%s] and no CreateDBInstance", rec.calls, rec.creates, tc.wantCall)
			}
			if sent := aws.ToString(rec.passwords[0]); (sent == refPassword) != tc.wantSentPW {
				t.Errorf("restore sent password %q, want sent = %v", sent, tc.wantSentPW)
			}
			if c := rec.cacheNow[0]; c[rds.PasswordCacheKey] != refPassword || c[rds.RestoreFlagCacheKay] != tc.wantRestored {
				t.Errorf("cache at restore time = %v, want the password and restore flag %q", c, tc.wantRestored)
			}
			if aws.ToString(cr.Status.AtProvider.DBInstanceStatus) != "creating" || aws.ToString(cr.Status.AtProvider.DBInstanceARN) != testARN {
				t.Errorf("status not filled from the restore output: %+v", cr.Status.AtProvider)
			}
			_, published := cre.ConnectionDetails[passwordKey]
			if published != tc.wantSentPW {
				t.Errorf("password published = %v, want %v (only when AWS got it)", published, tc.wantSentPW)
			}
		})
	}
}

func TestRestoreErrors(t *testing.T) {
	errBoom := errors.New("boom")
	cases := map[string]struct {
		cr        *svcapitypes.DBInstance
		kubeErr   error
		awsErr    error
		nilOutput bool
		wantErr   string
		wantCalls int
		wantCache map[string]string
	}{
		"SnapshotFails":    {cr: newTestCR(withAutogenerate, fromSnapshot), awsErr: errBoom, wantErr: errSnapshotRestoreFailed + ": boom", wantCalls: 1},
		"S3Fails":          {cr: newTestCR(withAutogenerate, fromS3), awsErr: errBoom, wantErr: errS3RestoreFailed + ": boom", wantCalls: 1},
		"PointInTimeFails": {cr: newTestCR(withAutogenerate, fromPointInTime), awsErr: errBoom, wantErr: errPointInTimeRestoreFailed + ": boom", wantCalls: 1},
		"CacheFailsNoAWSCall": {
			cr: newTestCR(withAutogenerate, fromSnapshot), kubeErr: errors.New("forbidden"),
			wantErr: rds.ErrCachePassword + ": cannot create object: forbidden",
		},
		"NilSource": {cr: newTestCR(withAutogenerate, func(cr *svcapitypes.DBInstance) {
			cr.Spec.ForProvider.RestoreFrom = &svcapitypes.RestoreDBInstanceBackupConfiguration{}
		}), wantErr: errNoRestoreSource},
		"UnknownSource": {cr: newTestCR(withAutogenerate, func(cr *svcapitypes.DBInstance) {
			cr.Spec.ForProvider.RestoreFrom = &svcapitypes.RestoreDBInstanceBackupConfiguration{Source: aws.String("Tape")}
		}), wantErr: errUnknownRestoreSource},
		"MissingSnapshotSection": {cr: newTestCR(withAutogenerate, func(cr *svcapitypes.DBInstance) {
			cr.Spec.ForProvider.RestoreFrom = &svcapitypes.RestoreDBInstanceBackupConfiguration{Source: aws.String("Snapshot")}
		}), wantErr: "restoreFrom.source Snapshot needs restoreFrom.snapshot"},
		"MissingS3Section": {cr: newTestCR(withAutogenerate, func(cr *svcapitypes.DBInstance) {
			cr.Spec.ForProvider.RestoreFrom = &svcapitypes.RestoreDBInstanceBackupConfiguration{Source: aws.String("S3")}
		}), wantErr: "restoreFrom.source S3 needs restoreFrom.s3"},
		"MissingPointInTimeSection": {cr: newTestCR(withAutogenerate, func(cr *svcapitypes.DBInstance) {
			cr.Spec.ForProvider.RestoreFrom = &svcapitypes.RestoreDBInstanceBackupConfiguration{Source: aws.String("PointInTime")}
		}), wantErr: "restoreFrom.source PointInTime needs restoreFrom.pointInTime"},
		"NoPasswordSourceKeepsSnapshotPassword": {cr: newTestCR(fromSnapshot), wantCalls: 1},
		"NilOutput":                             {cr: newTestCR(fromS3), nilOutput: true, wantCalls: 1},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			kube := newTestKube(t)
			kube.failWrites = tc.kubeErr
			rec := &restoreRecorder{t: t, kube: kube, err: tc.awsErr, nilOutput: tc.nilOutput}
			_, err := newCustomExternal(kube, rec.client()).Create(context.Background(), tc.cr)
			if got := errString(err); got != tc.wantErr {
				t.Fatalf("Create error = %q, want %q", got, tc.wantErr)
			}
			if len(rec.calls) != tc.wantCalls || rec.creates != 0 {
				t.Errorf("restore calls %v, creates %d; want %d restore calls and no create", rec.calls, rec.creates, tc.wantCalls)
			}
			if name == "NoPasswordSourceKeepsSnapshotPassword" && cacheData(t, kube, tc.cr) != nil {
				t.Error("want no cache and no restore flag without a password source")
			}
		})
	}
}

func TestReadReplicaWinsOverRestore(t *testing.T) {
	kube := newTestKube(t)
	rec := &restoreRecorder{t: t, kube: kube}
	cr := newTestCR(fromSnapshot, func(cr *svcapitypes.DBInstance) { cr.Spec.ForProvider.SourceDBInstanceID = aws.String("primary") })
	if _, err := newCustomExternal(kube, rec.client()).Create(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	if rec.replicas != 1 || len(rec.calls) != 0 || rec.creates != 0 {
		t.Errorf("replicas %d, restores %v, creates %d; want only the replica", rec.replicas, rec.calls, rec.creates)
	}
}

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
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	rds "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds/fake"
)

// createRecorder records CreateDBInstance inputs and the cache content seen at
// the time of the call.
type createRecorder struct {
	t        *testing.T
	kube     *testKube
	inputs   []*svcsdk.CreateDBInstanceInput
	cacheNow []map[string]string
	out      *svcsdk.CreateDBInstanceOutput
	err      error
}

func (r *createRecorder) client() *fake.MockRDSClient {
	return &fake.MockRDSClient{
		MockCreateDBInstance: func(ctx context.Context, in *svcsdk.CreateDBInstanceInput, _ ...func(*svcsdk.Options)) (*svcsdk.CreateDBInstanceOutput, error) {
			r.inputs = append(r.inputs, in)
			r.cacheNow = append(r.cacheNow, cacheDataCtx(ctx, r.t, r.kube, newTestCR()))
			if r.out == nil {
				return &svcsdk.CreateDBInstanceOutput{DBInstance: &svcsdktypes.DBInstance{DBInstanceStatus: aws.String("creating")}}, r.err
			}
			return r.out, r.err
		},
	}
}

func TestCreateWithPasswordRefCachesBeforeCreate(t *testing.T) {
	kube := newTestKube(t, pwSecret(refPassword))
	rec := &createRecorder{t: t, kube: kube}
	cr := newTestCR(withPasswordRef)

	if _, err := newCustomExternal(kube, rec.client()).Create(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	if len(rec.inputs) != 1 || aws.ToString(rec.inputs[0].MasterUserPassword) != refPassword {
		t.Fatalf("CreateDBInstance inputs = %+v, want one with the ref password", rec.inputs)
	}
	if got := rec.cacheNow[0][rds.PasswordCacheKey]; got != refPassword {
		t.Errorf("cached password at CreateDBInstance time = %q, want it cached before the call", got)
	}
}

func TestCreateAutogenerateReusesPasswordOnRetry(t *testing.T) {
	kube := newTestKube(t)
	rec := &createRecorder{t: t, kube: kube, err: errors.New("throttled")}
	e := newCustomExternal(kube, rec.client())

	for range 2 {
		if _, err := e.Create(context.Background(), newTestCR(withAutogenerate)); err == nil {
			t.Fatal("want the AWS error")
		}
	}
	first, second := aws.ToString(rec.inputs[0].MasterUserPassword), aws.ToString(rec.inputs[1].MasterUserPassword)
	if first == "" || first != second {
		t.Errorf("passwords sent = %q, %q; want one generated password reused on retry", first, second)
	}
}

func TestCreateStandaloneCases(t *testing.T) {
	cases := map[string]struct {
		cr        *svcapitypes.DBInstance
		objs      []client.Object
		awsErr    error
		wantErr   string
		wantCalls int
		check     func(t *testing.T, in *svcsdk.CreateDBInstanceInput)
	}{
		"NoPasswordSource": {
			cr:      newTestCR(),
			wantErr: "pre-create failed: " + rds.ErrNoMasterPasswordSource,
		},
		"MissingRefSecret": {
			cr:      newTestCR(withPasswordRef),
			wantErr: "pre-create failed: " + rds.ErrEmptyMasterPassword,
		},
		"EmptyRefValue": {
			cr:      newTestCR(withPasswordRef),
			objs:    []client.Object{pwSecret("")},
			wantErr: "pre-create failed: " + rds.ErrEmptyMasterPassword,
		},
		"EmptyRefFallsBackToAutogenerate": {
			cr:        newTestCR(withPasswordRef, withAutogenerate),
			objs:      []client.Object{pwSecret("")},
			wantCalls: 1,
			check: func(t *testing.T, in *svcsdk.CreateDBInstanceInput) {
				if aws.ToString(in.MasterUserPassword) == "" {
					t.Error("want a generated password")
				}
			},
		},
		"ClusterMemberSendsNoPassword": {
			cr: newTestCR(withAutogenerate, func(cr *svcapitypes.DBInstance) {
				cr.Spec.ForProvider.DBClusterIdentifier = aws.String("c1")
				cr.Spec.ForProvider.VPCSecurityGroupIDs = []string{sgA}
			}),
			wantCalls: 1,
			check: func(t *testing.T, in *svcsdk.CreateDBInstanceInput) {
				if in.MasterUserPassword != nil || in.VpcSecurityGroupIds != nil {
					t.Errorf("cluster member input: password %v, SGs %v; want neither", in.MasterUserPassword, in.VpcSecurityGroupIds)
				}
			},
		},
		"AWSErrorWrapped": {
			cr:        newTestCR(withPasswordRef),
			objs:      []client.Object{pwSecret("pw")},
			awsErr:    errors.New("boom"),
			wantErr:   errCreate + ": boom",
			wantCalls: 1,
		},
		"GP3BelowThresholdDropsIOPS": {
			cr: newTestCR(withPasswordRef, func(cr *svcapitypes.DBInstance) {
				cr.Spec.ForProvider.StorageType = aws.String("gp3")
				cr.Spec.ForProvider.AllocatedStorage = aws.Int64(20)
				cr.Spec.ForProvider.IOPS = aws.Int64(3000)
				cr.Spec.ForProvider.StorageThroughput = aws.Int64(125)
				cr.Spec.ForProvider.VPCSecurityGroupIDs = []string{sgA}
				cr.Spec.ForProvider.DBSecurityGroups = []string{"dbsg"}
				cr.Spec.ForProvider.EngineVersion = aws.String("16.4")
			}),
			objs:      []client.Object{pwSecret("pw")},
			wantCalls: 1,
			check: func(t *testing.T, in *svcsdk.CreateDBInstanceInput) {
				if in.Iops != nil || in.StorageThroughput != nil {
					t.Errorf("gp3 below threshold: iops %v, throughput %v; want neither", in.Iops, in.StorageThroughput)
				}
				if len(in.VpcSecurityGroupIds) != 1 || len(in.DBSecurityGroups) != 1 || aws.ToString(in.EngineVersion) != "16.4" {
					t.Errorf("input = %+v, want SGs and engine version from the spec", in)
				}
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			kube := newTestKube(t, tc.objs...)
			rec := &createRecorder{t: t, kube: kube, err: tc.awsErr}
			_, err := newCustomExternal(kube, rec.client()).Create(context.Background(), tc.cr)
			if got := errString(err); got != tc.wantErr {
				t.Fatalf("Create error = %q, want %q", got, tc.wantErr)
			}
			if len(rec.inputs) != tc.wantCalls {
				t.Fatalf("CreateDBInstance calls = %d, want %d", len(rec.inputs), tc.wantCalls)
			}
			if tc.check != nil {
				tc.check(t, rec.inputs[0])
			}
		})
	}
}

func TestCreatePublishesSentPasswordAndUsername(t *testing.T) {
	kube := newTestKube(t, pwSecret(refPassword))
	rec := &createRecorder{t: t, kube: kube, out: &svcsdk.CreateDBInstanceOutput{DBInstance: &svcsdktypes.DBInstance{
		DBInstanceStatus: aws.String("creating"),
		MasterUsername:   aws.String("aws-default-user"),
	}}}
	cr := newTestCR(withPasswordRef, func(cr *svcapitypes.DBInstance) { cr.Spec.ForProvider.MasterUsername = nil })

	cre, err := newCustomExternal(kube, rec.client()).Create(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"username": "aws-default-user", "password": refPassword}
	for k, v := range want {
		if got := string(cre.ConnectionDetails[k]); got != v {
			t.Errorf("connection detail %s = %q, want %q", k, got, v)
		}
	}
	if cr.Spec.ForProvider.MasterUsername != nil {
		t.Error("Create must not write the username into the spec")
	}
}

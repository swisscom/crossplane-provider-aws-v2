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
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	rds "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds/fake"
)

const tdeValue = "tde-s3cret"

func withTDERef(cr *svcapitypes.DBInstance) {
	cr.Spec.ForProvider.TDECredentialPasswordSecretRef = &xpv2.LocalSecretKeySelector{
		LocalSecretReference: xpv2.LocalSecretReference{Name: "tde"}, Key: passwordKey,
	}
}

func tdeSecret(value string) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "tde", Namespace: testNS}, Data: map[string][]byte{passwordKey: []byte(value)}}
}

func TestCreateSendsTDEPasswordFromSecret(t *testing.T) {
	kube := newTestKube(t, pwSecret("pw"), tdeSecret(tdeValue))
	rec := &createRecorder{t: t, kube: kube}
	if _, err := newCustomExternal(kube, rec.client()).Create(context.Background(), newTestCR(withPasswordRef, withTDERef)); err != nil {
		t.Fatal(err)
	}
	if got := aws.ToString(rec.inputs[0].TdeCredentialPassword); got != tdeValue {
		t.Errorf("TdeCredentialPassword = %q, want the Secret's value", got)
	}
}

func TestTDEPasswordCases(t *testing.T) {
	cases := map[string]struct {
		tde     func(*svcapitypes.DBInstance)
		objs    []client.Object
		wantPW  *string
		wantErr string
	}{
		"NoRef":         {},
		"FromSecret":    {tde: withTDERef, objs: []client.Object{tdeSecret(tdeValue)}, wantPW: aws.String(tdeValue)},
		"MissingSecret": {tde: withTDERef, wantErr: errTDEPassword},
		"EmptyValue":    {tde: withTDERef, objs: []client.Object{tdeSecret("")}, wantErr: errTDEPassword},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			opts := []func(*svcapitypes.DBInstance){withPasswordRef}
			if tc.tde != nil {
				opts = append(opts, tc.tde)
			}
			// Modify: rotate the master password so an Update runs.
			kube := newTestKube(t, append(tc.objs, pwSecret("new"))...)
			cr := newTestCR(opts...)
			seedCache(t, kube, cr, map[string]string{rds.PasswordCacheKey: oldValue})
			sent, _, err := observeAndUpdate(t, kube, cr, describeOut("available"))
			checkTDE(t, "Update", err, tc.wantErr, func() *string { return sent.TdeCredentialPassword }, tc.wantPW)

			// Create.
			kube = newTestKube(t, append(tc.objs, pwSecret("pw"))...)
			rec := &createRecorder{t: t, kube: kube}
			_, err = newCustomExternal(kube, rec.client()).Create(context.Background(), newTestCR(opts...))
			checkTDE(t, "Create", err, tc.wantErr, func() *string { return rec.inputs[0].TdeCredentialPassword }, tc.wantPW)
		})
	}
}

func checkTDE(t *testing.T, op string, err error, wantErr string, sent func() *string, want *string) {
	t.Helper()
	if wantErr != "" {
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("%s error = %v, want it to contain %q", op, err, wantErr)
		}
		if err != nil && strings.Contains(err.Error(), tdeValue) {
			t.Errorf("%s error leaks the TDE password: %v", op, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("%s: %v", op, err)
	}
	if got := sent(); aws.ToString(got) != aws.ToString(want) || (got == nil) != (want == nil) {
		t.Errorf("%s TdeCredentialPassword = %v, want %v", op, aws.ToString(got), aws.ToString(want))
	}
}

func TestTDERefIsNoDrift(t *testing.T) {
	cr := newTestCR(withPasswordRef, withTDERef)
	kube := newTestKube(t, pwSecret("pw"), tdeSecret(tdeValue))
	seedCache(t, kube, cr, map[string]string{rds.PasswordCacheKey: "pw"})
	obs, err := newCustomExternal(kube, &fake.MockRDSClient{MockDescribeDBInstances: describeFn(describeOut("available"), nil)}).Observe(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}
	if !obs.ResourceUpToDate {
		t.Errorf("the TDE ref alone must not be drift, diff: %s", obs.Diff)
	}
}

func TestRestoreSendsNoTDEPassword(t *testing.T) {
	kube := newTestKube(t, tdeSecret(tdeValue))
	var in *svcsdk.RestoreDBInstanceFromDBSnapshotInput
	rdsc := &fake.MockRDSClient{
		MockRestoreDBInstanceFromDBSnapshot: func(_ context.Context, i *svcsdk.RestoreDBInstanceFromDBSnapshotInput, _ ...func(*svcsdk.Options)) (*svcsdk.RestoreDBInstanceFromDBSnapshotOutput, error) {
			in = i
			return &svcsdk.RestoreDBInstanceFromDBSnapshotOutput{}, nil
		},
	}
	if _, err := newCustomExternal(kube, rdsc).Create(context.Background(), newTestCR(fromSnapshot, withTDERef)); err != nil {
		t.Fatal(err)
	}
	if in.TdeCredentialPassword != nil {
		t.Error("restores must not send the TDE password")
	}
}

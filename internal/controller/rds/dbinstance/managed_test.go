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

func withManaged(on bool) func(*svcapitypes.DBInstance) {
	return func(cr *svcapitypes.DBInstance) { cr.Spec.ForProvider.ManageMasterUserPassword = aws.Bool(on) }
}

func TestCreateManagedMasterPassword(t *testing.T) {
	cases := map[string]struct {
		cr      *svcapitypes.DBInstance
		objs    []client.Object
		wantErr string
	}{
		"Managed":                 {cr: newTestCR(withManaged(true))},
		"ManagedWithRef":          {cr: newTestCR(withManaged(true), withPasswordRef), objs: []client.Object{pwSecret("pw")}, wantErr: "pre-create failed: " + rds.ErrManagedPasswordConflict},
		"ManagedWithAutogenerate": {cr: newTestCR(withManaged(true), withAutogenerate), wantErr: "pre-create failed: " + rds.ErrManagedPasswordConflict},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			kube := newTestKube(t, tc.objs...)
			rec := &createRecorder{t: t, kube: kube}
			cre, err := newCustomExternal(kube, rec.client()).Create(context.Background(), tc.cr)
			if got := errString(err); got != tc.wantErr {
				t.Fatalf("Create error = %q, want %q", got, tc.wantErr)
			}
			if tc.wantErr != "" {
				if len(rec.inputs) != 0 {
					t.Error("CreateDBInstance must not be called")
				}
				return
			}
			in := rec.inputs[0]
			if in.MasterUserPassword != nil || !aws.ToBool(in.ManageMasterUserPassword) {
				t.Errorf("input: password %v, manage %v; want no password and manage=true", in.MasterUserPassword, in.ManageMasterUserPassword)
			}
			if cacheData(t, kube, tc.cr) != nil {
				t.Error("want no password cache for an AWS-managed password")
			}
			if pw, ok := cre.ConnectionDetails[passwordKey]; !ok || len(pw) != 0 {
				t.Errorf("password detail = %q (present %v), want an empty value", pw, ok)
			}
		})
	}
}

func managedSecret(db *svcsdktypes.DBInstance) {
	db.MasterUserSecret = &svcsdktypes.MasterUserSecret{SecretArn: aws.String("arn:aws:secretsmanager:secret")}
}

// observeAndUpdate runs Observe and, if not up to date, Update; it returns the
// sent ModifyDBInstance input (nil if none).
func observeAndUpdate(t *testing.T, kube *testKube, cr *svcapitypes.DBInstance, out *svcsdk.DescribeDBInstancesOutput) (*svcsdk.ModifyDBInstanceInput, map[string][]byte, error) {
	t.Helper()
	var sent *svcsdk.ModifyDBInstanceInput
	rdsc := &fake.MockRDSClient{
		MockDescribeDBInstances: describeFn(out, nil),
		MockModifyDBInstance: func(_ context.Context, in *svcsdk.ModifyDBInstanceInput, _ ...func(*svcsdk.Options)) (*svcsdk.ModifyDBInstanceOutput, error) {
			sent = in
			return &svcsdk.ModifyDBInstanceOutput{}, nil
		},
	}
	e := newCustomExternal(kube, rdsc)
	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}
	if obs.ResourceUpToDate {
		return nil, obs.ConnectionDetails, nil
	}
	upd, err := e.Update(context.Background(), cr)
	return sent, upd.ConnectionDetails, err
}

func TestManagedPasswordToggle(t *testing.T) {
	cases := map[string]struct {
		cr           *svcapitypes.DBInstance
		objs         []client.Object
		cache        map[string]string
		db           func(*svcsdktypes.DBInstance)
		wantManage   *bool
		wantPassword string // "" none, "*" any generated one
		wantErr      string
	}{
		"TurnOn": {
			cr:         newTestCR(withManaged(true), withPasswordRef),
			objs:       []client.Object{pwSecret("rotated")},
			cache:      map[string]string{rds.PasswordCacheKey: oldValue},
			wantManage: aws.Bool(true),
		},
		"TurnOffWithRef": {
			cr:           newTestCR(withManaged(false), withPasswordRef),
			objs:         []client.Object{pwSecret(refPassword)},
			db:           managedSecret,
			wantManage:   aws.Bool(false),
			wantPassword: refPassword,
		},
		"TurnOffWithAutogenerate": {
			cr:           newTestCR(withManaged(false), withAutogenerate),
			cache:        map[string]string{rds.PasswordCacheKey: "stale"},
			db:           managedSecret,
			wantManage:   aws.Bool(false),
			wantPassword: "*",
		},
		"TurnOffWithoutPassword": {
			cr:      newTestCR(withManaged(false)),
			db:      managedSecret,
			wantErr: "pre-update failed: " + rds.ErrManagedOffNeedsPassword,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			kube := newTestKube(t, tc.objs...)
			if tc.cache != nil {
				seedCache(t, kube, tc.cr, tc.cache)
			}
			out := describeOut("available")
			if tc.db != nil {
				tc.db(&out.DBInstances[0])
			}
			sent, details, err := observeAndUpdate(t, kube, tc.cr, out)
			if got := errString(err); got != tc.wantErr {
				t.Fatalf("Update error = %q, want %q", got, tc.wantErr)
			}
			if tc.wantErr != "" {
				return
			}
			if sent == nil {
				t.Fatal("want a ModifyDBInstance call")
			}
			if aws.ToBool(sent.ManageMasterUserPassword) != *tc.wantManage || sent.ManageMasterUserPassword == nil {
				t.Errorf("ManageMasterUserPassword = %v, want %v", sent.ManageMasterUserPassword, *tc.wantManage)
			}
			sentPW := aws.ToString(sent.MasterUserPassword)
			switch tc.wantPassword {
			case "":
				if sentPW != "" {
					t.Errorf("sent password %q, want none", sentPW)
				}
				if pw, ok := details[passwordKey]; ok {
					t.Errorf("published password %q, want none", pw)
				}
			case "*":
				if sentPW == "" || sentPW == "stale" {
					t.Errorf("sent password %q, want a newly generated one", sentPW)
				}
			default:
				if sentPW != tc.wantPassword {
					t.Errorf("sent password %q, want %q", sentPW, tc.wantPassword)
				}
			}
			if tc.wantPassword != "" {
				if got := string(details[passwordKey]); got != sentPW {
					t.Errorf("published %q, want the sent password", got)
				}
				if got := cacheData(t, kube, tc.cr)[rds.PasswordCacheKey]; got != sentPW {
					t.Errorf("cached %q, want the sent password", got)
				}
			}
		})
	}
}

func TestManagedInstanceIgnoresRotatedRef(t *testing.T) {
	cr := newTestCR(withManaged(true), withPasswordRef)
	kube := newTestKube(t, pwSecret("rotated"))
	seedCache(t, kube, cr, map[string]string{rds.PasswordCacheKey: oldValue})
	sent, details, err := observeAndUpdate(t, kube, cr, describeOut("available", managedSecret))
	if err != nil {
		t.Fatal(err)
	}
	if sent != nil {
		t.Errorf("ModifyDBInstance sent %+v, want no modify while AWS manages the password", sent)
	}
	if pw, ok := details[passwordKey]; !ok || len(pw) != 0 {
		t.Errorf("published password %q (present %v), want an empty value", pw, ok)
	}
}

func TestRestoreManagedPassword(t *testing.T) {
	kube := newTestKube(t)
	var in *svcsdk.RestoreDBInstanceFromDBSnapshotInput
	rdsc := &fake.MockRDSClient{
		MockRestoreDBInstanceFromDBSnapshot: func(_ context.Context, i *svcsdk.RestoreDBInstanceFromDBSnapshotInput, _ ...func(*svcsdk.Options)) (*svcsdk.RestoreDBInstanceFromDBSnapshotOutput, error) {
			in = i
			return &svcsdk.RestoreDBInstanceFromDBSnapshotOutput{}, nil
		},
	}
	cr := newTestCR(withManaged(true), fromSnapshot, func(cr *svcapitypes.DBInstance) {
		cr.Spec.ForProvider.MasterUserSecretKMSKeyID = aws.String("alias/secrets")
	})
	if _, err := newCustomExternal(kube, rdsc).Create(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	if !aws.ToBool(in.ManageMasterUserPassword) || aws.ToString(in.MasterUserSecretKmsKeyId) != "alias/secrets" {
		t.Errorf("restore input: manage %v, kms %v; want the spec values", in.ManageMasterUserPassword, in.MasterUserSecretKmsKeyId)
	}
	if cacheData(t, kube, cr) != nil {
		t.Error("want no cache and no restore flag for an AWS-managed password")
	}
}

// pendingPasswordAWS fakes an instance whose ModifyDBInstance applies the
// password server side and then fails in the given way.
type pendingPasswordAWS struct {
	kube    *testKube
	managed bool
	sent    []string
	// after the modify was applied: fail the cache write, or lose the response
	failCacheWrite, loseResponse bool
}

func (a *pendingPasswordAWS) client() *fake.MockRDSClient {
	return &fake.MockRDSClient{
		MockDescribeDBInstances: func(context.Context, *svcsdk.DescribeDBInstancesInput, ...func(*svcsdk.Options)) (*svcsdk.DescribeDBInstancesOutput, error) {
			if a.managed {
				return describeOut("available", managedSecret), nil
			}
			return describeOut("available"), nil
		},
		MockModifyDBInstance: func(_ context.Context, in *svcsdk.ModifyDBInstanceInput, _ ...func(*svcsdk.Options)) (*svcsdk.ModifyDBInstanceOutput, error) {
			a.sent = append(a.sent, aws.ToString(in.MasterUserPassword))
			if in.ManageMasterUserPassword != nil {
				a.managed = *in.ManageMasterUserPassword
			}
			if a.failCacheWrite {
				a.kube.failWrites = errors.New("forbidden")
			}
			if a.loseResponse {
				return nil, errors.New("RequestTimeout")
			}
			return &svcsdk.ModifyDBInstanceOutput{}, nil
		},
	}
}

func TestPasswordModifyAppliedButNotRecorded(t *testing.T) {
	cases := map[string]struct {
		cr             *svcapitypes.DBInstance
		objs           []client.Object
		managed        bool
		failCacheWrite bool
		loseResponse   bool
	}{
		"ManagedOffCacheWriteFails": {cr: newTestCR(withManaged(false), withAutogenerate), managed: true, failCacheWrite: true},
		"ManagedOffLostResponse":    {cr: newTestCR(withManaged(false), withAutogenerate), managed: true, loseResponse: true},
		"RotationLostResponse":      {cr: newTestCR(withPasswordRef), objs: []client.Object{pwSecret("rotated")}, loseResponse: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			kube := newTestKube(t, tc.objs...)
			// The cache still holds the password from before (e.g. before management was turned on).
			seedCache(t, kube, tc.cr, map[string]string{rds.PasswordCacheKey: oldPassword})
			a := &pendingPasswordAWS{kube: kube, managed: tc.managed, failCacheWrite: tc.failCacheWrite, loseResponse: tc.loseResponse}

			e := newCustomExternal(kube, a.client())
			if _, err := e.Observe(ctx, tc.cr); err != nil {
				t.Fatal(err)
			}
			if _, err := e.Update(ctx, tc.cr); err == nil {
				t.Fatal("want the Update to fail")
			}
			kube.failWrites = nil
			a.failCacheWrite, a.loseResponse = false, false
			if len(a.sent) != 1 || a.sent[0] == "" || a.sent[0] == oldPassword {
				t.Fatalf("sent %q, want one new password", a.sent)
			}

			// AWS has the new password now.
			e = newCustomExternal(kube, a.client())
			obs, err := e.Observe(ctx, tc.cr)
			if err != nil {
				t.Fatal(err)
			}
			if pw, ok := obs.ConnectionDetails[passwordKey]; ok {
				t.Errorf("published %q while the new password is unconfirmed, want no password", pw)
			}
			if obs.ResourceUpToDate {
				t.Fatal("reported up to date: nothing would re-send the password")
			}
			upd, err := e.Update(ctx, tc.cr)
			if err != nil {
				t.Fatal(err)
			}
			if len(a.sent) != 2 || a.sent[1] != a.sent[0] {
				t.Errorf("retry sent %q, want the same password again", a.sent)
			}
			if got := string(upd.ConnectionDetails[passwordKey]); got != a.sent[0] {
				t.Errorf("published %q after the retry, want the sent password", got)
			}
		})
	}
}

func TestManagedOffRetryAfterModifyErrorSendsSamePassword(t *testing.T) {
	ctx := context.Background()
	cr := newTestCR(withManaged(false), withAutogenerate)
	kube := newTestKube(t)
	var sent []string
	fail := true
	rdsc := &fake.MockRDSClient{
		MockDescribeDBInstances: describeFn(describeOut("available", managedSecret), nil),
		MockModifyDBInstance: func(_ context.Context, in *svcsdk.ModifyDBInstanceInput, _ ...func(*svcsdk.Options)) (*svcsdk.ModifyDBInstanceOutput, error) {
			sent = append(sent, aws.ToString(in.MasterUserPassword))
			if fail {
				return nil, errors.New("InvalidDBInstanceState")
			}
			return &svcsdk.ModifyDBInstanceOutput{}, nil
		},
	}
	for range 2 {
		e := newCustomExternal(kube, rdsc)
		if _, err := e.Observe(ctx, cr); err != nil {
			t.Fatal(err)
		}
		_, _ = e.Update(ctx, cr)
	}
	if len(sent) != 2 || sent[0] == "" || sent[0] != sent[1] {
		t.Errorf("sent %q, want the same generated password on retry", sent)
	}
}

func TestPasswordNotSentWhenItCannotBeRecorded(t *testing.T) {
	cr := newTestCR(withManaged(false), withAutogenerate)
	kube := newTestKube(t)
	modifies := 0
	rdsc := &fake.MockRDSClient{
		MockDescribeDBInstances: describeFn(describeOut("available", managedSecret), nil),
		MockModifyDBInstance: func(context.Context, *svcsdk.ModifyDBInstanceInput, ...func(*svcsdk.Options)) (*svcsdk.ModifyDBInstanceOutput, error) {
			modifies++
			return &svcsdk.ModifyDBInstanceOutput{}, nil
		},
	}
	e := newCustomExternal(kube, rdsc)
	if _, err := e.Observe(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	kube.failWrites = errors.New("forbidden")
	if _, err := e.Update(context.Background(), cr); err == nil || modifies != 0 {
		t.Errorf("Update err = %v, modifies = %d; want an error and no modify", err, modifies)
	}
}

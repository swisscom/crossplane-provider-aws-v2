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
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/smithy-go"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	rds "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds/fake"
)

func TestObserveNotFound(t *testing.T) {
	cases := map[string]struct {
		out        *svcsdk.DescribeDBInstancesOutput
		err        error
		wantExists bool
		wantErr    string
	}{
		"EmptyList":     {out: &svcsdk.DescribeDBInstancesOutput{}},
		"OtherInstance": {out: describeOut("available", func(db *svcsdktypes.DBInstance) { db.DBInstanceIdentifier = aws.String("other") })},
		"NotFoundError": {err: &smithy.GenericAPIError{Code: "DBInstanceNotFound"}},
		"OtherError":    {err: errors.New("throttled"), wantErr: errDescribe + ": throttled"},
		"NilResponse":   {wantErr: errEmptyDescribe},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			obs, err := newCustomExternal(newTestKube(t), &fake.MockRDSClient{MockDescribeDBInstances: describeFn(tc.out, tc.err)}).Observe(context.Background(), newTestCR())
			if got := errString(err); got != tc.wantErr {
				t.Fatalf("Observe error = %q, want %q", got, tc.wantErr)
			}
			if obs.ResourceExists != tc.wantExists {
				t.Errorf("ResourceExists = %v, want %v", obs.ResourceExists, tc.wantExists)
			}
		})
	}
}

func TestObserveConnectionDetails(t *testing.T) {
	cr := newTestCR()
	kube := newTestKube(t)
	seedCache(t, kube, cr, map[string]string{rds.PasswordCacheKey: "pw"})
	obs, err := newCustomExternal(kube, &fake.MockRDSClient{MockDescribeDBInstances: describeFn(describeOut("available"), nil)}).Observe(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"username": testUser, passwordKey: "pw", "endpoint": "db1.example", "port": "5432"}
	for k, v := range want {
		if got := string(obs.ConnectionDetails[k]); got != v {
			t.Errorf("connection detail %s = %q, want %q", k, got, v)
		}
	}
	if !xpv2.Available().Equal(cr.Status.GetCondition(xpv2.TypeReady)) {
		t.Errorf("Ready condition = %+v, want Available", cr.Status.GetCondition(xpv2.TypeReady))
	}
}

func TestObserveStatusConditions(t *testing.T) {
	cases := map[string]xpv2.ConditionReason{
		"available":    xpv2.ReasonAvailable,
		"modifying":    xpv2.ReasonAvailable,
		statusCreating: xpv2.ReasonCreating,
		"deleting":     xpv2.ReasonDeleting,
		"failed":       xpv2.ReasonUnavailable,
	}
	for status, want := range cases {
		t.Run(status, func(t *testing.T) {
			cr := newTestCR()
			if _, err := newCustomExternal(newTestKube(t), &fake.MockRDSClient{MockDescribeDBInstances: describeFn(describeOut(status), nil)}).Observe(context.Background(), cr); err != nil {
				t.Fatal(err)
			}
			if got := cr.Status.GetCondition(xpv2.TypeReady).Reason; got != want {
				t.Errorf("Ready reason = %q, want %q", got, want)
			}
		})
	}
}

type failingTracker struct{}

func (failingTracker) Track(context.Context, resource.ModernManaged) error { return errors.New("boom") }

func TestConnectError(t *testing.T) {
	c := &customConnector{kube: newTestKube(t), usage: failingTracker{}}
	if _, err := c.Connect(context.Background(), newTestCR()); err == nil || !strings.HasPrefix(err.Error(), errCreateSession) {
		t.Errorf("Connect error = %v, want it to start with %q", err, errCreateSession)
	}
}

// fill sets every exported field of v to a non-zero value, so the generated
// conversions run through all their branches.
func fill(v reflect.Value, depth int) {
	if depth > 6 || !v.CanSet() {
		return
	}
	switch v.Kind() { //nolint:exhaustive // API and SDK types use no other kinds
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fill(v.Elem(), depth+1)
	case reflect.Struct:
		if v.Type() == reflect.TypeFor[time.Time]() {
			v.Set(reflect.ValueOf(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
			return
		}
		for i := range v.NumField() {
			fill(v.Field(i), depth+1)
		}
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fill(s.Index(0), depth+1)
		v.Set(s)
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		k, e := reflect.New(v.Type().Key()).Elem(), reflect.New(v.Type().Elem()).Elem()
		fill(k, depth+1)
		fill(e, depth+1)
		m.SetMapIndex(k, e)
		v.Set(m)
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1)
	default:
	}
}

// fullDB is an instance with every field set, as DescribeDBInstances could
// return it.
func fullDB(status string) svcsdktypes.DBInstance {
	db := svcsdktypes.DBInstance{}
	fill(reflect.ValueOf(&db).Elem(), 0)
	db.DBInstanceIdentifier = aws.String(testName)
	db.DBInstanceStatus = aws.String(status)
	db.Engine = aws.String("postgres")
	db.PreferredMaintenanceWindow = aws.String("sun:03:00-sun:04:00")
	db.PreferredBackupWindow = aws.String("01:00-02:00")
	return db
}

func TestGeneratedPathsWithFullInstance(t *testing.T) {
	db := fullDB("available")
	var created *svcsdk.CreateDBInstanceInput
	var modified *svcsdk.ModifyDBInstanceInput
	rdsc := &fake.MockRDSClient{
		MockCreateDBInstance: func(_ context.Context, in *svcsdk.CreateDBInstanceInput, _ ...func(*svcsdk.Options)) (*svcsdk.CreateDBInstanceOutput, error) {
			created = in
			return &svcsdk.CreateDBInstanceOutput{DBInstance: &db}, nil
		},
		MockDescribeDBInstances: describeFn(&svcsdk.DescribeDBInstancesOutput{DBInstances: []svcsdktypes.DBInstance{db}}, nil),
		MockModifyDBInstance: func(_ context.Context, in *svcsdk.ModifyDBInstanceInput, _ ...func(*svcsdk.Options)) (*svcsdk.ModifyDBInstanceOutput, error) {
			modified = in
			return &svcsdk.ModifyDBInstanceOutput{DBInstance: &db}, nil
		},
		MockDescribeDBClusters: func(context.Context, *svcsdk.DescribeDBClustersInput, ...func(*svcsdk.Options)) (*svcsdk.DescribeDBClustersOutput, error) {
			return &svcsdk.DescribeDBClustersOutput{}, nil
		},
	}

	// A spec with every field set, minus the parts that pick another create path.
	cr := newTestCR()
	fill(reflect.ValueOf(&cr.Spec.ForProvider).Elem(), 0)
	p := &cr.Spec.ForProvider
	p.RestoreFrom, p.SourceDBInstanceID, p.SourceDBInstanceIDRef, p.SourceDBInstanceIDSelector, p.SourceDBClusterID = nil, nil, nil, nil, nil
	p.ManageMasterUserPassword, p.AutogeneratePassword, p.DBClusterIdentifier, p.TDECredentialPasswordSecretRef = nil, false, nil, nil
	p.Engine = aws.String("postgres")
	p.PreferredMaintenanceWindow = aws.String("sun:03:00-sun:04:00")
	p.PreferredBackupWindow = aws.String("01:00-02:00")
	withPasswordRef(cr)
	userSpec := p.DeepCopy()

	e := newCustomExternal(newTestKube(t, pwSecret("pw")), rdsc)
	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	if created == nil || !reflect.DeepEqual(p, userSpec) {
		t.Fatal("Create must call CreateDBInstance and leave the spec alone")
	}
	if aws.ToString(cr.Status.AtProvider.DBInstanceARN) == "" {
		t.Error("Create must fill status from the response")
	}
	if _, err := e.Observe(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Update(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	if modified == nil || aws.ToString(modified.DBInstanceIdentifier) != testName {
		t.Errorf("ModifyDBInstance input = %+v, want the external name", modified)
	}

	// An empty create response leaves status alone; the next observe fills it.
	empty := newTestCR(withPasswordRef)
	rdsc.MockCreateDBInstance = func(context.Context, *svcsdk.CreateDBInstanceInput, ...func(*svcsdk.Options)) (*svcsdk.CreateDBInstanceOutput, error) {
		return &svcsdk.CreateDBInstanceOutput{DBInstance: &svcsdktypes.DBInstance{}}, nil
	}
	if _, err := newCustomExternal(newTestKube(t, pwSecret("pw")), rdsc).Create(context.Background(), empty); err != nil {
		t.Fatal(err)
	}
}

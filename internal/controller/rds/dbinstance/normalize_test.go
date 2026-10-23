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
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds/fake"
)

const piKeyARN = "arn:aws:kms:eu-central-1:123456789012:key/1234abcd-12ab-34cd-56ef-1234567890ab"

// createThenObserve creates cr against an AWS that answers with db (as
// CreateDBInstance and DescribeDBInstances would), then observes it.
func createThenObserve(t *testing.T, cr *svcapitypes.DBInstance, db svcsdktypes.DBInstance) (upToDate bool, diff string) {
	t.Helper()
	rdsc := &fake.MockRDSClient{
		MockCreateDBInstance: func(context.Context, *svcsdk.CreateDBInstanceInput, ...func(*svcsdk.Options)) (*svcsdk.CreateDBInstanceOutput, error) {
			return &svcsdk.CreateDBInstanceOutput{DBInstance: &db}, nil
		},
		MockDescribeDBInstances: describeFn(&svcsdk.DescribeDBInstancesOutput{DBInstances: []svcsdktypes.DBInstance{db}}, nil),
	}
	e := newCustomExternal(newTestKube(t, pwSecret("pw")), rdsc)
	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}
	return obs.ResourceUpToDate, obs.Diff
}

func TestObservePerformanceInsightsKMSKey(t *testing.T) {
	cases := map[string]struct {
		spec         string
		wantUpToDate bool
	}{
		"KeyIDvsARN":             {spec: "1234abcd-12ab-34cd-56ef-1234567890ab", wantUpToDate: true},
		"SameARN":                {spec: piKeyARN, wantUpToDate: true},
		"AliasVsARN":             {spec: "alias/pi-key", wantUpToDate: true},
		"AliasARNvsARN":          {spec: "arn:aws:kms:eu-central-1:123456789012:alias/pi-key", wantUpToDate: true},
		"AliasARNOtherAccount":   {spec: "arn:aws:kms:eu-central-1:999999999999:alias/pi-key"},
		"AliasARNOtherRegion":    {spec: "arn:aws:kms:us-east-1:123456789012:alias/pi-key"},
		"AliasARNOtherPartition": {spec: "arn:aws-cn:kms:eu-central-1:123456789012:alias/pi-key"},
		"OtherKeyID":             {spec: "ffffffff-12ab-34cd-56ef-1234567890ab"},
		"OtherKeyARN":            {spec: "arn:aws:kms:eu-central-1:123456789012:key/ffffffff-12ab-34cd-56ef-1234567890ab"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := newTestCR(withPasswordRef, func(cr *svcapitypes.DBInstance) {
				cr.Spec.ForProvider.EnablePerformanceInsights = aws.Bool(true)
				cr.Spec.ForProvider.PerformanceInsightsKMSKeyID = aws.String(tc.spec)
			})
			upToDate, diff := createThenObserve(t, cr, dbOut("available", func(db *svcsdktypes.DBInstance) {
				db.PerformanceInsightsEnabled = aws.Bool(true)
				db.PerformanceInsightsKMSKeyId = aws.String(piKeyARN)
			}))
			if upToDate != tc.wantUpToDate {
				t.Fatalf("upToDate = %v, want %v; diff: %s", upToDate, tc.wantUpToDate, diff)
			}
			if !tc.wantUpToDate && !strings.Contains(diff, "PerformanceInsightsKMSKeyID") {
				t.Errorf("diff does not name the field: %s", diff)
			}
			if got := *cr.Spec.ForProvider.PerformanceInsightsKMSKeyID; got != tc.spec {
				t.Errorf("spec key = %q, want the user's %q", got, tc.spec)
			}
		})
	}
}

func TestObserveCaseNormalisedFields(t *testing.T) {
	cases := map[string]struct {
		spec  func(*svcapitypes.DBInstance, string)
		db    func(*svcsdktypes.DBInstance)
		user  string
		other string
		field string
	}{
		"Engine": {
			spec: func(cr *svcapitypes.DBInstance, v string) { cr.Spec.ForProvider.Engine = aws.String(v) },
			user: "Postgres", other: "mysql", field: "Engine",
		},
		"DBInstanceClass": {
			spec: func(cr *svcapitypes.DBInstance, v string) { cr.Spec.ForProvider.DBInstanceClass = aws.String(v) },
			user: "db.T3.Micro", other: "db.t3.small", field: "DBInstanceClass",
		},
		"StorageType": {
			spec: func(cr *svcapitypes.DBInstance, v string) { cr.Spec.ForProvider.StorageType = aws.String(v) },
			db:   func(db *svcsdktypes.DBInstance) { db.StorageType = aws.String("gp2") },
			user: "GP2", other: "io1", field: "StorageType",
		},
		"LicenseModel": {
			spec: func(cr *svcapitypes.DBInstance, v string) { cr.Spec.ForProvider.LicenseModel = aws.String(v) },
			db:   func(db *svcsdktypes.DBInstance) { db.LicenseModel = aws.String("postgresql-license") },
			user: "PostgreSQL-License", other: "bring-your-own-license", field: "LicenseModel",
		},
		"CACertificateIdentifier": {
			spec: func(cr *svcapitypes.DBInstance, v string) {
				cr.Spec.ForProvider.CACertificateIdentifier = aws.String(v)
			},
			db:   func(db *svcsdktypes.DBInstance) { db.CACertificateIdentifier = aws.String("rds-ca-rsa2048-g1") },
			user: "RDS-CA-RSA2048-G1", other: "rds-ca-ecc384-g1", field: "CACertificateIdentifier",
		},
		"StorageKMSKeyAlias": {
			spec: func(cr *svcapitypes.DBInstance, v string) { cr.Spec.ForProvider.KMSKeyID = aws.String(v) },
			db:   func(db *svcsdktypes.DBInstance) { db.KmsKeyId = aws.String(piKeyARN) },
			user: "alias/aws/rds", other: "ffffffff-12ab-34cd-56ef-1234567890ab", field: "KMSKeyID",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for _, v := range []struct {
				value        string
				wantUpToDate bool
			}{{tc.user, true}, {tc.other, false}} {
				cr := newTestCR(withPasswordRef, func(cr *svcapitypes.DBInstance) { tc.spec(cr, v.value) })
				db := dbOut("available")
				if tc.db != nil {
					tc.db(&db)
				}
				upToDate, diff := createThenObserve(t, cr, db)
				if upToDate != v.wantUpToDate {
					t.Errorf("%s=%q: upToDate = %v, want %v; diff: %s", tc.field, v.value, upToDate, v.wantUpToDate, diff)
				}
				if !v.wantUpToDate && !strings.Contains(diff, tc.field) {
					t.Errorf("%s=%q: diff does not name the field: %s", tc.field, v.value, diff)
				}
			}
		})
	}
}

func TestFirstObserveAfterCreateLateInitializes(t *testing.T) {
	cr := newTestCR(withPasswordRef, func(cr *svcapitypes.DBInstance) {
		cr.Spec.ForProvider.Engine = aws.String("Postgres")
		cr.Spec.ForProvider.MaxAllocatedStorage = aws.Int64(100)
	})
	db := dbOut("available", func(db *svcsdktypes.DBInstance) {
		db.AvailabilityZone = aws.String("eu-central-1a")
		db.MaxAllocatedStorage = nil // AWS omits it in the create response
	})
	rdsc := &fake.MockRDSClient{
		MockCreateDBInstance: func(context.Context, *svcsdk.CreateDBInstanceInput, ...func(*svcsdk.Options)) (*svcsdk.CreateDBInstanceOutput, error) {
			return &svcsdk.CreateDBInstanceOutput{DBInstance: &db}, nil
		},
		MockDescribeDBInstances: describeFn(&svcsdk.DescribeDBInstancesOutput{DBInstances: []svcsdktypes.DBInstance{db}}, nil),
	}
	e := newCustomExternal(newTestKube(t, pwSecret("pw")), rdsc)
	userSpec := cr.Spec.ForProvider.DeepCopy()

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(&cr.Spec.ForProvider, userSpec) {
		t.Fatal("Create changed the spec")
	}
	obs, err := e.Observe(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}
	if !obs.ResourceLateInitialized {
		t.Error("want the first Observe to late-initialize unset fields")
	}
	if got := aws.ToString(cr.Spec.ForProvider.AvailabilityZone); got != "eu-central-1a" {
		t.Errorf("availabilityZone = %q, want it late-initialized", got)
	}
	if aws.ToString(cr.Spec.ForProvider.Engine) != "Postgres" || aws.ToInt64(cr.Spec.ForProvider.MaxAllocatedStorage) != 100 {
		t.Errorf("set fields changed: engine %v, maxAllocatedStorage %v", cr.Spec.ForProvider.Engine, cr.Spec.ForProvider.MaxAllocatedStorage)
	}
}

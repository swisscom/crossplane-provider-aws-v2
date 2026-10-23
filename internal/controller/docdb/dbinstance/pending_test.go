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

	"github.com/aws/aws-sdk-go-v2/service/docdb"
	docdbtypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"k8s.io/utils/ptr"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
)

// AWS reports the old value until the maintenance window; a change it
// already holds must not be sent again on every poll.
func TestObservePendingModifiedValues(t *testing.T) {
	cases := map[string]struct {
		pending  *docdbtypes.PendingModifiedValues
		wantDiff []string
	}{
		"PendingEqualsDesired": {
			pending: &docdbtypes.PendingModifiedValues{DBInstanceClass: ptr.To("db.r6g.xlarge"), CACertificateIdentifier: ptr.To("rds-ca-rsa4096-g1")},
		},
		"PendingDiffersFromDesired": {
			pending:  &docdbtypes.PendingModifiedValues{DBInstanceClass: ptr.To("db.r6g.2xlarge")},
			wantDiff: []string{"caCertificateIdentifier", "dbInstanceClass"},
		},
		"NothingPending": {
			wantDiff: []string{"caCertificateIdentifier", "dbInstanceClass"},
		},
		"OnlyClassPending": {
			pending:  &docdbtypes.PendingModifiedValues{DBInstanceClass: ptr.To("db.r6g.xlarge")},
			wantDiff: []string{"caCertificateIdentifier"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := describeInstances(docdbtypes.DBInstance{
				DBInstanceIdentifier:    ptr.To("orders-1"),
				Engine:                  ptr.To(svcapitypes.EngineDocDB),
				DBInstanceStatus:        ptr.To("available"),
				DBInstanceClass:         ptr.To("db.r6g.large"),
				CACertificateIdentifier: ptr.To("rds-ca-rsa2048-g1"),
				PendingModifiedValues:   tc.pending,
			})
			cr := &svcapitypes.DBInstance{}
			meta.SetExternalName(cr, "orders-1")
			cr.Spec.ForProvider.DBInstanceClass = ptr.To("db.r6g.xlarge")
			cr.Spec.ForProvider.CACertificateIdentifier = ptr.To("rds-ca-rsa4096-g1")

			obs, err := newExternal(nil, c, []option{setupExternal}).Observe(context.Background(), cr)
			if err != nil {
				t.Fatalf("Observe(...): %v", err)
			}

			if want := compare.Message(tc.wantDiff); obs.Diff != want || obs.ResourceUpToDate != (len(tc.wantDiff) == 0) {
				t.Errorf("Observe(...) = up to date %v, diff %q; want diff %q", obs.ResourceUpToDate, obs.Diff, want)
			}
		})
	}
}

// AWS returns the window in lowercase.
func TestObserveMaintenanceWindowIgnoresCase(t *testing.T) {
	cases := map[string]struct {
		observed string
		upToDate bool
	}{
		"SameIgnoringCase": {observed: "sat:02:00-sat:03:00", upToDate: true},
		"OtherWindow":      {observed: "sun:02:00-sun:03:00"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := describeInstances(docdbtypes.DBInstance{
				DBInstanceIdentifier:       ptr.To("orders-1"),
				Engine:                     ptr.To(svcapitypes.EngineDocDB),
				DBInstanceStatus:           ptr.To("available"),
				PreferredMaintenanceWindow: ptr.To(tc.observed),
			})
			cr := &svcapitypes.DBInstance{}
			meta.SetExternalName(cr, "orders-1")
			cr.Spec.ForProvider.PreferredMaintenanceWindow = ptr.To("Sat:02:00-Sat:03:00")

			obs, err := newExternal(nil, c, []option{setupExternal}).Observe(context.Background(), cr)
			if err != nil {
				t.Fatalf("Observe(...): %v", err)
			}
			if obs.ResourceUpToDate != tc.upToDate {
				t.Errorf("Observe(...) = up to date %v, diff %q; want %v", obs.ResourceUpToDate, obs.Diff, tc.upToDate)
			}
			if !tc.upToDate && obs.Diff != compare.Message([]string{"preferredMaintenanceWindow"}) {
				t.Errorf("Observe(...) diff = %q", obs.Diff)
			}
		})
	}
}

// Create keeps the user's casing in the spec, so the first Observe after it
// must not see AWS's lowercase window as drift.
func TestCreateThenObserveWindowCase(t *testing.T) {
	c := describeInstances(docdbtypes.DBInstance{
		DBInstanceIdentifier:       ptr.To("mine"),
		Engine:                     ptr.To(svcapitypes.EngineDocDB),
		DBInstanceStatus:           ptr.To("available"),
		PreferredMaintenanceWindow: ptr.To("mon:03:00-mon:04:00"),
	})
	c.MockCreateDBInstance = func(context.Context, *docdb.CreateDBInstanceInput, ...func(*docdb.Options)) (*docdb.CreateDBInstanceOutput, error) {
		return &docdb.CreateDBInstanceOutput{DBInstance: &docdbtypes.DBInstance{DBInstanceIdentifier: ptr.To("mine"), PreferredMaintenanceWindow: ptr.To("mon:03:00-mon:04:00")}}, nil
	}
	cr := &svcapitypes.DBInstance{}
	meta.SetExternalName(cr, "mine")
	cr.Spec.ForProvider.PreferredMaintenanceWindow = ptr.To("Mon:03:00-Mon:04:00")
	e := newExternal(nil, c, []option{setupExternal})

	if _, err := e.Create(context.Background(), cr); err != nil {
		t.Fatalf("Create(...): %v", err)
	}
	obs, err := e.Observe(context.Background(), cr)
	if err != nil || !obs.ResourceUpToDate {
		t.Errorf("Observe(...) = up to date %v, diff %q, %v; want up to date", obs.ResourceUpToDate, obs.Diff, err)
	}
}

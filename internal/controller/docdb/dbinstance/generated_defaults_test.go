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

// These tests use the generated defaults only (no hand-written hooks), which
// is what every newly generated kind gets.

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/docdb"
	docdbtypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	fake "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/docdb/fake"
)

// defaultsName is the external name of the managed resource under test.
const defaultsName = "mine"

func defaultsCR(name string) *svcapitypes.DBInstance {
	cr := &svcapitypes.DBInstance{}
	cr.SetName(name)
	cr.SetNamespace("team-a")
	meta.SetExternalName(cr, name)
	return cr
}

func defaultsDescribe(ids ...string) func(context.Context, *docdb.DescribeDBInstancesInput, ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
	return func(context.Context, *docdb.DescribeDBInstancesInput, ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
		out := &docdb.DescribeDBInstancesOutput{}
		for _, id := range ids {
			out.DBInstances = append(out.DBInstances, docdbtypes.DBInstance{DBInstanceIdentifier: ptr.To(id)})
		}
		return out, nil
	}
}

func TestGeneratedObserveUsesExternalName(t *testing.T) {
	type want struct {
		exists   bool
		err      string
		statusID string
	}
	cases := map[string]struct {
		ids  []string
		want want
	}{
		"IgnoresForeignResources": {
			ids:  []string{"other-team-db", defaultsName},
			want: want{exists: true, statusID: defaultsName},
		},
		"ForeignResourceOnlyIsNotFound": {
			ids:  []string{"other-team-db"},
			want: want{exists: false},
		},
		// The default filter matches exactly: no prefixes, no case folding.
		"SimilarNameIsNotFound": {
			ids:  []string{"mine-2"},
			want: want{exists: false},
		},
		"DifferentCaseIsNotFound": {
			ids:  []string{"MINE"},
			want: want{exists: false},
		},
		"SeveralMatchesAreAnError": {
			ids:  []string{defaultsName, defaultsName},
			want: want{err: `found 2 DBInstance resources with external name "mine", expected one`},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := &fake.MockDocDBClient{MockDescribeDBInstances: defaultsDescribe(tc.ids...)}
			cr := defaultsCR(defaultsName)

			obs, err := newExternal(nil, c, nil).Observe(context.Background(), cr)

			switch {
			case tc.want.err == "" && err != nil:
				t.Fatalf("Observe: %v", err)
			case tc.want.err != "" && (err == nil || !strings.Contains(err.Error(), tc.want.err)):
				t.Fatalf("Observe error = %v, want %q", err, tc.want.err)
			}
			if got := c.Called.DescribeDBInstances[0].I.DBInstanceIdentifier; ptr.Deref(got, "") != defaultsName {
				t.Errorf("Describe input DBInstanceIdentifier = %v, want mine", ptr.Deref(got, "<nil>"))
			}
			if obs.ResourceExists != tc.want.exists {
				t.Errorf("ResourceExists = %v, want %v", obs.ResourceExists, tc.want.exists)
			}
			if got := ptr.Deref(cr.Status.AtProvider.DBInstanceIdentifier, ""); got != tc.want.statusID {
				t.Errorf("status.atProvider.dbInstanceIdentifier = %q, want %q", got, tc.want.statusID)
			}
		})
	}
}

func TestGeneratedInputsUseExternalName(t *testing.T) {
	cr := defaultsCR(defaultsName)

	mod, err := GenerateModifyDBInstanceInput(cr)
	if err != nil {
		t.Fatal(err)
	}
	del, err := GenerateDeleteDBInstanceInput(cr)
	if err != nil {
		t.Fatal(err)
	}

	if got := ptr.Deref(mod.DBInstanceIdentifier, "<nil>"); got != defaultsName {
		t.Errorf("Modify input DBInstanceIdentifier = %s, want mine", got)
	}
	if got := ptr.Deref(del.DBInstanceIdentifier, "<nil>"); got != defaultsName {
		t.Errorf("Delete input DBInstanceIdentifier = %s, want mine", got)
	}
}

func TestGeneratedCreateWritesStatusOnly(t *testing.T) {
	c := &fake.MockDocDBClient{
		MockCreateDBInstance: func(context.Context, *docdb.CreateDBInstanceInput, ...func(*docdb.Options)) (*docdb.CreateDBInstanceOutput, error) {
			// AWS omits the zone, CA and KMS key and lowercases the window.
			return &docdb.CreateDBInstanceOutput{DBInstance: &docdbtypes.DBInstance{
				DBInstanceIdentifier:       ptr.To(defaultsName),
				PreferredMaintenanceWindow: ptr.To("mon:03:00-mon:04:00"),
			}}, nil
		},
	}
	cr := defaultsCR(defaultsName)
	cr.Spec.ForProvider.PreferredMaintenanceWindow = ptr.To("Mon:03:00-Mon:04:00")
	cr.Spec.ForProvider.AvailabilityZone = ptr.To("eu-central-1a")
	cr.Spec.ForProvider.CACertificateIdentifier = ptr.To("rds-ca-rsa2048-g1")
	cr.Spec.ForProvider.PerformanceInsightsKMSKeyID = ptr.To("alias/pi")
	want := cr.Spec.DeepCopy()

	if _, err := newExternal(nil, c, nil).Create(context.Background(), cr); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if diff := cmp.Diff(want, &cr.Spec); diff != "" {
		t.Errorf("Create changed spec (-before +after):\n%s", diff)
	}
	if got := ptr.Deref(cr.Status.AtProvider.DBInstanceIdentifier, ""); got != defaultsName {
		t.Errorf("status.atProvider.dbInstanceIdentifier = %q, want mine", got)
	}
}

func TestGeneratedCreateSurvivesEmptyResponse(t *testing.T) {
	c := &fake.MockDocDBClient{
		MockCreateDBInstance: func(context.Context, *docdb.CreateDBInstanceInput, ...func(*docdb.Options)) (*docdb.CreateDBInstanceOutput, error) {
			return &docdb.CreateDBInstanceOutput{}, nil
		},
	}
	cr := defaultsCR(defaultsName)
	cr.Status.AtProvider.DBInstanceIdentifier = ptr.To(defaultsName)
	want := cr.Status.AtProvider.DeepCopy()

	if _, err := newExternal(nil, c, nil).Create(context.Background(), cr); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if diff := cmp.Diff(want, &cr.Status.AtProvider); diff != "" {
		t.Errorf("Create changed status (-before +after):\n%s", diff)
	}
}

func TestGeneratedObserveKeepsCustomObservation(t *testing.T) {
	c := &fake.MockDocDBClient{MockDescribeDBInstances: defaultsDescribe(defaultsName)}
	cr := defaultsCR(defaultsName)
	cr.Status.AtProvider.PreferredMaintenanceWindow = ptr.To("x")
	var seen *string
	recordWindow := func(e *external) {
		e.isUpToDate = func(_ context.Context, cr *svcapitypes.DBInstance, _ *docdb.DescribeDBInstancesOutput) (bool, string, error) {
			seen = cr.Status.AtProvider.PreferredMaintenanceWindow
			return true, "", nil
		}
	}

	if _, err := newExternal(nil, c, []option{recordWindow}).Observe(context.Background(), cr); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	if got := ptr.Deref(seen, "<nil>"); got != "x" {
		t.Errorf("isUpToDate saw status.atProvider.preferredMaintenanceWindow = %s, want x", got)
	}
}

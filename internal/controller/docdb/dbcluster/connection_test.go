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

package dbcluster

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/docdb"
	docdbtypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/aws/smithy-go"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	fake "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/docdb/fake"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
)

const (
	userKey = awsclient.ResourceCredentialsSecretUserKey
	portKey = awsclient.ResourceCredentialsSecretPortKey
)

func userAndPort(cd managed.ConnectionDetails) map[string]string {
	out := map[string]string{}
	for _, k := range []string{userKey, portKey} {
		if v, ok := cd[k]; ok {
			out[k] = string(v)
		}
	}
	return out
}

// A restore takes the username and port from the snapshot, so the spec
// usually has neither.
func TestConnectionDetailsFromObservedCluster(t *testing.T) {
	cases := map[string]struct {
		spec     []docDBModifier
		response docdbtypes.DBCluster
		want     map[string]string
	}{
		"FromCluster": {
			response: docdbtypes.DBCluster{MasterUsername: ptr.To("admin"), Port: ptr.To[int32](27017)},
			want:     map[string]string{userKey: "admin", portKey: "27017"},
		},
		"FromSpecWhenResponseLacksThem": {
			spec: []docDBModifier{withMasterUserName("spec-user"), withPort(27018)},
			want: map[string]string{userKey: "spec-user", portKey: "27018"},
		},
		"NoPortAtAll": {
			want: map[string]string{userKey: ""},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cluster := tc.response
			cluster.DBClusterIdentifier = ptr.To(testDBClusterIdentifier)
			cluster.Engine = ptr.To(svcapitypes.EngineDocDB)
			cluster.Status = ptr.To(svcapitypes.DocDBInstanceStateCreating)
			c := describeClusters(cluster)
			c.MockRestoreDBClusterFromSnapshot = func(context.Context, *docdb.RestoreDBClusterFromSnapshotInput, ...func(*docdb.Options)) (*docdb.RestoreDBClusterFromSnapshotOutput, error) {
				return &docdb.RestoreDBClusterFromSnapshotOutput{DBCluster: &cluster}, nil
			}
			ext := newExternal(nil, c, []option{setupExternal})
			cr := instance(append([]docDBModifier{withExternalName(testDBClusterIdentifier),
				withRestoreFromSnapshot(&svcapitypes.RestoreSnapshotConfiguration{SnapshotIdentifier: "snap"})}, tc.spec...)...)

			cre, err := wrap(ext).Create(context.Background(), cr)
			if err != nil {
				t.Fatalf("Create(...): %v", err)
			}
			if diff := cmp.Diff(tc.want, userAndPort(cre.ConnectionDetails)); diff != "" {
				t.Errorf("Create(...) connection details: -want, +got:\n%s", diff)
			}

			obs, err := ext.Observe(context.Background(), cr)
			if err != nil {
				t.Fatalf("Observe(...): %v", err)
			}
			if diff := cmp.Diff(tc.want, userAndPort(obs.ConnectionDetails)); diff != "" {
				t.Errorf("Observe(...) connection details: -want, +got:\n%s", diff)
			}
		})
	}
}

// Create must not need spec values that only the response knows.
func TestPostCreatePublishesObservedUserAndPort(t *testing.T) {
	c := &fake.MockDocDBClient{
		MockCreateDBCluster: func(context.Context, *docdb.CreateDBClusterInput, ...func(*docdb.Options)) (*docdb.CreateDBClusterOutput, error) {
			return &docdb.CreateDBClusterOutput{DBCluster: &docdbtypes.DBCluster{MasterUsername: ptr.To("admin"), Port: ptr.To[int32](27017)}}, nil
		},
	}
	cr := instance(withExternalName(testDBClusterIdentifier))

	cre, err := newExternal(nil, c, []option{setupExternal}).Create(context.Background(), cr)
	if err != nil {
		t.Fatalf("Create(...): %v", err)
	}

	want := map[string]string{userKey: "admin", portKey: "27017"}
	if diff := cmp.Diff(want, userAndPort(cre.ConnectionDetails)); diff != "" {
		t.Errorf("Create(...) connection details: -want, +got:\n%s", diff)
	}
}

// Create writes only status; a sparse response must not erase the spec.
func TestCreateKeepsSpecWhenResponseOmitsFields(t *testing.T) {
	c := &fake.MockDocDBClient{
		MockCreateDBCluster: func(context.Context, *docdb.CreateDBClusterInput, ...func(*docdb.Options)) (*docdb.CreateDBClusterOutput, error) {
			return &docdb.CreateDBClusterOutput{DBCluster: &docdbtypes.DBCluster{DBClusterArn: ptr.To("arn"), Engine: ptr.To("docdb")}}, nil
		},
	}
	cr := instance(withExternalName("c1"), withEngine("docdb"), withMasterUserName("admin"), withPort(27017))
	cr.Spec.ForProvider.StorageType = ptr.To("iopt1")
	cr.Spec.ForProvider.DeletionProtection = ptr.To(true)
	cr.Spec.ForProvider.ServerlessV2ScalingConfiguration = &svcapitypes.ServerlessV2ScalingConfiguration{MinCapacity: ptr.To(0.5), MaxCapacity: ptr.To(8.0)}
	want := cr.Spec.ForProvider.DeepCopy()

	if _, err := newExternal(nil, c, []option{setupExternal}).Create(context.Background(), cr); err != nil {
		t.Fatalf("Create(...): %v", err)
	}

	if diff := cmp.Diff(want, &cr.Spec.ForProvider); diff != "" {
		t.Errorf("Create(...) changed the spec: -want, +got:\n%s", diff)
	}
}

// AWS returns windows and group names in lowercase.
func TestObserveIgnoresCaseOfWindowAndParameterGroup(t *testing.T) {
	cases := map[string]struct {
		window, group string
		wantDiff      []string
	}{
		"SameIgnoringCase": {window: "sat:02:00-sat:03:00", group: "myparams"},
		"OtherWindow":      {window: "sun:02:00-sun:03:00", group: "myparams", wantDiff: []string{"preferredMaintenanceWindow"}},
		"OtherGroup":       {window: "sat:02:00-sat:03:00", group: "otherparams", wantDiff: []string{"dbClusterParameterGroupName"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := describeClusters(docdbtypes.DBCluster{
				DBClusterIdentifier:        ptr.To(testDBClusterIdentifier),
				Engine:                     ptr.To(svcapitypes.EngineDocDB),
				Status:                     ptr.To(svcapitypes.DocDBInstanceStateAvailable),
				PreferredMaintenanceWindow: ptr.To(tc.window),
				DBClusterParameterGroup:    ptr.To(tc.group),
			})
			cr := instance(withExternalName(testDBClusterIdentifier), withPreferredMaintenanceWindow("Sat:02:00-Sat:03:00"), withDBClusterParameterGroupName("MyParams"))

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

// The second Modify must not resend a group whose name differs only in case.
func TestUpdateSkipsParameterGroupDifferingInCase(t *testing.T) {
	c := updateClient()
	describe := c.MockDescribeDBClusters
	c.MockDescribeDBClusters = func(ctx context.Context, in *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
		out, err := describe(ctx, in, o...)
		out.DBClusters[0].DBClusterParameterGroup = ptr.To("myparams")
		return out, err
	}
	cr := instance(withExternalName(testDBClusterIdentifier), withDBClusterParameterGroupName("MyParams"))

	if _, err := newExternal(nil, c, []option{setupExternal}).Update(context.Background(), cr); err != nil {
		t.Fatalf("Update(...): %v", err)
	}
	if n := len(c.Called.ModifyDBCluster); n != 1 {
		t.Errorf("ModifyDBCluster calls = %d, want 1", n)
	}
}

func TestEngineUpgradeErrorIsWrapped(t *testing.T) {
	c := updateClient()
	modify := c.MockModifyDBCluster
	c.MockModifyDBCluster = func(ctx context.Context, in *docdb.ModifyDBClusterInput, o ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error) {
		if in.EngineVersion != nil {
			return nil, &smithy.GenericAPIError{Code: "InvalidDBClusterStateFault", Message: "busy"}
		}
		return modify(ctx, in, o...)
	}
	cr := instance(withExternalName(testDBClusterIdentifier), withEngineVersion("5.0.0"))

	_, err := newExternal(nil, c, []option{setupExternal}).Update(context.Background(), cr)

	if want := "cannot update DBCluster in AWS: InvalidDBClusterStateFault: busy"; err == nil || err.Error() != want {
		t.Errorf("Update(...) error = %v, want %q", err, want)
	}
}

/*
Copyright 2021 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS_IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dbcluster

import (
	"context"
	"go/token"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/docdb"
	docdbtypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/teeverr/provider-aws-v2/apis/docdb/v1alpha1"
	awsclient "github.com/teeverr/provider-aws-v2/internal/clients/aws"
	fake "github.com/teeverr/provider-aws-v2/internal/clients/docdb/fake"
	"github.com/teeverr/provider-aws-v2/internal/utils/compare"
	pointer "github.com/teeverr/provider-aws-v2/internal/utils/pointer"
)

var (
	testAvailabilityZone                 = "test-zone-a"
	testOtherAvailabilityZone            = "test-zone-b"
	testBackupRetentionPeriod            = 10
	testOtherBackupRetentionPeriod       = 1000
	testDBClusterIdentifier              = "some-db-cluster"
	testOtherDBClusterIdentifier         = "some-other-db-cluster"
	testDBClusterArn                     = "some-db-cluster-arn"
	testDBClusterParameterGroupName      = "some-db-cluster-parameter-group"
	testOtherDBClusterParameterGroupName = "some-other-db-cluster-parameter-group"
	testDBSubnetGroupName                = "some-db-subnet-group"
	testCloudWatchLog                    = "some-log"
	testOtherCloudWatchLog               = "some-other-log"
	testEngine                           = "some-engine"
	testEngineVersion                    = "4.0.0"
	testOtherEngineVersion               = "5.0.0"
	testKMSKeyID                         = "some-key"
	testMasterUserName                   = "some-user"
	testMasterUserPassword               = "some-pw"
	testEndpoint                         = "some-endpoint"
	testReaderEndpoint                   = "some-reader-endpoint"
	testPort                             = 3210
	testOtherPort                        = 5432
	testPresignedURL                     = "some-url"
	testPreferredBackupWindow            = "some-window"
	testOtherPreferredBackupWindow       = "some-other-window"
	testPreferredMaintenanceWindow       = "some-window"
	testOtherPreferredMaintenanceWindow  = "some-other-window"
	testTagKey                           = "some-tag-key"
	testTagValue                         = "some-tag-value"
	testOtherTagKey                      = "some-other-tag-key"
	testOtherTagValue                    = "some-other-tag-value"
	testOtherOtherTagKey                 = "some-other-other-tag-key"
	testOtherOtherTagValue               = "some-other-other-tag-value"
	testVpcSecurityGroup                 = "some-group"
	testOtherVpcSecurityGroup            = "some-other-group"
	testFinalDBSnapshotIdentifier        = "some-snapshot"
	testMasterPasswordSecretNamespace    = "some-namespace"
	testMasterPasswordSecretName         = "some-name"
	testMasterPasswordSecretKey          = "some-key"

	testErrDescribeDBClustersFailed = "DescribeDBClusters failed"
	testErrCreateDBClusterFailed    = "CreateDBCluster failed"
	testErrDeleteDBClusterFailed    = "DeleteDBCluster failed"
	testErrModifyDBClusterFailed    = "ModifyDBCluster failed"
	testErrGetSecret                = "testErrGetSecret"

	timeNow = time.Now()
)

type docDBModifier func(*svcapitypes.DBCluster)

func toStringArray(values ...string) []string {
	return append([]string{}, values...)
}

func toStringPtrArray(values ...string) []*string {
	ptrArr := make([]*string, len(values))
	for i, s := range values {
		ptrArr[i] = pointer.ToOrNilIfZeroValue(s)
	}
	return ptrArr
}

func instance(m ...docDBModifier) *svcapitypes.DBCluster {
	cr := &svcapitypes.DBCluster{}
	for _, f := range m {
		f(cr)
	}
	return cr
}

func withExternalName(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		meta.SetExternalName(o, value)

	}
}

func withDeletionTimestamp(v *metav1.Time) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.SetDeletionTimestamp(v)
	}
}

func withDBClusterIdentifier(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Status.AtProvider.DBClusterIdentifier = pointer.ToOrNilIfZeroValue(value)
	}
}

func withConditions(value ...xpv2.Condition) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Status.SetConditions(value...)
	}
}

func withStatus(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Status.AtProvider.Status = pointer.ToOrNilIfZeroValue(value)
	}
}

func withAvailabilityZones(values ...string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.AvailabilityZones = toStringPtrArray(values...)
	}
}

func withBackupRetentionPeriod(value int) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.BackupRetentionPeriod = pointer.ToIntAsInt64(value)
	}
}

func withDBClusterParameterGroupName(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.DBClusterParameterGroupName = pointer.ToOrNilIfZeroValue(value)
	}
}

func withStatusDBClusterParameterGroupName(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Status.AtProvider.DBClusterParameterGroup = pointer.ToOrNilIfZeroValue(value)
	}
}

func withStatusDBClusterArn(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Status.AtProvider.DBClusterARN = pointer.ToOrNilIfZeroValue(value)
	}
}

func withDBSubnetGroup(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.DBSubnetGroupName = pointer.ToOrNilIfZeroValue(value)
	}
}

func withDeletionProtection(value bool) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.DeletionProtection = ptr.To(value)
	}
}

func withEnableCloudWatchLogsExports(values ...string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.EnableCloudwatchLogsExports = toStringPtrArray(values...)
	}
}

func withStatusEnableCloudWatchLogsExports(values ...string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Status.AtProvider.EnabledCloudwatchLogsExports = toStringPtrArray(values...)
	}
}

func withEngine(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.Engine = pointer.ToOrNilIfZeroValue(value)
	}
}

func withEngineVersion(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.EngineVersion = pointer.ToOrNilIfZeroValue(value)
	}
}

func withStatusEngineVersion(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Status.AtProvider.EngineVersion = pointer.ToOrNilIfZeroValue(value)

	}
}

func withAllowMajorVersionUpgrade(value bool) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.AllowMajorVersionUpgrade = pointer.ToOrNilIfZeroValue(value)
	}
}

func withMasterUserName(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.MasterUsername = pointer.ToOrNilIfZeroValue(value)
	}
}

func withKmsKeyID(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.KMSKeyID = pointer.ToOrNilIfZeroValue(value)
	}
}

func withPort(value int) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.Port = pointer.ToIntAsInt64(value)
	}
}

func withEndpoint(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Status.AtProvider.Endpoint = pointer.ToOrNilIfZeroValue(value)
	}
}

func withReaderEndpoint(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Status.AtProvider.ReaderEndpoint = pointer.ToOrNilIfZeroValue(value)
	}
}

func withPreSignedURL(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.PreSignedURL = pointer.ToOrNilIfZeroValue(value)
	}
}

func withPreferredBackupWindow(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.PreferredBackupWindow = pointer.ToOrNilIfZeroValue(value)
	}
}

func withPreferredMaintenanceWindow(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.PreferredMaintenanceWindow = pointer.ToOrNilIfZeroValue(value)
	}
}

func withStorageEncrypted(value bool) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.StorageEncrypted = pointer.ToOrNilIfZeroValue(value)
	}
}

func withVpcSecurityGroupIds(values ...string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.VPCSecurityGroupIDs = toStringPtrArray(values...)
	}
}

func withTags(values ...*svcapitypes.Tag) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		if values != nil {
			o.Spec.ForProvider.Tags = values
		} else {
			o.Spec.ForProvider.Tags = []*svcapitypes.Tag{}
		}
	}
}

func withSkipFinalSnapshot(value bool) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.SkipFinalSnapshot = pointer.ToOrNilIfZeroValue(value)
	}
}

func withFinalDBSnapshotIdentifier(value string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.FinalDBSnapshotIdentifier = pointer.ToOrNilIfZeroValue(value)
	}
}

func withRestoreFromSnapshot(v *svcapitypes.RestoreSnapshotConfiguration) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.RestoreFrom = &svcapitypes.RestoreDBClusterBackupConfiguration{
			Snapshot: v,
			Source:   svcapitypes.RestoreSourceSnapshot,
		}
	}
}

func withRestoreToPointInTime(v *svcapitypes.RestorePointInTimeConfiguration) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.Spec.ForProvider.RestoreFrom = &svcapitypes.RestoreDBClusterBackupConfiguration{
			PointInTime: v,
			Source:      svcapitypes.RestoreSourcePointInTime,
		}
	}
}

func generateConnectionDetails(username, password, readerEndpoint, endpoint string, port int) managed.ConnectionDetails {

	mcd := managed.ConnectionDetails{
		awsclient.ResourceCredentialsSecretEndpointKey: []byte(endpoint),
		awsclient.ResourceCredentialsSecretPortKey:     []byte(strconv.Itoa(port)),
		awsclient.ResourceCredentialsSecretUserKey:     []byte(username),
		"readerEndpoint": []byte(readerEndpoint),
	}
	if password != "" {
		mcd[awsclient.ResourceCredentialsSecretPasswordKey] = []byte(password)
	}
	return mcd
}

// withMasterPasswordSecretRef sets the namespace of the DBCluster, which is
// also the namespace of the referenced secret.
func withMasterPasswordSecretRef(namespace, name, key string) docDBModifier {
	return func(o *svcapitypes.DBCluster) {
		o.SetNamespace(namespace)
		o.Spec.ForProvider.MasterUserPasswordSecretRef = &xpv2.LocalSecretKeySelector{
			LocalSecretReference: xpv2.LocalSecretReference{
				Name: name,
			},
			Key: key,
		}
	}
}

func TestObserve(t *testing.T) {
	type want struct {
		cr     *svcapitypes.DBCluster
		result managed.ExternalObservation
		err    error
		docdb  fake.MockDocDBClientCall
	}

	cases := map[string]struct {
		args
		want
	}{
		"AvailableState_and_changed_BackupRetentionPeriod_should_not_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier:   pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:                pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									BackupRetentionPeriod: pointer.ToIntAsInt32(testBackupRetentionPeriod),
								},
							},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withBackupRetentionPeriod(testOtherBackupRetentionPeriod),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withBackupRetentionPeriod(testOtherBackupRetentionPeriod),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        false,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
					Diff:                    "spec.forProvider fields differ from the observed state: backupRetentionPeriod",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
				},
			},
		},
		"AvailableState_and_same_BackupRetentionPeriod_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier:   pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:                pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									BackupRetentionPeriod: pointer.ToIntAsInt32(testBackupRetentionPeriod),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withBackupRetentionPeriod(testBackupRetentionPeriod),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withBackupRetentionPeriod(testBackupRetentionPeriod),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_no_configured_BackupRetentionPeriod_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier:   pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:                pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									BackupRetentionPeriod: pointer.ToIntAsInt32(testBackupRetentionPeriod),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withBackupRetentionPeriod(testBackupRetentionPeriod),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_same_DBClusterIdentifier_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:              pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_changed_DBClusterParameterGroupName_should_not_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier:     pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:                  pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									DBClusterParameterGroup: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
								},
							},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withDBClusterParameterGroupName(testOtherDBClusterParameterGroupName),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withDBClusterParameterGroupName(testOtherDBClusterParameterGroupName),
					withStatusDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        false,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
					Diff:                    "spec.forProvider fields differ from the observed state: dbClusterParameterGroupName",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
				},
			},
		},
		"AvailableState_and_same_DBClusterParameterGroupName_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier:     pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:                  pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									DBClusterParameterGroup: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withStatusDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_no_spec_DBClusterParameterGroupName_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier:     pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:                  pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									DBClusterParameterGroup: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withStatusDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_changed_DeletionProtection_should_not_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:              pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									DeletionProtection:  pointer.ToOrNilIfZeroValue(true),
								},
							},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withDeletionProtection(false),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withDeletionProtection(false),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        false,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
					Diff:                    "spec.forProvider fields differ from the observed state: deletionProtection",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
				},
			},
		},
		"AvailableState_and_same_DeletionProtection_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:              pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									DeletionProtection:  pointer.ToOrNilIfZeroValue(true),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withDeletionProtection(true),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withDeletionProtection(true),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_no_spec_DeletionProtection_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:              pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									DeletionProtection:  pointer.ToOrNilIfZeroValue(true),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withDeletionProtection(true),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_added_EnableCloudwatchLogsExports_should_not_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:              pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									EnabledCloudwatchLogsExports: []string{
										testCloudWatchLog,
									},
								},
							},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
					withStatusEnableCloudWatchLogsExports(
						testCloudWatchLog,
					),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        false,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
					Diff:                    "spec.forProvider fields differ from the observed state: enableCloudwatchLogsExports",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
				},
			},
		},
		"AvailableState_and_removed_EnableCloudwatchLogsExports_should_not_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:              pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									EnabledCloudwatchLogsExports: []string{
										testCloudWatchLog,
										testOtherCloudWatchLog,
									},
								},
							},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withEnableCloudWatchLogsExports(
						testCloudWatchLog,
					),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withEnableCloudWatchLogsExports(
						testCloudWatchLog,
					),
					withStatusEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        false,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
					Diff:                    "spec.forProvider fields differ from the observed state: enableCloudwatchLogsExports",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
				},
			},
		},
		"AvailableState_and_same_order_EnableCloudwatchLogsExports_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:              pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									EnabledCloudwatchLogsExports: []string{
										testCloudWatchLog,
										testOtherCloudWatchLog,
									},
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
					withStatusEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_different_order_EnableCloudwatchLogsExports_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:              pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									EnabledCloudwatchLogsExports: []string{
										testCloudWatchLog,
										testOtherCloudWatchLog,
									},
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withEnableCloudWatchLogsExports(
						testOtherCloudWatchLog,
						testCloudWatchLog,
					),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withEnableCloudWatchLogsExports(
						testOtherCloudWatchLog,
						testCloudWatchLog,
					),
					withStatusEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_no_spec_EnableCloudwatchLogsExports_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:              pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									EnabledCloudwatchLogsExports: []string{
										testCloudWatchLog,
										testOtherCloudWatchLog,
									},
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
					withStatusEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_changed_EngineVersion_should_not_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:              pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									EngineVersion:       pointer.ToOrNilIfZeroValue(testEngineVersion),
								},
							},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withEngineVersion(testOtherEngineVersion),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withEngineVersion(testOtherEngineVersion),
					withStatusEngineVersion(testEngineVersion),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        false,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
					Diff:                    "spec.forProvider fields differ from the observed state: engineVersion",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
				},
			},
		},
		"AvailableState_and_same_EngineVersion_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:              pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									EngineVersion:       pointer.ToOrNilIfZeroValue(testEngineVersion),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withEngineVersion(testEngineVersion),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withEngineVersion(testEngineVersion),
					withStatusEngineVersion(testEngineVersion),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_changed_Port_should_not_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:              pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									Port:                pointer.ToIntAsInt32(testPort),
								},
							},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withPort(testOtherPort),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withPort(testOtherPort),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        false,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", testOtherPort),
					Diff:                    "spec.forProvider fields differ from the observed state: port",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
				},
			},
		},
		"AvailableState_and_same_Port_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:              pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									Port:                pointer.ToIntAsInt32(testPort),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withPort(testPort),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withPort(testPort),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", testPort),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_no_spec_Port_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:              pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									Port:                pointer.ToIntAsInt32(testPort),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withPort(testPort),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", testPort),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_changed_PreferredBackupWindow_should_not_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier:   pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:                pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									PreferredBackupWindow: pointer.ToOrNilIfZeroValue(testPreferredBackupWindow),
								},
							},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withPreferredBackupWindow(testOtherPreferredBackupWindow),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withPreferredBackupWindow(testOtherPreferredBackupWindow),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        false,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
					Diff:                    "spec.forProvider fields differ from the observed state: preferredBackupWindow",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
				},
			},
		},
		"AvailableState_and_same_PreferredBackupWindow_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier:   pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:                pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									PreferredBackupWindow: pointer.ToOrNilIfZeroValue(testPreferredBackupWindow),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withPreferredBackupWindow(testPreferredBackupWindow),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withPreferredBackupWindow(testPreferredBackupWindow),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_no_spec_PreferredBackupWindow_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier:   pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:                pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									PreferredBackupWindow: pointer.ToOrNilIfZeroValue(testPreferredBackupWindow),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withPreferredBackupWindow(testPreferredBackupWindow),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_changed_PreferredMaintenanceWindow_should_not_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier:        pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:                     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									PreferredMaintenanceWindow: pointer.ToOrNilIfZeroValue(testPreferredMaintenanceWindow),
								},
							},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withPreferredMaintenanceWindow(testOtherPreferredMaintenanceWindow),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withPreferredMaintenanceWindow(testOtherPreferredMaintenanceWindow),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        false,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
					Diff:                    "spec.forProvider fields differ from the observed state: preferredMaintenanceWindow",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
				},
			},
		},
		"AvailableState_and_same_PreferredMaintenanceWindow_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier:        pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:                     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									PreferredMaintenanceWindow: pointer.ToOrNilIfZeroValue(testPreferredMaintenanceWindow),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withPreferredMaintenanceWindow(testPreferredBackupWindow),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withPreferredMaintenanceWindow(testPreferredBackupWindow),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_no_spec_PreferredMaintenanceWindow_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier:        pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:                     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									PreferredMaintenanceWindow: pointer.ToOrNilIfZeroValue(testPreferredMaintenanceWindow),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
					ConnectionDetails:       generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_Deleted_should_be_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier:        pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									Status:                     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
									PreferredMaintenanceWindow: pointer.ToOrNilIfZeroValue(testPreferredMaintenanceWindow),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withDeletionTimestamp(&metav1.Time{Time: timeNow}),
					withExternalName(testDBClusterIdentifier),
					withPreferredMaintenanceWindow(testOtherPreferredMaintenanceWindow),
					withVpcSecurityGroupIds(),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withDeletionTimestamp(&metav1.Time{Time: timeNow}),
					withPreferredMaintenanceWindow(testOtherPreferredMaintenanceWindow),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withVpcSecurityGroupIds(),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: generateConnectionDetails("", "", "", "", 0),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
				},
			},
		},
		"Empty_DescribeClustersOutput": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
				result: managed.ExternalObservation{
					ResourceExists:   false,
					ResourceUpToDate: false,
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
				},
			},
		},
		"ErrDescribeDBClustersWithContext": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusters: func(c context.Context, ddpgi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{}, errors.New(testErrDescribeDBClustersFailed)
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
				result: managed.ExternalObservation{
					ResourceExists:   false,
					ResourceUpToDate: false,
				},
				err: errors.Wrap(errors.New(testErrDescribeDBClustersFailed), errDescribe),
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			opts := []option{setupExternal}
			e := newExternal(tc.args.kube, tc.args.docdb, opts)
			o, err := e.Observe(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, o); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.docdb, tc.args.docdb.Called, cmpopts.IgnoreInterfaces(struct{ context.Context }{}), ignoreUnexported); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type want struct {
		cr     *svcapitypes.DBCluster
		result managed.ExternalCreation
		err    error
		docdb  fake.MockDocDBClientCall
	}

	cases := map[string]struct {
		args
		want
	}{
		"SuccessfulCreate": {
			args: args{
				kube: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
						s := obj.(*v1.Secret)
						s.Data = map[string][]byte{
							testMasterPasswordSecretKey: []byte(testMasterUserPassword),
						}
						return nil
					},
				},
				docdb: &fake.MockDocDBClient{
					MockCreateDBCluster: func(c context.Context, cdpgi *docdb.CreateDBClusterInput, o ...func(*docdb.Options)) (*docdb.CreateDBClusterOutput, error) {
						return &docdb.CreateDBClusterOutput{
							DBCluster: &docdbtypes.DBCluster{
								AvailabilityZones: []string{
									testAvailabilityZone,
									testOtherAvailabilityZone,
								},
								BackupRetentionPeriod:      pointer.ToIntAsInt32(testBackupRetentionPeriod),
								DBClusterParameterGroup:    &testDBClusterParameterGroupName,
								DBClusterIdentifier:        pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								DBClusterArn:               pointer.ToOrNilIfZeroValue(testDBClusterArn),
								DeletionProtection:         pointer.ToOrNilIfZeroValue(true),
								Endpoint:                   pointer.ToOrNilIfZeroValue(testEndpoint),
								Engine:                     &testEngine,
								EngineVersion:              &testEngineVersion,
								KmsKeyId:                   &testKMSKeyID,
								MasterUsername:             &testMasterUserName,
								ReaderEndpoint:             pointer.ToOrNilIfZeroValue(testReaderEndpoint),
								Port:                       pointer.ToIntAsInt32(testPort),
								PreferredBackupWindow:      &testPreferredBackupWindow,
								PreferredMaintenanceWindow: &testPreferredMaintenanceWindow,
								StorageEncrypted:           pointer.ToOrNilIfZeroValue(true),
							},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withAvailabilityZones(
						testAvailabilityZone,
						testOtherAvailabilityZone,
					),
					withBackupRetentionPeriod(testBackupRetentionPeriod),
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withDBSubnetGroup(testDBSubnetGroupName),
					withDeletionProtection(true),
					withEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
					withEngine(testEngine),
					withEngineVersion(testEngineVersion),
					withKmsKeyID(testKMSKeyID),
					withMasterUserName(testMasterUserName),
					withPort(testPort),
					withPreSignedURL(testPresignedURL),
					withPreferredBackupWindow(testPreferredBackupWindow),
					withPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
					withStorageEncrypted(true),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
					withVpcSecurityGroupIds(
						testVpcSecurityGroup,
						testOtherVpcSecurityGroup,
					),
					withMasterPasswordSecretRef(testMasterPasswordSecretNamespace, testMasterPasswordSecretName, testMasterPasswordSecretKey),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Creating()),
					withAvailabilityZones(
						testAvailabilityZone,
						testOtherAvailabilityZone,
					),
					withBackupRetentionPeriod(testBackupRetentionPeriod),
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withDBSubnetGroup(testDBSubnetGroupName),
					withStatusDBClusterArn(testDBClusterArn),
					withDeletionProtection(true),
					withEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
					withEngine(testEngine),
					withEngineVersion(testEngineVersion),
					withKmsKeyID(testKMSKeyID),
					withMasterUserName(testMasterUserName),
					withPort(testPort),
					withPreSignedURL(testPresignedURL),
					withPreferredBackupWindow(testPreferredBackupWindow),
					withPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
					withStorageEncrypted(true),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
					withVpcSecurityGroupIds(
						testVpcSecurityGroup,
						testOtherVpcSecurityGroup,
					),
					withEndpoint(testEndpoint),
					withReaderEndpoint(testReaderEndpoint),
					withMasterPasswordSecretRef(testMasterPasswordSecretNamespace, testMasterPasswordSecretName, testMasterPasswordSecretKey),
					withStatusDBClusterParameterGroupName(testDBClusterParameterGroupName),
				),
				result: managed.ExternalCreation{
					ConnectionDetails: generateConnectionDetails(
						testMasterUserName,
						testMasterUserPassword,
						testReaderEndpoint,
						testEndpoint,
						testPort,
					),
				},
				docdb: fake.MockDocDBClientCall{
					CreateDBCluster: []*fake.CallCreateDBCluster{
						{
							Ctx: context.Background(),
							I: &docdb.CreateDBClusterInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								AvailabilityZones: toStringArray(
									testAvailabilityZone,
									testOtherAvailabilityZone,
								),
								BackupRetentionPeriod:       pointer.ToIntAsInt32(testBackupRetentionPeriod),
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
								DBSubnetGroupName:           pointer.ToOrNilIfZeroValue(testDBSubnetGroupName),
								DeletionProtection:          pointer.ToOrNilIfZeroValue(true),
								EnableCloudwatchLogsExports: toStringArray(
									testCloudWatchLog,
									testOtherCloudWatchLog,
								),
								Engine:                     pointer.ToOrNilIfZeroValue(testEngine),
								EngineVersion:              pointer.ToOrNilIfZeroValue(testEngineVersion),
								KmsKeyId:                   pointer.ToOrNilIfZeroValue(testKMSKeyID),
								MasterUsername:             pointer.ToOrNilIfZeroValue(testMasterUserName),
								MasterUserPassword:         pointer.ToOrNilIfZeroValue(testMasterUserPassword),
								Port:                       pointer.ToIntAsInt32(testPort),
								PreSignedUrl:               pointer.ToOrNilIfZeroValue(testPresignedURL),
								PreferredBackupWindow:      pointer.ToOrNilIfZeroValue(testPreferredBackupWindow),
								PreferredMaintenanceWindow: pointer.ToOrNilIfZeroValue(testPreferredMaintenanceWindow),
								StorageEncrypted:           pointer.ToOrNilIfZeroValue(true),
								VpcSecurityGroupIds: toStringArray(
									testVpcSecurityGroup,
									testOtherVpcSecurityGroup,
								),
								Tags: []docdbtypes.Tag{
									{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
									{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
								},
							},
						},
					},
				},
			},
		},
		"SuccessfulRestoreFromSnapshot": {
			args: args{
				kube: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
						s := obj.(*v1.Secret)
						s.Data = map[string][]byte{
							testMasterPasswordSecretKey: []byte(testMasterUserPassword),
						}
						return nil
					},
				},
				docdb: &fake.MockDocDBClient{
					MockRestoreDBClusterFromSnapshot: func(c context.Context, cdpgi *docdb.RestoreDBClusterFromSnapshotInput, o ...func(*docdb.Options)) (*docdb.RestoreDBClusterFromSnapshotOutput, error) {
						return &docdb.RestoreDBClusterFromSnapshotOutput{
							DBCluster: &docdbtypes.DBCluster{
								AvailabilityZones: []string{
									testAvailabilityZone,
									testOtherAvailabilityZone,
								},
								BackupRetentionPeriod:      pointer.ToIntAsInt32(testBackupRetentionPeriod),
								DBClusterParameterGroup:    &testDBClusterParameterGroupName,
								DBClusterIdentifier:        pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								DBClusterArn:               pointer.ToOrNilIfZeroValue(testDBClusterArn),
								DeletionProtection:         pointer.ToOrNilIfZeroValue(true),
								Endpoint:                   pointer.ToOrNilIfZeroValue(testEndpoint),
								Engine:                     &testEngine,
								EngineVersion:              &testEngineVersion,
								KmsKeyId:                   &testKMSKeyID,
								MasterUsername:             &testMasterUserName,
								ReaderEndpoint:             pointer.ToOrNilIfZeroValue(testReaderEndpoint),
								Port:                       pointer.ToIntAsInt32(testPort),
								PreferredBackupWindow:      &testPreferredBackupWindow,
								PreferredMaintenanceWindow: &testPreferredMaintenanceWindow,
								StorageEncrypted:           pointer.ToOrNilIfZeroValue(true),
							},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withAvailabilityZones(
						testAvailabilityZone,
						testOtherAvailabilityZone,
					),
					withBackupRetentionPeriod(testBackupRetentionPeriod),
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withDBSubnetGroup(testDBSubnetGroupName),
					withDeletionProtection(true),
					withEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
					withEngine(testEngine),
					withEngineVersion(testEngineVersion),
					withKmsKeyID(testKMSKeyID),
					withMasterUserName(testMasterUserName),
					withPort(testPort),
					withPreSignedURL(testPresignedURL),
					withPreferredBackupWindow(testPreferredBackupWindow),
					withPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
					withStorageEncrypted(true),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
					withVpcSecurityGroupIds(
						testVpcSecurityGroup,
						testOtherVpcSecurityGroup,
					),
					withMasterPasswordSecretRef(testMasterPasswordSecretNamespace, testMasterPasswordSecretName, testMasterPasswordSecretKey),
					withRestoreFromSnapshot(&svcapitypes.RestoreSnapshotConfiguration{
						SnapshotIdentifier: "abcd",
					}),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Creating()),
					withAvailabilityZones(
						testAvailabilityZone,
						testOtherAvailabilityZone,
					),
					withBackupRetentionPeriod(testBackupRetentionPeriod),
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withDBSubnetGroup(testDBSubnetGroupName),
					withStatusDBClusterArn(testDBClusterArn),
					withDeletionProtection(true),
					withEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
					withEngine(testEngine),
					withEngineVersion(testEngineVersion),
					withKmsKeyID(testKMSKeyID),
					withMasterUserName(testMasterUserName),
					withPort(testPort),
					withPreSignedURL(testPresignedURL),
					withPreferredBackupWindow(testPreferredBackupWindow),
					withPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
					withStorageEncrypted(true),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
					withVpcSecurityGroupIds(
						testVpcSecurityGroup,
						testOtherVpcSecurityGroup,
					),
					withEndpoint(testEndpoint),
					withReaderEndpoint(testReaderEndpoint),
					withMasterPasswordSecretRef(testMasterPasswordSecretNamespace, testMasterPasswordSecretName, testMasterPasswordSecretKey),
					withStatusDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withRestoreFromSnapshot(&svcapitypes.RestoreSnapshotConfiguration{
						SnapshotIdentifier: "abcd",
					}),
				),
				result: managed.ExternalCreation{
					ConnectionDetails: generateConnectionDetails(
						testMasterUserName,
						"", // set by ModifyDBCluster after the restore
						testReaderEndpoint,
						testEndpoint,
						testPort,
					),
				},
				docdb: fake.MockDocDBClientCall{
					RestoreDBClusterFromSnapshot: []*fake.CallRestoreDBClusterFromSnapshot{
						{
							Ctx: context.Background(),
							I: &docdb.RestoreDBClusterFromSnapshotInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								AvailabilityZones: toStringArray(
									testAvailabilityZone,
									testOtherAvailabilityZone,
								),
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
								DBSubnetGroupName:           pointer.ToOrNilIfZeroValue(testDBSubnetGroupName),
								DeletionProtection:          pointer.ToOrNilIfZeroValue(true),
								EnableCloudwatchLogsExports: toStringArray(
									testCloudWatchLog,
									testOtherCloudWatchLog,
								),
								Engine:        pointer.ToOrNilIfZeroValue(testEngine),
								EngineVersion: pointer.ToOrNilIfZeroValue(testEngineVersion),
								KmsKeyId:      pointer.ToOrNilIfZeroValue(testKMSKeyID),
								Port:          pointer.ToIntAsInt32(testPort),
								VpcSecurityGroupIds: toStringArray(
									testVpcSecurityGroup,
									testOtherVpcSecurityGroup,
								),
								Tags: []docdbtypes.Tag{
									{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
									{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
								},
								SnapshotIdentifier: ptr.To("abcd"),
							},
						},
					},
				},
			},
		},
		"SuccessfulRestoreFromPointInTime": {
			args: args{
				kube: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
						s := obj.(*v1.Secret)
						s.Data = map[string][]byte{
							testMasterPasswordSecretKey: []byte(testMasterUserPassword),
						}
						return nil
					},
				},
				docdb: &fake.MockDocDBClient{
					MockRestoreDBClusterToPointInTime: func(c context.Context, cdpgi *docdb.RestoreDBClusterToPointInTimeInput, o ...func(*docdb.Options)) (*docdb.RestoreDBClusterToPointInTimeOutput, error) {
						return &docdb.RestoreDBClusterToPointInTimeOutput{
							DBCluster: &docdbtypes.DBCluster{
								AvailabilityZones: []string{
									testAvailabilityZone,
									testOtherAvailabilityZone,
								},
								BackupRetentionPeriod:      pointer.ToIntAsInt32(testBackupRetentionPeriod),
								DBClusterParameterGroup:    &testDBClusterParameterGroupName,
								DBClusterIdentifier:        pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								DBClusterArn:               pointer.ToOrNilIfZeroValue(testDBClusterArn),
								DeletionProtection:         pointer.ToOrNilIfZeroValue(true),
								Endpoint:                   pointer.ToOrNilIfZeroValue(testEndpoint),
								Engine:                     &testEngine,
								EngineVersion:              &testEngineVersion,
								KmsKeyId:                   &testKMSKeyID,
								MasterUsername:             &testMasterUserName,
								ReaderEndpoint:             pointer.ToOrNilIfZeroValue(testReaderEndpoint),
								Port:                       pointer.ToIntAsInt32(testPort),
								PreferredBackupWindow:      &testPreferredBackupWindow,
								PreferredMaintenanceWindow: &testPreferredMaintenanceWindow,
								StorageEncrypted:           pointer.ToOrNilIfZeroValue(true),
							},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withAvailabilityZones(
						testAvailabilityZone,
						testOtherAvailabilityZone,
					),
					withBackupRetentionPeriod(testBackupRetentionPeriod),
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withDBSubnetGroup(testDBSubnetGroupName),
					withDeletionProtection(true),
					withEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
					withEngine(testEngine),
					withEngineVersion(testEngineVersion),
					withKmsKeyID(testKMSKeyID),
					withMasterUserName(testMasterUserName),
					withPort(testPort),
					withPreSignedURL(testPresignedURL),
					withPreferredBackupWindow(testPreferredBackupWindow),
					withPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
					withStorageEncrypted(true),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
					withVpcSecurityGroupIds(
						testVpcSecurityGroup,
						testOtherVpcSecurityGroup,
					),
					withMasterPasswordSecretRef(testMasterPasswordSecretNamespace, testMasterPasswordSecretName, testMasterPasswordSecretKey),
					withRestoreToPointInTime(&svcapitypes.RestorePointInTimeConfiguration{
						RestoreTime:               &metav1.Time{Time: timeNow},
						UseLatestRestorableTime:   ptr.To(true),
						SourceDBClusterIdentifier: "abcd",
						RestoreType:               ptr.To("full-copy"),
					}),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Creating()),
					withAvailabilityZones(
						testAvailabilityZone,
						testOtherAvailabilityZone,
					),
					withBackupRetentionPeriod(testBackupRetentionPeriod),
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withDBSubnetGroup(testDBSubnetGroupName),
					withStatusDBClusterArn(testDBClusterArn),
					withDeletionProtection(true),
					withEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
					withEngine(testEngine),
					withEngineVersion(testEngineVersion),
					withKmsKeyID(testKMSKeyID),
					withMasterUserName(testMasterUserName),
					withPort(testPort),
					withPreSignedURL(testPresignedURL),
					withPreferredBackupWindow(testPreferredBackupWindow),
					withPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
					withStorageEncrypted(true),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
					withVpcSecurityGroupIds(
						testVpcSecurityGroup,
						testOtherVpcSecurityGroup,
					),
					withEndpoint(testEndpoint),
					withReaderEndpoint(testReaderEndpoint),
					withMasterPasswordSecretRef(testMasterPasswordSecretNamespace, testMasterPasswordSecretName, testMasterPasswordSecretKey),
					withStatusDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withRestoreToPointInTime(&svcapitypes.RestorePointInTimeConfiguration{
						RestoreTime:               &metav1.Time{Time: timeNow},
						UseLatestRestorableTime:   ptr.To(true),
						SourceDBClusterIdentifier: "abcd",
						RestoreType:               ptr.To("full-copy"),
					}),
				),
				result: managed.ExternalCreation{
					ConnectionDetails: generateConnectionDetails(
						testMasterUserName,
						"", // set by ModifyDBCluster after the restore
						testReaderEndpoint,
						testEndpoint,
						testPort,
					),
				},
				docdb: fake.MockDocDBClientCall{
					RestoreDBClusterToPointInTime: []*fake.CallRestoreDBClusterToPointInTime{
						{
							Ctx: context.Background(),
							I: &docdb.RestoreDBClusterToPointInTimeInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								DBSubnetGroupName:   pointer.ToOrNilIfZeroValue(testDBSubnetGroupName),
								DeletionProtection:  pointer.ToOrNilIfZeroValue(true),
								EnableCloudwatchLogsExports: toStringArray(
									testCloudWatchLog,
									testOtherCloudWatchLog,
								),
								KmsKeyId: pointer.ToOrNilIfZeroValue(testKMSKeyID),
								Port:     pointer.ToIntAsInt32(testPort),
								VpcSecurityGroupIds: toStringArray(
									testVpcSecurityGroup,
									testOtherVpcSecurityGroup,
								),
								Tags: []docdbtypes.Tag{
									{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
									{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
								},
								RestoreToTime:             &timeNow,
								UseLatestRestorableTime:   ptr.To(true),
								SourceDBClusterIdentifier: ptr.To("abcd"),
								RestoreType:               ptr.To("full-copy"),
							},
						},
					},
				},
			},
		},
		"ErrorCreate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockCreateDBCluster: func(c context.Context, cdpgi *docdb.CreateDBClusterInput, o ...func(*docdb.Options)) (*docdb.CreateDBClusterOutput, error) {
						return &docdb.CreateDBClusterOutput{}, errors.New(testErrCreateDBClusterFailed)
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Creating()),
				),
				result: managed.ExternalCreation{},
				err:    errors.Wrap(errors.New(testErrCreateDBClusterFailed), errCreate),
				docdb: fake.MockDocDBClientCall{
					CreateDBCluster: []*fake.CallCreateDBCluster{
						{
							Ctx: context.Background(),
							I: &docdb.CreateDBClusterInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
				},
			},
		},
		"ErrorCreate_masterPassword_secret_not_found": {
			args: args{
				kube: &test.MockClient{
					MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
						return errors.New(testErrGetSecret)
					},
				},
				docdb: &fake.MockDocDBClient{
					MockCreateDBCluster: func(c context.Context, cdpgi *docdb.CreateDBClusterInput, o ...func(*docdb.Options)) (*docdb.CreateDBClusterOutput, error) {
						return &docdb.CreateDBClusterOutput{
							DBCluster: &docdbtypes.DBCluster{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								DBClusterArn:        pointer.ToOrNilIfZeroValue(testDBClusterArn),
								Endpoint:            pointer.ToOrNilIfZeroValue(testEndpoint),
								ReaderEndpoint:      pointer.ToOrNilIfZeroValue(testReaderEndpoint),
								Port:                pointer.ToIntAsInt32(testPort),
							},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withMasterPasswordSecretRef(testMasterPasswordSecretNamespace, testMasterPasswordSecretName, testMasterPasswordSecretKey),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Creating()),
					withMasterPasswordSecretRef(testMasterPasswordSecretNamespace, testMasterPasswordSecretName, testMasterPasswordSecretKey),
				),
				result: managed.ExternalCreation{},
				err:    errors.Wrap(errors.Wrap(errors.New(testErrGetSecret), errGetPasswordSecretFailed), "pre-create failed"),
				docdb:  fake.MockDocDBClientCall{},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			opts := []option{setupExternal}
			e := wrapExternal(newExternal(tc.args.kube, tc.args.docdb, opts))
			o, err := e.Create(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, o); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.docdb, tc.args.docdb.Called, cmpopts.IgnoreInterfaces(struct{ context.Context }{}), ignoreUnexported); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type want struct {
		cr    *svcapitypes.DBCluster
		err   error
		docdb fake.MockDocDBClientCall
	}

	cases := map[string]struct {
		args
		want
	}{
		"SuccessfulDelete": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDeleteDBCluster: func(c context.Context, ddpgi *docdb.DeleteDBClusterInput, o ...func(*docdb.Options)) (*docdb.DeleteDBClusterOutput, error) {
						return &docdb.DeleteDBClusterOutput{}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withFinalDBSnapshotIdentifier(testFinalDBSnapshotIdentifier),
					withSkipFinalSnapshot(true),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Deleting()),
					withFinalDBSnapshotIdentifier(testFinalDBSnapshotIdentifier),
					withSkipFinalSnapshot(true),
				),
				docdb: fake.MockDocDBClientCall{
					DeleteDBCluster: []*fake.CallDeleteDBCluster{
						{
							Ctx: context.Background(),
							I: &docdb.DeleteDBClusterInput{
								DBClusterIdentifier:       pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								FinalDBSnapshotIdentifier: pointer.ToOrNilIfZeroValue(testFinalDBSnapshotIdentifier),
								SkipFinalSnapshot:         pointer.ToOrNilIfZeroValue(true),
							},
						},
					},
				},
			},
		},
		"ErrorDelete": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDeleteDBCluster: func(c context.Context, ddpgi *docdb.DeleteDBClusterInput, o ...func(*docdb.Options)) (*docdb.DeleteDBClusterOutput, error) {
						return &docdb.DeleteDBClusterOutput{}, errors.New(testErrDeleteDBClusterFailed)
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withConditions(xpv2.Deleting()),
				),
				err: errors.Wrap(errors.New(testErrDeleteDBClusterFailed), errDelete),
				docdb: fake.MockDocDBClientCall{
					DeleteDBCluster: []*fake.CallDeleteDBCluster{
						{
							Ctx: context.Background(),
							I: &docdb.DeleteDBClusterInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			opts := []option{setupExternal}
			e := newExternal(tc.args.kube, tc.args.docdb, opts)
			_, err := e.Delete(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.docdb, tc.args.docdb.Called, cmpopts.IgnoreInterfaces(struct{ context.Context }{}), ignoreUnexported); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type want struct {
		cr     *svcapitypes.DBCluster
		err    error
		result managed.ExternalUpdate
		docdb  fake.MockDocDBClientCall
	}

	cases := map[string]struct {
		args
		want
	}{
		"SuccessfulRegularUpdate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBCluster: func(c context.Context, mdpgi *docdb.ModifyDBClusterInput, o ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error) {
						return &docdb.ModifyDBClusterOutput{
							DBCluster: &docdbtypes.DBCluster{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								DBClusterArn:        pointer.ToOrNilIfZeroValue(testDBClusterArn),
							},
						}, nil
					},
					MockDescribeDBClusters: func(ctx context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier:     pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									DBClusterParameterGroup: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
									EngineVersion:           pointer.ToOrNilIfZeroValue(testEngineVersion),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{
								{
									Key:   pointer.ToOrNilIfZeroValue(testOtherOtherTagKey),
									Value: pointer.ToOrNilIfZeroValue(testOtherOtherTagValue),
								},
							},
						}, nil
					},
					MockAddTagsToResource: func(_ context.Context, attri *docdb.AddTagsToResourceInput, _ ...func(*docdb.Options)) (*docdb.AddTagsToResourceOutput, error) {
						return &docdb.AddTagsToResourceOutput{}, nil
					},
					MockRemoveTagsFromResource: func(_ context.Context, rtfri *docdb.RemoveTagsFromResourceInput, _ ...func(*docdb.Options)) (*docdb.RemoveTagsFromResourceOutput, error) {
						return &docdb.RemoveTagsFromResourceOutput{}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withAvailabilityZones(
						testAvailabilityZone,
						testOtherAvailabilityZone,
					),
					withBackupRetentionPeriod(testBackupRetentionPeriod),
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withDBSubnetGroup(testDBSubnetGroupName),
					withDeletionProtection(true),
					withEngine(testEngine),
					withEngineVersion(testEngineVersion),
					withKmsKeyID(testKMSKeyID),
					withMasterUserName(testMasterUserName),
					withPort(testPort),
					withPreSignedURL(testPresignedURL),
					withPreferredBackupWindow(testPreferredBackupWindow),
					withPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
					withStorageEncrypted(true),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
					withVpcSecurityGroupIds(
						testVpcSecurityGroup,
						testOtherVpcSecurityGroup,
					),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withAvailabilityZones(
						testAvailabilityZone,
						testOtherAvailabilityZone,
					),
					withBackupRetentionPeriod(testBackupRetentionPeriod),
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withDBSubnetGroup(testDBSubnetGroupName),
					withDeletionProtection(true),
					withEngine(testEngine),
					withEngineVersion(testEngineVersion),
					withKmsKeyID(testKMSKeyID),
					withMasterUserName(testMasterUserName),
					withPort(testPort),
					withPreSignedURL(testPresignedURL),
					withPreferredBackupWindow(testPreferredBackupWindow),
					withPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
					withStorageEncrypted(true),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
					withVpcSecurityGroupIds(
						testVpcSecurityGroup,
						testOtherVpcSecurityGroup,
					),
				),
				docdb: fake.MockDocDBClientCall{
					ModifyDBCluster: []*fake.CallModifyDBCluster{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBClusterInput{
								DBClusterIdentifier:         pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								BackupRetentionPeriod:       pointer.ToIntAsInt32(testBackupRetentionPeriod),
								DBClusterParameterGroupName: nil,
								DeletionProtection:          pointer.ToOrNilIfZeroValue(true),
								EngineVersion:               nil,
								Port:                        pointer.ToIntAsInt32(testPort),
								PreferredBackupWindow:       pointer.ToOrNilIfZeroValue(testPreferredBackupWindow),
								PreferredMaintenanceWindow:  pointer.ToOrNilIfZeroValue(testPreferredMaintenanceWindow),
								VpcSecurityGroupIds: toStringArray(
									testVpcSecurityGroup,
									testOtherVpcSecurityGroup,
								),
								CloudwatchLogsExportConfiguration: &docdbtypes.CloudwatchLogsExportConfiguration{
									DisableLogTypes: []string{},
									EnableLogTypes:  []string{},
								},
							},
						},
					},
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBClusterArn),
							},
						},
					},
					AddTagsToResource: []*fake.CallAddTagsToResource{
						{
							I: &docdb.AddTagsToResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBClusterArn),
								Tags: []docdbtypes.Tag{
									{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
									{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
								},
							},
						},
					},
					RemoveTagsFromResource: []*fake.CallRemoveTagsFromResource{
						{
							I: &docdb.RemoveTagsFromResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBClusterArn),
								TagKeys: []string{
									testOtherOtherTagKey,
								},
							},
						},
					},
				},
			},
		},
		"Successful_EngineVersion_DBClusterParameterGroupName_Update": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBCluster: func(c context.Context, mdpgi *docdb.ModifyDBClusterInput, o ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error) {
						return &docdb.ModifyDBClusterOutput{
							DBCluster: &docdbtypes.DBCluster{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								DBClusterArn:        pointer.ToOrNilIfZeroValue(testDBClusterArn),
							},
						}, nil
					},
					MockDescribeDBClusters: func(ctx context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier:     pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
									DBClusterParameterGroup: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
									EngineVersion:           pointer.ToOrNilIfZeroValue(testEngineVersion),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{
								{
									Key:   pointer.ToOrNilIfZeroValue(testOtherOtherTagKey),
									Value: pointer.ToOrNilIfZeroValue(testOtherOtherTagValue),
								},
							},
						}, nil
					},
					MockAddTagsToResource: func(_ context.Context, attri *docdb.AddTagsToResourceInput, _ ...func(*docdb.Options)) (*docdb.AddTagsToResourceOutput, error) {
						return &docdb.AddTagsToResourceOutput{}, nil
					},
					MockRemoveTagsFromResource: func(_ context.Context, rtfri *docdb.RemoveTagsFromResourceInput, _ ...func(*docdb.Options)) (*docdb.RemoveTagsFromResourceOutput, error) {
						return &docdb.RemoveTagsFromResourceOutput{}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withAvailabilityZones(
						testAvailabilityZone,
						testOtherAvailabilityZone,
					),
					withBackupRetentionPeriod(testBackupRetentionPeriod),
					withDBClusterParameterGroupName(testOtherDBClusterParameterGroupName),
					withDBSubnetGroup(testDBSubnetGroupName),
					withDeletionProtection(true),
					withEngine(testEngine),
					withEngineVersion(testOtherEngineVersion),
					withAllowMajorVersionUpgrade(true),
					withKmsKeyID(testKMSKeyID),
					withMasterUserName(testMasterUserName),
					withPort(testPort),
					withPreSignedURL(testPresignedURL),
					withPreferredBackupWindow(testPreferredBackupWindow),
					withPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
					withStorageEncrypted(true),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
					withVpcSecurityGroupIds(
						testVpcSecurityGroup,
						testOtherVpcSecurityGroup,
					),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withAvailabilityZones(
						testAvailabilityZone,
						testOtherAvailabilityZone,
					),
					withBackupRetentionPeriod(testBackupRetentionPeriod),
					withDBClusterParameterGroupName(testOtherDBClusterParameterGroupName),
					withDBSubnetGroup(testDBSubnetGroupName),
					withDeletionProtection(true),
					withEngine(testEngine),
					withEngineVersion(testOtherEngineVersion),
					withAllowMajorVersionUpgrade(true),
					withKmsKeyID(testKMSKeyID),
					withMasterUserName(testMasterUserName),
					withPort(testPort),
					withPreSignedURL(testPresignedURL),
					withPreferredBackupWindow(testPreferredBackupWindow),
					withPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
					withStorageEncrypted(true),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
					withVpcSecurityGroupIds(
						testVpcSecurityGroup,
						testOtherVpcSecurityGroup,
					),
				),
				docdb: fake.MockDocDBClientCall{
					ModifyDBCluster: []*fake.CallModifyDBCluster{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBClusterInput{
								AllowMajorVersionUpgrade:    pointer.ToOrNilIfZeroValue(true),
								DBClusterIdentifier:         pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								BackupRetentionPeriod:       pointer.ToIntAsInt32(testBackupRetentionPeriod),
								DBClusterParameterGroupName: nil,
								DeletionProtection:          pointer.ToOrNilIfZeroValue(true),
								EngineVersion:               nil,
								Port:                        pointer.ToIntAsInt32(testPort),
								PreferredBackupWindow:       pointer.ToOrNilIfZeroValue(testPreferredBackupWindow),
								PreferredMaintenanceWindow:  pointer.ToOrNilIfZeroValue(testPreferredMaintenanceWindow),
								VpcSecurityGroupIds: toStringArray(
									testVpcSecurityGroup,
									testOtherVpcSecurityGroup,
								),
								CloudwatchLogsExportConfiguration: &docdbtypes.CloudwatchLogsExportConfiguration{
									DisableLogTypes: []string{},
									EnableLogTypes:  []string{},
								},
							},
						},
						{ // Modify call for engineVersion/dbClusterParameterGroupName in postUpdate
							Ctx: context.Background(),
							I: &docdb.ModifyDBClusterInput{
								AllowMajorVersionUpgrade:    pointer.ToOrNilIfZeroValue(true),
								DBClusterIdentifier:         pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testOtherDBClusterParameterGroupName),
								EngineVersion:               pointer.ToOrNilIfZeroValue(testOtherEngineVersion),
							},
						},
					},
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},

					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBClusterArn),
							},
						},
					},
					AddTagsToResource: []*fake.CallAddTagsToResource{
						{
							I: &docdb.AddTagsToResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBClusterArn),
								Tags: []docdbtypes.Tag{
									{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
									{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
								},
							},
						},
					},
					RemoveTagsFromResource: []*fake.CallRemoveTagsFromResource{
						{
							I: &docdb.RemoveTagsFromResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBClusterArn),
								TagKeys: []string{
									testOtherOtherTagKey,
								},
							},
						},
					},
				},
			},
		},
		"SuccessfulUpdate_enable_CloudwatchLogsExportConfiguration": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBCluster: func(c context.Context, mdpgi *docdb.ModifyDBClusterInput, o ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error) {
						return &docdb.ModifyDBClusterOutput{
							DBCluster: &docdbtypes.DBCluster{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								DBClusterArn:        pointer.ToOrNilIfZeroValue(testDBClusterArn),
							},
						}, nil
					},
					MockDescribeDBClusters: func(ctx context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testOtherDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testOtherDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withEnableCloudWatchLogsExports(
						testCloudWatchLog,
						testOtherCloudWatchLog,
					),
				),
				docdb: fake.MockDocDBClientCall{
					ModifyDBCluster: []*fake.CallModifyDBCluster{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBClusterInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								CloudwatchLogsExportConfiguration: &docdbtypes.CloudwatchLogsExportConfiguration{
									DisableLogTypes: []string{},
									EnableLogTypes: toStringArray(
										testCloudWatchLog,
										testOtherCloudWatchLog,
									),
								},
							},
						},
					},
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBClusterArn),
							},
						},
					},
				},
			},
		},
		"SuccessfulUpdate_disable_CloudwatchLogsExportConfiguration": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBCluster: func(c context.Context, mdpgi *docdb.ModifyDBClusterInput, o ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error) {
						return &docdb.ModifyDBClusterOutput{
							DBCluster: &docdbtypes.DBCluster{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								DBClusterArn:        pointer.ToOrNilIfZeroValue(testDBClusterArn),
							},
						}, nil
					},
					MockDescribeDBClusters: func(ctx context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testOtherDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withEnableCloudWatchLogsExports(),
					withStatusEnableCloudWatchLogsExports(
						testCloudWatchLog,
					),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testOtherDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withEnableCloudWatchLogsExports(),
					withStatusEnableCloudWatchLogsExports(
						testCloudWatchLog,
					),
				),
				docdb: fake.MockDocDBClientCall{
					ModifyDBCluster: []*fake.CallModifyDBCluster{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBClusterInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								CloudwatchLogsExportConfiguration: &docdbtypes.CloudwatchLogsExportConfiguration{
									DisableLogTypes: toStringArray(
										testCloudWatchLog,
									),
									EnableLogTypes: toStringArray(),
								},
							},
						},
					},
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBClusterArn),
							},
						},
					},
				},
			},
		},
		"SuccessfulUpdate_enable_and_disable_CloudwatchLogsExportConfiguration": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBCluster: func(c context.Context, mdpgi *docdb.ModifyDBClusterInput, o ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error) {
						return &docdb.ModifyDBClusterOutput{
							DBCluster: &docdbtypes.DBCluster{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								DBClusterArn:        pointer.ToOrNilIfZeroValue(testDBClusterArn),
							},
						}, nil
					},
					MockDescribeDBClusters: func(ctx context.Context, ddi *docdb.DescribeDBClustersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
						return &docdb.DescribeDBClustersOutput{
							DBClusters: []docdbtypes.DBCluster{
								{
									DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
				},
				cr: instance(
					withDBClusterIdentifier(testOtherDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withEnableCloudWatchLogsExports(
						testOtherCloudWatchLog,
					),
					withStatusEnableCloudWatchLogsExports(
						testCloudWatchLog,
					),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testOtherDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
					withEnableCloudWatchLogsExports(
						testOtherCloudWatchLog,
					),
					withStatusEnableCloudWatchLogsExports(
						testCloudWatchLog,
					),
				),
				docdb: fake.MockDocDBClientCall{
					ModifyDBCluster: []*fake.CallModifyDBCluster{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBClusterInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								CloudwatchLogsExportConfiguration: &docdbtypes.CloudwatchLogsExportConfiguration{
									DisableLogTypes: toStringArray(
										testCloudWatchLog,
									),
									EnableLogTypes: toStringArray(
										testOtherCloudWatchLog,
									),
								},
							},
						},
					},
					DescribeDBClusters: []*fake.CallDescribeDBClusters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClustersInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBClusterArn),
							},
						},
					},
				},
			},
		},
		"ErrorUpdate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBCluster: func(c context.Context, mdpgi *docdb.ModifyDBClusterInput, o ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error) {
						return &docdb.ModifyDBClusterOutput{}, errors.New(testErrModifyDBClusterFailed)
					},
				},
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBClusterIdentifier(testDBClusterIdentifier),
					withExternalName(testDBClusterIdentifier),
				),
				err: errors.Wrap(errors.New(testErrModifyDBClusterFailed), errUpdate),
				docdb: fake.MockDocDBClientCall{
					ModifyDBCluster: []*fake.CallModifyDBCluster{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBClusterInput{
								DBClusterIdentifier: pointer.ToOrNilIfZeroValue(testDBClusterIdentifier),
								CloudwatchLogsExportConfiguration: &docdbtypes.CloudwatchLogsExportConfiguration{
									DisableLogTypes: []string{},
									EnableLogTypes:  []string{},
								},
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			opts := []option{setupExternal}
			e := newExternal(tc.args.kube, tc.args.docdb, opts)
			o, err := e.Update(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, o); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.docdb, tc.args.docdb.Called, cmpopts.IgnoreInterfaces(struct{ context.Context }{}), ignoreUnexported); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
		})
	}
}

type args struct {
	docdb *fake.MockDocDBClient
	kube  client.Client
	cr    *svcapitypes.DBCluster
}

var ignoreUnexported = cmp.FilterPath(func(p cmp.Path) bool {
	sf, ok := p.Index(-1).(cmp.StructField)
	return ok && !token.IsExported(sf.Name())
}, cmp.Ignore())

func TestGetPasswordFromRefUsesNamespaceOfDBCluster(t *testing.T) {
	cr := instance(
		withMasterPasswordSecretRef("team-a", testMasterPasswordSecretName, testMasterPasswordSecretKey),
	)
	cr.Spec.WriteConnectionSecretToReference = &xpv2.LocalSecretReference{Name: "conn"}
	var got []client.ObjectKey
	kube := &test.MockClient{
		MockGet: func(_ context.Context, key client.ObjectKey, obj client.Object) error {
			got = append(got, key)
			s := obj.(*v1.Secret)
			if key.Name == testMasterPasswordSecretName {
				s.Data = map[string][]byte{testMasterPasswordSecretKey: []byte("new")}
			} else {
				s.Data = map[string][]byte{awsclient.ResourceCredentialsSecretPasswordKey: []byte("old")}
			}
			return nil
		},
	}
	h := &hooks{kube: kube}
	pw, changed, err := h.getPasswordFromRef(context.Background(), cr)
	if err != nil {
		t.Fatal(err)
	}
	if pw != "new" || !changed {
		t.Errorf("getPasswordFromRef() = %q, %v; want %q, true", pw, changed, "new")
	}
	want := []client.ObjectKey{
		{Namespace: "team-a", Name: testMasterPasswordSecretName},
		{Namespace: "team-a", Name: "conn"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("secret keys: -want, +got:\n%s", diff)
	}
}

func TestDiffParameters(t *testing.T) {
	observed := docdbtypes.DBCluster{
		BackupRetentionPeriod:            ptr.To[int32](7),
		CopyTagsToSnapshot:               ptr.To(false),
		DBClusterParameterGroup:          ptr.To("default.docdb5.0"),
		DeletionProtection:               ptr.To(true),
		EnabledCloudwatchLogsExports:     []string{"audit", "profiler"},
		EngineVersion:                    ptr.To("5.0.0"),
		NetworkType:                      ptr.To("IPV4"),
		Port:                             ptr.To[int32](27017),
		PreferredBackupWindow:            ptr.To("01:00-01:30"),
		PreferredMaintenanceWindow:       ptr.To("sat:02:00-sat:03:00"),
		ServerlessV2ScalingConfiguration: &docdbtypes.ServerlessV2ScalingConfigurationInfo{MaxCapacity: ptr.To[float64](8), MinCapacity: ptr.To(0.5)},
		StorageType:                      ptr.To("standard"),
		VpcSecurityGroups: []docdbtypes.VpcSecurityGroupMembership{
			{VpcSecurityGroupId: ptr.To("sg-1")}, {VpcSecurityGroupId: ptr.To("sg-2")},
		},
	}
	upToDate := svcapitypes.DBClusterParameters{
		BackupRetentionPeriod:            ptr.To[int64](7),
		CopyTagsToSnapshot:               ptr.To(false),
		DBClusterParameterGroupName:      ptr.To("default.docdb5.0"),
		DeletionProtection:               ptr.To(true),
		EnableCloudwatchLogsExports:      []*string{ptr.To("profiler"), ptr.To("audit")},
		EngineVersion:                    ptr.To("5.0.0"),
		ManageMasterUserPassword:         ptr.To(false),
		NetworkType:                      ptr.To("IPV4"),
		Port:                             ptr.To[int64](27017),
		PreferredBackupWindow:            ptr.To("01:00-01:30"),
		PreferredMaintenanceWindow:       ptr.To("sat:02:00-sat:03:00"),
		ServerlessV2ScalingConfiguration: &svcapitypes.ServerlessV2ScalingConfiguration{MaxCapacity: ptr.To[float64](8)},
		StorageType:                      ptr.To("standard"),
		VPCSecurityGroupIDs:              []*string{ptr.To("sg-2"), ptr.To("sg-1")},
	}
	cases := map[string]struct {
		modify func(p *svcapitypes.DBClusterParameters)
		want   []string
	}{
		"UpToDate":        {modify: func(*svcapitypes.DBClusterParameters) {}},
		"NothingSet":      {modify: func(p *svcapitypes.DBClusterParameters) { *p = svcapitypes.DBClusterParameters{} }},
		"OlderEngine":     {modify: func(p *svcapitypes.DBClusterParameters) { p.EngineVersion = ptr.To("4.0.0") }},
		"NewerEngine":     {modify: func(p *svcapitypes.DBClusterParameters) { p.EngineVersion = ptr.To("6.0.0") }, want: []string{"engineVersion"}},
		"BackupRetention": {modify: func(p *svcapitypes.DBClusterParameters) { p.BackupRetentionPeriod = ptr.To[int64](1) }, want: []string{"backupRetentionPeriod"}},
		"CopyTags":        {modify: func(p *svcapitypes.DBClusterParameters) { p.CopyTagsToSnapshot = ptr.To(true) }, want: []string{"copyTagsToSnapshot"}},
		"DeletionProt":    {modify: func(p *svcapitypes.DBClusterParameters) { p.DeletionProtection = ptr.To(false) }, want: []string{"deletionProtection"}},
		"NetworkType":     {modify: func(p *svcapitypes.DBClusterParameters) { p.NetworkType = ptr.To("DUAL") }, want: []string{"networkType"}},
		"Port":            {modify: func(p *svcapitypes.DBClusterParameters) { p.Port = ptr.To[int64](27018) }, want: []string{"port"}},
		"BackupWindow":    {modify: func(p *svcapitypes.DBClusterParameters) { p.PreferredBackupWindow = ptr.To("02:00-02:30") }, want: []string{"preferredBackupWindow"}},
		"MaintenanceWin":  {modify: func(p *svcapitypes.DBClusterParameters) { p.PreferredMaintenanceWindow = ptr.To("sun:02:00-sun:03:00") }, want: []string{"preferredMaintenanceWindow"}},
		"Serverless": {modify: func(p *svcapitypes.DBClusterParameters) {
			p.ServerlessV2ScalingConfiguration.MinCapacity = ptr.To[float64](1)
		}, want: []string{"serverlessV2ScalingConfiguration"}},
		"StorageType":      {modify: func(p *svcapitypes.DBClusterParameters) { p.StorageType = ptr.To("iopt1") }, want: []string{"storageType"}},
		"ParameterGroup":   {modify: func(p *svcapitypes.DBClusterParameters) { p.DBClusterParameterGroupName = ptr.To("custom") }, want: []string{"dbClusterParameterGroupName"}},
		"CloudwatchLogs":   {modify: func(p *svcapitypes.DBClusterParameters) { p.EnableCloudwatchLogsExports = []*string{ptr.To("audit")} }, want: []string{"enableCloudwatchLogsExports"}},
		"NoCloudwatchLogs": {modify: func(p *svcapitypes.DBClusterParameters) { p.EnableCloudwatchLogsExports = []*string{} }, want: []string{"enableCloudwatchLogsExports"}},
		"SecurityGroups":   {modify: func(p *svcapitypes.DBClusterParameters) { p.VPCSecurityGroupIDs = []*string{ptr.To("sg-3")} }, want: []string{"vpcSecurityGroupIDs"}},
		"ManagedPassword":  {modify: func(p *svcapitypes.DBClusterParameters) { p.ManageMasterUserPassword = ptr.To(true) }, want: []string{"manageMasterUserPassword"}},
		"Several": {
			modify: func(p *svcapitypes.DBClusterParameters) {
				p.Port = ptr.To[int64](1)
				p.StorageType = ptr.To("iopt1")
			},
			want: []string{"port", "storageType"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := upToDate.DeepCopy()
			tc.modify(p)
			got := diffParameters(p, &docdb.DescribeDBClustersOutput{DBClusters: []docdbtypes.DBCluster{observed}})
			if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("diffParameters(...): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestLateInitializeServerlessV2ScalingConfiguration(t *testing.T) {
	p := &svcapitypes.DBClusterParameters{}
	out := &docdb.DescribeDBClustersOutput{DBClusters: []docdbtypes.DBCluster{{
		ServerlessV2ScalingConfiguration: &docdbtypes.ServerlessV2ScalingConfigurationInfo{MaxCapacity: ptr.To[float64](4), MinCapacity: ptr.To[float64](0.5)},
	}}}
	if err := lateInitialize(p, out); err != nil {
		t.Fatal(err)
	}
	want := &svcapitypes.ServerlessV2ScalingConfiguration{MaxCapacity: ptr.To[float64](4), MinCapacity: ptr.To[float64](0.5)}
	if diff := cmp.Diff(want, p.ServerlessV2ScalingConfiguration); diff != "" {
		t.Errorf("lateInitialize(...): -want, +got:\n%s", diff)
	}
}

// passwordKube returns the master password secret with desired and the
// connection secret with published as password.
func passwordKube(desired, published string) *test.MockClient {
	return &test.MockClient{
		MockGet: func(_ context.Context, key client.ObjectKey, obj client.Object) error {
			s := obj.(*v1.Secret)
			if key.Name == testMasterPasswordSecretName {
				s.Data = map[string][]byte{testMasterPasswordSecretKey: []byte(desired)}
			} else {
				s.Data = map[string][]byte{awsclient.ResourceCredentialsSecretPasswordKey: []byte(published)}
			}
			return nil
		},
	}
}

func passwordCluster(m ...docDBModifier) *svcapitypes.DBCluster {
	cr := instance(append([]docDBModifier{
		withExternalName(testDBClusterIdentifier),
		withMasterPasswordSecretRef(testMasterPasswordSecretNamespace, testMasterPasswordSecretName, testMasterPasswordSecretKey),
	}, m...)...)
	cr.Spec.WriteConnectionSecretToReference = &xpv2.LocalSecretReference{Name: "conn"}
	return cr
}

// The reconciler publishes the connection details of Observe before it calls
// Update. A changed password must not be published by Observe, or preUpdate
// would not send it to AWS.
func TestPasswordChange(t *testing.T) {
	ctx := context.Background()
	cr := passwordCluster()
	describe := &docdb.DescribeDBClustersOutput{DBClusters: []docdbtypes.DBCluster{{DBClusterArn: ptr.To(testDBClusterArn)}}}
	client := &fake.MockDocDBClient{
		MockDescribeDBClusters: func(context.Context, *docdb.DescribeDBClustersInput, ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
			return describe, nil
		},
	}
	h := &hooks{kube: passwordKube("new", "old"), client: client}

	upToDate, diff, err := h.isUpToDate(ctx, cr, describe)
	if err != nil || upToDate {
		t.Fatalf("isUpToDate(...) = %v, %v; want false, nil", upToDate, err)
	}
	if want := compare.Message([]string{"masterUserPasswordSecretRef"}); diff != want {
		t.Errorf("isUpToDate(...) diff = %q, want %q", diff, want)
	}

	obs, err := h.postObserve(ctx, cr, describe, managed.ExternalObservation{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := obs.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey]; ok {
		t.Errorf("postObserve(...) published the changed password")
	}

	in := &docdb.ModifyDBClusterInput{}
	if err := h.preUpdate(ctx, cr, in); err != nil {
		t.Fatal(err)
	}
	if got := aws.ToString(in.MasterUserPassword); got != "new" {
		t.Errorf("preUpdate(...) MasterUserPassword = %q, want %q", got, "new")
	}

	upd, err := h.postUpdate(ctx, cr, &docdb.ModifyDBClusterOutput{DBCluster: &docdbtypes.DBCluster{DBClusterArn: ptr.To(testDBClusterArn)}}, managed.ExternalUpdate{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := managed.ConnectionDetails{awsclient.ResourceCredentialsSecretPasswordKey: []byte("new")}
	if diff := cmp.Diff(want, upd.ConnectionDetails); diff != "" {
		t.Errorf("postUpdate(...) connection details: -want, +got:\n%s", diff)
	}

	// Once published, the password is up to date and published by Observe.
	h.kube = passwordKube("new", "new")
	if upToDate, _, err := h.isUpToDate(ctx, cr, describe); err != nil || !upToDate {
		t.Errorf("isUpToDate(...) after update = %v, %v; want true, nil", upToDate, err)
	}
	obs, _ = h.postObserve(ctx, cr, describe, managed.ExternalObservation{}, nil)
	if got := string(obs.ConnectionDetails[awsclient.ResourceCredentialsSecretPasswordKey]); got != "new" {
		t.Errorf("postObserve(...) password = %q, want %q", got, "new")
	}
}

func TestPreUpdateManageMasterUserPassword(t *testing.T) {
	managedSecret := &svcapitypes.ClusterMasterUserSecret{SecretARN: ptr.To("arn")}
	cases := map[string]struct {
		manage   *bool
		observed *svcapitypes.ClusterMasterUserSecret
		want     *docdb.ModifyDBClusterInput
	}{
		"NotSet": {
			want: &docdb.ModifyDBClusterInput{},
		},
		"AlreadyManaged": {
			manage:   ptr.To(true),
			observed: managedSecret,
			want:     &docdb.ModifyDBClusterInput{},
		},
		"SwitchOn": {
			manage: ptr.To(true),
			want:   &docdb.ModifyDBClusterInput{ManageMasterUserPassword: ptr.To(true), MasterUserSecretKmsKeyId: ptr.To("key")},
		},
		"SwitchOff": {
			manage:   ptr.To(false),
			observed: managedSecret,
			want:     &docdb.ModifyDBClusterInput{ManageMasterUserPassword: ptr.To(false), MasterUserPassword: ptr.To("pw")},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := passwordCluster()
			cr.Spec.ForProvider.ManageMasterUserPassword = tc.manage
			cr.Status.AtProvider.MasterUserSecret = tc.observed
			in := &docdb.ModifyDBClusterInput{ManageMasterUserPassword: tc.manage, MasterUserSecretKmsKeyId: ptr.To("key")}
			h := &hooks{kube: passwordKube("pw", "pw")}
			if err := h.preUpdate(context.Background(), cr, in); err != nil {
				t.Fatal(err)
			}
			tc.want.DBClusterIdentifier = ptr.To(testDBClusterIdentifier)
			if diff := cmp.Diff(tc.want, in, ignoreUnexported, cmpopts.IgnoreFields(docdb.ModifyDBClusterInput{}, "CloudwatchLogsExportConfiguration")); diff != "" {
				t.Errorf("preUpdate(...): -want, +got:\n%s", diff)
			}
		})
	}
}

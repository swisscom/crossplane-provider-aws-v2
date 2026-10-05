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

package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// S3RestoreBackupConfiguration defines the details of the S3 backup to restore from.
type S3RestoreBackupConfiguration struct {
	// BucketName is the name of the S3 bucket containing the backup to restore.
	BucketName *string `json:"bucketName"`

	// IngestionRoleARN is the IAM role RDS can assume that will allow it to access the contents of the S3 bucket.
	IngestionRoleARN *string `json:"ingestionRoleARN"`

	// Prefix is the path prefix of the S3 bucket within which the backup to restore is located.
	// +optional
	Prefix *string `json:"prefix,omitempty"`

	// SourceEngine is the engine used to create the backup.
	// Must be "mysql".
	SourceEngine *string `json:"sourceEngine"`

	// SourceEngineVersion is the version of the engine used to create the backup.
	// Example: "5.7.30"
	SourceEngineVersion *string `json:"sourceEngineVersion"`
}

// SnapshotRestoreBackupConfiguration defines the details of the snapshot to restore from.
type SnapshotRestoreBackupConfiguration struct {
	// SnapshotIdentifier is the identifier of the snapshot to restore.
	SnapshotIdentifier *string `json:"snapshotIdentifier"`
}

// PointInTimeRestoreBackupConfiguration defines the details of the time to restore from
type PointInTimeRestoreBackupConfiguration struct {
	// RestoreTime is the date and time (UTC) to restore from.
	// Must be before the latest restorable time for the DB instance.
	// Can't be specified if the useLatestRestorableTime parameter is enabled.
	// Example: 2011-09-07T23:45:00Z
	// +optional
	RestoreTime *metav1.Time `json:"restoreTime,omitempty"`

	// UseLatestRestorableTime indicates that the DB instance is restored from the latest backup
	// Can't be specified if the restoreTime parameter is provided.
	// +optional
	UseLatestRestorableTime bool `json:"useLatestRestorableTime"`

	// SourceDBInstanceAutomatedBackupsArn specifies the Amazon Resource Name (ARN) of the replicated automated backups
	// from which to restore. Example: arn:aws:rds:useast-1:123456789012:auto-backup:ab-L2IJCEXJP7XQ7HOJ4SIEXAMPLE
	// +optional
	SourceDBInstanceAutomatedBackupsArn *string `json:"sourceDBInstanceAutomatedBackupsArn,omitempty"`

	// SourceDBInstanceIdentifier specifies the identifier of the source DB instance from which to restore. Constraints:
	// Must match the identifier of an existing DB instance.
	// +optional
	SourceDBInstanceIdentifier *string `json:"sourceDBInstanceIdentifier,omitempty"`

	// SourceDbiResourceID specifies the resource ID of the source DB instance from which to restore.
	// +optional
	SourceDbiResourceID *string `json:"sourceDbiResourceId,omitempty"`
}

// RestoreDBInstanceBackupConfiguration defines the backup to restore a new DBCluster from.
type RestoreDBInstanceBackupConfiguration struct {
	// S3 specifies the details of the S3 backup to restore from.
	// +optional
	S3 *S3RestoreBackupConfiguration `json:"s3,omitempty"`

	// Snapshot specifies the details of the snapshot to restore from.
	// +optional
	Snapshot *SnapshotRestoreBackupConfiguration `json:"snapshot,omitempty"`

	// PointInTime specifies the details of the point in time restore.
	// +optional
	PointInTime *PointInTimeRestoreBackupConfiguration `json:"pointInTime,omitempty"`

	// Source is the type of the backup to restore when creating a new  DBCluster or DBInstance.
	// S3, Snapshot and PointInTime are supported.
	// +kubebuilder:validation:Enum=S3;Snapshot;PointInTime
	Source *string `json:"source"`
}

// CustomDBInstanceParameters are custom parameters for the DBInstance
type CustomDBInstanceParameters struct {
	// AutogeneratePassword indicates whether the controller should generate
	// a random password for the master user if one is not provided via
	// MasterUserPasswordSecretRef.
	//
	// If a password is generated, it will
	// be stored as a secret at the location specified by MasterUserPasswordSecretRef.
	// +optional
	AutogeneratePassword bool `json:"autogeneratePassword,omitempty"`

	// A list of database security groups to associate with this DB instance
	DBSecurityGroups []string `json:"dbSecurityGroups,omitempty"`

	// The version number of the database engine to use.
	//
	// For a list of valid engine versions, use the DescribeDBEngineVersions operation.
	//
	// The following are the database engines and links to information about the
	// major and minor versions that are available with Amazon RDS. Not every database
	// engine is available for every Amazon Web Services Region.
	//
	// Amazon Aurora
	//
	// Not applicable. The version number of the database engine to be used by the
	// DB instance is managed by the DB cluster.
	//
	// Amazon RDS Custom for Oracle
	//
	// A custom engine version (CEV) that you have previously created. This setting
	// is required for RDS Custom for Oracle. The CEV name has the following format:
	// 19.customized_string. A valid CEV name is 19.my_cev1. For more information,
	// see Creating an RDS Custom for Oracle DB instance (https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/custom-creating.html#custom-creating.create)
	// in the Amazon RDS User Guide.
	//
	// Amazon RDS Custom for SQL Server
	//
	// See RDS Custom for SQL Server general requirements (https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/custom-reqs-limits-MS.html)
	// in the Amazon RDS User Guide.
	//
	// MariaDB
	//
	// For information, see MariaDB on Amazon RDS Versions (https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/CHAP_MariaDB.html#MariaDB.Concepts.VersionMgmt)
	// in the Amazon RDS User Guide.
	//
	// Microsoft SQL Server
	//
	// For information, see Microsoft SQL Server Versions on Amazon RDS (https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/CHAP_SQLServer.html#SQLServer.Concepts.General.VersionSupport)
	// in the Amazon RDS User Guide.
	//
	// MySQL
	//
	// For information, see MySQL on Amazon RDS Versions (https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/CHAP_MySQL.html#MySQL.Concepts.VersionMgmt)
	// in the Amazon RDS User Guide.
	//
	// Oracle
	//
	// For information, see Oracle Database Engine Release Notes (https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Appendix.Oracle.PatchComposition.html)
	// in the Amazon RDS User Guide.
	//
	// PostgreSQL
	//
	// For information, see Amazon RDS for PostgreSQL versions and extensions (https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/CHAP_PostgreSQL.html#PostgreSQL.Concepts)
	// in the Amazon RDS User Guide.
	//
	// Note: Downgrades are not allowed by AWS and attempts to set a lower version
	// will be ignored.
	EngineVersion *string `json:"engineVersion,omitempty"`

	// The DB instance snapshot identifier of the new DB instance snapshot created
	// when SkipFinalSnapshot is disabled.
	//
	// Specifying this parameter and also skipping the creation of a final DB instance
	// snapshot with the SkipFinalShapshot parameter results in an error.
	//
	// Constraints:
	//
	//    * Must be 1 to 255 letters, numbers, or hyphens.
	//
	//    * First character must be a letter
	//
	//    * Can't end with a hyphen or contain two consecutive hyphens
	// +immutable
	// +optional
	FinalDBSnapshotIdentifier string `json:"finalDBSnapshotIdentifier,omitempty"`

	// The password for the master database user. This password can contain any
	// printable ASCII character except "/", """, or "@".
	//
	// Constraints: Must contain from 8 to 41 characters.
	// +optional
	MasterUserPasswordSecretRef *xpv2.LocalSecretKeySelector `json:"masterUserPasswordSecretRef,omitempty"`

	// A value that indicates whether to skip the creation of a final DB instance
	// snapshot before the DB instance is deleted. If skip is specified, no DB instance
	// snapshot is created. If skip isn't specified, a DB instance snapshot is created
	// before the DB instance is deleted. By default, skip isn't specified, and the
	// DB instance snapshot is created. By default, this parameter is disabled.
	//
	// You must specify a FinalDBSnapshotIdentifier parameter if SkipFinalSnapshot
	// is disabled.
	// +immutable
	// +optional
	SkipFinalSnapshot bool `json:"skipFinalSnapshot,omitempty"`

	// The identifier of the Multi-AZ DB cluster that will act as the source for
	// the read replica. Each DB cluster can have up to 15 read replicas.
	//
	// Constraints:
	//
	//    * Must be the identifier of an existing Multi-AZ DB cluster.
	//
	//    * Can't be specified if the SourceDBInstanceIdentifier parameter is also
	//    specified.
	//
	//    * The specified DB cluster must have automatic backups enabled, that is,
	//    its backup retention period must be greater than 0.
	//
	//    * The source DB cluster must be in the same Amazon Web Services Region
	//    as the read replica. Cross-Region replication isn't supported.
	// +immutable

	SourceDBClusterID *string `json:"sourceDBClusterID,omitempty"`

	// The identifier of the DB instance that will act as the source for the read
	// replica. Each DB instance can have up to 15 read replicas, with the exception of
	// Oracle and SQL Server, which can have up to five.
	//
	// Constraints:
	//
	//   - Must be the identifier of an existing Db2, MariaDB, MySQL, Oracle,
	//   PostgreSQL, or SQL Server DB instance.
	//
	//    * Can't be specified if the SourceDBClusterIdentifier parameter is also
	//    specified.
	//
	//   - For the limitations of Oracle read replicas, see [Version and licensing considerations for RDS for Oracle replicas]in the Amazon RDS User
	//   Guide.
	//
	//   - For the limitations of SQL Server read replicas, see [Read replica limitations with SQL Server]in the Amazon RDS User
	//   Guide.
	//
	//   - The specified DB instance must have automatic backups enabled, that is, its
	//   backup retention period must be greater than 0.
	//
	// [Read replica limitations with SQL Server]: https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/SQLServer.ReadReplicas.html#SQLServer.ReadReplicas.Limitations
	// [Version and licensing considerations for RDS for Oracle replicas]: https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/oracle-read-replicas.limitations.html#oracle-read-replicas.limitations.versions-and-licenses
	// +immutable
	// +crossplane:generate:reference:type=github.com/teeverr/provider-aws-v2/apis/rds/v1alpha1.DBInstance
	SourceDBInstanceID *string `json:"sourceDBInstanceID,omitempty"`

	// SourceDBInstanceIDRef is a reference to a DBInstance used to set
	// SourceDBInstanceID.
	// +optional
	SourceDBInstanceIDRef *xpv2.NamespacedReference `json:"sourceDBInstanceIDRef,omitempty"`

	// SourceDBInstanceIDSelector selects a reference to a DBInstance used to
	// set SourceDBInstanceID.
	// +optional
	SourceDBInstanceIDSelector *xpv2.NamespacedSelector `json:"sourceDBInstanceIDSelector,omitempty"`

	// A list of Amazon EC2 VPC security groups to authorize on this DB instance.
	// This change is asynchronously applied as soon as possible.
	//
	// This setting doesn't apply to RDS Custom.
	//
	// Amazon Aurora
	// Not applicable. The associated list of EC2 VPC security groups is managed
	// by the DB cluster. For more information, see ModifyDBCluster.
	//
	// Constraints:
	//    * If supplied, must match existing VpcSecurityGroupIds.
	VPCSecurityGroupIDs []string `json:"vpcSecurityGroupIDs,omitempty"`

	// A value that indicates whether the modifications in this request and any
	// pending modifications are asynchronously applied as soon as possible, regardless
	// of the PreferredMaintenanceWindow setting for the DB instance. By default,
	// this parameter is disabled.
	//
	// If this parameter is disabled, changes to the DB instance are applied during
	// the next maintenance window. Some parameter changes can cause an outage and
	// are applied on the next call to RebootDBInstance, or the next failure reboot.
	// Review the table of parameters in Modifying a DB Instance (https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Overview.DBInstance.Modifying.html)
	// in the Amazon RDS User Guide. to see the impact of enabling or disabling
	// ApplyImmediately for each modified parameter and to determine when the changes
	// are applied.
	ApplyImmediately *bool `json:"applyImmediately,omitempty"`

	// RestoreFrom specifies the details of the backup to restore when creating a new DBInstance.
	// +optional
	RestoreFrom *RestoreDBInstanceBackupConfiguration `json:"restoreFrom,omitempty"`

	// DeleteAutomatedBackups indicates whether to remove automated backups
	// immediately after the DB instance is deleted. The default is to
	// remove automated backups immediately after the DB instance is
	// deleted.
	// +optional
	DeleteAutomatedBackups *bool `json:"deleteAutomatedBackups,omitempty"`

	// TagsIgnore contains rules that tell the reconciler to pretend matching
	// tags don't exist during diff/updates. A rule key supports either exact
	// match (e.g. "c7n:policy") or a simple prefix glob using a trailing *
	// (e.g. "c7n:*" or "prefix*"). In all cases tags starting with the
	// prefix "aws:" are always ignored (implicit rule "aws:*").
	// +optional
	TagsIgnore []TagIgnoreRule `json:"tagsIgnore,omitempty"`
}

// CustomDBInstanceObservation includes the custom status fields of DBInstance.
type CustomDBInstanceObservation struct {
	// AWS API calls don't return any field which explicitly indicates the role of database, which would be really convenient.
	// DatabaseRole works on the similar principle as the Role field in AWS UI("Aurora and RDS" > "Databases").

	// The database role may be Standalone, Primary or Replica.
	DatabaseRole *string `json:"databaseRole,omitempty"`

	// AvailabilityZone is the Availability Zone where the DB instance is located.
	// +optional
	AvailabilityZone *string `json:"availabilityZone,omitempty"`

	// ObservedTags exposes the full, unfiltered set of external tags returned
	// by AWS for observability. These are never used for diffing directly.
	// +optional
	ObservedTags []*Tag `json:"observedTags,omitempty"`
}

// TagIgnoreRule defines a single rule for ignoring a tag during diffing.
// The Key may be an exact tag key or a prefix pattern that ends with *.
type TagIgnoreRule struct {
	// Key of the ignore rule. Supports exact key match or prefix* glob.
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

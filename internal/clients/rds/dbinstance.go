/*
Copyright 2019 The Crossplane Authors.

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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"

	"github.com/teeverr/provider-aws-v2/apis/rds/v1alpha1"
	"github.com/teeverr/provider-aws-v2/internal/utils/pointer"
)

// ProcessorFeatures converts the processor features of p to SDK types.
func ProcessorFeatures(p *v1alpha1.DBInstanceParameters) []svcsdktypes.ProcessorFeature {
	if len(p.ProcessorFeatures) == 0 {
		return nil
	}
	res := make([]svcsdktypes.ProcessorFeature, 0, len(p.ProcessorFeatures))
	for _, val := range p.ProcessorFeatures {
		if val == nil {
			continue
		}
		res = append(res, svcsdktypes.ProcessorFeature{Name: val.Name, Value: val.Value})
	}
	return res
}

// Tags converts the tags of p to SDK types.
func Tags(p *v1alpha1.DBInstanceParameters) []svcsdktypes.Tag {
	if len(p.Tags) == 0 {
		return nil
	}
	res := make([]svcsdktypes.Tag, 0, len(p.Tags))
	for _, val := range p.Tags {
		if val == nil {
			continue
		}
		res = append(res, svcsdktypes.Tag{Key: val.Key, Value: val.Value})
	}
	return res
}

// GenerateRestoreDBInstanceFromS3Input from RDSInstanceSpec
func GenerateRestoreDBInstanceFromS3Input(name, password string, p *v1alpha1.DBInstanceParameters) *svcsdk.RestoreDBInstanceFromS3Input {
	// Partially duplicates GenerateCreateDBInstanceInput - make sure any relevant changes are applied there too.
	return &svcsdk.RestoreDBInstanceFromS3Input{
		DBInstanceIdentifier:               aws.String(name),
		AllocatedStorage:                   pointer.Int32(p.AllocatedStorage),
		AutoMinorVersionUpgrade:            p.AutoMinorVersionUpgrade,
		AvailabilityZone:                   p.AvailabilityZone,
		BackupRetentionPeriod:              pointer.Int32(p.BackupRetentionPeriod),
		CopyTagsToSnapshot:                 p.CopyTagsToSnapshot,
		DBInstanceClass:                    p.DBInstanceClass,
		DBName:                             p.DBName,
		DBParameterGroupName:               p.DBParameterGroupName,
		DBSecurityGroups:                   p.DBSecurityGroups,
		DBSubnetGroupName:                  p.DBSubnetGroupName,
		DeletionProtection:                 p.DeletionProtection,
		EnableCloudwatchLogsExports:        aws.ToStringSlice(p.EnableCloudwatchLogsExports),
		EnableIAMDatabaseAuthentication:    p.EnableIAMDatabaseAuthentication,
		EnablePerformanceInsights:          p.EnablePerformanceInsights,
		Engine:                             p.Engine,
		EngineVersion:                      p.EngineVersion,
		Iops:                               pointer.Int32(p.IOPS),
		KmsKeyId:                           p.KMSKeyID,
		LicenseModel:                       p.LicenseModel,
		MasterUserPassword:                 pointer.ToOrNilIfZeroValue(password),
		MasterUsername:                     p.MasterUsername,
		MonitoringInterval:                 pointer.Int32(p.MonitoringInterval),
		MonitoringRoleArn:                  p.MonitoringRoleARN,
		MultiAZ:                            p.MultiAZ,
		OptionGroupName:                    p.OptionGroupName,
		PerformanceInsightsKMSKeyId:        p.PerformanceInsightsKMSKeyID,
		PerformanceInsightsRetentionPeriod: pointer.Int32(p.PerformanceInsightsRetentionPeriod),
		Port:                               pointer.Int32(p.Port),
		PreferredBackupWindow:              p.PreferredBackupWindow,
		PreferredMaintenanceWindow:         p.PreferredMaintenanceWindow,
		PubliclyAccessible:                 p.PubliclyAccessible,
		S3BucketName:                       p.RestoreFrom.S3.BucketName,
		S3IngestionRoleArn:                 p.RestoreFrom.S3.IngestionRoleARN,
		S3Prefix:                           p.RestoreFrom.S3.Prefix,
		SourceEngine:                       p.RestoreFrom.S3.SourceEngine,
		SourceEngineVersion:                p.RestoreFrom.S3.SourceEngineVersion,
		StorageEncrypted:                   p.StorageEncrypted,
		StorageType:                        p.StorageType,
		VpcSecurityGroupIds:                p.VPCSecurityGroupIDs,
		ProcessorFeatures:                  ProcessorFeatures(p),
		Tags:                               Tags(p),
	}
}

// GenerateRestoreDBInstanceFromSnapshotInput from RDSInstanceSpec
func GenerateRestoreDBInstanceFromSnapshotInput(name string, p *v1alpha1.DBInstanceParameters) *svcsdk.RestoreDBInstanceFromDBSnapshotInput {
	// Partially duplicates GenerateCreateDBInstanceInput - make sure any relevant changes are applied there too.
	return &svcsdk.RestoreDBInstanceFromDBSnapshotInput{
		DBInstanceIdentifier:            aws.String(name),
		AutoMinorVersionUpgrade:         p.AutoMinorVersionUpgrade,
		AvailabilityZone:                p.AvailabilityZone,
		CopyTagsToSnapshot:              p.CopyTagsToSnapshot,
		DBInstanceClass:                 p.DBInstanceClass,
		DBParameterGroupName:            p.DBParameterGroupName,
		DBSnapshotIdentifier:            p.RestoreFrom.Snapshot.SnapshotIdentifier,
		DBSubnetGroupName:               p.DBSubnetGroupName,
		DeletionProtection:              p.DeletionProtection,
		Domain:                          p.Domain,
		DomainIAMRoleName:               p.DomainIAMRoleName,
		EnableCloudwatchLogsExports:     aws.ToStringSlice(p.EnableCloudwatchLogsExports),
		EnableIAMDatabaseAuthentication: p.EnableIAMDatabaseAuthentication,
		Engine:                          p.Engine,
		Iops:                            pointer.Int32(p.IOPS),
		LicenseModel:                    p.LicenseModel,
		MultiAZ:                         p.MultiAZ,
		OptionGroupName:                 p.OptionGroupName,
		Port:                            pointer.Int32(p.Port),
		PubliclyAccessible:              p.PubliclyAccessible,
		StorageType:                     p.StorageType,
		VpcSecurityGroupIds:             p.VPCSecurityGroupIDs,
		ProcessorFeatures:               ProcessorFeatures(p),
		Tags:                            Tags(p),
	}
}

// GenerateRestoreDBInstanceToPointInTimeInput from RDSInstanceSpec
func GenerateRestoreDBInstanceToPointInTimeInput(name string, p *v1alpha1.DBInstanceParameters) *svcsdk.RestoreDBInstanceToPointInTimeInput {
	// Partially duplicates GenerateCreateDBInstanceInput - make sure any relevant changes are applied there too.
	var restoreTime *time.Time
	if p.RestoreFrom.PointInTime.RestoreTime != nil {
		t := p.RestoreFrom.PointInTime.RestoreTime.UTC()
		restoreTime = &t
	}
	return &svcsdk.RestoreDBInstanceToPointInTimeInput{
		AutoMinorVersionUpgrade:         p.AutoMinorVersionUpgrade,
		AvailabilityZone:                p.AvailabilityZone,
		CopyTagsToSnapshot:              p.CopyTagsToSnapshot,
		DBInstanceClass:                 p.DBInstanceClass,
		DBName:                          p.DBName,
		DBParameterGroupName:            p.DBParameterGroupName,
		DBSubnetGroupName:               p.DBSubnetGroupName,
		DeletionProtection:              p.DeletionProtection,
		Domain:                          p.Domain,
		DomainIAMRoleName:               p.DomainIAMRoleName,
		EnableCloudwatchLogsExports:     aws.ToStringSlice(p.EnableCloudwatchLogsExports),
		EnableIAMDatabaseAuthentication: p.EnableIAMDatabaseAuthentication,
		Engine:                          p.Engine,
		Iops:                            pointer.Int32(p.IOPS),
		LicenseModel:                    p.LicenseModel,
		MultiAZ:                         p.MultiAZ,
		OptionGroupName:                 p.OptionGroupName,
		Port:                            pointer.Int32(p.Port),
		PubliclyAccessible:              p.PubliclyAccessible,
		StorageType:                     p.StorageType,
		VpcSecurityGroupIds:             p.VPCSecurityGroupIDs,
		ProcessorFeatures:               ProcessorFeatures(p),
		Tags:                            Tags(p),

		TargetDBInstanceIdentifier:          aws.String(name),
		RestoreTime:                         restoreTime,
		UseLatestRestorableTime:             aws.Bool(p.RestoreFrom.PointInTime.UseLatestRestorableTime),
		SourceDBInstanceAutomatedBackupsArn: p.RestoreFrom.PointInTime.SourceDBInstanceAutomatedBackupsArn,
		SourceDBInstanceIdentifier:          p.RestoreFrom.PointInTime.SourceDBInstanceIdentifier,
		SourceDbiResourceId:                 p.RestoreFrom.PointInTime.SourceDbiResourceID,
	}
}

// GenerateCreateDBInstanceReadReplicaInput returns a create input.
func GenerateCreateDBInstanceReadReplicaInput(cr *v1alpha1.DBInstance) *svcsdk.CreateDBInstanceReadReplicaInput {
	p := &cr.Spec.ForProvider
	return &svcsdk.CreateDBInstanceReadReplicaInput{
		AllocatedStorage:                   pointer.Int32(p.AllocatedStorage),
		AutoMinorVersionUpgrade:            p.AutoMinorVersionUpgrade,
		AvailabilityZone:                   p.AvailabilityZone,
		CopyTagsToSnapshot:                 p.CopyTagsToSnapshot,
		CustomIamInstanceProfile:           p.CustomIAMInstanceProfile,
		DBInstanceClass:                    p.DBInstanceClass,
		DBParameterGroupName:               p.DBParameterGroupName,
		DBSubnetGroupName:                  p.DBSubnetGroupName,
		DedicatedLogVolume:                 p.DedicatedLogVolume,
		DeletionProtection:                 p.DeletionProtection,
		Domain:                             p.Domain,
		DomainAuthSecretArn:                p.DomainAuthSecretARN,
		DomainDnsIps:                       aws.ToStringSlice(p.DomainDNSIPs),
		DomainFqdn:                         p.DomainFqdn,
		DomainIAMRoleName:                  p.DomainIAMRoleName,
		DomainOu:                           p.DomainOu,
		EnableCloudwatchLogsExports:        aws.ToStringSlice(p.EnableCloudwatchLogsExports),
		EnableCustomerOwnedIp:              p.EnableCustomerOwnedIP,
		EnableIAMDatabaseAuthentication:    p.EnableIAMDatabaseAuthentication,
		EnablePerformanceInsights:          p.EnablePerformanceInsights,
		Iops:                               pointer.Int32(p.IOPS),
		KmsKeyId:                           p.KMSKeyID,
		MaxAllocatedStorage:                pointer.Int32(p.MaxAllocatedStorage),
		MonitoringInterval:                 pointer.Int32(p.MonitoringInterval),
		MonitoringRoleArn:                  p.MonitoringRoleARN,
		MultiAZ:                            p.MultiAZ,
		NetworkType:                        p.NetworkType,
		OptionGroupName:                    p.OptionGroupName,
		PerformanceInsightsKMSKeyId:        p.PerformanceInsightsKMSKeyID,
		PerformanceInsightsRetentionPeriod: pointer.Int32(p.PerformanceInsightsRetentionPeriod),
		Port:                               pointer.Int32(p.Port),
		ProcessorFeatures:                  ProcessorFeatures(p),
		PubliclyAccessible:                 p.PubliclyAccessible,
		SourceDBClusterIdentifier:          p.SourceDBClusterID,
		SourceDBInstanceIdentifier:         p.SourceDBInstanceID,
		StorageThroughput:                  pointer.Int32(p.StorageThroughput),
		StorageType:                        p.StorageType,
		Tags:                               Tags(p),
		VpcSecurityGroupIds:                p.VPCSecurityGroupIDs,
	}
}

/*
Copyright 2023 The Crossplane Authors.

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

package utils

import (
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
)

// SetPmvDBInstance folds any PendingModifiedValues in the DescribeDBInstancesOutput into the
// observed fields, so that they are taken into account during isUpToDate checks.
func SetPmvDBInstance(obs *svcsdk.DescribeDBInstancesOutput) { //nolint:gocyclo
	if len(obs.DBInstances) == 0 || obs.DBInstances[0].PendingModifiedValues == nil {
		return
	}
	db := &obs.DBInstances[0]
	pmv := db.PendingModifiedValues
	if pmv.AllocatedStorage != nil {
		db.AllocatedStorage = pmv.AllocatedStorage
	}
	if pmv.AutomationMode != "" {
		db.AutomationMode = pmv.AutomationMode
	}
	if pmv.BackupRetentionPeriod != nil {
		db.BackupRetentionPeriod = pmv.BackupRetentionPeriod
	}
	if pmv.CACertificateIdentifier != nil {
		db.CACertificateIdentifier = pmv.CACertificateIdentifier
	}
	if pmv.DBInstanceClass != nil {
		db.DBInstanceClass = pmv.DBInstanceClass
	}
	if pmv.DBSubnetGroupName != nil {
		db.DBSubnetGroup = &svcsdktypes.DBSubnetGroup{DBSubnetGroupName: pmv.DBSubnetGroupName}
	}
	if pmv.DedicatedLogVolume != nil {
		db.DedicatedLogVolume = pmv.DedicatedLogVolume
	}
	if pmv.Iops != nil {
		db.Iops = pmv.Iops
	}
	if pmv.LicenseModel != nil {
		db.LicenseModel = pmv.LicenseModel
	}
	if pmv.MultiAZ != nil {
		db.MultiAZ = pmv.MultiAZ
	}
	if pmv.MultiTenant != nil {
		db.MultiTenant = pmv.MultiTenant
	}
	db.AdditionalStorageVolumes = applyPendingAdditionalStorageVolumes(db.AdditionalStorageVolumes, pmv.AdditionalStorageVolumes)
	if pmv.PendingCloudwatchLogsExports != nil {
		db.EnabledCloudwatchLogsExports = applyPendingCloudwatchLogsExports(db.EnabledCloudwatchLogsExports, pmv.PendingCloudwatchLogsExports)
	}
	if pmv.Port != nil {
		if db.Endpoint == nil {
			db.Endpoint = &svcsdktypes.Endpoint{}
		}
		db.Endpoint.Port = pmv.Port
		db.DbInstancePort = pmv.Port
	}
	if pmv.ProcessorFeatures != nil {
		db.ProcessorFeatures = pmv.ProcessorFeatures
	}
	if pmv.StorageThroughput != nil {
		db.StorageThroughput = pmv.StorageThroughput
	}
	if pmv.StorageType != nil {
		db.StorageType = pmv.StorageType
	}
}

// applyPendingAdditionalStorageVolumes overlays the pending values of each
// volume (matched by name) onto the observed volumes.
func applyPendingAdditionalStorageVolumes(observed []svcsdktypes.AdditionalStorageVolumeOutput, pending []svcsdktypes.AdditionalStorageVolume) []svcsdktypes.AdditionalStorageVolumeOutput {
	for _, p := range pending {
		i := slices.IndexFunc(observed, func(o svcsdktypes.AdditionalStorageVolumeOutput) bool {
			return aws.ToString(o.VolumeName) == aws.ToString(p.VolumeName)
		})
		if i < 0 {
			observed = append(observed, svcsdktypes.AdditionalStorageVolumeOutput{VolumeName: p.VolumeName})
			i = len(observed) - 1
		}
		o := &observed[i]
		if p.AllocatedStorage != nil {
			o.AllocatedStorage = p.AllocatedStorage
		}
		if p.IOPS != nil {
			o.IOPS = p.IOPS
		}
		if p.MaxAllocatedStorage != nil {
			o.MaxAllocatedStorage = p.MaxAllocatedStorage
		}
		if p.StorageThroughput != nil {
			o.StorageThroughput = p.StorageThroughput
		}
		if p.StorageType != nil {
			o.StorageType = p.StorageType
		}
	}
	return observed
}

func applyPendingCloudwatchLogsExports(enabled []string, pending *svcsdktypes.PendingCloudwatchLogsExports) []string {
	if pending.LogTypesToDisable != nil {
		enabled = slices.DeleteFunc(slices.Clone(enabled), func(s string) bool {
			return slices.Contains(pending.LogTypesToDisable, s)
		})
	}
	for _, e := range pending.LogTypesToEnable {
		if !slices.Contains(enabled, e) {
			enabled = append(enabled, e)
		}
	}
	return enabled
}

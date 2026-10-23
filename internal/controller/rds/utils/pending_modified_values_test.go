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

package utils

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestSetPmvDBInstance(t *testing.T) {
	out := &svcsdk.DescribeDBInstancesOutput{DBInstances: []svcsdktypes.DBInstance{{
		AllocatedStorage:             aws.Int32(20),
		EnabledCloudwatchLogsExports: []string{"audit", "error"},
		AdditionalStorageVolumes:     []svcsdktypes.AdditionalStorageVolumeOutput{{VolumeName: aws.String("v1"), AllocatedStorage: aws.Int32(10)}},
		PendingModifiedValues: &svcsdktypes.PendingModifiedValues{
			AllocatedStorage:        aws.Int32(50),
			AutomationMode:          svcsdktypes.AutomationModeFull,
			BackupRetentionPeriod:   aws.Int32(7),
			CACertificateIdentifier: aws.String("ca"),
			DBInstanceClass:         aws.String("db.t3.small"),
			DBSubnetGroupName:       aws.String("subnets"),
			DedicatedLogVolume:      aws.Bool(true),
			Iops:                    aws.Int32(3000),
			LicenseModel:            aws.String("lm"),
			MultiAZ:                 aws.Bool(true),
			MultiTenant:             aws.Bool(true),
			Port:                    aws.Int32(5433),
			ProcessorFeatures:       []svcsdktypes.ProcessorFeature{{Name: aws.String("coreCount")}},
			StorageThroughput:       aws.Int32(125),
			StorageType:             aws.String("gp3"),
			AdditionalStorageVolumes: []svcsdktypes.AdditionalStorageVolume{
				{VolumeName: aws.String("v1"), AllocatedStorage: aws.Int32(30), IOPS: aws.Int32(1), MaxAllocatedStorage: aws.Int32(2), StorageThroughput: aws.Int32(3), StorageType: aws.String("io2")},
				{VolumeName: aws.String("v2")},
			},
			PendingCloudwatchLogsExports: &svcsdktypes.PendingCloudwatchLogsExports{LogTypesToDisable: []string{"audit"}, LogTypesToEnable: []string{"general", "error"}},
		},
	}}}
	SetPmvDBInstance(out)
	db := out.DBInstances[0]

	if aws.ToInt32(db.AllocatedStorage) != 50 || aws.ToString(db.DBInstanceClass) != "db.t3.small" || aws.ToString(db.DBSubnetGroup.DBSubnetGroupName) != "subnets" ||
		aws.ToInt32(db.Endpoint.Port) != 5433 || aws.ToInt32(db.DbInstancePort) != 5433 || aws.ToString(db.StorageType) != "gp3" || !aws.ToBool(db.MultiAZ) {
		t.Errorf("pending values not applied: %+v", db)
	}
	if diff := cmp.Diff([]string{"error", "general"}, db.EnabledCloudwatchLogsExports); diff != "" {
		t.Errorf("log exports: -want, +got:\n%s", diff)
	}
	wantVolumes := []svcsdktypes.AdditionalStorageVolumeOutput{
		{VolumeName: aws.String("v1"), AllocatedStorage: aws.Int32(30), IOPS: aws.Int32(1), MaxAllocatedStorage: aws.Int32(2), StorageThroughput: aws.Int32(3), StorageType: aws.String("io2")},
		{VolumeName: aws.String("v2")},
	}
	if diff := cmp.Diff(wantVolumes, db.AdditionalStorageVolumes, cmpopts.IgnoreUnexported(svcsdktypes.AdditionalStorageVolumeOutput{})); diff != "" {
		t.Errorf("volumes: -want, +got:\n%s", diff)
	}
}

func TestSetPmvDBInstanceWithoutPending(t *testing.T) {
	SetPmvDBInstance(&svcsdk.DescribeDBInstancesOutput{})
	out := &svcsdk.DescribeDBInstancesOutput{DBInstances: []svcsdktypes.DBInstance{{AllocatedStorage: aws.Int32(20)}}}
	SetPmvDBInstance(out)
	if aws.ToInt32(out.DBInstances[0].AllocatedStorage) != 20 {
		t.Error("without pending values nothing may change")
	}
}

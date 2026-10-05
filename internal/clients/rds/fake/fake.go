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

// Package fake provides a mock RDS client for tests.
package fake

import (
	"context"

	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
)

// MockRDSClient is a mock RDS client. Unset mock functions return empty
// outputs.
type MockRDSClient struct {
	MockListTagsForResource             func(ctx context.Context, input *svcsdk.ListTagsForResourceInput, optFns ...func(*svcsdk.Options)) (*svcsdk.ListTagsForResourceOutput, error)
	MockDescribeDBInstances             func(ctx context.Context, input *svcsdk.DescribeDBInstancesInput, optFns ...func(*svcsdk.Options)) (*svcsdk.DescribeDBInstancesOutput, error)
	MockCreateDBInstance                func(ctx context.Context, input *svcsdk.CreateDBInstanceInput, optFns ...func(*svcsdk.Options)) (*svcsdk.CreateDBInstanceOutput, error)
	MockModifyDBInstance                func(ctx context.Context, input *svcsdk.ModifyDBInstanceInput, optFns ...func(*svcsdk.Options)) (*svcsdk.ModifyDBInstanceOutput, error)
	MockDeleteDBInstance                func(ctx context.Context, input *svcsdk.DeleteDBInstanceInput, optFns ...func(*svcsdk.Options)) (*svcsdk.DeleteDBInstanceOutput, error)
	MockAddTagsToResource               func(ctx context.Context, input *svcsdk.AddTagsToResourceInput, optFns ...func(*svcsdk.Options)) (*svcsdk.AddTagsToResourceOutput, error)
	MockCreateDBInstanceReadReplica     func(ctx context.Context, input *svcsdk.CreateDBInstanceReadReplicaInput, optFns ...func(*svcsdk.Options)) (*svcsdk.CreateDBInstanceReadReplicaOutput, error)
	MockDescribeDBClusters              func(ctx context.Context, input *svcsdk.DescribeDBClustersInput, optFns ...func(*svcsdk.Options)) (*svcsdk.DescribeDBClustersOutput, error)
	MockRemoveTagsFromResource          func(ctx context.Context, input *svcsdk.RemoveTagsFromResourceInput, optFns ...func(*svcsdk.Options)) (*svcsdk.RemoveTagsFromResourceOutput, error)
	MockRestoreDBInstanceFromDBSnapshot func(ctx context.Context, input *svcsdk.RestoreDBInstanceFromDBSnapshotInput, optFns ...func(*svcsdk.Options)) (*svcsdk.RestoreDBInstanceFromDBSnapshotOutput, error)
	MockRestoreDBInstanceFromS3         func(ctx context.Context, input *svcsdk.RestoreDBInstanceFromS3Input, optFns ...func(*svcsdk.Options)) (*svcsdk.RestoreDBInstanceFromS3Output, error)
	MockRestoreDBInstanceToPointInTime  func(ctx context.Context, input *svcsdk.RestoreDBInstanceToPointInTimeInput, optFns ...func(*svcsdk.Options)) (*svcsdk.RestoreDBInstanceToPointInTimeOutput, error)
}

// DescribeDBInstances calls MockDescribeDBInstances.
func (m *MockRDSClient) DescribeDBInstances(ctx context.Context, input *svcsdk.DescribeDBInstancesInput, optFns ...func(*svcsdk.Options)) (*svcsdk.DescribeDBInstancesOutput, error) {
	if m.MockDescribeDBInstances == nil {
		return &svcsdk.DescribeDBInstancesOutput{}, nil
	}
	return m.MockDescribeDBInstances(ctx, input, optFns...)
}

// CreateDBInstance calls MockCreateDBInstance.
func (m *MockRDSClient) CreateDBInstance(ctx context.Context, input *svcsdk.CreateDBInstanceInput, optFns ...func(*svcsdk.Options)) (*svcsdk.CreateDBInstanceOutput, error) {
	if m.MockCreateDBInstance == nil {
		return &svcsdk.CreateDBInstanceOutput{}, nil
	}
	return m.MockCreateDBInstance(ctx, input, optFns...)
}

// ModifyDBInstance calls MockModifyDBInstance.
func (m *MockRDSClient) ModifyDBInstance(ctx context.Context, input *svcsdk.ModifyDBInstanceInput, optFns ...func(*svcsdk.Options)) (*svcsdk.ModifyDBInstanceOutput, error) {
	if m.MockModifyDBInstance == nil {
		return &svcsdk.ModifyDBInstanceOutput{}, nil
	}
	return m.MockModifyDBInstance(ctx, input, optFns...)
}

// DeleteDBInstance calls MockDeleteDBInstance.
func (m *MockRDSClient) DeleteDBInstance(ctx context.Context, input *svcsdk.DeleteDBInstanceInput, optFns ...func(*svcsdk.Options)) (*svcsdk.DeleteDBInstanceOutput, error) {
	if m.MockDeleteDBInstance == nil {
		return &svcsdk.DeleteDBInstanceOutput{}, nil
	}
	return m.MockDeleteDBInstance(ctx, input, optFns...)
}

// AddTagsToResource calls MockAddTagsToResource.
func (m *MockRDSClient) AddTagsToResource(ctx context.Context, input *svcsdk.AddTagsToResourceInput, optFns ...func(*svcsdk.Options)) (*svcsdk.AddTagsToResourceOutput, error) {
	if m.MockAddTagsToResource == nil {
		return &svcsdk.AddTagsToResourceOutput{}, nil
	}
	return m.MockAddTagsToResource(ctx, input, optFns...)
}

// CreateDBInstanceReadReplica calls MockCreateDBInstanceReadReplica.
func (m *MockRDSClient) CreateDBInstanceReadReplica(ctx context.Context, input *svcsdk.CreateDBInstanceReadReplicaInput, optFns ...func(*svcsdk.Options)) (*svcsdk.CreateDBInstanceReadReplicaOutput, error) {
	if m.MockCreateDBInstanceReadReplica == nil {
		return &svcsdk.CreateDBInstanceReadReplicaOutput{}, nil
	}
	return m.MockCreateDBInstanceReadReplica(ctx, input, optFns...)
}

// DescribeDBClusters calls MockDescribeDBClusters.
func (m *MockRDSClient) DescribeDBClusters(ctx context.Context, input *svcsdk.DescribeDBClustersInput, optFns ...func(*svcsdk.Options)) (*svcsdk.DescribeDBClustersOutput, error) {
	if m.MockDescribeDBClusters == nil {
		return &svcsdk.DescribeDBClustersOutput{}, nil
	}
	return m.MockDescribeDBClusters(ctx, input, optFns...)
}

// RemoveTagsFromResource calls MockRemoveTagsFromResource.
func (m *MockRDSClient) RemoveTagsFromResource(ctx context.Context, input *svcsdk.RemoveTagsFromResourceInput, optFns ...func(*svcsdk.Options)) (*svcsdk.RemoveTagsFromResourceOutput, error) {
	if m.MockRemoveTagsFromResource == nil {
		return &svcsdk.RemoveTagsFromResourceOutput{}, nil
	}
	return m.MockRemoveTagsFromResource(ctx, input, optFns...)
}

// RestoreDBInstanceFromDBSnapshot calls MockRestoreDBInstanceFromDBSnapshot.
func (m *MockRDSClient) RestoreDBInstanceFromDBSnapshot(ctx context.Context, input *svcsdk.RestoreDBInstanceFromDBSnapshotInput, optFns ...func(*svcsdk.Options)) (*svcsdk.RestoreDBInstanceFromDBSnapshotOutput, error) {
	if m.MockRestoreDBInstanceFromDBSnapshot == nil {
		return &svcsdk.RestoreDBInstanceFromDBSnapshotOutput{}, nil
	}
	return m.MockRestoreDBInstanceFromDBSnapshot(ctx, input, optFns...)
}

// RestoreDBInstanceFromS3 calls MockRestoreDBInstanceFromS3.
func (m *MockRDSClient) RestoreDBInstanceFromS3(ctx context.Context, input *svcsdk.RestoreDBInstanceFromS3Input, optFns ...func(*svcsdk.Options)) (*svcsdk.RestoreDBInstanceFromS3Output, error) {
	if m.MockRestoreDBInstanceFromS3 == nil {
		return &svcsdk.RestoreDBInstanceFromS3Output{}, nil
	}
	return m.MockRestoreDBInstanceFromS3(ctx, input, optFns...)
}

// RestoreDBInstanceToPointInTime calls MockRestoreDBInstanceToPointInTime.
func (m *MockRDSClient) RestoreDBInstanceToPointInTime(ctx context.Context, input *svcsdk.RestoreDBInstanceToPointInTimeInput, optFns ...func(*svcsdk.Options)) (*svcsdk.RestoreDBInstanceToPointInTimeOutput, error) {
	if m.MockRestoreDBInstanceToPointInTime == nil {
		return &svcsdk.RestoreDBInstanceToPointInTimeOutput{}, nil
	}
	return m.MockRestoreDBInstanceToPointInTime(ctx, input, optFns...)
}

// ListTagsForResource calls MockListTagsForResource.
func (m *MockRDSClient) ListTagsForResource(ctx context.Context, input *svcsdk.ListTagsForResourceInput, optFns ...func(*svcsdk.Options)) (*svcsdk.ListTagsForResourceOutput, error) {
	if m.MockListTagsForResource == nil {
		return &svcsdk.ListTagsForResourceOutput{}, nil
	}
	return m.MockListTagsForResource(ctx, input, optFns...)
}

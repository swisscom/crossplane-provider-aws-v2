/*
Copyright 2021 The Crossplane Authors.

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

// Package fake provides a mock DocumentDB client for tests.
package fake

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/docdb"
)

// MockDocDBClient is a mock DocumentDB client that records its calls in
// Called. Unset mock functions return empty outputs.
type MockDocDBClient struct {
	MockListTagsForResource              func(context.Context, *docdb.ListTagsForResourceInput, ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error)
	MockAddTagsToResource                func(context.Context, *docdb.AddTagsToResourceInput, ...func(*docdb.Options)) (*docdb.AddTagsToResourceOutput, error)
	MockRemoveTagsFromResource           func(context.Context, *docdb.RemoveTagsFromResourceInput, ...func(*docdb.Options)) (*docdb.RemoveTagsFromResourceOutput, error)
	MockDescribeDBInstances              func(context.Context, *docdb.DescribeDBInstancesInput, ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error)
	MockCreateDBInstance                 func(context.Context, *docdb.CreateDBInstanceInput, ...func(*docdb.Options)) (*docdb.CreateDBInstanceOutput, error)
	MockModifyDBInstance                 func(context.Context, *docdb.ModifyDBInstanceInput, ...func(*docdb.Options)) (*docdb.ModifyDBInstanceOutput, error)
	MockDeleteDBInstance                 func(context.Context, *docdb.DeleteDBInstanceInput, ...func(*docdb.Options)) (*docdb.DeleteDBInstanceOutput, error)
	MockDescribeDBClusters               func(context.Context, *docdb.DescribeDBClustersInput, ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error)
	MockCreateDBCluster                  func(context.Context, *docdb.CreateDBClusterInput, ...func(*docdb.Options)) (*docdb.CreateDBClusterOutput, error)
	MockModifyDBCluster                  func(context.Context, *docdb.ModifyDBClusterInput, ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error)
	MockDeleteDBCluster                  func(context.Context, *docdb.DeleteDBClusterInput, ...func(*docdb.Options)) (*docdb.DeleteDBClusterOutput, error)
	MockRestoreDBClusterFromSnapshot     func(context.Context, *docdb.RestoreDBClusterFromSnapshotInput, ...func(*docdb.Options)) (*docdb.RestoreDBClusterFromSnapshotOutput, error)
	MockRestoreDBClusterToPointInTime    func(context.Context, *docdb.RestoreDBClusterToPointInTimeInput, ...func(*docdb.Options)) (*docdb.RestoreDBClusterToPointInTimeOutput, error)
	MockDescribeDBSubnetGroups           func(context.Context, *docdb.DescribeDBSubnetGroupsInput, ...func(*docdb.Options)) (*docdb.DescribeDBSubnetGroupsOutput, error)
	MockCreateDBSubnetGroup              func(context.Context, *docdb.CreateDBSubnetGroupInput, ...func(*docdb.Options)) (*docdb.CreateDBSubnetGroupOutput, error)
	MockModifyDBSubnetGroup              func(context.Context, *docdb.ModifyDBSubnetGroupInput, ...func(*docdb.Options)) (*docdb.ModifyDBSubnetGroupOutput, error)
	MockDeleteDBSubnetGroup              func(context.Context, *docdb.DeleteDBSubnetGroupInput, ...func(*docdb.Options)) (*docdb.DeleteDBSubnetGroupOutput, error)
	MockDescribeDBClusterParameterGroups func(context.Context, *docdb.DescribeDBClusterParameterGroupsInput, ...func(*docdb.Options)) (*docdb.DescribeDBClusterParameterGroupsOutput, error)
	MockDescribeDBClusterParameters      func(context.Context, *docdb.DescribeDBClusterParametersInput, ...func(*docdb.Options)) (*docdb.DescribeDBClusterParametersOutput, error)
	MockCreateDBClusterParameterGroup    func(context.Context, *docdb.CreateDBClusterParameterGroupInput, ...func(*docdb.Options)) (*docdb.CreateDBClusterParameterGroupOutput, error)
	MockModifyDBClusterParameterGroup    func(context.Context, *docdb.ModifyDBClusterParameterGroupInput, ...func(*docdb.Options)) (*docdb.ModifyDBClusterParameterGroupOutput, error)
	MockDeleteDBClusterParameterGroup    func(context.Context, *docdb.DeleteDBClusterParameterGroupInput, ...func(*docdb.Options)) (*docdb.DeleteDBClusterParameterGroupOutput, error)

	Called MockDocDBClientCall
}

// MockDocDBClientCall records the calls of a MockDocDBClient.
type MockDocDBClientCall struct {
	ListTagsForResource              []*CallListTagsForResource
	AddTagsToResource                []*CallAddTagsToResource
	RemoveTagsFromResource           []*CallRemoveTagsFromResource
	DescribeDBInstances              []*CallDescribeDBInstances
	CreateDBInstance                 []*CallCreateDBInstance
	ModifyDBInstance                 []*CallModifyDBInstance
	DeleteDBInstance                 []*CallDeleteDBInstance
	DescribeDBClusters               []*CallDescribeDBClusters
	CreateDBCluster                  []*CallCreateDBCluster
	ModifyDBCluster                  []*CallModifyDBCluster
	DeleteDBCluster                  []*CallDeleteDBCluster
	RestoreDBClusterFromSnapshot     []*CallRestoreDBClusterFromSnapshot
	RestoreDBClusterToPointInTime    []*CallRestoreDBClusterToPointInTime
	DescribeDBSubnetGroups           []*CallDescribeDBSubnetGroups
	CreateDBSubnetGroup              []*CallCreateDBSubnetGroup
	ModifyDBSubnetGroup              []*CallModifyDBSubnetGroup
	DeleteDBSubnetGroup              []*CallDeleteDBSubnetGroup
	DescribeDBClusterParameterGroups []*CallDescribeDBClusterParameterGroups
	DescribeDBClusterParameters      []*CallDescribeDBClusterParameters
	CreateDBClusterParameterGroup    []*CallCreateDBClusterParameterGroup
	ModifyDBClusterParameterGroup    []*CallModifyDBClusterParameterGroup
	DeleteDBClusterParameterGroup    []*CallDeleteDBClusterParameterGroup
}

// CallListTagsForResource is a call of ListTagsForResource.
type CallListTagsForResource struct {
	Ctx  context.Context
	I    *docdb.ListTagsForResourceInput
	Opts []func(*docdb.Options)
}

// ListTagsForResource records the call and calls MockListTagsForResource.
func (m *MockDocDBClient) ListTagsForResource(ctx context.Context, i *docdb.ListTagsForResourceInput, opts ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
	m.Called.ListTagsForResource = append(m.Called.ListTagsForResource, &CallListTagsForResource{Ctx: ctx, I: i, Opts: opts})
	if m.MockListTagsForResource == nil {
		return &docdb.ListTagsForResourceOutput{}, nil
	}
	return m.MockListTagsForResource(ctx, i, opts...)
}

// CallAddTagsToResource is a call of AddTagsToResource.
type CallAddTagsToResource struct {
	Ctx  context.Context
	I    *docdb.AddTagsToResourceInput
	Opts []func(*docdb.Options)
}

// AddTagsToResource records the call and calls MockAddTagsToResource.
func (m *MockDocDBClient) AddTagsToResource(ctx context.Context, i *docdb.AddTagsToResourceInput, opts ...func(*docdb.Options)) (*docdb.AddTagsToResourceOutput, error) {
	m.Called.AddTagsToResource = append(m.Called.AddTagsToResource, &CallAddTagsToResource{Ctx: ctx, I: i, Opts: opts})
	if m.MockAddTagsToResource == nil {
		return &docdb.AddTagsToResourceOutput{}, nil
	}
	return m.MockAddTagsToResource(ctx, i, opts...)
}

// CallRemoveTagsFromResource is a call of RemoveTagsFromResource.
type CallRemoveTagsFromResource struct {
	Ctx  context.Context
	I    *docdb.RemoveTagsFromResourceInput
	Opts []func(*docdb.Options)
}

// RemoveTagsFromResource records the call and calls MockRemoveTagsFromResource.
func (m *MockDocDBClient) RemoveTagsFromResource(ctx context.Context, i *docdb.RemoveTagsFromResourceInput, opts ...func(*docdb.Options)) (*docdb.RemoveTagsFromResourceOutput, error) {
	m.Called.RemoveTagsFromResource = append(m.Called.RemoveTagsFromResource, &CallRemoveTagsFromResource{Ctx: ctx, I: i, Opts: opts})
	if m.MockRemoveTagsFromResource == nil {
		return &docdb.RemoveTagsFromResourceOutput{}, nil
	}
	return m.MockRemoveTagsFromResource(ctx, i, opts...)
}

// CallDescribeDBInstances is a call of DescribeDBInstances.
type CallDescribeDBInstances struct {
	Ctx  context.Context
	I    *docdb.DescribeDBInstancesInput
	Opts []func(*docdb.Options)
}

// DescribeDBInstances records the call and calls MockDescribeDBInstances.
func (m *MockDocDBClient) DescribeDBInstances(ctx context.Context, i *docdb.DescribeDBInstancesInput, opts ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
	m.Called.DescribeDBInstances = append(m.Called.DescribeDBInstances, &CallDescribeDBInstances{Ctx: ctx, I: i, Opts: opts})
	if m.MockDescribeDBInstances == nil {
		return &docdb.DescribeDBInstancesOutput{}, nil
	}
	return m.MockDescribeDBInstances(ctx, i, opts...)
}

// CallCreateDBInstance is a call of CreateDBInstance.
type CallCreateDBInstance struct {
	Ctx  context.Context
	I    *docdb.CreateDBInstanceInput
	Opts []func(*docdb.Options)
}

// CreateDBInstance records the call and calls MockCreateDBInstance.
func (m *MockDocDBClient) CreateDBInstance(ctx context.Context, i *docdb.CreateDBInstanceInput, opts ...func(*docdb.Options)) (*docdb.CreateDBInstanceOutput, error) {
	m.Called.CreateDBInstance = append(m.Called.CreateDBInstance, &CallCreateDBInstance{Ctx: ctx, I: i, Opts: opts})
	if m.MockCreateDBInstance == nil {
		return &docdb.CreateDBInstanceOutput{}, nil
	}
	return m.MockCreateDBInstance(ctx, i, opts...)
}

// CallModifyDBInstance is a call of ModifyDBInstance.
type CallModifyDBInstance struct {
	Ctx  context.Context
	I    *docdb.ModifyDBInstanceInput
	Opts []func(*docdb.Options)
}

// ModifyDBInstance records the call and calls MockModifyDBInstance.
func (m *MockDocDBClient) ModifyDBInstance(ctx context.Context, i *docdb.ModifyDBInstanceInput, opts ...func(*docdb.Options)) (*docdb.ModifyDBInstanceOutput, error) {
	m.Called.ModifyDBInstance = append(m.Called.ModifyDBInstance, &CallModifyDBInstance{Ctx: ctx, I: i, Opts: opts})
	if m.MockModifyDBInstance == nil {
		return &docdb.ModifyDBInstanceOutput{}, nil
	}
	return m.MockModifyDBInstance(ctx, i, opts...)
}

// CallDeleteDBInstance is a call of DeleteDBInstance.
type CallDeleteDBInstance struct {
	Ctx  context.Context
	I    *docdb.DeleteDBInstanceInput
	Opts []func(*docdb.Options)
}

// DeleteDBInstance records the call and calls MockDeleteDBInstance.
func (m *MockDocDBClient) DeleteDBInstance(ctx context.Context, i *docdb.DeleteDBInstanceInput, opts ...func(*docdb.Options)) (*docdb.DeleteDBInstanceOutput, error) {
	m.Called.DeleteDBInstance = append(m.Called.DeleteDBInstance, &CallDeleteDBInstance{Ctx: ctx, I: i, Opts: opts})
	if m.MockDeleteDBInstance == nil {
		return &docdb.DeleteDBInstanceOutput{}, nil
	}
	return m.MockDeleteDBInstance(ctx, i, opts...)
}

// CallDescribeDBClusters is a call of DescribeDBClusters.
type CallDescribeDBClusters struct {
	Ctx  context.Context
	I    *docdb.DescribeDBClustersInput
	Opts []func(*docdb.Options)
}

// DescribeDBClusters records the call and calls MockDescribeDBClusters.
func (m *MockDocDBClient) DescribeDBClusters(ctx context.Context, i *docdb.DescribeDBClustersInput, opts ...func(*docdb.Options)) (*docdb.DescribeDBClustersOutput, error) {
	m.Called.DescribeDBClusters = append(m.Called.DescribeDBClusters, &CallDescribeDBClusters{Ctx: ctx, I: i, Opts: opts})
	if m.MockDescribeDBClusters == nil {
		return &docdb.DescribeDBClustersOutput{}, nil
	}
	return m.MockDescribeDBClusters(ctx, i, opts...)
}

// CallCreateDBCluster is a call of CreateDBCluster.
type CallCreateDBCluster struct {
	Ctx  context.Context
	I    *docdb.CreateDBClusterInput
	Opts []func(*docdb.Options)
}

// CreateDBCluster records the call and calls MockCreateDBCluster.
func (m *MockDocDBClient) CreateDBCluster(ctx context.Context, i *docdb.CreateDBClusterInput, opts ...func(*docdb.Options)) (*docdb.CreateDBClusterOutput, error) {
	m.Called.CreateDBCluster = append(m.Called.CreateDBCluster, &CallCreateDBCluster{Ctx: ctx, I: i, Opts: opts})
	if m.MockCreateDBCluster == nil {
		return &docdb.CreateDBClusterOutput{}, nil
	}
	return m.MockCreateDBCluster(ctx, i, opts...)
}

// CallModifyDBCluster is a call of ModifyDBCluster.
type CallModifyDBCluster struct {
	Ctx  context.Context
	I    *docdb.ModifyDBClusterInput
	Opts []func(*docdb.Options)
}

// ModifyDBCluster records the call and calls MockModifyDBCluster.
func (m *MockDocDBClient) ModifyDBCluster(ctx context.Context, i *docdb.ModifyDBClusterInput, opts ...func(*docdb.Options)) (*docdb.ModifyDBClusterOutput, error) {
	m.Called.ModifyDBCluster = append(m.Called.ModifyDBCluster, &CallModifyDBCluster{Ctx: ctx, I: i, Opts: opts})
	if m.MockModifyDBCluster == nil {
		return &docdb.ModifyDBClusterOutput{}, nil
	}
	return m.MockModifyDBCluster(ctx, i, opts...)
}

// CallDeleteDBCluster is a call of DeleteDBCluster.
type CallDeleteDBCluster struct {
	Ctx  context.Context
	I    *docdb.DeleteDBClusterInput
	Opts []func(*docdb.Options)
}

// DeleteDBCluster records the call and calls MockDeleteDBCluster.
func (m *MockDocDBClient) DeleteDBCluster(ctx context.Context, i *docdb.DeleteDBClusterInput, opts ...func(*docdb.Options)) (*docdb.DeleteDBClusterOutput, error) {
	m.Called.DeleteDBCluster = append(m.Called.DeleteDBCluster, &CallDeleteDBCluster{Ctx: ctx, I: i, Opts: opts})
	if m.MockDeleteDBCluster == nil {
		return &docdb.DeleteDBClusterOutput{}, nil
	}
	return m.MockDeleteDBCluster(ctx, i, opts...)
}

// CallRestoreDBClusterFromSnapshot is a call of RestoreDBClusterFromSnapshot.
type CallRestoreDBClusterFromSnapshot struct {
	Ctx  context.Context
	I    *docdb.RestoreDBClusterFromSnapshotInput
	Opts []func(*docdb.Options)
}

// RestoreDBClusterFromSnapshot records the call and calls MockRestoreDBClusterFromSnapshot.
func (m *MockDocDBClient) RestoreDBClusterFromSnapshot(ctx context.Context, i *docdb.RestoreDBClusterFromSnapshotInput, opts ...func(*docdb.Options)) (*docdb.RestoreDBClusterFromSnapshotOutput, error) {
	m.Called.RestoreDBClusterFromSnapshot = append(m.Called.RestoreDBClusterFromSnapshot, &CallRestoreDBClusterFromSnapshot{Ctx: ctx, I: i, Opts: opts})
	if m.MockRestoreDBClusterFromSnapshot == nil {
		return &docdb.RestoreDBClusterFromSnapshotOutput{}, nil
	}
	return m.MockRestoreDBClusterFromSnapshot(ctx, i, opts...)
}

// CallRestoreDBClusterToPointInTime is a call of RestoreDBClusterToPointInTime.
type CallRestoreDBClusterToPointInTime struct {
	Ctx  context.Context
	I    *docdb.RestoreDBClusterToPointInTimeInput
	Opts []func(*docdb.Options)
}

// RestoreDBClusterToPointInTime records the call and calls MockRestoreDBClusterToPointInTime.
func (m *MockDocDBClient) RestoreDBClusterToPointInTime(ctx context.Context, i *docdb.RestoreDBClusterToPointInTimeInput, opts ...func(*docdb.Options)) (*docdb.RestoreDBClusterToPointInTimeOutput, error) {
	m.Called.RestoreDBClusterToPointInTime = append(m.Called.RestoreDBClusterToPointInTime, &CallRestoreDBClusterToPointInTime{Ctx: ctx, I: i, Opts: opts})
	if m.MockRestoreDBClusterToPointInTime == nil {
		return &docdb.RestoreDBClusterToPointInTimeOutput{}, nil
	}
	return m.MockRestoreDBClusterToPointInTime(ctx, i, opts...)
}

// CallDescribeDBSubnetGroups is a call of DescribeDBSubnetGroups.
type CallDescribeDBSubnetGroups struct {
	Ctx  context.Context
	I    *docdb.DescribeDBSubnetGroupsInput
	Opts []func(*docdb.Options)
}

// DescribeDBSubnetGroups records the call and calls MockDescribeDBSubnetGroups.
func (m *MockDocDBClient) DescribeDBSubnetGroups(ctx context.Context, i *docdb.DescribeDBSubnetGroupsInput, opts ...func(*docdb.Options)) (*docdb.DescribeDBSubnetGroupsOutput, error) {
	m.Called.DescribeDBSubnetGroups = append(m.Called.DescribeDBSubnetGroups, &CallDescribeDBSubnetGroups{Ctx: ctx, I: i, Opts: opts})
	if m.MockDescribeDBSubnetGroups == nil {
		return &docdb.DescribeDBSubnetGroupsOutput{}, nil
	}
	return m.MockDescribeDBSubnetGroups(ctx, i, opts...)
}

// CallCreateDBSubnetGroup is a call of CreateDBSubnetGroup.
type CallCreateDBSubnetGroup struct {
	Ctx  context.Context
	I    *docdb.CreateDBSubnetGroupInput
	Opts []func(*docdb.Options)
}

// CreateDBSubnetGroup records the call and calls MockCreateDBSubnetGroup.
func (m *MockDocDBClient) CreateDBSubnetGroup(ctx context.Context, i *docdb.CreateDBSubnetGroupInput, opts ...func(*docdb.Options)) (*docdb.CreateDBSubnetGroupOutput, error) {
	m.Called.CreateDBSubnetGroup = append(m.Called.CreateDBSubnetGroup, &CallCreateDBSubnetGroup{Ctx: ctx, I: i, Opts: opts})
	if m.MockCreateDBSubnetGroup == nil {
		return &docdb.CreateDBSubnetGroupOutput{}, nil
	}
	return m.MockCreateDBSubnetGroup(ctx, i, opts...)
}

// CallModifyDBSubnetGroup is a call of ModifyDBSubnetGroup.
type CallModifyDBSubnetGroup struct {
	Ctx  context.Context
	I    *docdb.ModifyDBSubnetGroupInput
	Opts []func(*docdb.Options)
}

// ModifyDBSubnetGroup records the call and calls MockModifyDBSubnetGroup.
func (m *MockDocDBClient) ModifyDBSubnetGroup(ctx context.Context, i *docdb.ModifyDBSubnetGroupInput, opts ...func(*docdb.Options)) (*docdb.ModifyDBSubnetGroupOutput, error) {
	m.Called.ModifyDBSubnetGroup = append(m.Called.ModifyDBSubnetGroup, &CallModifyDBSubnetGroup{Ctx: ctx, I: i, Opts: opts})
	if m.MockModifyDBSubnetGroup == nil {
		return &docdb.ModifyDBSubnetGroupOutput{}, nil
	}
	return m.MockModifyDBSubnetGroup(ctx, i, opts...)
}

// CallDeleteDBSubnetGroup is a call of DeleteDBSubnetGroup.
type CallDeleteDBSubnetGroup struct {
	Ctx  context.Context
	I    *docdb.DeleteDBSubnetGroupInput
	Opts []func(*docdb.Options)
}

// DeleteDBSubnetGroup records the call and calls MockDeleteDBSubnetGroup.
func (m *MockDocDBClient) DeleteDBSubnetGroup(ctx context.Context, i *docdb.DeleteDBSubnetGroupInput, opts ...func(*docdb.Options)) (*docdb.DeleteDBSubnetGroupOutput, error) {
	m.Called.DeleteDBSubnetGroup = append(m.Called.DeleteDBSubnetGroup, &CallDeleteDBSubnetGroup{Ctx: ctx, I: i, Opts: opts})
	if m.MockDeleteDBSubnetGroup == nil {
		return &docdb.DeleteDBSubnetGroupOutput{}, nil
	}
	return m.MockDeleteDBSubnetGroup(ctx, i, opts...)
}

// CallDescribeDBClusterParameterGroups is a call of DescribeDBClusterParameterGroups.
type CallDescribeDBClusterParameterGroups struct {
	Ctx  context.Context
	I    *docdb.DescribeDBClusterParameterGroupsInput
	Opts []func(*docdb.Options)
}

// DescribeDBClusterParameterGroups records the call and calls MockDescribeDBClusterParameterGroups.
func (m *MockDocDBClient) DescribeDBClusterParameterGroups(ctx context.Context, i *docdb.DescribeDBClusterParameterGroupsInput, opts ...func(*docdb.Options)) (*docdb.DescribeDBClusterParameterGroupsOutput, error) {
	m.Called.DescribeDBClusterParameterGroups = append(m.Called.DescribeDBClusterParameterGroups, &CallDescribeDBClusterParameterGroups{Ctx: ctx, I: i, Opts: opts})
	if m.MockDescribeDBClusterParameterGroups == nil {
		return &docdb.DescribeDBClusterParameterGroupsOutput{}, nil
	}
	return m.MockDescribeDBClusterParameterGroups(ctx, i, opts...)
}

// CallDescribeDBClusterParameters is a call of DescribeDBClusterParameters.
type CallDescribeDBClusterParameters struct {
	Ctx  context.Context
	I    *docdb.DescribeDBClusterParametersInput
	Opts []func(*docdb.Options)
}

// DescribeDBClusterParameters records the call and calls MockDescribeDBClusterParameters.
func (m *MockDocDBClient) DescribeDBClusterParameters(ctx context.Context, i *docdb.DescribeDBClusterParametersInput, opts ...func(*docdb.Options)) (*docdb.DescribeDBClusterParametersOutput, error) {
	m.Called.DescribeDBClusterParameters = append(m.Called.DescribeDBClusterParameters, &CallDescribeDBClusterParameters{Ctx: ctx, I: i, Opts: opts})
	if m.MockDescribeDBClusterParameters == nil {
		return &docdb.DescribeDBClusterParametersOutput{}, nil
	}
	return m.MockDescribeDBClusterParameters(ctx, i, opts...)
}

// CallCreateDBClusterParameterGroup is a call of CreateDBClusterParameterGroup.
type CallCreateDBClusterParameterGroup struct {
	Ctx  context.Context
	I    *docdb.CreateDBClusterParameterGroupInput
	Opts []func(*docdb.Options)
}

// CreateDBClusterParameterGroup records the call and calls MockCreateDBClusterParameterGroup.
func (m *MockDocDBClient) CreateDBClusterParameterGroup(ctx context.Context, i *docdb.CreateDBClusterParameterGroupInput, opts ...func(*docdb.Options)) (*docdb.CreateDBClusterParameterGroupOutput, error) {
	m.Called.CreateDBClusterParameterGroup = append(m.Called.CreateDBClusterParameterGroup, &CallCreateDBClusterParameterGroup{Ctx: ctx, I: i, Opts: opts})
	if m.MockCreateDBClusterParameterGroup == nil {
		return &docdb.CreateDBClusterParameterGroupOutput{}, nil
	}
	return m.MockCreateDBClusterParameterGroup(ctx, i, opts...)
}

// CallModifyDBClusterParameterGroup is a call of ModifyDBClusterParameterGroup.
type CallModifyDBClusterParameterGroup struct {
	Ctx  context.Context
	I    *docdb.ModifyDBClusterParameterGroupInput
	Opts []func(*docdb.Options)
}

// ModifyDBClusterParameterGroup records the call and calls MockModifyDBClusterParameterGroup.
func (m *MockDocDBClient) ModifyDBClusterParameterGroup(ctx context.Context, i *docdb.ModifyDBClusterParameterGroupInput, opts ...func(*docdb.Options)) (*docdb.ModifyDBClusterParameterGroupOutput, error) {
	m.Called.ModifyDBClusterParameterGroup = append(m.Called.ModifyDBClusterParameterGroup, &CallModifyDBClusterParameterGroup{Ctx: ctx, I: i, Opts: opts})
	if m.MockModifyDBClusterParameterGroup == nil {
		return &docdb.ModifyDBClusterParameterGroupOutput{}, nil
	}
	return m.MockModifyDBClusterParameterGroup(ctx, i, opts...)
}

// CallDeleteDBClusterParameterGroup is a call of DeleteDBClusterParameterGroup.
type CallDeleteDBClusterParameterGroup struct {
	Ctx  context.Context
	I    *docdb.DeleteDBClusterParameterGroupInput
	Opts []func(*docdb.Options)
}

// DeleteDBClusterParameterGroup records the call and calls MockDeleteDBClusterParameterGroup.
func (m *MockDocDBClient) DeleteDBClusterParameterGroup(ctx context.Context, i *docdb.DeleteDBClusterParameterGroupInput, opts ...func(*docdb.Options)) (*docdb.DeleteDBClusterParameterGroupOutput, error) {
	m.Called.DeleteDBClusterParameterGroup = append(m.Called.DeleteDBClusterParameterGroup, &CallDeleteDBClusterParameterGroup{Ctx: ctx, I: i, Opts: opts})
	if m.MockDeleteDBClusterParameterGroup == nil {
		return &docdb.DeleteDBClusterParameterGroupOutput{}, nil
	}
	return m.MockDeleteDBClusterParameterGroup(ctx, i, opts...)
}

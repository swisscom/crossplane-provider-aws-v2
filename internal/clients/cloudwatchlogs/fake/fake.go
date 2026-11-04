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

// Package fake provides a mock CloudWatch Logs client for tests.
package fake

import (
	"context"

	svcsdk "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
)

// MockClient is a mock CloudWatch Logs client. Unset mock functions return
// empty outputs. Each call's input is appended to Calls under the operation
// name.
type MockClient struct {
	MockDescribeLogGroups             func(context.Context, *svcsdk.DescribeLogGroupsInput) (*svcsdk.DescribeLogGroupsOutput, error)
	MockCreateLogGroup                func(context.Context, *svcsdk.CreateLogGroupInput) (*svcsdk.CreateLogGroupOutput, error)
	MockDeleteLogGroup                func(context.Context, *svcsdk.DeleteLogGroupInput) (*svcsdk.DeleteLogGroupOutput, error)
	MockAssociateKmsKey               func(context.Context, *svcsdk.AssociateKmsKeyInput) (*svcsdk.AssociateKmsKeyOutput, error)
	MockDeleteRetentionPolicy         func(context.Context, *svcsdk.DeleteRetentionPolicyInput) (*svcsdk.DeleteRetentionPolicyOutput, error)
	MockDisassociateKmsKey            func(context.Context, *svcsdk.DisassociateKmsKeyInput) (*svcsdk.DisassociateKmsKeyOutput, error)
	MockListTagsForResource           func(context.Context, *svcsdk.ListTagsForResourceInput) (*svcsdk.ListTagsForResourceOutput, error)
	MockPutLogGroupDeletionProtection func(context.Context, *svcsdk.PutLogGroupDeletionProtectionInput) (*svcsdk.PutLogGroupDeletionProtectionOutput, error)
	MockPutRetentionPolicy            func(context.Context, *svcsdk.PutRetentionPolicyInput) (*svcsdk.PutRetentionPolicyOutput, error)
	MockTagResource                   func(context.Context, *svcsdk.TagResourceInput) (*svcsdk.TagResourceOutput, error)
	MockUntagResource                 func(context.Context, *svcsdk.UntagResourceInput) (*svcsdk.UntagResourceOutput, error)
	MockDescribeResourcePolicies      func(context.Context, *svcsdk.DescribeResourcePoliciesInput) (*svcsdk.DescribeResourcePoliciesOutput, error)
	MockPutResourcePolicy             func(context.Context, *svcsdk.PutResourcePolicyInput) (*svcsdk.PutResourcePolicyOutput, error)
	MockDeleteResourcePolicy          func(context.Context, *svcsdk.DeleteResourcePolicyInput) (*svcsdk.DeleteResourcePolicyOutput, error)

	Calls map[string][]any
}

func (m *MockClient) record(op string, in any) {
	if m.Calls == nil {
		m.Calls = map[string][]any{}
	}
	m.Calls[op] = append(m.Calls[op], in)
}

// DescribeLogGroups calls MockDescribeLogGroups.
func (m *MockClient) DescribeLogGroups(ctx context.Context, in *svcsdk.DescribeLogGroupsInput, _ ...func(*svcsdk.Options)) (*svcsdk.DescribeLogGroupsOutput, error) {
	m.record("DescribeLogGroups", in)
	if m.MockDescribeLogGroups == nil {
		return &svcsdk.DescribeLogGroupsOutput{}, nil
	}
	return m.MockDescribeLogGroups(ctx, in)
}

// CreateLogGroup calls MockCreateLogGroup.
func (m *MockClient) CreateLogGroup(ctx context.Context, in *svcsdk.CreateLogGroupInput, _ ...func(*svcsdk.Options)) (*svcsdk.CreateLogGroupOutput, error) {
	m.record("CreateLogGroup", in)
	if m.MockCreateLogGroup == nil {
		return &svcsdk.CreateLogGroupOutput{}, nil
	}
	return m.MockCreateLogGroup(ctx, in)
}

// DeleteLogGroup calls MockDeleteLogGroup.
func (m *MockClient) DeleteLogGroup(ctx context.Context, in *svcsdk.DeleteLogGroupInput, _ ...func(*svcsdk.Options)) (*svcsdk.DeleteLogGroupOutput, error) {
	m.record("DeleteLogGroup", in)
	if m.MockDeleteLogGroup == nil {
		return &svcsdk.DeleteLogGroupOutput{}, nil
	}
	return m.MockDeleteLogGroup(ctx, in)
}

// AssociateKmsKey calls MockAssociateKmsKey.
func (m *MockClient) AssociateKmsKey(ctx context.Context, in *svcsdk.AssociateKmsKeyInput, _ ...func(*svcsdk.Options)) (*svcsdk.AssociateKmsKeyOutput, error) {
	m.record("AssociateKmsKey", in)
	if m.MockAssociateKmsKey == nil {
		return &svcsdk.AssociateKmsKeyOutput{}, nil
	}
	return m.MockAssociateKmsKey(ctx, in)
}

// DeleteRetentionPolicy calls MockDeleteRetentionPolicy.
func (m *MockClient) DeleteRetentionPolicy(ctx context.Context, in *svcsdk.DeleteRetentionPolicyInput, _ ...func(*svcsdk.Options)) (*svcsdk.DeleteRetentionPolicyOutput, error) {
	m.record("DeleteRetentionPolicy", in)
	if m.MockDeleteRetentionPolicy == nil {
		return &svcsdk.DeleteRetentionPolicyOutput{}, nil
	}
	return m.MockDeleteRetentionPolicy(ctx, in)
}

// DisassociateKmsKey calls MockDisassociateKmsKey.
func (m *MockClient) DisassociateKmsKey(ctx context.Context, in *svcsdk.DisassociateKmsKeyInput, _ ...func(*svcsdk.Options)) (*svcsdk.DisassociateKmsKeyOutput, error) {
	m.record("DisassociateKmsKey", in)
	if m.MockDisassociateKmsKey == nil {
		return &svcsdk.DisassociateKmsKeyOutput{}, nil
	}
	return m.MockDisassociateKmsKey(ctx, in)
}

// ListTagsForResource calls MockListTagsForResource.
func (m *MockClient) ListTagsForResource(ctx context.Context, in *svcsdk.ListTagsForResourceInput, _ ...func(*svcsdk.Options)) (*svcsdk.ListTagsForResourceOutput, error) {
	m.record("ListTagsForResource", in)
	if m.MockListTagsForResource == nil {
		return &svcsdk.ListTagsForResourceOutput{}, nil
	}
	return m.MockListTagsForResource(ctx, in)
}

// PutLogGroupDeletionProtection calls MockPutLogGroupDeletionProtection.
func (m *MockClient) PutLogGroupDeletionProtection(ctx context.Context, in *svcsdk.PutLogGroupDeletionProtectionInput, _ ...func(*svcsdk.Options)) (*svcsdk.PutLogGroupDeletionProtectionOutput, error) {
	m.record("PutLogGroupDeletionProtection", in)
	if m.MockPutLogGroupDeletionProtection == nil {
		return &svcsdk.PutLogGroupDeletionProtectionOutput{}, nil
	}
	return m.MockPutLogGroupDeletionProtection(ctx, in)
}

// PutRetentionPolicy calls MockPutRetentionPolicy.
func (m *MockClient) PutRetentionPolicy(ctx context.Context, in *svcsdk.PutRetentionPolicyInput, _ ...func(*svcsdk.Options)) (*svcsdk.PutRetentionPolicyOutput, error) {
	m.record("PutRetentionPolicy", in)
	if m.MockPutRetentionPolicy == nil {
		return &svcsdk.PutRetentionPolicyOutput{}, nil
	}
	return m.MockPutRetentionPolicy(ctx, in)
}

// TagResource calls MockTagResource.
func (m *MockClient) TagResource(ctx context.Context, in *svcsdk.TagResourceInput, _ ...func(*svcsdk.Options)) (*svcsdk.TagResourceOutput, error) {
	m.record("TagResource", in)
	if m.MockTagResource == nil {
		return &svcsdk.TagResourceOutput{}, nil
	}
	return m.MockTagResource(ctx, in)
}

// UntagResource calls MockUntagResource.
func (m *MockClient) UntagResource(ctx context.Context, in *svcsdk.UntagResourceInput, _ ...func(*svcsdk.Options)) (*svcsdk.UntagResourceOutput, error) {
	m.record("UntagResource", in)
	if m.MockUntagResource == nil {
		return &svcsdk.UntagResourceOutput{}, nil
	}
	return m.MockUntagResource(ctx, in)
}

// DescribeResourcePolicies calls MockDescribeResourcePolicies.
func (m *MockClient) DescribeResourcePolicies(ctx context.Context, in *svcsdk.DescribeResourcePoliciesInput, _ ...func(*svcsdk.Options)) (*svcsdk.DescribeResourcePoliciesOutput, error) {
	m.record("DescribeResourcePolicies", in)
	if m.MockDescribeResourcePolicies == nil {
		return &svcsdk.DescribeResourcePoliciesOutput{}, nil
	}
	return m.MockDescribeResourcePolicies(ctx, in)
}

// PutResourcePolicy calls MockPutResourcePolicy.
func (m *MockClient) PutResourcePolicy(ctx context.Context, in *svcsdk.PutResourcePolicyInput, _ ...func(*svcsdk.Options)) (*svcsdk.PutResourcePolicyOutput, error) {
	m.record("PutResourcePolicy", in)
	if m.MockPutResourcePolicy == nil {
		return &svcsdk.PutResourcePolicyOutput{}, nil
	}
	return m.MockPutResourcePolicy(ctx, in)
}

// DeleteResourcePolicy calls MockDeleteResourcePolicy.
func (m *MockClient) DeleteResourcePolicy(ctx context.Context, in *svcsdk.DeleteResourcePolicyInput, _ ...func(*svcsdk.Options)) (*svcsdk.DeleteResourcePolicyOutput, error) {
	m.record("DeleteResourcePolicy", in)
	if m.MockDeleteResourcePolicy == nil {
		return &svcsdk.DeleteResourcePolicyOutput{}, nil
	}
	return m.MockDeleteResourcePolicy(ctx, in)
}

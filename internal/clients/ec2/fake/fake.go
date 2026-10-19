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

// Package fake provides a mock EC2 client for tests.
package fake

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// MockEC2Client is a mock EC2 client. Unset mock functions return empty
// outputs. Each call's input is appended to Calls under the operation name.
type MockEC2Client struct {
	MockDescribeSecurityGroups        func(context.Context, *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error)
	MockCreateSecurityGroup           func(context.Context, *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error)
	MockDeleteSecurityGroup           func(context.Context, *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error)
	MockCreateTags                    func(context.Context, *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error)
	MockDeleteTags                    func(context.Context, *ec2.DeleteTagsInput) (*ec2.DeleteTagsOutput, error)
	MockRevokeSecurityGroupEgress     func(context.Context, *ec2.RevokeSecurityGroupEgressInput) (*ec2.RevokeSecurityGroupEgressOutput, error)
	MockRevokeSecurityGroupIngress    func(context.Context, *ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error)
	MockAuthorizeSecurityGroupIngress func(context.Context, *ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
	MockAuthorizeSecurityGroupEgress  func(context.Context, *ec2.AuthorizeSecurityGroupEgressInput) (*ec2.AuthorizeSecurityGroupEgressOutput, error)
	MockDescribeSecurityGroupRules    func(context.Context, *ec2.DescribeSecurityGroupRulesInput) (*ec2.DescribeSecurityGroupRulesOutput, error)
	MockModifySecurityGroupRules      func(context.Context, *ec2.ModifySecurityGroupRulesInput) (*ec2.ModifySecurityGroupRulesOutput, error)

	Calls map[string][]any
}

func (m *MockEC2Client) record(op string, in any) {
	if m.Calls == nil {
		m.Calls = map[string][]any{}
	}
	m.Calls[op] = append(m.Calls[op], in)
}

// DescribeSecurityGroups calls MockDescribeSecurityGroups.
func (m *MockEC2Client) DescribeSecurityGroups(ctx context.Context, in *ec2.DescribeSecurityGroupsInput, _ ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	m.record("DescribeSecurityGroups", in)
	if m.MockDescribeSecurityGroups == nil {
		return &ec2.DescribeSecurityGroupsOutput{}, nil
	}
	return m.MockDescribeSecurityGroups(ctx, in)
}

// CreateSecurityGroup calls MockCreateSecurityGroup.
func (m *MockEC2Client) CreateSecurityGroup(ctx context.Context, in *ec2.CreateSecurityGroupInput, _ ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error) {
	m.record("CreateSecurityGroup", in)
	if m.MockCreateSecurityGroup == nil {
		return &ec2.CreateSecurityGroupOutput{}, nil
	}
	return m.MockCreateSecurityGroup(ctx, in)
}

// DeleteSecurityGroup calls MockDeleteSecurityGroup.
func (m *MockEC2Client) DeleteSecurityGroup(ctx context.Context, in *ec2.DeleteSecurityGroupInput, _ ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error) {
	m.record("DeleteSecurityGroup", in)
	if m.MockDeleteSecurityGroup == nil {
		return &ec2.DeleteSecurityGroupOutput{}, nil
	}
	return m.MockDeleteSecurityGroup(ctx, in)
}

// CreateTags calls MockCreateTags.
func (m *MockEC2Client) CreateTags(ctx context.Context, in *ec2.CreateTagsInput, _ ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error) {
	m.record("CreateTags", in)
	if m.MockCreateTags == nil {
		return &ec2.CreateTagsOutput{}, nil
	}
	return m.MockCreateTags(ctx, in)
}

// DeleteTags calls MockDeleteTags.
func (m *MockEC2Client) DeleteTags(ctx context.Context, in *ec2.DeleteTagsInput, _ ...func(*ec2.Options)) (*ec2.DeleteTagsOutput, error) {
	m.record("DeleteTags", in)
	if m.MockDeleteTags == nil {
		return &ec2.DeleteTagsOutput{}, nil
	}
	return m.MockDeleteTags(ctx, in)
}

// RevokeSecurityGroupEgress calls MockRevokeSecurityGroupEgress.
func (m *MockEC2Client) RevokeSecurityGroupEgress(ctx context.Context, in *ec2.RevokeSecurityGroupEgressInput, _ ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupEgressOutput, error) {
	m.record("RevokeSecurityGroupEgress", in)
	if m.MockRevokeSecurityGroupEgress == nil {
		return &ec2.RevokeSecurityGroupEgressOutput{}, nil
	}
	return m.MockRevokeSecurityGroupEgress(ctx, in)
}

// RevokeSecurityGroupIngress calls MockRevokeSecurityGroupIngress.
func (m *MockEC2Client) RevokeSecurityGroupIngress(ctx context.Context, in *ec2.RevokeSecurityGroupIngressInput, _ ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	m.record("RevokeSecurityGroupIngress", in)
	if m.MockRevokeSecurityGroupIngress == nil {
		return &ec2.RevokeSecurityGroupIngressOutput{}, nil
	}
	return m.MockRevokeSecurityGroupIngress(ctx, in)
}

// AuthorizeSecurityGroupIngress calls MockAuthorizeSecurityGroupIngress.
func (m *MockEC2Client) AuthorizeSecurityGroupIngress(ctx context.Context, in *ec2.AuthorizeSecurityGroupIngressInput, _ ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	m.record("AuthorizeSecurityGroupIngress", in)
	if m.MockAuthorizeSecurityGroupIngress == nil {
		return &ec2.AuthorizeSecurityGroupIngressOutput{}, nil
	}
	return m.MockAuthorizeSecurityGroupIngress(ctx, in)
}

// AuthorizeSecurityGroupEgress calls MockAuthorizeSecurityGroupEgress.
func (m *MockEC2Client) AuthorizeSecurityGroupEgress(ctx context.Context, in *ec2.AuthorizeSecurityGroupEgressInput, _ ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupEgressOutput, error) {
	m.record("AuthorizeSecurityGroupEgress", in)
	if m.MockAuthorizeSecurityGroupEgress == nil {
		return &ec2.AuthorizeSecurityGroupEgressOutput{}, nil
	}
	return m.MockAuthorizeSecurityGroupEgress(ctx, in)
}

// DescribeSecurityGroupRules calls MockDescribeSecurityGroupRules.
func (m *MockEC2Client) DescribeSecurityGroupRules(ctx context.Context, in *ec2.DescribeSecurityGroupRulesInput, _ ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupRulesOutput, error) {
	m.record("DescribeSecurityGroupRules", in)
	if m.MockDescribeSecurityGroupRules == nil {
		return &ec2.DescribeSecurityGroupRulesOutput{}, nil
	}
	return m.MockDescribeSecurityGroupRules(ctx, in)
}

// ModifySecurityGroupRules calls MockModifySecurityGroupRules.
func (m *MockEC2Client) ModifySecurityGroupRules(ctx context.Context, in *ec2.ModifySecurityGroupRulesInput, _ ...func(*ec2.Options)) (*ec2.ModifySecurityGroupRulesOutput, error) {
	m.record("ModifySecurityGroupRules", in)
	if m.MockModifySecurityGroupRules == nil {
		return &ec2.ModifySecurityGroupRulesOutput{}, nil
	}
	return m.MockModifySecurityGroupRules(ctx, in)
}

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

// Package fake provides a mock KMS client for tests.
package fake

import (
	"context"

	svcsdk "github.com/aws/aws-sdk-go-v2/service/kms"
)

// MockClient is a mock KMS client. Unset mock functions return empty
// outputs. Each call's input is appended to Calls under the operation name,
// and the operation name to Ops.
type MockClient struct {
	MockDescribeKey          func(context.Context, *svcsdk.DescribeKeyInput) (*svcsdk.DescribeKeyOutput, error)
	MockCreateKey            func(context.Context, *svcsdk.CreateKeyInput) (*svcsdk.CreateKeyOutput, error)
	MockScheduleKeyDeletion  func(context.Context, *svcsdk.ScheduleKeyDeletionInput) (*svcsdk.ScheduleKeyDeletionOutput, error)
	MockCancelKeyDeletion    func(context.Context, *svcsdk.CancelKeyDeletionInput) (*svcsdk.CancelKeyDeletionOutput, error)
	MockEnableKey            func(context.Context, *svcsdk.EnableKeyInput) (*svcsdk.EnableKeyOutput, error)
	MockDisableKey           func(context.Context, *svcsdk.DisableKeyInput) (*svcsdk.DisableKeyOutput, error)
	MockEnableKeyRotation    func(context.Context, *svcsdk.EnableKeyRotationInput) (*svcsdk.EnableKeyRotationOutput, error)
	MockDisableKeyRotation   func(context.Context, *svcsdk.DisableKeyRotationInput) (*svcsdk.DisableKeyRotationOutput, error)
	MockGetKeyRotationStatus func(context.Context, *svcsdk.GetKeyRotationStatusInput) (*svcsdk.GetKeyRotationStatusOutput, error)
	MockGetKeyPolicy         func(context.Context, *svcsdk.GetKeyPolicyInput) (*svcsdk.GetKeyPolicyOutput, error)
	MockPutKeyPolicy         func(context.Context, *svcsdk.PutKeyPolicyInput) (*svcsdk.PutKeyPolicyOutput, error)
	MockUpdateKeyDescription func(context.Context, *svcsdk.UpdateKeyDescriptionInput) (*svcsdk.UpdateKeyDescriptionOutput, error)
	MockListResourceTags     func(context.Context, *svcsdk.ListResourceTagsInput) (*svcsdk.ListResourceTagsOutput, error)
	MockTagResource          func(context.Context, *svcsdk.TagResourceInput) (*svcsdk.TagResourceOutput, error)
	MockUntagResource        func(context.Context, *svcsdk.UntagResourceInput) (*svcsdk.UntagResourceOutput, error)
	MockListAliases          func(context.Context, *svcsdk.ListAliasesInput) (*svcsdk.ListAliasesOutput, error)
	MockCreateAlias          func(context.Context, *svcsdk.CreateAliasInput) (*svcsdk.CreateAliasOutput, error)
	MockUpdateAlias          func(context.Context, *svcsdk.UpdateAliasInput) (*svcsdk.UpdateAliasOutput, error)
	MockDeleteAlias          func(context.Context, *svcsdk.DeleteAliasInput) (*svcsdk.DeleteAliasOutput, error)
	MockListGrants           func(context.Context, *svcsdk.ListGrantsInput) (*svcsdk.ListGrantsOutput, error)
	MockCreateGrant          func(context.Context, *svcsdk.CreateGrantInput) (*svcsdk.CreateGrantOutput, error)
	MockRevokeGrant          func(context.Context, *svcsdk.RevokeGrantInput) (*svcsdk.RevokeGrantOutput, error)

	Calls map[string][]any
	Ops   []string
}

// call records the input of op and calls f, or returns an empty output if f
// is nil.
func call[I, O any](ctx context.Context, m *MockClient, op string, in *I, f func(context.Context, *I) (*O, error)) (*O, error) {
	if m.Calls == nil {
		m.Calls = map[string][]any{}
	}
	m.Calls[op] = append(m.Calls[op], in)
	m.Ops = append(m.Ops, op)
	if f == nil {
		return new(O), nil
	}
	return f(ctx, in)
}

// DescribeKey calls MockDescribeKey.
func (m *MockClient) DescribeKey(ctx context.Context, in *svcsdk.DescribeKeyInput, _ ...func(*svcsdk.Options)) (*svcsdk.DescribeKeyOutput, error) {
	return call(ctx, m, "DescribeKey", in, m.MockDescribeKey)
}

// CreateKey calls MockCreateKey.
func (m *MockClient) CreateKey(ctx context.Context, in *svcsdk.CreateKeyInput, _ ...func(*svcsdk.Options)) (*svcsdk.CreateKeyOutput, error) {
	return call(ctx, m, "CreateKey", in, m.MockCreateKey)
}

// ScheduleKeyDeletion calls MockScheduleKeyDeletion.
func (m *MockClient) ScheduleKeyDeletion(ctx context.Context, in *svcsdk.ScheduleKeyDeletionInput, _ ...func(*svcsdk.Options)) (*svcsdk.ScheduleKeyDeletionOutput, error) {
	return call(ctx, m, "ScheduleKeyDeletion", in, m.MockScheduleKeyDeletion)
}

// CancelKeyDeletion calls MockCancelKeyDeletion.
func (m *MockClient) CancelKeyDeletion(ctx context.Context, in *svcsdk.CancelKeyDeletionInput, _ ...func(*svcsdk.Options)) (*svcsdk.CancelKeyDeletionOutput, error) {
	return call(ctx, m, "CancelKeyDeletion", in, m.MockCancelKeyDeletion)
}

// EnableKey calls MockEnableKey.
func (m *MockClient) EnableKey(ctx context.Context, in *svcsdk.EnableKeyInput, _ ...func(*svcsdk.Options)) (*svcsdk.EnableKeyOutput, error) {
	return call(ctx, m, "EnableKey", in, m.MockEnableKey)
}

// DisableKey calls MockDisableKey.
func (m *MockClient) DisableKey(ctx context.Context, in *svcsdk.DisableKeyInput, _ ...func(*svcsdk.Options)) (*svcsdk.DisableKeyOutput, error) {
	return call(ctx, m, "DisableKey", in, m.MockDisableKey)
}

// EnableKeyRotation calls MockEnableKeyRotation.
func (m *MockClient) EnableKeyRotation(ctx context.Context, in *svcsdk.EnableKeyRotationInput, _ ...func(*svcsdk.Options)) (*svcsdk.EnableKeyRotationOutput, error) {
	return call(ctx, m, "EnableKeyRotation", in, m.MockEnableKeyRotation)
}

// DisableKeyRotation calls MockDisableKeyRotation.
func (m *MockClient) DisableKeyRotation(ctx context.Context, in *svcsdk.DisableKeyRotationInput, _ ...func(*svcsdk.Options)) (*svcsdk.DisableKeyRotationOutput, error) {
	return call(ctx, m, "DisableKeyRotation", in, m.MockDisableKeyRotation)
}

// GetKeyRotationStatus calls MockGetKeyRotationStatus.
func (m *MockClient) GetKeyRotationStatus(ctx context.Context, in *svcsdk.GetKeyRotationStatusInput, _ ...func(*svcsdk.Options)) (*svcsdk.GetKeyRotationStatusOutput, error) {
	return call(ctx, m, "GetKeyRotationStatus", in, m.MockGetKeyRotationStatus)
}

// GetKeyPolicy calls MockGetKeyPolicy.
func (m *MockClient) GetKeyPolicy(ctx context.Context, in *svcsdk.GetKeyPolicyInput, _ ...func(*svcsdk.Options)) (*svcsdk.GetKeyPolicyOutput, error) {
	return call(ctx, m, "GetKeyPolicy", in, m.MockGetKeyPolicy)
}

// PutKeyPolicy calls MockPutKeyPolicy.
func (m *MockClient) PutKeyPolicy(ctx context.Context, in *svcsdk.PutKeyPolicyInput, _ ...func(*svcsdk.Options)) (*svcsdk.PutKeyPolicyOutput, error) {
	return call(ctx, m, "PutKeyPolicy", in, m.MockPutKeyPolicy)
}

// UpdateKeyDescription calls MockUpdateKeyDescription.
func (m *MockClient) UpdateKeyDescription(ctx context.Context, in *svcsdk.UpdateKeyDescriptionInput, _ ...func(*svcsdk.Options)) (*svcsdk.UpdateKeyDescriptionOutput, error) {
	return call(ctx, m, "UpdateKeyDescription", in, m.MockUpdateKeyDescription)
}

// ListResourceTags calls MockListResourceTags.
func (m *MockClient) ListResourceTags(ctx context.Context, in *svcsdk.ListResourceTagsInput, _ ...func(*svcsdk.Options)) (*svcsdk.ListResourceTagsOutput, error) {
	return call(ctx, m, "ListResourceTags", in, m.MockListResourceTags)
}

// TagResource calls MockTagResource.
func (m *MockClient) TagResource(ctx context.Context, in *svcsdk.TagResourceInput, _ ...func(*svcsdk.Options)) (*svcsdk.TagResourceOutput, error) {
	return call(ctx, m, "TagResource", in, m.MockTagResource)
}

// UntagResource calls MockUntagResource.
func (m *MockClient) UntagResource(ctx context.Context, in *svcsdk.UntagResourceInput, _ ...func(*svcsdk.Options)) (*svcsdk.UntagResourceOutput, error) {
	return call(ctx, m, "UntagResource", in, m.MockUntagResource)
}

// ListAliases calls MockListAliases.
func (m *MockClient) ListAliases(ctx context.Context, in *svcsdk.ListAliasesInput, _ ...func(*svcsdk.Options)) (*svcsdk.ListAliasesOutput, error) {
	return call(ctx, m, "ListAliases", in, m.MockListAliases)
}

// CreateAlias calls MockCreateAlias.
func (m *MockClient) CreateAlias(ctx context.Context, in *svcsdk.CreateAliasInput, _ ...func(*svcsdk.Options)) (*svcsdk.CreateAliasOutput, error) {
	return call(ctx, m, "CreateAlias", in, m.MockCreateAlias)
}

// UpdateAlias calls MockUpdateAlias.
func (m *MockClient) UpdateAlias(ctx context.Context, in *svcsdk.UpdateAliasInput, _ ...func(*svcsdk.Options)) (*svcsdk.UpdateAliasOutput, error) {
	return call(ctx, m, "UpdateAlias", in, m.MockUpdateAlias)
}

// DeleteAlias calls MockDeleteAlias.
func (m *MockClient) DeleteAlias(ctx context.Context, in *svcsdk.DeleteAliasInput, _ ...func(*svcsdk.Options)) (*svcsdk.DeleteAliasOutput, error) {
	return call(ctx, m, "DeleteAlias", in, m.MockDeleteAlias)
}

// ListGrants calls MockListGrants.
func (m *MockClient) ListGrants(ctx context.Context, in *svcsdk.ListGrantsInput, _ ...func(*svcsdk.Options)) (*svcsdk.ListGrantsOutput, error) {
	return call(ctx, m, "ListGrants", in, m.MockListGrants)
}

// CreateGrant calls MockCreateGrant.
func (m *MockClient) CreateGrant(ctx context.Context, in *svcsdk.CreateGrantInput, _ ...func(*svcsdk.Options)) (*svcsdk.CreateGrantOutput, error) {
	return call(ctx, m, "CreateGrant", in, m.MockCreateGrant)
}

// RevokeGrant calls MockRevokeGrant.
func (m *MockClient) RevokeGrant(ctx context.Context, in *svcsdk.RevokeGrantInput, _ ...func(*svcsdk.Options)) (*svcsdk.RevokeGrantOutput, error) {
	return call(ctx, m, "RevokeGrant", in, m.MockRevokeGrant)
}

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

// Package fake provides a mock OpenSearch client for tests.
package fake

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/opensearch"
)

// MockOpenSearchClient is a mock OpenSearch client. Unset mock functions
// return empty outputs. Each call's input is appended to Calls under the
// operation name.
type MockOpenSearchClient struct {
	MockDescribeDomain     func(context.Context, *opensearch.DescribeDomainInput) (*opensearch.DescribeDomainOutput, error)
	MockCreateDomain       func(context.Context, *opensearch.CreateDomainInput) (*opensearch.CreateDomainOutput, error)
	MockUpdateDomainConfig func(context.Context, *opensearch.UpdateDomainConfigInput) (*opensearch.UpdateDomainConfigOutput, error)
	MockDeleteDomain       func(context.Context, *opensearch.DeleteDomainInput) (*opensearch.DeleteDomainOutput, error)
	MockAddTags            func(context.Context, *opensearch.AddTagsInput) (*opensearch.AddTagsOutput, error)
	MockListTags           func(context.Context, *opensearch.ListTagsInput) (*opensearch.ListTagsOutput, error)
	MockRemoveTags         func(context.Context, *opensearch.RemoveTagsInput) (*opensearch.RemoveTagsOutput, error)
	MockUpgradeDomain      func(context.Context, *opensearch.UpgradeDomainInput) (*opensearch.UpgradeDomainOutput, error)

	Calls map[string][]any
}

func (m *MockOpenSearchClient) record(op string, in any) {
	if m.Calls == nil {
		m.Calls = map[string][]any{}
	}
	m.Calls[op] = append(m.Calls[op], in)
}

// DescribeDomain calls MockDescribeDomain.
func (m *MockOpenSearchClient) DescribeDomain(ctx context.Context, in *opensearch.DescribeDomainInput, _ ...func(*opensearch.Options)) (*opensearch.DescribeDomainOutput, error) {
	m.record("DescribeDomain", in)
	if m.MockDescribeDomain == nil {
		return &opensearch.DescribeDomainOutput{}, nil
	}
	return m.MockDescribeDomain(ctx, in)
}

// CreateDomain calls MockCreateDomain.
func (m *MockOpenSearchClient) CreateDomain(ctx context.Context, in *opensearch.CreateDomainInput, _ ...func(*opensearch.Options)) (*opensearch.CreateDomainOutput, error) {
	m.record("CreateDomain", in)
	if m.MockCreateDomain == nil {
		return &opensearch.CreateDomainOutput{}, nil
	}
	return m.MockCreateDomain(ctx, in)
}

// UpdateDomainConfig calls MockUpdateDomainConfig.
func (m *MockOpenSearchClient) UpdateDomainConfig(ctx context.Context, in *opensearch.UpdateDomainConfigInput, _ ...func(*opensearch.Options)) (*opensearch.UpdateDomainConfigOutput, error) {
	m.record("UpdateDomainConfig", in)
	if m.MockUpdateDomainConfig == nil {
		return &opensearch.UpdateDomainConfigOutput{}, nil
	}
	return m.MockUpdateDomainConfig(ctx, in)
}

// DeleteDomain calls MockDeleteDomain.
func (m *MockOpenSearchClient) DeleteDomain(ctx context.Context, in *opensearch.DeleteDomainInput, _ ...func(*opensearch.Options)) (*opensearch.DeleteDomainOutput, error) {
	m.record("DeleteDomain", in)
	if m.MockDeleteDomain == nil {
		return &opensearch.DeleteDomainOutput{}, nil
	}
	return m.MockDeleteDomain(ctx, in)
}

// AddTags calls MockAddTags.
func (m *MockOpenSearchClient) AddTags(ctx context.Context, in *opensearch.AddTagsInput, _ ...func(*opensearch.Options)) (*opensearch.AddTagsOutput, error) {
	m.record("AddTags", in)
	if m.MockAddTags == nil {
		return &opensearch.AddTagsOutput{}, nil
	}
	return m.MockAddTags(ctx, in)
}

// ListTags calls MockListTags.
func (m *MockOpenSearchClient) ListTags(ctx context.Context, in *opensearch.ListTagsInput, _ ...func(*opensearch.Options)) (*opensearch.ListTagsOutput, error) {
	m.record("ListTags", in)
	if m.MockListTags == nil {
		return &opensearch.ListTagsOutput{}, nil
	}
	return m.MockListTags(ctx, in)
}

// RemoveTags calls MockRemoveTags.
func (m *MockOpenSearchClient) RemoveTags(ctx context.Context, in *opensearch.RemoveTagsInput, _ ...func(*opensearch.Options)) (*opensearch.RemoveTagsOutput, error) {
	m.record("RemoveTags", in)
	if m.MockRemoveTags == nil {
		return &opensearch.RemoveTagsOutput{}, nil
	}
	return m.MockRemoveTags(ctx, in)
}

// UpgradeDomain calls MockUpgradeDomain.
func (m *MockOpenSearchClient) UpgradeDomain(ctx context.Context, in *opensearch.UpgradeDomainInput, _ ...func(*opensearch.Options)) (*opensearch.UpgradeDomainOutput, error) {
	m.record("UpgradeDomain", in)
	if m.MockUpgradeDomain == nil {
		return &opensearch.UpgradeDomainOutput{}, nil
	}
	return m.MockUpgradeDomain(ctx, in)
}

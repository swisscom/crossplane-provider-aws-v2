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

package dbsubnetgroup

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/docdb"
	docdbtypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/aws/smithy-go"
	"k8s.io/utils/ptr"

	fake "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/docdb/fake"
)

// AWS stores subnet group names in lowercase.
func TestObserveMatchesExternalNameIgnoringCase(t *testing.T) {
	c := &fake.MockDocDBClient{
		MockDescribeDBSubnetGroups: func(context.Context, *docdb.DescribeDBSubnetGroupsInput, ...func(*docdb.Options)) (*docdb.DescribeDBSubnetGroupsOutput, error) {
			return &docdb.DescribeDBSubnetGroupsOutput{DBSubnetGroups: []docdbtypes.DBSubnetGroup{{DBSubnetGroupName: ptr.To("subnets")}}}, nil
		},
	}
	e := newExternal(nil, c, []option{setupExternal})

	obs, err := e.Observe(context.Background(), subnetGroup(withExternalName("Subnets")))

	if err != nil || !obs.ResourceExists {
		t.Errorf("Observe(...) = %+v, %v; want existing, nil", obs, err)
	}
}

func TestTypedNotFound(t *testing.T) {
	notFound := &smithy.GenericAPIError{Code: "DBSubnetGroupNotFoundFault", Message: "gone"}
	c := &fake.MockDocDBClient{
		MockDescribeDBSubnetGroups: func(context.Context, *docdb.DescribeDBSubnetGroupsInput, ...func(*docdb.Options)) (*docdb.DescribeDBSubnetGroupsOutput, error) {
			return nil, notFound
		},
		MockDeleteDBSubnetGroup: func(context.Context, *docdb.DeleteDBSubnetGroupInput, ...func(*docdb.Options)) (*docdb.DeleteDBSubnetGroupOutput, error) {
			return nil, notFound
		},
	}
	e := newExternal(nil, c, []option{setupExternal})
	cr := subnetGroup(withExternalName("subnets"))

	obs, err := e.Observe(context.Background(), cr)
	if err != nil || obs.ResourceExists {
		t.Errorf("Observe(...) = %+v, %v; want not existing, nil", obs, err)
	}
	if _, err := e.Delete(context.Background(), cr); err != nil {
		t.Errorf("Delete(...): %v, want nil", err)
	}
}

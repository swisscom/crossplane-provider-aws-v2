/*
Copyright 2021 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS_IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dbclusterparametergroup

import (
	"context"
	"go/token"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/docdb"
	docdbtypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	fake "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/docdb/fake"
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

const (
	testDBClusterParameterGroupName = "some-db-cluster-parameter-group"
	testDBClusterParameterGroupARN  = "some-arn"
	testDescription                 = "some-description"
	testOtherDescription            = "some-other-description"
	testFamily                      = "some-family"
	testOtherFamily                 = "some-other-family"
	testParameterName               = "some-parameter-name"
	testParameterValue              = "some-parameter-value"
	testOtherParameterName          = "some-other-parameter-name"
	testOtherParameterValue         = "some-other-parameter-value"
	testTagKey                      = "some-tag-key"
	testTagValue                    = "some-tag-value"
	testOtherTagKey                 = "some-other-tag-key"
	testOtherTagValue               = "some-other-tag-value"

	testErrDescribeDBClusterParametersFailed      = "DescribeDBClusterParameters failed"
	testErrDescribeDBClusterParameterGroupsFailed = "DescribeDBClusterParameterGroups failed"
	testErrCreateDBClusterParameterGroupFailed    = "CreateDBClusterParameterGroup failed"
	testErrDeleteDBClusterParameterGroupFailed    = "DeleteDBClusterParameterGroup failed"
	testErrModifyDBClusterParameterGroupFailed    = "ModifyDBClusterParameterGroup failed"
	testErrBoom                                   = "boom"
)

type args struct {
	docdb *fake.MockDocDBClient
	kube  client.Client
	cr    *svcapitypes.DBClusterParameterGroup
}

type docDBModifier func(*svcapitypes.DBClusterParameterGroup)

func instance(m ...docDBModifier) *svcapitypes.DBClusterParameterGroup {
	cr := &svcapitypes.DBClusterParameterGroup{}
	for _, f := range m {
		f(cr)
	}
	return cr
}

func withExternalName(value string) docDBModifier {
	return func(o *svcapitypes.DBClusterParameterGroup) {
		meta.SetExternalName(o, value)

	}
}

func withDBClusterParameterGroupName(value string) docDBModifier {
	return func(o *svcapitypes.DBClusterParameterGroup) {
		o.Status.AtProvider.DBClusterParameterGroupName = pointer.ToOrNilIfZeroValue(value)
	}
}

func withDescription(value string) docDBModifier {
	return func(o *svcapitypes.DBClusterParameterGroup) {
		o.Spec.ForProvider.Description = pointer.ToOrNilIfZeroValue(value)
	}
}

func withDBParameterGroupFamily(value string) docDBModifier {
	return func(o *svcapitypes.DBClusterParameterGroup) {
		o.Spec.ForProvider.DBParameterGroupFamily = pointer.ToOrNilIfZeroValue(value)
	}
}

func withDBClusterParameterGroupARN(value string) docDBModifier {
	return func(o *svcapitypes.DBClusterParameterGroup) {
		o.Status.AtProvider.DBClusterParameterGroupARN = pointer.ToOrNilIfZeroValue(value)
	}
}

func withConditions(value ...xpv2.Condition) docDBModifier {
	return func(o *svcapitypes.DBClusterParameterGroup) {
		o.Status.SetConditions(value...)
	}
}

func withParameters(values ...*svcapitypes.CustomParameter) docDBModifier {
	return func(o *svcapitypes.DBClusterParameterGroup) {
		if values == nil {
			o.Spec.ForProvider.Parameters = []*svcapitypes.CustomParameter{}
		} else {
			o.Spec.ForProvider.Parameters = values
		}
	}
}

func withTags(values ...*svcapitypes.Tag) docDBModifier {
	return func(o *svcapitypes.DBClusterParameterGroup) {
		if values != nil {
			o.Spec.ForProvider.Tags = values
		} else {
			o.Spec.ForProvider.Tags = []*svcapitypes.Tag{}
		}
	}
}

func TestObserve(t *testing.T) {
	type want struct {
		cr     *svcapitypes.DBClusterParameterGroup
		result managed.ExternalObservation
		err    error
		docdb  fake.MockDocDBClientCall
	}

	cases := map[string]struct {
		args
		want
	}{
		"AvailableState_and_UpToDate_with_no_parameters": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusterParameterGroups: func(c context.Context, ddpgi *docdb.DescribeDBClusterParameterGroupsInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClusterParameterGroupsOutput, error) {
						return &docdb.DescribeDBClusterParameterGroupsOutput{
							DBClusterParameterGroups: []docdbtypes.DBClusterParameterGroup{
								{
									DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
								},
							},
						}, nil
					},
					MockDescribeDBClusterParameters: func(ctx context.Context, ddpi *docdb.DescribeDBClusterParametersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClusterParametersOutput, error) {
						return &docdb.DescribeDBClusterParametersOutput{
							Parameters: []docdbtypes.Parameter{},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{}, nil
					},
				},
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
				),
			},
			want: want{
				cr: instance(
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withExternalName(testDBClusterParameterGroupName),
					withParameters(),
					withConditions(xpv2.Available()),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusterParameterGroups: []*fake.CallDescribeDBClusterParameterGroups{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParameterGroupsInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
					DescribeDBClusterParameters: []*fake.CallDescribeDBClusterParameters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParametersInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParametersInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_UpToDate_with_one_parameter": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusterParameterGroups: func(c context.Context, ddpgi *docdb.DescribeDBClusterParameterGroupsInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClusterParameterGroupsOutput, error) {
						return &docdb.DescribeDBClusterParameterGroupsOutput{
							DBClusterParameterGroups: []docdbtypes.DBClusterParameterGroup{
								{
									DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
								},
							},
						}, nil
					},
					MockDescribeDBClusterParameters: func(ctx context.Context, ddpi *docdb.DescribeDBClusterParametersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClusterParametersOutput, error) {
						return &docdb.DescribeDBClusterParametersOutput{
							Parameters: []docdbtypes.Parameter{
								{
									ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
									ParameterValue: pointer.ToOrNilIfZeroValue(testParameterValue),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{}, nil
					},
				},
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
					withParameters(
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testParameterValue),
						},
					),
				),
			},
			want: want{
				cr: instance(
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withExternalName(testDBClusterParameterGroupName),
					withParameters(
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testParameterValue),
						},
					),
					withConditions(xpv2.Available()),
				),
				result: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusterParameterGroups: []*fake.CallDescribeDBClusterParameterGroups{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParameterGroupsInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
					DescribeDBClusterParameters: []*fake.CallDescribeDBClusterParameters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParametersInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParametersInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_UpToDate_with_one_spec_and_two_default_parameters": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusterParameterGroups: func(c context.Context, ddpgi *docdb.DescribeDBClusterParameterGroupsInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClusterParameterGroupsOutput, error) {
						return &docdb.DescribeDBClusterParameterGroupsOutput{
							DBClusterParameterGroups: []docdbtypes.DBClusterParameterGroup{
								{
									DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
								},
							},
						}, nil
					},
					MockDescribeDBClusterParameters: func(ctx context.Context, ddpi *docdb.DescribeDBClusterParametersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClusterParametersOutput, error) {
						return &docdb.DescribeDBClusterParametersOutput{
							Parameters: []docdbtypes.Parameter{
								{
									ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
									ParameterValue: pointer.ToOrNilIfZeroValue(testParameterValue),
								},
								{
									ParameterName:  pointer.ToOrNilIfZeroValue(testOtherParameterName),
									ParameterValue: pointer.ToOrNilIfZeroValue(testOtherParameterValue),
								},
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{}, nil
					},
				},
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
					withParameters(
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testParameterValue),
						},
					),
				),
			},
			want: want{
				cr: instance(
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withExternalName(testDBClusterParameterGroupName),
					withParameters(
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testParameterValue),
						},
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testOtherParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testOtherParameterValue),
						},
					),
					withConditions(xpv2.Available()),
				),
				result: managed.ExternalObservation{
					ResourceExists:          true,
					ResourceUpToDate:        true,
					ResourceLateInitialized: true,
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusterParameterGroups: []*fake.CallDescribeDBClusterParameterGroups{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParameterGroupsInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
					DescribeDBClusterParameters: []*fake.CallDescribeDBClusterParameters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParametersInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParametersInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{},
						},
					},
				},
			},
		},
		"AvailableState_and_changed_description": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusterParameterGroups: func(c context.Context, ddpgi *docdb.DescribeDBClusterParameterGroupsInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClusterParameterGroupsOutput, error) {
						return &docdb.DescribeDBClusterParameterGroupsOutput{
							DBClusterParameterGroups: []docdbtypes.DBClusterParameterGroup{
								{
									DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
									Description:                 pointer.ToOrNilIfZeroValue(testDescription),
								},
							},
						}, nil
					},
					MockDescribeDBClusterParameters: func(ctx context.Context, ddpi *docdb.DescribeDBClusterParametersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClusterParametersOutput, error) {
						return &docdb.DescribeDBClusterParametersOutput{
							Parameters: []docdbtypes.Parameter{},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{}, nil
					},
				},
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
					withDescription(testOtherDescription),
				),
			},
			want: want{
				cr: instance(
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withExternalName(testDBClusterParameterGroupName),
					withDescription(testOtherDescription),
					withParameters(),
				),
				result: managed.ExternalObservation{
					ResourceExists:   false,
					ResourceUpToDate: false,
				},
				err: errors.Wrap(errors.New(errModifyDescription), "isUpToDate check failed"),
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusterParameterGroups: []*fake.CallDescribeDBClusterParameterGroups{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParameterGroupsInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
					DescribeDBClusterParameters: []*fake.CallDescribeDBClusterParameters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParametersInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
				},
			},
		},
		"AvailableState_and_changed_DBParameterGroupFamily": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusterParameterGroups: func(c context.Context, ddpgi *docdb.DescribeDBClusterParameterGroupsInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClusterParameterGroupsOutput, error) {
						return &docdb.DescribeDBClusterParameterGroupsOutput{
							DBClusterParameterGroups: []docdbtypes.DBClusterParameterGroup{
								{
									DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
									DBParameterGroupFamily:      pointer.ToOrNilIfZeroValue(testFamily),
								},
							},
						}, nil
					},
					MockDescribeDBClusterParameters: func(ctx context.Context, ddpi *docdb.DescribeDBClusterParametersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClusterParametersOutput, error) {
						return &docdb.DescribeDBClusterParametersOutput{
							Parameters: []docdbtypes.Parameter{},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{}, nil
					},
				},
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
					withDBParameterGroupFamily(testOtherFamily),
				),
			},
			want: want{
				cr: instance(
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withExternalName(testDBClusterParameterGroupName),
					withDBParameterGroupFamily(testOtherFamily),
					withParameters(),
				),
				result: managed.ExternalObservation{
					ResourceExists:   false,
					ResourceUpToDate: false,
				},
				err: errors.Wrap(errors.New(errModifyFamily), "isUpToDate check failed"),
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusterParameterGroups: []*fake.CallDescribeDBClusterParameterGroups{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParameterGroupsInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
					DescribeDBClusterParameters: []*fake.CallDescribeDBClusterParameters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParametersInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
				},
			},
		},
		"AvailableState_and_changed_parameter": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusterParameterGroups: func(c context.Context, ddpgi *docdb.DescribeDBClusterParameterGroupsInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClusterParameterGroupsOutput, error) {
						return &docdb.DescribeDBClusterParameterGroupsOutput{
							DBClusterParameterGroups: []docdbtypes.DBClusterParameterGroup{
								{
									DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
								},
							},
						}, nil
					},
					MockDescribeDBClusterParameters: func(ctx context.Context, ddpi *docdb.DescribeDBClusterParametersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClusterParametersOutput, error) {
						return &docdb.DescribeDBClusterParametersOutput{
							Parameters: []docdbtypes.Parameter{
								{
									ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
									ParameterValue: pointer.ToOrNilIfZeroValue(testParameterValue),
								},
							},
						}, nil
					},
				},
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
					withParameters(
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testOtherParameterValue),
						},
					),
				),
			},
			want: want{
				cr: instance(
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withExternalName(testDBClusterParameterGroupName),
					withParameters(
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testOtherParameterValue),
						},
					),
					withConditions(xpv2.Available()),
				),
				result: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: false,
					Diff:             "spec.forProvider fields differ from the observed state: parameters",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusterParameterGroups: []*fake.CallDescribeDBClusterParameterGroups{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParameterGroupsInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
					DescribeDBClusterParameters: []*fake.CallDescribeDBClusterParameters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParametersInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParametersInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
				},
			},
		},
		"ErrDescribeDBClusterParameterGroupsWithContext": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusterParameterGroups: func(c context.Context, ddpgi *docdb.DescribeDBClusterParameterGroupsInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClusterParameterGroupsOutput, error) {
						return &docdb.DescribeDBClusterParameterGroupsOutput{}, errors.New(testErrDescribeDBClusterParameterGroupsFailed)
					},
				},
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
				),
				result: managed.ExternalObservation{
					ResourceExists:   false,
					ResourceUpToDate: false,
				},
				err: errors.Wrap(errors.New(testErrDescribeDBClusterParameterGroupsFailed), errDescribe),
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusterParameterGroups: []*fake.CallDescribeDBClusterParameterGroups{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParameterGroupsInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
				},
			},
		},
		"ErrDescribeDBClusterParameters": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBClusterParameterGroups: func(c context.Context, ddpgi *docdb.DescribeDBClusterParameterGroupsInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClusterParameterGroupsOutput, error) {
						return &docdb.DescribeDBClusterParameterGroupsOutput{
							DBClusterParameterGroups: []docdbtypes.DBClusterParameterGroup{
								{
									DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
								},
							},
						}, nil
					},
					MockDescribeDBClusterParameters: func(ctx context.Context, ddpi *docdb.DescribeDBClusterParametersInput, o ...func(*docdb.Options)) (*docdb.DescribeDBClusterParametersOutput, error) {
						return &docdb.DescribeDBClusterParametersOutput{}, errors.New(testErrDescribeDBClusterParametersFailed)
					},
				},
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
				),
				result: managed.ExternalObservation{
					ResourceExists:   false,
					ResourceUpToDate: false,
				},
				err: errors.Wrap(errors.Wrap(errors.New(testErrDescribeDBClusterParametersFailed), errDescribeParameters), "late-init failed"),
				docdb: fake.MockDocDBClientCall{
					DescribeDBClusterParameterGroups: []*fake.CallDescribeDBClusterParameterGroups{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParameterGroupsInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
					DescribeDBClusterParameters: []*fake.CallDescribeDBClusterParameters{
						{
							Ctx: context.Background(),
							I: &docdb.DescribeDBClusterParametersInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			opts := []option{setupExternal}
			e := newExternal(tc.args.kube, tc.args.docdb, opts)
			o, err := e.Observe(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, o); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.docdb, tc.args.docdb.Called, cmpopts.IgnoreInterfaces(struct{ context.Context }{}), ignoreUnexported); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	type want struct {
		cr     *svcapitypes.DBClusterParameterGroup
		result managed.ExternalCreation
		err    error
		docdb  fake.MockDocDBClientCall
	}

	cases := map[string]struct {
		args
		want
	}{
		"SuccessfulCreate_no_parameters": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockCreateDBClusterParameterGroup: func(c context.Context, cdpgi *docdb.CreateDBClusterParameterGroupInput, o ...func(*docdb.Options)) (*docdb.CreateDBClusterParameterGroupOutput, error) {
						return &docdb.CreateDBClusterParameterGroupOutput{
							DBClusterParameterGroup: &docdbtypes.DBClusterParameterGroup{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						}, nil
					},
				},
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
				),
			},
			want: want{
				cr: instance(
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withExternalName(testDBClusterParameterGroupName),
					withConditions(xpv2.Creating()),
				),
				result: managed.ExternalCreation{},
				docdb: fake.MockDocDBClientCall{
					CreateDBClusterParameterGroup: []*fake.CallCreateDBClusterParameterGroup{
						{
							Ctx: context.Background(),
							I: &docdb.CreateDBClusterParameterGroupInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
				},
			},
		},
		"SuccessfulCreate_with_parameters": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockCreateDBClusterParameterGroup: func(c context.Context, cdpgi *docdb.CreateDBClusterParameterGroupInput, o ...func(*docdb.Options)) (*docdb.CreateDBClusterParameterGroupOutput, error) {
						return &docdb.CreateDBClusterParameterGroupOutput{
							DBClusterParameterGroup: &docdbtypes.DBClusterParameterGroup{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						}, nil
					},
				},
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
					withParameters(
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testParameterValue),
						},
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testOtherParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testOtherParameterValue),
						},
					),
				),
			},
			want: want{
				cr: instance(
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withExternalName(testDBClusterParameterGroupName),
					withConditions(xpv2.Creating()),
					withParameters(
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testParameterValue),
						},
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testOtherParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testOtherParameterValue),
						},
					),
				),
				result: managed.ExternalCreation{},
				docdb: fake.MockDocDBClientCall{
					CreateDBClusterParameterGroup: []*fake.CallCreateDBClusterParameterGroup{
						{
							Ctx: context.Background(),
							I: &docdb.CreateDBClusterParameterGroupInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
				},
			},
		},
		"ErrorCreate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockCreateDBClusterParameterGroup: func(c context.Context, cdpgi *docdb.CreateDBClusterParameterGroupInput, o ...func(*docdb.Options)) (*docdb.CreateDBClusterParameterGroupOutput, error) {
						return &docdb.CreateDBClusterParameterGroupOutput{}, errors.New(testErrCreateDBClusterParameterGroupFailed)
					},
				},
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
					withConditions(xpv2.Creating()),
				),
				result: managed.ExternalCreation{},
				err:    errors.Wrap(errors.New(testErrCreateDBClusterParameterGroupFailed), errCreate),
				docdb: fake.MockDocDBClientCall{
					CreateDBClusterParameterGroup: []*fake.CallCreateDBClusterParameterGroup{
						{
							Ctx: context.Background(),
							I: &docdb.CreateDBClusterParameterGroupInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
				},
			},
		},
		"ErrorCreate_Parameters": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockCreateDBClusterParameterGroup: func(c context.Context, cdpgi *docdb.CreateDBClusterParameterGroupInput, o ...func(*docdb.Options)) (*docdb.CreateDBClusterParameterGroupOutput, error) {
						return &docdb.CreateDBClusterParameterGroupOutput{
							DBClusterParameterGroup: &docdbtypes.DBClusterParameterGroup{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						}, nil
					},
				},
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
					withParameters(
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testParameterValue),
						},
					),
				),
			},
			want: want{
				cr: instance(
					withDBClusterParameterGroupName(testDBClusterParameterGroupName),
					withExternalName(testDBClusterParameterGroupName),
					withConditions(xpv2.Creating()),
					withParameters(
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testParameterValue),
						},
					),
				),
				result: managed.ExternalCreation{},
				docdb: fake.MockDocDBClientCall{
					CreateDBClusterParameterGroup: []*fake.CallCreateDBClusterParameterGroup{
						{
							Ctx: context.Background(),
							I: &docdb.CreateDBClusterParameterGroupInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			opts := []option{setupExternal}
			e := newExternal(tc.args.kube, tc.args.docdb, opts)
			o, err := e.Create(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, o); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.docdb, tc.args.docdb.Called, cmpopts.IgnoreInterfaces(struct{ context.Context }{}), ignoreUnexported); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	type want struct {
		cr    *svcapitypes.DBClusterParameterGroup
		err   error
		docdb fake.MockDocDBClientCall
	}

	cases := map[string]struct {
		args
		want
	}{
		"SuccessfulDelete": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDeleteDBClusterParameterGroup: func(c context.Context, ddpgi *docdb.DeleteDBClusterParameterGroupInput, o ...func(*docdb.Options)) (*docdb.DeleteDBClusterParameterGroupOutput, error) {
						return &docdb.DeleteDBClusterParameterGroupOutput{}, nil
					},
				},
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
					withConditions(xpv2.Deleting()),
				),
				docdb: fake.MockDocDBClientCall{
					DeleteDBClusterParameterGroup: []*fake.CallDeleteDBClusterParameterGroup{
						{
							Ctx: context.Background(),
							I: &docdb.DeleteDBClusterParameterGroupInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
				},
			},
		},
		"ErrorDelete": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDeleteDBClusterParameterGroup: func(c context.Context, ddpgi *docdb.DeleteDBClusterParameterGroupInput, o ...func(*docdb.Options)) (*docdb.DeleteDBClusterParameterGroupOutput, error) {
						return &docdb.DeleteDBClusterParameterGroupOutput{}, errors.New(testErrDeleteDBClusterParameterGroupFailed)
					},
				},
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
					withConditions(xpv2.Deleting()),
				),
				err: errors.Wrap(errors.New(testErrDeleteDBClusterParameterGroupFailed), errDelete),
				docdb: fake.MockDocDBClientCall{
					DeleteDBClusterParameterGroup: []*fake.CallDeleteDBClusterParameterGroup{
						{
							Ctx: context.Background(),
							I: &docdb.DeleteDBClusterParameterGroupInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			opts := []option{setupExternal}
			e := newExternal(tc.args.kube, tc.args.docdb, opts)
			_, err := e.Delete(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.docdb, tc.args.docdb.Called, cmpopts.IgnoreInterfaces(struct{ context.Context }{}), ignoreUnexported); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	type want struct {
		cr     *svcapitypes.DBClusterParameterGroup
		err    error
		result managed.ExternalUpdate
		docdb  fake.MockDocDBClientCall
	}

	cases := map[string]struct {
		args
		want
	}{
		"SuccessfulUpdate_parameters": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBClusterParameterGroup: func(c context.Context, mdpgi *docdb.ModifyDBClusterParameterGroupInput, o ...func(*docdb.Options)) (*docdb.ModifyDBClusterParameterGroupOutput, error) {
						return &docdb.ModifyDBClusterParameterGroupOutput{
							DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{},
						}, nil
					},
					MockAddTagsToResource: func(_ context.Context, attri *docdb.AddTagsToResourceInput, _ ...func(*docdb.Options)) (*docdb.AddTagsToResourceOutput, error) {
						return &docdb.AddTagsToResourceOutput{}, nil
					},
				},
				cr: instance(
					withDBClusterParameterGroupARN(testDBClusterParameterGroupARN),
					withExternalName(testDBClusterParameterGroupName),
					withParameters(
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testParameterValue),
						},
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testOtherParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testOtherParameterValue),
						},
					),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
				),
			},
			want: want{
				cr: instance(
					withDBClusterParameterGroupARN(testDBClusterParameterGroupARN),
					withExternalName(testDBClusterParameterGroupName),
					withConditions(xpv2.Available()),
					withParameters(
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testParameterValue),
						},
						&svcapitypes.CustomParameter{
							ParameterName:  pointer.ToOrNilIfZeroValue(testOtherParameterName),
							ParameterValue: pointer.ToOrNilIfZeroValue(testOtherParameterValue),
						},
					),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
				),
				docdb: fake.MockDocDBClientCall{
					ModifyDBClusterParameterGroup: []*fake.CallModifyDBClusterParameterGroup{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBClusterParameterGroupInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
								Parameters: []docdbtypes.Parameter{
									{
										ParameterName:  pointer.ToOrNilIfZeroValue(testParameterName),
										ParameterValue: pointer.ToOrNilIfZeroValue(testParameterValue),
									},
									{
										ParameterName:  pointer.ToOrNilIfZeroValue(testOtherParameterName),
										ParameterValue: pointer.ToOrNilIfZeroValue(testOtherParameterValue),
									},
								},
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupARN),
							},
						},
					},
					AddTagsToResource: []*fake.CallAddTagsToResource{
						{
							I: &docdb.AddTagsToResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupARN),
								Tags: []docdbtypes.Tag{
									{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
									{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
								},
							},
						},
					},
				},
			},
		},
		"ErrorUpdate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBClusterParameterGroup: func(c context.Context, mdpgi *docdb.ModifyDBClusterParameterGroupInput, o ...func(*docdb.Options)) (*docdb.ModifyDBClusterParameterGroupOutput, error) {
						return &docdb.ModifyDBClusterParameterGroupOutput{}, errors.New(testErrModifyDBClusterParameterGroupFailed)
					},
				},
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBClusterParameterGroupName),
				),
				err: errors.Wrap(errors.New(testErrModifyDBClusterParameterGroupFailed), errUpdate),
				docdb: fake.MockDocDBClientCall{
					ModifyDBClusterParameterGroup: []*fake.CallModifyDBClusterParameterGroup{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBClusterParameterGroupInput{
								DBClusterParameterGroupName: pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName),
								Parameters:                  []docdbtypes.Parameter{},
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			opts := []option{setupExternal}
			e := newExternal(tc.args.kube, tc.args.docdb, opts)
			o, err := e.Update(context.Background(), tc.args.cr)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.cr, tc.args.cr, test.EquateConditions()); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, o); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.docdb, tc.args.docdb.Called, cmpopts.IgnoreInterfaces(struct{ context.Context }{}), ignoreUnexported); diff != "" {
				t.Errorf("r: -want, +got:\n%s", diff)
			}
		})
	}
}

var ignoreUnexported = cmp.FilterPath(func(p cmp.Path) bool {
	sf, ok := p.Index(-1).(cmp.StructField)
	return ok && !token.IsExported(sf.Name())
}, cmp.Ignore())

func TestDescribeParametersFollowsMarker(t *testing.T) {
	client := &fake.MockDocDBClient{
		MockDescribeDBClusterParameters: func(_ context.Context, in *docdb.DescribeDBClusterParametersInput, _ ...func(*docdb.Options)) (*docdb.DescribeDBClusterParametersOutput, error) {
			if in.Marker == nil {
				return &docdb.DescribeDBClusterParametersOutput{
					Marker:     pointer.ToOrNilIfZeroValue("next"),
					Parameters: []docdbtypes.Parameter{{ParameterName: pointer.ToOrNilIfZeroValue(testParameterName)}},
				}, nil
			}
			return &docdb.DescribeDBClusterParametersOutput{
				Parameters: []docdbtypes.Parameter{{ParameterName: pointer.ToOrNilIfZeroValue(testOtherParameterName)}},
			}, nil
		},
	}
	h := &hooks{client: client}
	got, err := h.describeParameters(context.Background(), pointer.ToOrNilIfZeroValue(testDBClusterParameterGroupName))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(client.Called.DescribeDBClusterParameters) != 2 {
		t.Errorf("describeParameters(...) returned %d parameters in %d calls, want 2 in 2", len(got), len(client.Called.DescribeDBClusterParameters))
	}
	if m := client.Called.DescribeDBClusterParameters[1].I.Marker; pointer.StringValue(m) != "next" {
		t.Errorf("second call Marker = %q, want %q", pointer.StringValue(m), "next")
	}
}

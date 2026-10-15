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

package dbinstance

import (
	"context"
	"go/token"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/docdb"
	docdbtypes "github.com/aws/aws-sdk-go-v2/service/docdb/types"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	cpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/docdb/v1alpha1"
	awsclient "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/aws"
	fake "github.com/swisscom/crossplane-provider-aws-v2/internal/clients/docdb/fake"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/compare"
	pointer "github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
)

var (
	testDBIdentifier                    = "some-db-name"
	testDBInstanceArn                   = "aws:test:docdb:test-region:test-account:test-db"
	testAddress                         = "127.0.0.1"
	testHostedZone                      = "some-hosted-zone"
	testPort                            = 27017
	testAvailabilityZone                = "test-zone-a"
	testOtherAvailabilityZone           = "test-zone-b"
	testCACertificateIdentifier         = "some-certificate"
	testCACertificateDetails            = docdbtypes.CertificateDetails{CAIdentifier: pointer.ToOrNilIfZeroValue(testCACertificateIdentifier)}
	testOtherCACertificateIdentifier    = "some-other-certificate"
	testDBInstanceClass                 = "some-db-instance-class"
	testOtherDBInstanceClass            = "some-other-db-instance-class"
	testPreferredMaintenanceWindow      = "1 day"
	testOtherPreferredMaintenanceWindow = "10 days"
	testPromotionTier                   = 42
	testOtherPromotionTier              = 9000
	testTagKey                          = "some-tag-key"
	testTagValue                        = "some-tag-value"
	testOtherTagKey                     = "some-other-tag-key"
	testOtherTagValue                   = "some-other-tag-value"

	testErrDescribeDBInstancesFailed = "DescribeDBInstances failed"
	testErrCreateDBInstanceFailed    = "CreateDBInstance failed"
	testErrDeleteDBInstanceFailed    = "DeleteDBInstance failed"
	testErrModifyDBInstanceFailed    = "ModifyDBInstance failed"
)

type docDBModifier func(*svcapitypes.DBInstance)

func instance(m ...docDBModifier) *svcapitypes.DBInstance {
	cr := &svcapitypes.DBInstance{}
	for _, f := range m {
		f(cr)
	}
	return cr
}

func withExternalName(value string) docDBModifier {
	return func(o *svcapitypes.DBInstance) {
		meta.SetExternalName(o, value)
	}
}

func withDBIdentifier(value string) docDBModifier {
	return func(o *svcapitypes.DBInstance) {
		o.Status.AtProvider.DBInstanceIdentifier = pointer.ToOrNilIfZeroValue(value)
	}
}

func withDBInstanceArn(value string) docDBModifier {
	return func(o *svcapitypes.DBInstance) {
		o.Status.AtProvider.DBInstanceARN = pointer.ToOrNilIfZeroValue(value)
	}
}

func withEndpoint(value *svcapitypes.Endpoint) docDBModifier {
	return func(o *svcapitypes.DBInstance) {
		o.Status.AtProvider.Endpoint = value
	}
}

func withStatus(value string) docDBModifier {
	return func(o *svcapitypes.DBInstance) {
		o.Status.AtProvider.DBInstanceStatus = pointer.ToOrNilIfZeroValue(value)
	}
}

func withConditions(value ...xpv2.Condition) docDBModifier {
	return func(o *svcapitypes.DBInstance) {
		o.Status.SetConditions(value...)
	}
}

func withAvailabilityZone(value string) docDBModifier {
	return func(o *svcapitypes.DBInstance) {
		o.Spec.ForProvider.AvailabilityZone = pointer.ToOrNilIfZeroValue(value)
	}
}

func withAutoMinorVersionUpgrade(value bool) docDBModifier {
	return func(o *svcapitypes.DBInstance) {
		o.Spec.ForProvider.AutoMinorVersionUpgrade = ptr.To(value)
	}
}

func withCACertificateIdentifier(value string) docDBModifier {
	return func(o *svcapitypes.DBInstance) {
		o.Spec.ForProvider.CACertificateIdentifier = pointer.ToOrNilIfZeroValue(value)
	}
}

func withStatusCACertificateIdentifier(value string) docDBModifier {
	return func(o *svcapitypes.DBInstance) {
		o.Status.AtProvider.CertificateDetails = &svcapitypes.CertificateDetails{
			CAIdentifier: pointer.ToOrNilIfZeroValue(value),
		}
	}
}

func withDBInstanceClass(value string) docDBModifier {
	return func(o *svcapitypes.DBInstance) {
		o.Spec.ForProvider.DBInstanceClass = pointer.ToOrNilIfZeroValue(value)
	}
}

func withPreferredMaintenanceWindow(value string) docDBModifier {
	return func(o *svcapitypes.DBInstance) {
		o.Spec.ForProvider.PreferredMaintenanceWindow = pointer.ToOrNilIfZeroValue(value)
	}
}

func withStatusPreferredMaintenanceWindow(value string) docDBModifier {
	return func(o *svcapitypes.DBInstance) {
		o.Status.AtProvider.PreferredMaintenanceWindow = pointer.ToOrNilIfZeroValue(value)
	}
}

func withPromotionTier(value int) docDBModifier {
	return func(o *svcapitypes.DBInstance) {
		o.Spec.ForProvider.PromotionTier = pointer.ToIntAsInt64(value)
	}
}

func withTags(values ...*svcapitypes.Tag) docDBModifier {
	return func(o *svcapitypes.DBInstance) {
		o.Spec.ForProvider.Tags = values
	}
}

func generateConnectionDetails(address string, port int) managed.ConnectionDetails {
	return managed.ConnectionDetails{
		awsclient.ResourceCredentialsSecretEndpointKey: []byte(address),
		awsclient.ResourceCredentialsSecretPortKey:     []byte(strconv.Itoa(port)),
	}
}

func TestObserve(t *testing.T) {
	type want struct {
		cr     *svcapitypes.DBInstance
		result managed.ExternalObservation
		err    error
		docdb  fake.MockDocDBClientCall
	}

	cases := map[string]struct {
		args
		want
	}{
		"AvailableState_and_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withConditions(xpv2.Available()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{I: &docdb.ListTagsForResourceInput{ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn)}},
					},
				},
			},
		},
		"AvailableState_and_outdated_AvailabilityZone": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								AvailabilityZone:     pointer.ToOrNilIfZeroValue(testAvailabilityZone),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withAvailabilityZone(testOtherAvailabilityZone),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withConditions(xpv2.Available()),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withAvailabilityZone(testOtherAvailabilityZone),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
				},
			},
		},
		"AvailableState_AutoMinorVersionUpgrade_diff_ignored": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:        pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
								DBInstanceIdentifier:    pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:           pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Endpoint:                &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								AutoMinorVersionUpgrade: pointer.ToOrNilIfZeroValue(true),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withAutoMinorVersionUpgrade(false),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withConditions(xpv2.Available()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withAutoMinorVersionUpgrade(false),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{I: &docdb.ListTagsForResourceInput{ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn)}},
					},
				},
			},
		},
		"AvailableState_and_outdated_CACertificateIdentifier": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:        pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
								DBInstanceIdentifier:    pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:           pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Endpoint:                &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								CACertificateIdentifier: pointer.ToOrNilIfZeroValue(testCACertificateIdentifier),
								CertificateDetails:      pointer.ToOrNilIfZeroValue(testCACertificateDetails),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withCACertificateIdentifier(testOtherCACertificateIdentifier),
					withStatusCACertificateIdentifier(testOtherCACertificateIdentifier),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withConditions(xpv2.Available()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withCACertificateIdentifier(testOtherCACertificateIdentifier),
					withStatusCACertificateIdentifier(testCACertificateIdentifier),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: caCertificateIdentifier",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"AvailableState_and_outdated_DBInstanceClass": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								DBInstanceClass:      pointer.ToOrNilIfZeroValue(testDBInstanceClass),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceClass(testOtherDBInstanceClass),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withConditions(xpv2.Available()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withDBInstanceClass(testOtherDBInstanceClass),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: dbInstanceClass",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"AvailableState_and_outdated_PreferredMaintenanceWindow": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:           pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
								DBInstanceIdentifier:       pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:              pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Endpoint:                   &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								PreferredMaintenanceWindow: pointer.ToOrNilIfZeroValue(testPreferredMaintenanceWindow),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withPreferredMaintenanceWindow(testOtherPreferredMaintenanceWindow),
					withStatusPreferredMaintenanceWindow(testOtherPreferredMaintenanceWindow),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withConditions(xpv2.Available()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withPreferredMaintenanceWindow(testOtherPreferredMaintenanceWindow),
					withStatusPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: preferredMaintenanceWindow",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"AvailableState_and_outdated_PromotionTier": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								PromotionTier:        pointer.ToIntAsInt32(testPromotionTier),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withPromotionTier(testOtherPromotionTier),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withConditions(xpv2.Available()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withPromotionTier(testOtherPromotionTier),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: promotionTier",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"AvailableState_and_outdated_tags": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{
							{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)}),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withConditions(xpv2.Available()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)}),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: tags",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn)},
						},
					},
				},
			},
		},
		"AvailableState_and_same_tags": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateAvailable),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{
							{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)}),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateAvailable),
					withConditions(xpv2.Available()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)}),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
				},
			},
		},
		"FailedState_and_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateFailed),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateFailed),
					withConditions(xpv2.Unavailable()),
				),
				result: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
				},
			},
		},
		"FailedState_and_outdated_AvailabilityZone": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateFailed),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								AvailabilityZone:     pointer.ToOrNilIfZeroValue(testAvailabilityZone),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withAvailabilityZone(testOtherAvailabilityZone),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withConditions(xpv2.Unavailable()),
					withStatus(svcapitypes.DocDBInstanceStateFailed),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withAvailabilityZone(testOtherAvailabilityZone),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
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
		"FailedState_AutoMinorVersionUpgrade_diff_ignored": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:        pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateFailed),
								DBInstanceIdentifier:    pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:                &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								AutoMinorVersionUpgrade: pointer.ToOrNilIfZeroValue(true),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withAutoMinorVersionUpgrade(false),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateFailed),
					withConditions(xpv2.Unavailable()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withAutoMinorVersionUpgrade(false),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{I: &docdb.ListTagsForResourceInput{}},
					},
				},
			},
		},
		"FailedState_and_outdated_CACertificateIdentifier": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:        pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateFailed),
								DBInstanceIdentifier:    pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:                &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								CACertificateIdentifier: pointer.ToOrNilIfZeroValue(testCACertificateIdentifier),
								CertificateDetails:      pointer.ToOrNilIfZeroValue(testCACertificateDetails),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withCACertificateIdentifier(testOtherCACertificateIdentifier),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateFailed),
					withConditions(xpv2.Unavailable()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withCACertificateIdentifier(testOtherCACertificateIdentifier),
					withStatusCACertificateIdentifier(testCACertificateIdentifier),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: caCertificateIdentifier",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"FailedState_and_outdated_DBInstanceClass": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateFailed),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								DBInstanceClass:      pointer.ToOrNilIfZeroValue(testDBInstanceClass),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceClass(testOtherDBInstanceClass),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateFailed),
					withConditions(xpv2.Unavailable()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withDBInstanceClass(testOtherDBInstanceClass),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: dbInstanceClass",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"FailedState_and_outdated_PreferredMaintenanceWindow": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:           pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateFailed),
								DBInstanceIdentifier:       pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:                   &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								PreferredMaintenanceWindow: pointer.ToOrNilIfZeroValue(testPreferredMaintenanceWindow),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withPreferredMaintenanceWindow(testOtherPreferredMaintenanceWindow),
					withStatusPreferredMaintenanceWindow(testOtherPreferredMaintenanceWindow),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateFailed),
					withConditions(xpv2.Unavailable()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withPreferredMaintenanceWindow(testOtherPreferredMaintenanceWindow),
					withStatusPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: preferredMaintenanceWindow",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"FailedState_and_outdated_PromotionTier": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateFailed),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								PromotionTier:        pointer.ToIntAsInt32(testPromotionTier),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withPromotionTier(testOtherPromotionTier),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateFailed),
					withConditions(xpv2.Unavailable()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withPromotionTier(testOtherPromotionTier),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: promotionTier",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"FailedState_and_outdated_tags": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateFailed),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{
							{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)}),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateFailed),
					withConditions(xpv2.Unavailable()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)}),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: tags",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
				},
			},
		},
		"FailedState_and_same_tags": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateFailed),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{
							{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)}),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateFailed),
					withConditions(xpv2.Unavailable()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)}),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
				},
			},
		},
		"DeletingState_and_UpToDate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateDeleting),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateDeleting),
					withConditions(xpv2.Deleting()),
				),
				result: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
				},
			},
		},
		"DeletingState_and_outdated_AvailabilityZone": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateDeleting),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								AvailabilityZone:     pointer.ToOrNilIfZeroValue(testAvailabilityZone),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withAvailabilityZone(testOtherAvailabilityZone),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withConditions(xpv2.Deleting()),
					withStatus(svcapitypes.DocDBInstanceStateDeleting),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withAvailabilityZone(testOtherAvailabilityZone),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
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
		"DeletingState_AutoMinorVersionUpgrade_diff_ignored": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:        pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateDeleting),
								DBInstanceIdentifier:    pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:                &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								AutoMinorVersionUpgrade: pointer.ToOrNilIfZeroValue(true),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withAutoMinorVersionUpgrade(false),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateDeleting),
					withConditions(xpv2.Deleting()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withAutoMinorVersionUpgrade(false),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{I: &docdb.ListTagsForResourceInput{}},
					},
				},
			},
		},
		"DeletingState_and_outdated_CACertificateIdentifier": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:        pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateDeleting),
								DBInstanceIdentifier:    pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:                &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								CACertificateIdentifier: pointer.ToOrNilIfZeroValue(testCACertificateIdentifier),
								CertificateDetails:      pointer.ToOrNilIfZeroValue(testCACertificateDetails),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withCACertificateIdentifier(testOtherCACertificateIdentifier),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateDeleting),
					withConditions(xpv2.Deleting()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withCACertificateIdentifier(testOtherCACertificateIdentifier),
					withStatusCACertificateIdentifier(testCACertificateIdentifier),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: caCertificateIdentifier",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"DeletingState_and_outdated_DBInstanceClass": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateDeleting),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								DBInstanceClass:      pointer.ToOrNilIfZeroValue(testDBInstanceClass),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceClass(testOtherDBInstanceClass),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateDeleting),
					withConditions(xpv2.Deleting()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withDBInstanceClass(testOtherDBInstanceClass),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: dbInstanceClass",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"DeletingState_and_outdated_PreferredMaintenanceWindow": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:           pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateDeleting),
								DBInstanceIdentifier:       pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:                   &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								PreferredMaintenanceWindow: pointer.ToOrNilIfZeroValue(testPreferredMaintenanceWindow),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withPreferredMaintenanceWindow(testOtherPreferredMaintenanceWindow),
					withStatusPreferredMaintenanceWindow(testOtherPreferredMaintenanceWindow),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateDeleting),
					withConditions(xpv2.Deleting()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withPreferredMaintenanceWindow(testOtherPreferredMaintenanceWindow),
					withStatusPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: preferredMaintenanceWindow",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"DeletingState_and_outdated_PromotionTier": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateDeleting),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								PromotionTier:        pointer.ToIntAsInt32(testPromotionTier),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withPromotionTier(testOtherPromotionTier),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateDeleting),
					withConditions(xpv2.Deleting()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withPromotionTier(testOtherPromotionTier),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: promotionTier",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"DeletingState_and_outdated_tags": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateDeleting),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{
							{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)}),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateDeleting),
					withConditions(xpv2.Deleting()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)}),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: tags",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
				},
			},
		},
		"DeletingState_and_same_tags": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateDeleting),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{
							{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)}),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateDeleting),
					withConditions(xpv2.Deleting()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)}),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
				},
			},
		},
		"CreatingState": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateCreating),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateCreating),
					withConditions(xpv2.Creating()),
				),
				result: managed.ExternalObservation{
					ResourceExists:   true,
					ResourceUpToDate: true,
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
				},
			},
		},
		"CreatingState_and_outdated_AvailabilityZone": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateCreating),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								AvailabilityZone:     pointer.ToOrNilIfZeroValue(testAvailabilityZone),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withAvailabilityZone(testOtherAvailabilityZone),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withConditions(xpv2.Creating()),
					withStatus(svcapitypes.DocDBInstanceStateCreating),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withAvailabilityZone(testOtherAvailabilityZone),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
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
		"CreatingState_AutoMinorVersionUpgrade_diff_ignored": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:        pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateCreating),
								DBInstanceIdentifier:    pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:                &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								AutoMinorVersionUpgrade: pointer.ToOrNilIfZeroValue(true),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withAutoMinorVersionUpgrade(false),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateCreating),
					withConditions(xpv2.Creating()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withAutoMinorVersionUpgrade(false),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{I: &docdb.ListTagsForResourceInput{}},
					},
				},
			},
		},
		"CreatingState_and_outdated_CACertificateIdentifier": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:        pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateCreating),
								DBInstanceIdentifier:    pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:                &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								CACertificateIdentifier: pointer.ToOrNilIfZeroValue(testCACertificateIdentifier),
								CertificateDetails:      pointer.ToOrNilIfZeroValue(testCACertificateDetails),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withCACertificateIdentifier(testOtherCACertificateIdentifier),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateCreating),
					withConditions(xpv2.Creating()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withCACertificateIdentifier(testOtherCACertificateIdentifier),
					withStatusCACertificateIdentifier(testCACertificateIdentifier),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: caCertificateIdentifier",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"CreatingState_and_outdated_DBInstanceClass": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateCreating),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								DBInstanceClass:      pointer.ToOrNilIfZeroValue(testDBInstanceClass),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceClass(testOtherDBInstanceClass),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateCreating),
					withConditions(xpv2.Creating()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withDBInstanceClass(testOtherDBInstanceClass),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: dbInstanceClass",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"CreatingState_and_outdated_PreferredMaintenanceWindow": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:           pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateCreating),
								DBInstanceIdentifier:       pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:                   &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								PreferredMaintenanceWindow: pointer.ToOrNilIfZeroValue(testPreferredMaintenanceWindow),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withPreferredMaintenanceWindow(testOtherPreferredMaintenanceWindow),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateCreating),
					withConditions(xpv2.Creating()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withPreferredMaintenanceWindow(testOtherPreferredMaintenanceWindow),
					withStatusPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: preferredMaintenanceWindow",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"CreatingState_and_outdated_PromotionTier": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateCreating),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
								PromotionTier:        pointer.ToIntAsInt32(testPromotionTier),
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withPromotionTier(testOtherPromotionTier),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withStatus(svcapitypes.DocDBInstanceStateCreating),
					withConditions(xpv2.Creating()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withPromotionTier(testOtherPromotionTier),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: promotionTier",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"CreatingState_and_outdated_tags": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateCreating),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{
							{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)}),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateCreating),
					withConditions(xpv2.Creating()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)}),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  false,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
					Diff:              "spec.forProvider fields differ from the observed state: tags",
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn)},
						},
					},
				},
			},
		},
		"CreatingState_and_same_tags": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{
							{
								DBInstanceStatus:     pointer.ToOrNilIfZeroValue(svcapitypes.DocDBInstanceStateCreating),
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Endpoint:             &docdbtypes.Endpoint{Address: pointer.ToOrNilIfZeroValue(testAddress), Port: pointer.ToIntAsInt32(testPort)},
							},
						}}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{TagList: []docdbtypes.Tag{
							{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						}}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)}),
				),
			},
			want: want{
				cr: instance(
					withExternalName(testDBIdentifier),
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withStatus(svcapitypes.DocDBInstanceStateCreating),
					withConditions(xpv2.Creating()),
					withEndpoint(&svcapitypes.Endpoint{
						Address: pointer.ToOrNilIfZeroValue(testAddress),
						Port:    pointer.ToIntAsInt64(testPort),
					}),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)}),
				),
				result: managed.ExternalObservation{
					ResourceExists:    true,
					ResourceUpToDate:  true,
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
				},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
				},
			},
		},
		"EmptyDescribeInstancesOutput": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{}}, nil
					},
				},
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
				),
				result: managed.ExternalObservation{},
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
							Opts: nil,
						},
					},
				},
			},
		},
		"ErrorDescribeDBInstances": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDescribeDBInstances: func(c context.Context, ddi *docdb.DescribeDBInstancesInput, o ...func(*docdb.Options)) (*docdb.DescribeDBInstancesOutput, error) {
						return nil, errors.New(testErrDescribeDBInstancesFailed)
					},
				},
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
				),
				result: managed.ExternalObservation{},
				err:    awsclient.Wrap(cpresource.Ignore(IsNotFound, errors.New(testErrDescribeDBInstancesFailed)), errDescribe),
				docdb: fake.MockDocDBClientCall{
					DescribeDBInstances: []*fake.CallDescribeDBInstances{
						{
							Ctx: context.Background(),
							I: describeInput(instance(
								withExternalName(testDBIdentifier),
								withDBIdentifier(testDBIdentifier),
							)),
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
		cr     *svcapitypes.DBInstance
		result managed.ExternalCreation
		err    error
		docdb  fake.MockDocDBClientCall
	}

	cases := map[string]struct {
		args
		want
	}{
		"SuccessfulCreate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockCreateDBInstance: func(c context.Context, cdi *docdb.CreateDBInstanceInput, opts ...func(*docdb.Options)) (*docdb.CreateDBInstanceOutput, error) {
						return &docdb.CreateDBInstanceOutput{
							DBInstance: &docdbtypes.DBInstance{
								AutoMinorVersionUpgrade: pointer.ToOrNilIfZeroValue(true),
								AvailabilityZone:        &testAvailabilityZone,
								CACertificateIdentifier: &testCACertificateIdentifier,
								CertificateDetails:      pointer.ToOrNilIfZeroValue(testCACertificateDetails),
								DBInstanceClass:         &testDBInstanceClass,
								DBInstanceIdentifier:    pointer.ToOrNilIfZeroValue(testDBIdentifier),
								Endpoint: &docdbtypes.Endpoint{
									Address:      pointer.ToOrNilIfZeroValue(testAddress),
									HostedZoneId: pointer.ToOrNilIfZeroValue(testHostedZone),
									Port:         pointer.ToIntAsInt32(testPort),
								},
								PreferredMaintenanceWindow: &testPreferredMaintenanceWindow,
								PromotionTier:              pointer.ToIntAsInt32(testPromotionTier),
							},
						}, nil
					},
				},
				cr: instance(
					withExternalName(testDBIdentifier),
					withAutoMinorVersionUpgrade(true),
					withAvailabilityZone(testAvailabilityZone),
					withCACertificateIdentifier(testCACertificateIdentifier),
					withDBInstanceClass(testDBInstanceClass),
					withPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
					withPromotionTier(testPromotionTier),
				),
			},
			want: want{
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
					withAutoMinorVersionUpgrade(true),
					withAvailabilityZone(testAvailabilityZone),
					withCACertificateIdentifier(testCACertificateIdentifier),
					withStatusCACertificateIdentifier(testCACertificateIdentifier),
					withDBInstanceClass(testDBInstanceClass),
					withPreferredMaintenanceWindow(testPreferredMaintenanceWindow),
					withPromotionTier(testPromotionTier),
					withConditions(xpv2.Creating()),
					withEndpoint(&svcapitypes.Endpoint{
						Address:      pointer.ToOrNilIfZeroValue(testAddress),
						HostedZoneID: pointer.ToOrNilIfZeroValue(testHostedZone),
						Port:         pointer.ToIntAsInt64(testPort),
					}),
				),
				result: managed.ExternalCreation{
					ConnectionDetails: generateConnectionDetails(testAddress, testPort),
				},
				docdb: fake.MockDocDBClientCall{
					CreateDBInstance: []*fake.CallCreateDBInstance{
						{
							Ctx: context.Background(),
							I: &docdb.CreateDBInstanceInput{
								CACertificateIdentifier:    pointer.ToOrNilIfZeroValue(testCACertificateIdentifier),
								DBInstanceIdentifier:       pointer.ToOrNilIfZeroValue(testDBIdentifier),
								AutoMinorVersionUpgrade:    pointer.ToOrNilIfZeroValue(true),
								AvailabilityZone:           pointer.ToOrNilIfZeroValue(testAvailabilityZone),
								DBInstanceClass:            pointer.ToOrNilIfZeroValue(testDBInstanceClass),
								PreferredMaintenanceWindow: pointer.ToOrNilIfZeroValue(testPreferredMaintenanceWindow),
								PromotionTier:              pointer.ToIntAsInt32(testPromotionTier),
							},
						},
					},
				},
			},
		},
		"ErrorCreate": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockCreateDBInstance: func(c context.Context, cdi *docdb.CreateDBInstanceInput, opts ...func(*docdb.Options)) (*docdb.CreateDBInstanceOutput, error) {
						return nil, errors.New(testErrCreateDBInstanceFailed)
					},
				},
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
					withConditions(xpv2.Creating()),
				),
				result: managed.ExternalCreation{},
				err:    errors.Wrap(errors.New(testErrCreateDBInstanceFailed), errCreate),
				docdb: fake.MockDocDBClientCall{
					CreateDBInstance: []*fake.CallCreateDBInstance{
						{
							Ctx: context.Background(),
							I: &docdb.CreateDBInstanceInput{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
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
		cr    *svcapitypes.DBInstance
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
					MockDeleteDBInstance: func(c context.Context, ddi *docdb.DeleteDBInstanceInput, o ...func(*docdb.Options)) (*docdb.DeleteDBInstanceOutput, error) {
						return &docdb.DeleteDBInstanceOutput{
							DBInstance: &docdbtypes.DBInstance{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
							},
						}, nil
					},
				},
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
					withConditions(xpv2.Deleting()),
				),
				docdb: fake.MockDocDBClientCall{
					DeleteDBInstance: []*fake.CallDeleteDBInstance{
						{
							Ctx: context.Background(),
							I: &docdb.DeleteDBInstanceInput{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
							},
						},
					},
				},
			},
		},
		"ErrorDelete": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockDeleteDBInstance: func(c context.Context, ddi *docdb.DeleteDBInstanceInput, o ...func(*docdb.Options)) (*docdb.DeleteDBInstanceOutput, error) {
						return nil, errors.New(testErrDeleteDBInstanceFailed)
					},
				},
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
					withConditions(xpv2.Deleting()),
				),
				err: errors.Wrap(errors.New(testErrDeleteDBInstanceFailed), errDelete),
				docdb: fake.MockDocDBClientCall{
					DeleteDBInstance: []*fake.CallDeleteDBInstance{
						{
							Ctx: context.Background(),
							I: &docdb.DeleteDBInstanceInput{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
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
		cr     *svcapitypes.DBInstance
		result managed.ExternalUpdate
		err    error
		docdb  fake.MockDocDBClientCall
	}

	cases := map[string]struct {
		args
		want
	}{
		"SuccessfulUpdate_no_tags": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBInstance: func(c context.Context, mdi *docdb.ModifyDBInstanceInput, o ...func(*docdb.Options)) (*docdb.ModifyDBInstanceOutput, error) {
						return &docdb.ModifyDBInstanceOutput{
							DBInstance: &docdbtypes.DBInstance{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{}, nil
					},
				},
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withExternalName(testDBIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withExternalName(testDBIdentifier),
				),
				result: managed.ExternalUpdate{},
				docdb: fake.MockDocDBClientCall{
					ModifyDBInstance: []*fake.CallModifyDBInstance{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBInstanceInput{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
				},
			},
		},
		"SuccessfulUpdate_same_tags": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBInstance: func(c context.Context, mdi *docdb.ModifyDBInstanceInput, o ...func(*docdb.Options)) (*docdb.ModifyDBInstanceOutput, error) {
						return &docdb.ModifyDBInstanceOutput{
							DBInstance: &docdbtypes.DBInstance{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{
								{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
							},
						}, nil
					},
				},
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)}),
				),
			},
			want: want{
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withTags(&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)}),
				),
				result: managed.ExternalUpdate{},
				docdb: fake.MockDocDBClientCall{
					ModifyDBInstance: []*fake.CallModifyDBInstance{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBInstanceInput{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
				},
			},
		},
		"SuccessfulUpdate_add_tag": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBInstance: func(c context.Context, mdi *docdb.ModifyDBInstanceInput, o ...func(*docdb.Options)) (*docdb.ModifyDBInstanceOutput, error) {
						return &docdb.ModifyDBInstanceOutput{
							DBInstance: &docdbtypes.DBInstance{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{
								{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
							},
						}, nil
					},
					MockAddTagsToResource: func(_ context.Context, attri *docdb.AddTagsToResourceInput, _ ...func(*docdb.Options)) (*docdb.AddTagsToResourceOutput, error) {
						return &docdb.AddTagsToResourceOutput{}, nil
					},
				},
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
				),
			},
			want: want{
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
				),
				result: managed.ExternalUpdate{},
				docdb: fake.MockDocDBClientCall{
					ModifyDBInstance: []*fake.CallModifyDBInstance{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBInstanceInput{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
					AddTagsToResource: []*fake.CallAddTagsToResource{
						{
							I: &docdb.AddTagsToResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Tags: []docdbtypes.Tag{
									{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
								},
							},
						},
					},
				},
			},
		},
		"SuccessfulUpdate_remove_tag": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBInstance: func(c context.Context, mdi *docdb.ModifyDBInstanceInput, o ...func(*docdb.Options)) (*docdb.ModifyDBInstanceOutput, error) {
						return &docdb.ModifyDBInstanceOutput{
							DBInstance: &docdbtypes.DBInstance{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{
								{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
							},
						}, nil
					},
					MockRemoveTagsFromResource: func(_ context.Context, rtfri *docdb.RemoveTagsFromResourceInput, _ ...func(*docdb.Options)) (*docdb.RemoveTagsFromResourceOutput, error) {
						return &docdb.RemoveTagsFromResourceOutput{}, nil
					},
				},
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
				),
			},
			want: want{
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
				),
				result: managed.ExternalUpdate{},
				docdb: fake.MockDocDBClientCall{
					ModifyDBInstance: []*fake.CallModifyDBInstance{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBInstanceInput{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
					RemoveTagsFromResource: []*fake.CallRemoveTagsFromResource{
						{
							I: &docdb.RemoveTagsFromResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								TagKeys:      []string{testTagKey},
							},
						},
					},
				},
			},
		},
		"SuccessfulUpdate_overwrite_tag": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBInstance: func(c context.Context, mdi *docdb.ModifyDBInstanceInput, o ...func(*docdb.Options)) (*docdb.ModifyDBInstanceOutput, error) {
						return &docdb.ModifyDBInstanceOutput{
							DBInstance: &docdbtypes.DBInstance{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{
								{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
							},
						}, nil
					},
					MockAddTagsToResource: func(_ context.Context, attri *docdb.AddTagsToResourceInput, _ ...func(*docdb.Options)) (*docdb.AddTagsToResourceOutput, error) {
						return &docdb.AddTagsToResourceOutput{}, nil
					},
					MockRemoveTagsFromResource: func(_ context.Context, rtfri *docdb.RemoveTagsFromResourceInput, _ ...func(*docdb.Options)) (*docdb.RemoveTagsFromResourceOutput, error) {
						return &docdb.RemoveTagsFromResourceOutput{}, nil
					},
				},
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
				),
			},
			want: want{
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
				),
				result: managed.ExternalUpdate{},
				docdb: fake.MockDocDBClientCall{
					ModifyDBInstance: []*fake.CallModifyDBInstance{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBInstanceInput{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
					RemoveTagsFromResource: []*fake.CallRemoveTagsFromResource{
						{
							I: &docdb.RemoveTagsFromResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								TagKeys:      []string{testTagKey},
							},
						},
					},
					AddTagsToResource: []*fake.CallAddTagsToResource{
						{
							I: &docdb.AddTagsToResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Tags: []docdbtypes.Tag{
									{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
								},
							},
						},
					},
				},
			},
		},
		"SuccessfulUpdate_remove_and_add_tag": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBInstance: func(c context.Context, mdi *docdb.ModifyDBInstanceInput, o ...func(*docdb.Options)) (*docdb.ModifyDBInstanceOutput, error) {
						return &docdb.ModifyDBInstanceOutput{
							DBInstance: &docdbtypes.DBInstance{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{
							TagList: []docdbtypes.Tag{
								{Key: pointer.ToOrNilIfZeroValue(testTagKey), Value: pointer.ToOrNilIfZeroValue(testTagValue)},
							},
						}, nil
					},
					MockAddTagsToResource: func(_ context.Context, attri *docdb.AddTagsToResourceInput, _ ...func(*docdb.Options)) (*docdb.AddTagsToResourceOutput, error) {

						return &docdb.AddTagsToResourceOutput{}, nil
					},
					MockRemoveTagsFromResource: func(_ context.Context, rtfri *docdb.RemoveTagsFromResourceInput, _ ...func(*docdb.Options)) (*docdb.RemoveTagsFromResourceOutput, error) {
						return &docdb.RemoveTagsFromResourceOutput{}, nil
					},
				},
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
				),
			},
			want: want{
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withTags(
						&svcapitypes.Tag{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
					),
				),
				result: managed.ExternalUpdate{},
				docdb: fake.MockDocDBClientCall{
					ModifyDBInstance: []*fake.CallModifyDBInstance{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBInstanceInput{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
					RemoveTagsFromResource: []*fake.CallRemoveTagsFromResource{
						{
							I: &docdb.RemoveTagsFromResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								TagKeys:      []string{testTagKey},
							},
						},
					},
					AddTagsToResource: []*fake.CallAddTagsToResource{
						{
							I: &docdb.AddTagsToResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
								Tags: []docdbtypes.Tag{
									{Key: pointer.ToOrNilIfZeroValue(testOtherTagKey), Value: pointer.ToOrNilIfZeroValue(testOtherTagValue)},
								},
							},
						},
					},
				},
			},
		},
		"SuccessfulUpdate_AutoMinorVersionUpgrade_not_sent": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBInstance: func(c context.Context, mdi *docdb.ModifyDBInstanceInput, o ...func(*docdb.Options)) (*docdb.ModifyDBInstanceOutput, error) {
						if mdi.AutoMinorVersionUpgrade != nil {
							return nil, errors.New("AutoMinorVersionUpgrade must not be sent to DocumentDB ModifyDBInstance")
						}
						return &docdb.ModifyDBInstanceOutput{
							DBInstance: &docdbtypes.DBInstance{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
								DBInstanceArn:        pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						}, nil
					},
					MockListTagsForResource: func(_ context.Context, ltfri *docdb.ListTagsForResourceInput, _ ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
						return &docdb.ListTagsForResourceOutput{}, nil
					},
				},
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withExternalName(testDBIdentifier),
					withAutoMinorVersionUpgrade(true),
				),
			},
			want: want{
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withDBInstanceArn(testDBInstanceArn),
					withExternalName(testDBIdentifier),
					withAutoMinorVersionUpgrade(true),
				),
				result: managed.ExternalUpdate{},
				docdb: fake.MockDocDBClientCall{
					ModifyDBInstance: []*fake.CallModifyDBInstance{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBInstanceInput{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
							},
						},
					},
					ListTagsForResource: []*fake.CallListTagsForResource{
						{
							I: &docdb.ListTagsForResourceInput{
								ResourceName: pointer.ToOrNilIfZeroValue(testDBInstanceArn),
							},
						},
					},
				},
			},
		},
		"ErrorModifyDBInstanceWithContext": {
			args: args{
				docdb: &fake.MockDocDBClient{
					MockModifyDBInstance: func(c context.Context, mdi *docdb.ModifyDBInstanceInput, o ...func(*docdb.Options)) (*docdb.ModifyDBInstanceOutput, error) {
						return nil, errors.New(testErrModifyDBInstanceFailed)
					},
				},
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
				),
			},
			want: want{
				cr: instance(
					withDBIdentifier(testDBIdentifier),
					withExternalName(testDBIdentifier),
				),
				err: errors.Wrap(errors.New(testErrModifyDBInstanceFailed), errUpdate),
				docdb: fake.MockDocDBClientCall{
					ModifyDBInstance: []*fake.CallModifyDBInstance{
						{
							Ctx: context.Background(),
							I: &docdb.ModifyDBInstanceInput{
								DBInstanceIdentifier: pointer.ToOrNilIfZeroValue(testDBIdentifier),
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

type args struct {
	docdb *fake.MockDocDBClient
	kube  client.Client
	cr    *svcapitypes.DBInstance
}

var ignoreUnexported = cmp.FilterPath(func(p cmp.Path) bool {
	sf, ok := p.Index(-1).(cmp.StructField)
	return ok && !token.IsExported(sf.Name())
}, cmp.Ignore())

func describeInput(cr *svcapitypes.DBInstance) *docdb.DescribeDBInstancesInput {
	in, _ := GenerateDescribeDBInstancesInput(cr)
	return in
}

func TestIsUpToDate(t *testing.T) {
	observed := docdbtypes.DBInstance{
		DBInstanceArn:               ptr.To("arn"),
		DBInstanceClass:             ptr.To("db.r5.large"),
		AutoMinorVersionUpgrade:     ptr.To(true),
		PerformanceInsightsEnabled:  ptr.To(true),
		PerformanceInsightsKMSKeyId: ptr.To("arn:aws:kms:eu-central-1:123456789012:key/key-id"),
		PromotionTier:               ptr.To[int32](1),
	}
	cases := map[string]struct {
		p    svcapitypes.DBInstanceParameters
		want string
	}{
		"Unmanaged": {},
		"Equal": {
			p: svcapitypes.DBInstanceParameters{
				DBInstanceClass:             ptr.To("db.r5.large"),
				PromotionTier:               ptr.To[int64](1),
				EnablePerformanceInsights:   ptr.To(true),
				PerformanceInsightsKMSKeyID: ptr.To("key-id"),
			},
		},
		"AutoMinorVersionUpgradeIgnored": {
			p: svcapitypes.DBInstanceParameters{AutoMinorVersionUpgrade: ptr.To(false)},
		},
		"Differ": {
			p: svcapitypes.DBInstanceParameters{
				DBInstanceClass:             ptr.To("db.r5.xlarge"),
				PromotionTier:               ptr.To[int64](2),
				EnablePerformanceInsights:   ptr.To(false),
				PerformanceInsightsKMSKeyID: ptr.To("other-key"),
			},
			want: compare.Message([]string{"dbInstanceClass", "promotionTier", "enablePerformanceInsights", "performanceInsightsKMSKeyID"}),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cr := instance()
			cr.Spec.ForProvider = tc.p
			h := &hooks{client: &fake.MockDocDBClient{
				MockListTagsForResource: func(context.Context, *docdb.ListTagsForResourceInput, ...func(*docdb.Options)) (*docdb.ListTagsForResourceOutput, error) {
					return &docdb.ListTagsForResourceOutput{}, nil
				},
			}}
			upToDate, diff, err := h.isUpToDate(context.Background(), cr, &docdb.DescribeDBInstancesOutput{DBInstances: []docdbtypes.DBInstance{observed}})
			if err != nil {
				t.Fatal(err)
			}
			if upToDate != (tc.want == "") || diff != tc.want {
				t.Errorf("isUpToDate(...) = %v, %q; want %v, %q", upToDate, diff, tc.want == "", tc.want)
			}
		})
	}
}

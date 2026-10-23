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

package dbinstance

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	svcsdk "github.com/aws/aws-sdk-go-v2/service/rds"
	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/clients/rds/fake"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/controller/rds/utils"
)

// sdkError is a failed call as the SDK returns it, with a request ID.
func sdkError(requestID string) error {
	return &smithy.OperationError{
		ServiceID:     "RDS",
		OperationName: "Op",
		Err: &awshttp.ResponseError{
			ResponseError: &smithyhttp.ResponseError{
				Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusBadRequest}},
				Err:      &smithy.GenericAPIError{Code: "InvalidParameterValue", Message: "nope"},
			},
			RequestID: requestID,
		},
	}
}

// The request ID differs on every call; in a condition message it would
// change the status on every reconcile.
func TestAWSErrorsHaveNoRequestID(t *testing.T) {
	cases := map[string]struct {
		run  func(t *testing.T, apiErr error) error
		want string
	}{
		"CreateDBInstanceReadReplica": {
			run: func(t *testing.T, apiErr error) error {
				t.Helper()
				c := &fake.MockRDSClient{
					MockCreateDBInstanceReadReplica: func(context.Context, *svcsdk.CreateDBInstanceReadReplicaInput, ...func(*svcsdk.Options)) (*svcsdk.CreateDBInstanceReadReplicaOutput, error) {
						return nil, apiErr
					},
				}
				cr := newTestCR(func(cr *svcapitypes.DBInstance) { cr.Spec.ForProvider.SourceDBInstanceID = aws.String("primary") })
				_, err := newCustomExternal(newTestKube(t), c).Create(context.Background(), cr)
				return err
			},
			want: errCreateReadReplica + ": InvalidParameterValue: nope",
		},
		"AddTagsToResource": {
			run: func(t *testing.T, apiErr error) error {
				t.Helper()
				c := tagFailClient(apiErr)
				cr := newTestCR(func(cr *svcapitypes.DBInstance) { cr.Spec.ForProvider.Tags = specTags(tagTeam, "new") })
				return observeUpdate(t, c, cr)
			},
			want: errAddTags + ": InvalidParameterValue: nope",
		},
		"RemoveTagsFromResource": {
			run: func(t *testing.T, apiErr error) error {
				t.Helper()
				c := tagFailClient(apiErr)
				cr := newTestCR(func(cr *svcapitypes.DBInstance) {
					meta.AddAnnotations(cr, map[string]string{utils.AnnotationManagedTagKeys: tagTeam})
				})
				return observeUpdate(t, c, cr)
			},
			want: errRemoveTags + ": InvalidParameterValue: nope",
		},
		"DescribeDBClusters": {
			run: func(t *testing.T, apiErr error) error {
				t.Helper()
				c := &fake.MockRDSClient{
					MockDescribeDBInstances: describeFn(describeOut("available", func(db *svcsdktypes.DBInstance) {
						db.DBClusterIdentifier = aws.String("cluster1")
					}), nil),
					MockDescribeDBClusters: func(context.Context, *svcsdk.DescribeDBClustersInput, ...func(*svcsdk.Options)) (*svcsdk.DescribeDBClustersOutput, error) {
						return nil, apiErr
					},
				}
				_, err := newCustomExternal(newTestKube(t), c).Observe(context.Background(), newTestCR())
				return err
			},
			want: errDescribeCluster + ": InvalidParameterValue: nope",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			first, second := tc.run(t, sdkError("req-1")), tc.run(t, sdkError("req-2"))
			if first == nil || second == nil {
				t.Fatalf("errors = %v, %v; want both set", first, second)
			}
			if first.Error() != second.Error() {
				t.Errorf("same AWS error, different strings:\n%q\n%q", first, second)
			}
			if !strings.HasSuffix(first.Error(), tc.want) || strings.Contains(first.Error(), "req-1") {
				t.Errorf("error = %q, want it to end in %q without the request ID", first, tc.want)
			}
		})
	}
}

// tagFailClient has AWS tag team=old and fails both tag calls.
func tagFailClient(apiErr error) *fake.MockRDSClient {
	return &fake.MockRDSClient{
		MockDescribeDBInstances: describeFn(describeOut("available", func(db *svcsdktypes.DBInstance) {
			db.TagList = awsTagList(tagTeam, oldValue)
		}), nil),
		MockModifyDBInstance: func(context.Context, *svcsdk.ModifyDBInstanceInput, ...func(*svcsdk.Options)) (*svcsdk.ModifyDBInstanceOutput, error) {
			return &svcsdk.ModifyDBInstanceOutput{}, nil
		},
		MockAddTagsToResource: func(context.Context, *svcsdk.AddTagsToResourceInput, ...func(*svcsdk.Options)) (*svcsdk.AddTagsToResourceOutput, error) {
			return nil, apiErr
		},
		MockRemoveTagsFromResource: func(context.Context, *svcsdk.RemoveTagsFromResourceInput, ...func(*svcsdk.Options)) (*svcsdk.RemoveTagsFromResourceOutput, error) {
			return nil, apiErr
		},
	}
}

func observeUpdate(t *testing.T, c *fake.MockRDSClient, cr *svcapitypes.DBInstance) error {
	t.Helper()
	e := newCustomExternal(newTestKube(t), c)
	if _, err := e.Observe(context.Background(), cr); err != nil {
		t.Fatal(err)
	}
	_, err := e.Update(context.Background(), cr)
	return err
}

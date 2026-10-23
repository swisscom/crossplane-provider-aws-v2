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

package aws

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func TestWrap(t *testing.T) {
	if Wrap(nil, "msg") != nil {
		t.Fatal("Wrap(nil) must be nil")
	}
	err := Wrap(&smithy.GenericAPIError{Code: "NotFound", Message: "gone"}, "describe")
	if got, want := err.Error(), "describe: NotFound: gone"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	var ae smithy.APIError
	if !errors.As(err, &ae) || ae.ErrorCode() != "NotFound" {
		t.Fatalf("wrapped error must unwrap to the API error, got %v", err)
	}
	if got, want := Wrap(errors.New("boom"), "x").Error(), "x: boom"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// sdkError builds the chain the SDK returns for a failed call.
func sdkError(apiErr error) error {
	return &smithy.OperationError{
		ServiceID:     "RDS",
		OperationName: "DescribeDBInstances",
		Err: &awshttp.ResponseError{
			ResponseError: &smithyhttp.ResponseError{
				Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusNotFound}},
				Err:      apiErr,
			},
			RequestID: "req-4711",
		},
	}
}

func TestWrapKeepsAPIErrorChain(t *testing.T) {
	t.Run("GenericAPIError", func(t *testing.T) {
		err := Wrap(&smithy.GenericAPIError{Code: "X", Message: "m"}, "msg")
		var ae smithy.APIError
		if !errors.As(err, &ae) || ae.ErrorCode() != "X" || ae.ErrorMessage() != "m" {
			t.Fatalf("errors.As(%v) = %v, want the API error X: m", err, ae)
		}
		if err.Error() != "msg: X: m" {
			t.Errorf("Error() = %q, want %q", err.Error(), "msg: X: m")
		}
	})
	t.Run("TypedSDKFault", func(t *testing.T) {
		err := Wrap(sdkError(&rdstypes.DBInstanceNotFoundFault{Message: aws.String("gone")}), "cannot describe")
		var nf *rdstypes.DBInstanceNotFoundFault
		if !errors.As(err, &nf) {
			t.Fatalf("errors.As(%v, *DBInstanceNotFoundFault) = false", err)
		}
		var ae smithy.APIError
		if !errors.As(err, &ae) || ae.ErrorFault() != smithy.FaultClient {
			t.Errorf("errors.As(%v, APIError) lost the fault", err)
		}
		if got, want := err.Error(), "cannot describe: DBInstanceNotFound: gone"; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
		if strings.Contains(err.Error(), "req-4711") {
			t.Errorf("Error() %q contains the request ID", err.Error())
		}
	})
	t.Run("NonAPIErrorKeepsChain", func(t *testing.T) {
		base := errors.New("boom")
		if err := Wrap(base, "x"); !errors.Is(err, base) {
			t.Errorf("errors.Is(%v, base) = false", err)
		}
	})
}

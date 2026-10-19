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
	"fmt"

	"github.com/aws/smithy-go"
	xperrors "github.com/crossplane/crossplane-runtime/v2/pkg/errors"
)

// GlobalRegion is used for global services such as IAM.
const GlobalRegion = "us-east-1"

// Wrap wraps err with msg. AWS API errors are reduced to their code and
// message, which drops the per-request ID that would otherwise change the
// resource status on every reconcile. The result still unwraps to err, so
// errors.As finds the API error. Wrap returns nil if err is nil.
func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		return xperrors.Wrap(&apiError{err: err, text: fmt.Sprintf("%s: %s", ae.ErrorCode(), ae.ErrorMessage())}, msg)
	}
	return xperrors.Wrap(err, msg)
}

// apiError reports an AWS API error by code and message only.
type apiError struct {
	err  error
	text string
}

func (e *apiError) Error() string { return e.text }

func (e *apiError) Unwrap() error { return e.err }

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

package utils

import (
	"github.com/aws/smithy-go"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
)

// AWS error codes the ec2 controllers handle.
const (
	CodeGroupNotFound      = "InvalidGroup.NotFound"
	CodeDependency         = "DependencyViolation"
	CodePermissionNotFound = "InvalidPermission.NotFound"
)

// ErrorCode returns the AWS error code of err, or "".
func ErrorCode(err error) string {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		return ae.ErrorCode()
	}
	return ""
}

// IsGroupNotFound reports whether err means that the security group does not exist.
func IsGroupNotFound(err error) bool {
	return ErrorCode(err) == CodeGroupNotFound
}

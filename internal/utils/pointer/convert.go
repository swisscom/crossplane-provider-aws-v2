/*
Copyright 2023 The Crossplane Authors.

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

package pointer

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"time"
)

// SlicePtrToValue converts a slice of pointers to a slice of its respective
// values.
func SlicePtrToValue[T any](slice []*T) []T {
	if slice == nil {
		return nil
	}

	var defaultValue T
	res := make([]T, len(slice))
	for i, s := range slice {
		res[i] = ptr.Deref[T](s, defaultValue)
	}
	return res
}

// SliceValueToPtr converts a slice of values to a slice of pointers.
func SliceValueToPtr[T any](slice []T) []*T {
	if slice == nil {
		return nil
	}

	res := make([]*T, len(slice))
	for i, s := range slice {
		res[i] = ptr.To[T](s)
	}
	return res
}

// TimeToMetaTime converts a standard Go time.Time to a K8s metav1.Time.
func TimeToMetaTime(t *time.Time) *metav1.Time {
	if t == nil {
		return nil
	}
	return &metav1.Time{Time: *t}
}

// Int32 converts an *int64 to an *int32. Values out of the int32 range are
// truncated; AWS APIs do not accept them anyway.
func Int32(v *int64) *int32 {
	if v == nil {
		return nil
	}
	i := int32(*v) //nolint:gosec // see above
	return &i
}

// Int64 converts an *int32 to an *int64.
func Int64(v *int32) *int64 {
	if v == nil {
		return nil
	}
	i := int64(*v)
	return &i
}

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

package pointer

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"
)

func TestValues(t *testing.T) {
	cases := map[string]struct {
		got  any
		want any
	}{
		"StringNil":     {got: StringValue(nil), want: ""},
		"StringSet":     {got: StringValue(ptr.To("a")), want: "a"},
		"BoolNil":       {got: BoolValue(nil), want: false},
		"BoolSet":       {got: BoolValue(ptr.To(true)), want: true},
		"Int64Nil":      {got: Int64Value(nil), want: int64(0)},
		"Int64Set":      {got: Int64Value(ptr.To(int64(7))), want: int64(7)},
		"Int32Nil":      {got: Int32Value(nil), want: int32(0)},
		"Int32Set":      {got: Int32Value(ptr.To(int32(7))), want: int32(7)},
		"Int32OfNil":    {got: Int32(nil), want: (*int32)(nil)},
		"Int32OfSet":    {got: Int32(ptr.To(int64(5))), want: ptr.To(int32(5))},
		"Int64OfNil":    {got: Int64(nil), want: (*int64)(nil)},
		"Int64OfSet":    {got: Int64(ptr.To(int32(5))), want: ptr.To(int64(5))},
		"ToIntZero":     {got: ToIntAsInt64(0), want: (*int64)(nil)},
		"ToIntSet":      {got: ToIntAsInt64(3), want: ptr.To(int64(3))},
		"ToIntZeroOK":   {got: ToIntAsInt64Zero(0), want: ptr.To(int64(0))},
		"ToInt32Zero":   {got: ToIntAsInt32(0), want: (*int32)(nil)},
		"ToInt32Set":    {got: ToIntAsInt32(3), want: ptr.To(int32(3))},
		"ToInt32PtrNil": {got: ToIntAsInt32Ptr(nil), want: (*int32)(nil)},
		"ToInt32PtrSet": {got: ToIntAsInt32Ptr(ptr.To(4)), want: ptr.To(int32(4))},
		"FromInt32Nil":  {got: ToInt32FromIntPtr(nil), want: (*int)(nil)},
		"FromInt32Set":  {got: ToInt32FromIntPtr(ptr.To(int32(4))), want: ptr.To(4)},
		"OrNilZero":     {got: ToOrNilIfZeroValue(""), want: (*string)(nil)},
		"OrNilSet":      {got: ToOrNilIfZeroValue("a"), want: ptr.To("a")},
		"OrNilZeroInt":  {got: ToOrNilIfZeroValue(0), want: (*int)(nil)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, tc.got); diff != "" {
				t.Errorf("-want, +got:\n%s", diff)
			}
		})
	}
}

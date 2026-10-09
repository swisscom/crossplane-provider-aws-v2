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

package compare

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

type scaling struct {
	Max *float64
	Min *float64
}

type tag struct {
	Key   *string
	Value *string
}

func TestIsSubset(t *testing.T) {
	now := time.Now()
	cases := map[string]struct {
		desired  any
		observed any
		want     bool
	}{
		"NilPointerMatchesAnything":  {desired: (*string)(nil), observed: ptr.To("a"), want: true},
		"EqualPointers":              {desired: ptr.To("a"), observed: ptr.To("a"), want: true},
		"DifferentPointers":          {desired: ptr.To(int64(1)), observed: ptr.To(int64(2)), want: false},
		"SetVsNil":                   {desired: ptr.To(false), observed: (*bool)(nil), want: false},
		"FloatEqual":                 {desired: ptr.To(0.5), observed: ptr.To(0.5), want: true},
		"NilSliceMatchesAnything":    {desired: []*string(nil), observed: []*string{ptr.To("a")}, want: true},
		"EmptySliceMustBeEmpty":      {desired: []*string{}, observed: []*string{ptr.To("a")}, want: false},
		"EmptySliceVsNil":            {desired: []*string{}, observed: []*string(nil), want: true},
		"SliceOrderDoesNotMatter":    {desired: []*string{ptr.To("a"), ptr.To("b")}, observed: []*string{ptr.To("b"), ptr.To("a")}, want: true},
		"SliceDuplicates":            {desired: []*string{ptr.To("a"), ptr.To("a")}, observed: []*string{ptr.To("a"), ptr.To("b")}, want: false},
		"SliceLength":                {desired: []*string{ptr.To("a")}, observed: []*string{ptr.To("a"), ptr.To("b")}, want: false},
		"StructPartial":              {desired: &scaling{Max: ptr.To(4.0)}, observed: &scaling{Max: ptr.To(4.0), Min: ptr.To(0.5)}, want: true},
		"StructDiffers":              {desired: &scaling{Min: ptr.To(1.0)}, observed: &scaling{Max: ptr.To(4.0), Min: ptr.To(0.5)}, want: false},
		"StructVsNil":                {desired: &scaling{Min: ptr.To(1.0)}, observed: (*scaling)(nil), want: false},
		"StructSlice":                {desired: []*tag{{Key: ptr.To("k"), Value: ptr.To("v")}}, observed: []*tag{{Key: ptr.To("k"), Value: ptr.To("v")}}, want: true},
		"StructSliceDiffers":         {desired: []*tag{{Key: ptr.To("k"), Value: ptr.To("v")}}, observed: []*tag{{Key: ptr.To("k"), Value: ptr.To("w")}}, want: false},
		"Map":                        {desired: map[string]*string{"a": ptr.To("1")}, observed: map[string]*string{"a": ptr.To("1")}, want: true},
		"MapMissingKey":              {desired: map[string]*string{"a": ptr.To("1")}, observed: map[string]*string{"b": ptr.To("1")}, want: false},
		"MapExtraKey":                {desired: map[string]*string{"a": ptr.To("1")}, observed: map[string]*string{"a": ptr.To("1"), "b": ptr.To("2")}, want: false},
		"TimeEqual":                  {desired: &metav1.Time{Time: now}, observed: &metav1.Time{Time: now.UTC()}, want: true},
		"TimeDiffers":                {desired: &metav1.Time{Time: now}, observed: &metav1.Time{Time: now.Add(time.Second)}, want: false},
		"UntypedNilDesired":          {desired: nil, observed: ptr.To("a"), want: true},
		"ScalarValues":               {desired: "a", observed: "a", want: true},
		"ScalarValuesDiffer":         {desired: "a", observed: "b", want: false},
		"NilObservedWithEmptyStruct": {desired: &scaling{}, observed: (*scaling)(nil), want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := IsSubset(tc.desired, tc.observed); got != tc.want {
				t.Errorf("IsSubset(%v, %v): want %v, got %v", tc.desired, tc.observed, tc.want, got)
			}
		})
	}
}

func TestWithout(t *testing.T) {
	got := Without([]string{"a", "b", "c"}, "b", "x")
	if diff := cmp.Diff([]string{"a", "c"}, got); diff != "" {
		t.Errorf("Without(...): -want, +got:\n%s", diff)
	}
}

func TestKMSKeyIDEqual(t *testing.T) {
	arn := "arn:aws:kms:eu-central-1:123456789012:key/1234abcd"
	cases := map[string]struct {
		a, b string
		want bool
	}{
		"Same":         {a: "1234abcd", b: "1234abcd", want: true},
		"IDAndARN":     {a: "1234abcd", b: arn, want: true},
		"ARNAndID":     {a: arn, b: "1234abcd", want: true},
		"OtherKey":     {a: "5678", b: arn, want: false},
		"PartialMatch": {a: "abcd", b: arn, want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := KMSKeyIDEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("KMSKeyIDEqual(%q, %q): want %v, got %v", tc.a, tc.b, tc.want, got)
			}
		})
	}
}

func TestMessage(t *testing.T) {
	if got := Message(nil); got != "" {
		t.Errorf("Message(nil): want empty, got %q", got)
	}
	want := "spec.forProvider fields differ from the observed state: a, b"
	if got := Message([]string{"a", "b"}); got != want {
		t.Errorf("Message(...): want %q, got %q", want, got)
	}
}

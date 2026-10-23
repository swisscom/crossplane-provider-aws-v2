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
	"time"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestSlicePtrToValue(t *testing.T) {
	cases := map[string]struct {
		in   []*string
		want []string
	}{
		"Nil":        {in: nil, want: nil},
		"Empty":      {in: []*string{}, want: []string{}},
		"Values":     {in: []*string{ptr.To("a"), ptr.To("b")}, want: []string{"a", "b"}},
		"NilElement": {in: []*string{ptr.To("a"), nil}, want: []string{"a", ""}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, SlicePtrToValue(tc.in)); diff != "" {
				t.Errorf("SlicePtrToValue(...): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestSliceValueToPtr(t *testing.T) {
	cases := map[string]struct {
		in   []string
		want []*string
	}{
		"Nil":    {in: nil, want: nil},
		"Empty":  {in: []string{}, want: []*string{}},
		"Values": {in: []string{"a", ""}, want: []*string{ptr.To("a"), ptr.To("")}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, SliceValueToPtr(tc.in)); diff != "" {
				t.Errorf("SliceValueToPtr(...): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestTimeToMetaTime(t *testing.T) {
	now := time.Now()
	if got := TimeToMetaTime(nil); got != nil {
		t.Errorf("TimeToMetaTime(nil): want nil, got %v", got)
	}
	got := TimeToMetaTime(&now)
	if want := (&metav1.Time{Time: now}); !got.Equal(want) {
		t.Errorf("TimeToMetaTime(...): want %v, got %v", want, got)
	}
}

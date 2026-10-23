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

	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"
)

func TestPending(t *testing.T) {
	t.Run("NilPendingKeepsObserved", func(t *testing.T) {
		o := ptr.To("a")
		Pending(&o, nil)
		if *o != "a" {
			t.Errorf("observed changed to %q", *o)
		}
	})
	t.Run("NilObservedTargetIsNoOp", func(t *testing.T) {
		Pending[string](nil, ptr.To("b")) // must not panic
	})
	t.Run("PendingReplacesObserved", func(t *testing.T) {
		o, p := ptr.To("a"), ptr.To("b")
		Pending(&o, p)
		if *o != "b" {
			t.Errorf("observed: want %q, got %q", "b", *o)
		}
	})
	t.Run("PendingFillsUnsetObserved", func(t *testing.T) {
		var o *string
		Pending(&o, ptr.To("b"))
		if o == nil || *o != "b" {
			t.Errorf("observed: want %q, got %v", "b", o)
		}
	})
	t.Run("ObservedDoesNotAliasPending", func(t *testing.T) {
		o, p := ptr.To("a"), ptr.To("b")
		Pending(&o, p)
		*o = "changed"
		if *p != "b" {
			t.Errorf("pending was modified through observed: %q", *p)
		}
	})
}

func TestPendingSlice(t *testing.T) {
	t.Run("NilAndEmptyPendingKeepObserved", func(t *testing.T) {
		for name, p := range map[string][]string{"Nil": nil, "Empty": {}} {
			o := []string{"a"}
			PendingSlice(&o, p)
			if diff := cmp.Diff([]string{"a"}, o); diff != "" {
				t.Errorf("%s: observed changed: -want, +got:\n%s", name, diff)
			}
		}
	})
	t.Run("NilObservedTargetIsNoOp", func(t *testing.T) {
		PendingSlice[string](nil, []string{"b"}) // must not panic
	})
	t.Run("PendingReplacesObserved", func(t *testing.T) {
		o := []string{"a"}
		PendingSlice(&o, []string{"b", "c"})
		if diff := cmp.Diff([]string{"b", "c"}, o); diff != "" {
			t.Errorf("observed: -want, +got:\n%s", diff)
		}
	})
	t.Run("ObservedDoesNotAliasPending", func(t *testing.T) {
		p := []string{"b"}
		var o []string
		PendingSlice(&o, p)
		o[0] = "changed"
		if p[0] != "b" {
			t.Errorf("pending was modified through observed: %q", p[0])
		}
	})
}

// instanceParams stands in for a CRD's ForProvider; the same type holds the
// pending values here, so the test needs no second struct.
type instanceParams struct {
	Class   *string
	Storage *int64
	Tags    []*tag
}

func TestPendingWithIsSubset(t *testing.T) {
	observe := func(current, pending instanceParams) instanceParams {
		Pending(&current.Class, pending.Class)
		Pending(&current.Storage, pending.Storage)
		PendingSlice(&current.Tags, pending.Tags)
		return current
	}
	current := instanceParams{Class: ptr.To("small"), Storage: ptr.To(int64(10))}
	pending := instanceParams{Class: ptr.To("large")}
	cases := map[string]struct {
		desired instanceParams
		want    bool
	}{
		"DesiredEqualsPending":        {desired: instanceParams{Class: ptr.To("large")}, want: true},
		"RevertedToCurrent":           {desired: instanceParams{Class: ptr.To("small")}, want: false},
		"DifferentFromBoth":           {desired: instanceParams{Class: ptr.To("medium")}, want: false},
		"UnpendingFieldStillCompared": {desired: instanceParams{Class: ptr.To("large"), Storage: ptr.To(int64(20))}, want: false},
		"UnpendingFieldMatches":       {desired: instanceParams{Class: ptr.To("large"), Storage: ptr.To(int64(10))}, want: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := IsSubset(tc.desired, observe(current, pending)); got != tc.want {
				t.Errorf("IsSubset(...): want %v, got %v", tc.want, got)
			}
		})
	}
	if *current.Class != "small" {
		t.Error("the overlay must not change the current value it was given a copy of")
	}
}

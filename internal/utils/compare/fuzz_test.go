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
	"slices"
	"testing"

	"k8s.io/utils/ptr"
)

type fuzzSpec struct {
	Items []*item
	List  []*string
}

// decodeItems builds items from byte pairs: a zero-ish byte leaves the field
// unset, so most items are partly specified and the matching has real work.
func decodeItems(data []byte) []*item {
	var out []*item
	for i := 0; i+1 < len(data) && len(out) < 6; i += 2 {
		it := &item{}
		if data[i]%5 != 0 {
			it.A = ptr.To(int64(data[i] % 4))
		}
		if data[i+1]%4 != 0 {
			it.B = ptr.To(string(rune('a' + data[i+1]%3)))
		}
		out = append(out, it)
	}
	return out
}

// decodeList builds a plain-value list; a zero-ish byte is a nil element.
func decodeList(data []byte) []*string {
	var out []*string
	for i := 0; i < len(data) && len(out) < 6; i++ {
		if data[i]%4 == 0 {
			out = append(out, nil)
			continue
		}
		out = append(out, ptr.To(string(rune('a'+data[i]%3))))
	}
	return out
}

func decodeSpec(data []byte) fuzzSpec {
	return fuzzSpec{Items: decodeItems(data), List: decodeList(data)}
}

// shuffled returns a deterministic permutation of in; nil stays nil. A
// negative seed reverses the list.
func shuffled[T any](in []T, seed int) []T {
	if in == nil {
		return nil
	}
	out := append([]T(nil), in...)
	if seed < 0 {
		slices.Reverse(out)
		return out
	}
	for i := len(out) - 1; i > 0; i-- {
		j := (seed + i*7) % (i + 1)
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// permutations calls fn with every ordering of in, until fn returns true.
func permutations[T any](in []T, fn func([]T) bool) bool {
	var rec func(k int, cur []T) bool
	rec = func(k int, cur []T) bool {
		if k == len(cur) {
			return fn(cur)
		}
		for i := k; i < len(cur); i++ {
			cur[k], cur[i] = cur[i], cur[k]
			if rec(k+1, cur) {
				return true
			}
			cur[k], cur[i] = cur[i], cur[k]
		}
		return false
	}
	return rec(0, append([]T(nil), in...))
}

// oracle is the definition: some ordering of observed fits desired position by
// position. It is only usable for tiny lists.
func oracle[T any](d, o []T) bool {
	if d == nil {
		return true
	}
	if len(d) != len(o) {
		return false
	}
	return permutations(o, func(p []T) bool {
		for i := range d {
			if !IsSubset(d[i], p[i]) {
				return false
			}
		}
		return true
	})
}

func FuzzIsSubsetProperties(f *testing.F) {
	f.Add([]byte("202000"), []byte("000010")) // order-dependent result of the old greedy matching
	f.Add([]byte(""), []byte("00"))
	f.Add([]byte("0009201029B0"), []byte("0009B9001910")) // a search reused the marks of the one before
	f.Add([]byte{1, 1, 2, 2}, []byte{1, 1, 2, 2})
	f.Fuzz(func(t *testing.T, dRaw, oRaw []byte) {
		d, o := decodeSpec(dRaw), decodeSpec(oRaw)
		// Each list is checked alone, so one list can't hide a bug in the other.
		checkList(t, "items", d.Items, o.Items, dRaw, oRaw)
		checkList(t, "plain", d.List, o.List, dRaw, oRaw)

		if !IsSubset(d, d) {
			t.Fatalf("not reflexive: %v", dRaw)
		}
		if !IsSubset(fuzzSpec{}, o) {
			t.Fatalf("empty desired must always match: %v", oRaw)
		}
		if IsSubset(d, o) != (IsSubset(fuzzSpec{Items: d.Items}, fuzzSpec{Items: o.Items}) &&
			IsSubset(fuzzSpec{List: d.List}, fuzzSpec{List: o.List})) {
			t.Fatalf("fields are not independent: d=%v o=%v", dRaw, oRaw)
		}
		checkMonotonic(t, d, o, dRaw, oRaw)
	})
}

// checkMonotonic asserts that managing fewer fields never breaks a match.
func checkMonotonic(t *testing.T, d, o fuzzSpec, dRaw, oRaw []byte) {
	t.Helper()
	if len(d.Items) == 0 || !IsSubset(d, o) {
		return
	}
	relaxed := *d.Items[0]
	relaxed.B = nil
	d.Items = append([]*item{&relaxed}, d.Items[1:]...)
	if !IsSubset(d, o) {
		t.Fatalf("unsetting a desired field broke a match: d=%v o=%v", dRaw, oRaw)
	}
}

// checkList asserts the properties of IsSubset for one list field.
func checkList[T any](t *testing.T, name string, d, o []T, dRaw, oRaw []byte) {
	t.Helper()
	wrap := func(l []T) any { return struct{ L []T }{l} }
	got := IsSubset(wrap(d), wrap(o))
	if want := oracle(d, o); got != want {
		t.Fatalf("%s: d=%v o=%v: IsSubset=%v, brute force=%v", name, dRaw, oRaw, got, want)
	}
	for _, seed := range []int{-1, len(dRaw), len(oRaw)} {
		if IsSubset(wrap(shuffled(d, seed)), wrap(o)) != got || IsSubset(wrap(d), wrap(shuffled(o, seed))) != got {
			t.Fatalf("%s: order dependent: d=%v o=%v seed=%d want %v", name, dRaw, oRaw, seed, got)
		}
	}
}

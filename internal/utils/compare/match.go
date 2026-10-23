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

import "reflect"

// maxAugmentLen caps the exact matching, so a huge list can't make us build a
// huge table. Above it the first-fit result stands. AWS lists are far smaller.
const maxAugmentLen = 1024

const (
	memoUnknown int8 = iota
	memoFit
	memoMiss
)

// matcher pairs every desired element with its own observed element.
type matcher struct {
	w       *walker
	d, o    reflect.Value
	matchO  []int  // observed index -> desired index, or -1
	memo    []int8 // n*n cache, so each pair is compared once; nil above the cap
	visited []bool
}

// matchSlices reports whether the desired elements can be assigned one-to-one
// to observed elements. Both slices must have the same length.
func (w *walker) matchSlices(d, o reflect.Value) bool {
	if isPlain(d.Type().Elem()) {
		return w.matchPlain(d, o)
	}
	n := d.Len()
	m := &matcher{w: w, d: d, o: o, matchO: make([]int, n)}
	for j := range m.matchO {
		m.matchO[j] = -1
	}
	if n <= maxAugmentLen && w.spend(n*n/8) { // the table costs memory
		m.memo = make([]int8, n*n)
		m.visited = make([]bool, n)
	}
	// First fit is exact when it succeeds, and that is the common case. When
	// an element fits no free partner it is re-routed at once; if that fails
	// too, no perfect matching exists (see augment), so stop instead of
	// scanning on.
	for i := range n {
		if m.take(i) {
			continue
		}
		if m.memo == nil || !m.reroute(i) {
			return false
		}
	}
	return w.left >= 0
}

// reroute finds a partner for desired i by moving earlier pairs. It fails only
// when no perfect matching exists, or when the work budget runs out.
func (m *matcher) reroute(i int) bool {
	clear(m.visited)
	return m.augment(i)
}

// take gives desired i the first free observed element it fits.
func (m *matcher) take(i int) bool {
	for j := range m.matchO {
		if m.matchO[j] < 0 && m.fits(i, j) {
			m.matchO[j] = i
			return true
		}
	}
	return false
}

// augment finds a partner for desired i, moving earlier pairs if it must.
func (m *matcher) augment(i int) bool {
	return m.takeFree(i) || m.moveOthers(i)
}

// takeFree looks for a free partner first: it ends the search without moving
// anyone.
func (m *matcher) takeFree(i int) bool {
	for j := range m.matchO {
		if !m.w.spend(1) { // scanning is work too, even with cached results
			return false
		}
		if m.matchO[j] < 0 && m.fits(i, j) {
			m.matchO[j] = i
			return true
		}
	}
	return false
}

// moveOthers tries every taken partner that fits i, asking its owner to move.
func (m *matcher) moveOthers(i int) bool {
	for j := range m.matchO {
		if !m.w.spend(1) {
			return false
		}
		if m.matchO[j] < 0 || m.visited[j] || !m.fits(i, j) {
			continue
		}
		m.visited[j] = true
		if m.augment(m.matchO[j]) {
			m.matchO[j] = i
			return true
		}
	}
	return false
}

func (m *matcher) fits(i, j int) bool {
	if m.memo == nil {
		return m.w.subset(m.d.Index(i), m.o.Index(j))
	}
	c := &m.memo[i*m.d.Len()+j]
	if *c == memoUnknown {
		*c = memoMiss
		if m.w.subset(m.d.Index(i), m.o.Index(j)) {
			*c = memoFit
		}
	}
	return *c == memoFit
}

// isPlain reports whether t is a basic value, or a pointer to one.
func isPlain(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() { //nolint:exhaustive // only basic kinds count
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// matchPlain is matchSlices for plain values: it counts instead of searching,
// so a long list of strings stays linear.
func (w *walker) matchPlain(d, o reflect.Value) bool {
	if !w.spend(d.Len() + o.Len()) {
		return false
	}
	left := make(map[any]int, o.Len())
	for j := range o.Len() {
		if v, ok := plainValue(o.Index(j)); ok {
			left[v]++
		}
	}
	for i := range d.Len() {
		v, ok := plainValue(d.Index(i))
		if !ok {
			continue // a nil element takes whatever is left over
		}
		if left[v] == 0 {
			return false
		}
		left[v]--
	}
	// Both lists are equally long, so what is left is exactly the wildcards.
	return true
}

// plainValue returns the value behind e, or false for a nil pointer.
func plainValue(e reflect.Value) (any, bool) {
	if e.Kind() == reflect.Pointer {
		if e.IsNil() {
			return nil, false
		}
		e = e.Elem()
	}
	return e.Interface(), true
}

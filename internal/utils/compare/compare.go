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

// Package compare compares desired spec fields with observed ones.
package compare

import (
	"reflect"
	"slices"
	"strings"
	"time"
)

// IsSubset reports whether observed has every value that is set in desired.
//
// Unset values in desired (nil pointers, slices, maps and funcs) match
// anything: the field is not managed. An unset observed value is the zero
// value, so a pointer to an empty struct in desired matches nil, but a
// pointer to "" or false does not. Slices must have the same length and are
// matched regardless of order, because AWS does not keep the order of most
// lists; elements are paired one-to-one, and above 1024 struct elements only
// the first-fit pairing is tried. Maps must have the same keys, and every
// desired key must exist in observed. Values of different types differ.
//
// A call does a bounded amount of work (about a million comparison steps,
// far more than any real AWS list needs). If a list is so large and so
// awkward that the budget runs out, the values are reported as different.
func IsSubset(desired, observed any) bool {
	w := &walker{left: workBudget}
	ok := w.subset(reflect.ValueOf(desired), reflect.ValueOf(observed))
	return ok && w.left >= 0
}

// workBudget caps the work of one IsSubset call. Matching lists can need far
// more than a linear number of comparisons, and the lists are tenant input, so
// an unbounded search could pin a reconcile worker. Running out can only turn
// "up to date" into "differs", which leads to a visible Modify.
const workBudget = 1 << 20

// walker carries the work left for one IsSubset call.
type walker struct{ left int }

// spend charges n units and reports whether any budget is left.
func (w *walker) spend(n int) bool {
	w.left -= n
	return w.left >= 0
}

var timeType = reflect.TypeFor[time.Time]()

func (w *walker) subset(d, o reflect.Value) bool { //nolint:gocyclo // one case per kind
	if !d.IsValid() || isUnset(d) {
		return true
	}
	if !w.spend(1) {
		return false
	}
	if !o.IsValid() {
		o = reflect.Zero(d.Type())
	}
	// A mismatch is reported as a difference instead of panicking below.
	if d.Type() != o.Type() {
		return false
	}
	switch d.Kind() { //nolint:exhaustive // the default case handles scalars
	case reflect.Pointer, reflect.Interface:
		return w.subsetPointer(d, o)
	case reflect.Slice:
		return d.Len() == o.Len() && w.matchSlices(d, o)
	case reflect.Array:
		return w.subsetArray(d, o)
	case reflect.Map:
		return w.subsetMap(d, o)
	case reflect.Struct:
		return w.subsetStruct(d, o)
	case reflect.Func:
		return false // funcs can't be compared
	default:
		return d.Equal(o)
	}
}

func isUnset(v reflect.Value) bool {
	switch v.Kind() { //nolint:exhaustive // only these kinds can be nil
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map, reflect.Func:
		return v.IsNil()
	default:
		return false
	}
}

func (w *walker) subsetPointer(d, o reflect.Value) bool {
	if !o.IsNil() {
		return w.subset(d.Elem(), o.Elem())
	}
	// A missing struct is an empty one, but a missing string or bool is not
	// the same as "" or false.
	if d.Kind() == reflect.Pointer && d.Elem().Kind() == reflect.Struct {
		return w.subset(d.Elem(), reflect.Zero(d.Type().Elem()))
	}
	return false
}

func (w *walker) subsetArray(d, o reflect.Value) bool {
	for i := range d.Len() {
		if !w.subset(d.Index(i), o.Index(i)) {
			return false
		}
	}
	return true
}

func (w *walker) subsetMap(d, o reflect.Value) bool {
	if d.Len() != o.Len() {
		return false
	}
	for _, k := range d.MapKeys() {
		ov := o.MapIndex(k)
		if !ov.IsValid() || !w.subset(d.MapIndex(k), ov) {
			return false
		}
	}
	return true
}

func (w *walker) subsetStruct(d, o reflect.Value) bool {
	if d.Type() == timeType {
		return d.Interface().(time.Time).Equal(o.Interface().(time.Time)) //nolint:forcetypeassert // checked above
	}
	for i := range d.NumField() {
		if d.Type().Field(i).IsExported() && !w.subset(d.Field(i), o.Field(i)) {
			return false
		}
	}
	return true
}

// Without returns diff without the given field names.
func Without(diff []string, names ...string) []string {
	return slices.DeleteFunc(diff, func(s string) bool { return slices.Contains(names, s) })
}

// KMSKeyIDEqual reports whether two KMS key identifiers refer to the same
// key, when one is a key ID and the other a key ARN (AWS usually returns the
// ARN). Aliases can't be resolved and only match themselves.
func KMSKeyIDEqual(a, b string) bool {
	return a == b || strings.HasSuffix(a, ":key/"+b) || strings.HasSuffix(b, ":key/"+a)
}

// Message formats the names of differing fields for
// managed.ExternalObservation.Diff.
func Message(diff []string) string {
	if len(diff) == 0 {
		return ""
	}
	return "spec.forProvider fields differ from the observed state: " + strings.Join(diff, ", ")
}

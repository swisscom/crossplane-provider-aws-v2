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
// Unset values in desired (nil pointers, slices and maps) match anything:
// the field is not managed. Empty, non-nil slices and maps must be empty in
// observed. Slices are compared regardless of order, because AWS does not
// keep the order of most lists.
func IsSubset(desired, observed any) bool {
	return isSubset(reflect.ValueOf(desired), reflect.ValueOf(observed))
}

var timeType = reflect.TypeFor[time.Time]()

func isSubset(d, o reflect.Value) bool { //nolint:gocyclo // one case per kind
	if !d.IsValid() {
		return true
	}
	if !o.IsValid() {
		o = reflect.Zero(d.Type())
	}
	switch d.Kind() { //nolint:exhaustive // the default case handles scalars
	case reflect.Pointer, reflect.Interface:
		if d.IsNil() {
			return true
		}
		if o.IsNil() {
			return false
		}
		return isSubset(d.Elem(), o.Elem())
	case reflect.Slice:
		if d.IsNil() {
			return true
		}
		if d.Len() != o.Len() {
			return false
		}
		used := make([]bool, o.Len())
	next:
		for i := range d.Len() {
			for j := range o.Len() {
				if !used[j] && isSubset(d.Index(i), o.Index(j)) {
					used[j] = true
					continue next
				}
			}
			return false
		}
		return true
	case reflect.Map:
		if d.IsNil() {
			return true
		}
		if d.Len() != o.Len() {
			return false
		}
		for _, k := range d.MapKeys() {
			if !isSubset(d.MapIndex(k), o.MapIndex(k)) {
				return false
			}
		}
		return true
	case reflect.Struct:
		if d.Type() == timeType {
			return d.Interface().(time.Time).Equal(o.Interface().(time.Time)) //nolint:forcetypeassert // checked above
		}
		for i := range d.NumField() {
			if d.Type().Field(i).IsExported() && !isSubset(d.Field(i), o.Field(i)) {
				return false
			}
		}
		return true
	default:
		return d.Equal(o)
	}
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

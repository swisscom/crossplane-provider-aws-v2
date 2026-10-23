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

	"k8s.io/utils/ptr"
)

func TestIdentifierEqual(t *testing.T) {
	const mydb = "mydb"
	cases := map[string]struct {
		a, b string
		want bool
	}{
		"DifferentCase":     {a: "MyDB", b: mydb, want: true},
		"Same":              {a: mydb, b: mydb, want: true},
		"Empty":             {a: "", b: "", want: true},
		"DifferentLength":   {a: mydb, b: "mydb2", want: false},
		"DifferentName":     {a: mydb, b: "yourdb", want: false},
		"KelvinSignIsNotK":  {a: "Key", b: "key", want: false},
		"LongSIsNotS":       {a: "ſ", b: "s", want: false},
		"NonASCIISame":      {a: "é", b: "é", want: true},
		"NonASCIICaseKept":  {a: "É", b: "é", want: false},
		"DigitsAndHyphens":  {a: "My-DB-01", b: "my-db-01", want: true},
		"NonLettersDiffer":  {a: "@", b: "`", want: false},
		"BracketsNotFolded": {a: "[", b: "{", want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := IdentifierEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("IdentifierEqual(%q, %q): want %v, got %v", tc.a, tc.b, tc.want, got)
			}
			if got := IdentifierEqual(tc.b, tc.a); got != tc.want {
				t.Errorf("IdentifierEqual(%q, %q) must be symmetric: want %v, got %v", tc.b, tc.a, tc.want, got)
			}
		})
	}
}

func TestEqualIgnoreCase(t *testing.T) {
	cases := map[string]struct {
		desired, observed *string
		want              bool
	}{
		"NilDesired":       {desired: nil, observed: ptr.To("x"), want: true},
		"NilBoth":          {desired: nil, observed: nil, want: true},
		"NilObserved":      {desired: ptr.To("x"), observed: nil, want: false},
		"EmptyVsNil":       {desired: ptr.To(""), observed: nil, want: false},
		"LowercasedWindow": {desired: ptr.To("Sun:05:00-Sun:06:00"), observed: ptr.To("sun:05:00-sun:06:00"), want: true},
		"DifferentDay":     {desired: ptr.To("sun:05:00-sun:06:00"), observed: ptr.To("mon:05:00-mon:06:00"), want: false},
		"KelvinSignIsNotK": {desired: ptr.To("K"), observed: ptr.To("k"), want: false},
		"EmptyBoth":        {desired: ptr.To(""), observed: ptr.To(""), want: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := EqualIgnoreCase(tc.desired, tc.observed); got != tc.want {
				t.Errorf("EqualIgnoreCase(%v, %v): want %v, got %v", tc.desired, tc.observed, tc.want, got)
			}
		})
	}
}

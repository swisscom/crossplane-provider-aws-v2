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

package policy

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestStringOrSet_Equal(t *testing.T) {
	cases := map[string]struct {
		a, b  StringOrSet
		equal bool
	}{
		"BothNil": {
			a:     nil,
			b:     nil,
			equal: true,
		},
		"BothEmpty": {
			a:     NewStringOrSet(),
			b:     NewStringOrSet(),
			equal: true,
		},
		"EqualDifferentOrder": {
			a:     NewStringOrSet("a", "b"),
			b:     NewStringOrSet("b", "a"),
			equal: true,
		},
		"NotEqual": {
			a:     NewStringOrSet("a", "b"),
			b:     NewStringOrSet("a", "c"),
			equal: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(tc.a, tc.b); (diff == "") != tc.equal {
				t.Errorf("diff: %s", diff)
			}
		})
	}
}

func TestStringOrSet_MarshalJSON(t *testing.T) {
	cases := map[string]struct {
		set  StringOrSet
		want string
	}{
		"nil": {
			set:  nil,
			want: "null",
		},
		"Empty": {
			set:  NewStringOrSet(),
			want: "[]",
		},
		"Single": {
			set:  NewStringOrSet("a"),
			want: `["a"]`,
		},
		"Multiple": {
			set:  NewStringOrSet("a", "c", "b"),
			want: `["a","b","c"]`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			b, err := tc.set.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tc.want {
				t.Errorf("got %s, want %s", b, tc.want)
			}
		})
	}
}

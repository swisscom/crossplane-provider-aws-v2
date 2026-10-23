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

package jsonpatch

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/ptr"
)

type inner struct {
	Size *int64  `json:"size,omitempty"`
	Kind *string `json:"kind,omitempty"`
}

type params struct {
	Name  *string  `json:"name,omitempty"`
	Class *string  `json:"class,omitempty"`
	Inner *inner   `json:"inner,omitempty"`
	List  []string `json:"list,omitempty"`
}

func TestCreateJSONPatch(t *testing.T) {
	cases := map[string]struct {
		source, destination any
		want                string
	}{
		"Equal":         {source: params{Name: ptr.To("a")}, destination: params{Name: ptr.To("a")}, want: `{}`},
		"ChangedField":  {source: params{Name: ptr.To("a")}, destination: params{Name: ptr.To("b")}, want: `{"name":"b"}`},
		"AddedField":    {source: params{}, destination: params{Class: ptr.To("c")}, want: `{"class":"c"}`},
		"RemovedField":  {source: params{Name: ptr.To("a")}, destination: params{}, want: `{"name":null}`},
		"NestedChange":  {source: params{Inner: &inner{Size: ptr.To(int64(1)), Kind: ptr.To("x")}}, destination: params{Inner: &inner{Size: ptr.To(int64(2)), Kind: ptr.To("x")}}, want: `{"inner":{"size":2}}`},
		"ListReplaced":  {source: params{List: []string{"a", "b"}}, destination: params{List: []string{"a", "c"}}, want: `{"list":["a","c"]}`},
		"NilBoth":       {source: nil, destination: nil, want: `{}`},
		"NilToEmptyMap": {source: nil, destination: map[string]any{}, want: `{}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := CreateJSONPatch(tc.source, tc.destination)
			if err != nil {
				t.Fatalf("CreateJSONPatch(...): unexpected error: %v", err)
			}
			if diff := cmp.Diff(tc.want, string(got)); diff != "" {
				t.Errorf("CreateJSONPatch(...): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestCreateJSONPatchErrors(t *testing.T) {
	cases := map[string]struct {
		source, destination any
	}{
		"SourceNotMarshalable":      {source: math.Inf(1), destination: params{}},
		"DestinationNotMarshalable": {source: params{}, destination: math.Inf(1)},
		"ArrayAgainstObject":        {source: []string{"a"}, destination: params{}},
		"ScalarDocuments":           {source: 1, destination: 2},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got, err := CreateJSONPatch(tc.source, tc.destination); err == nil {
				t.Errorf("CreateJSONPatch(...): want an error, got patch %s", got)
			}
		})
	}
}

// The patch is used to find out which fields of a modify request changed:
// only those must be set once it is read back into the same type.
func TestCreateJSONPatchRoundTrip(t *testing.T) {
	current := params{Name: ptr.To("db"), Class: ptr.To("small"), Inner: &inner{Size: ptr.To(int64(10)), Kind: ptr.To("gp3")}}
	desired := params{Name: ptr.To("db"), Class: ptr.To("large"), Inner: &inner{Size: ptr.To(int64(10)), Kind: ptr.To("io1")}}
	raw, err := CreateJSONPatch(current, desired)
	if err != nil {
		t.Fatalf("CreateJSONPatch(...): unexpected error: %v", err)
	}
	var got params
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	want := params{Class: ptr.To("large"), Inner: &inner{Kind: ptr.To("io1")}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("patch %s: -want, +got:\n%s", raw, diff)
	}
}

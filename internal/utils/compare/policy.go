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
	"encoding/json"
	"reflect"
)

// PolicyEqual reports whether the observed IAM policy document equals the
// desired one. Both are compared as JSON, where a single value equals a list
// with only that value, because AWS returns policies reformatted. Documents
// that aren't JSON are compared as strings. A nil desired policy is not
// managed and equals anything.
func PolicyEqual(desired, observed *string) bool {
	if desired == nil {
		return true
	}
	o := ""
	if observed != nil {
		o = *observed
	}
	if *desired == o {
		return true
	}
	var d, ob any
	if json.Unmarshal([]byte(*desired), &d) != nil || json.Unmarshal([]byte(o), &ob) != nil {
		return false
	}
	return reflect.DeepEqual(normalizePolicy(d), normalizePolicy(ob))
}

func normalizePolicy(v any) any {
	switch t := v.(type) {
	case []any:
		if len(t) == 1 {
			return normalizePolicy(t[0])
		}
		for i := range t {
			t[i] = normalizePolicy(t[i])
		}
	case map[string]any:
		for k := range t {
			t[k] = normalizePolicy(t[k])
		}
	}
	return v
}

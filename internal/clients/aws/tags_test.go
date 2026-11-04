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

package aws

import "testing"

func TestIsAWSTag(t *testing.T) {
	cases := map[string]bool{
		"aws:cloudformation:stack-name": true,
		"aws:":                          true,
		"team":                          false,
		"AWS:upper":                     true,
		"my-aws:tag":                    false,
	}
	for key, want := range cases {
		if got := IsAWSTag(key); got != want {
			t.Errorf("IsAWSTag(%q) = %v, want %v", key, got, want)
		}
	}
}

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

import "encoding/json"

// ParsePolicyBytes from a byte array representing a raw JSOn string.
func ParsePolicyBytes(raw []byte) (Policy, error) {
	policy := Policy{}
	err := json.Unmarshal(raw, &policy)
	return policy, err
}

// ParsePolicyString from a raw JSON string.
func ParsePolicyString(raw string) (Policy, error) {
	return ParsePolicyBytes([]byte(raw))
}

// ParsePolicyStringPtr from a raw JSON string pointer.
func ParsePolicyStringPtr(raw *string) (*Policy, error) {
	if raw == nil {
		return nil, nil
	}
	pol, err := ParsePolicyBytes([]byte(*raw))
	return &pol, err
}

// ParsePolicyObject parses a policy from an object (i.e. an API struct) which
// can be marshalled into JSON.
func ParsePolicyObject(obj any) (Policy, error) {
	input, err := json.Marshal(obj)
	if err != nil {
		return Policy{}, err
	}
	return ParsePolicyBytes(input)
}

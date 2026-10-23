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

package utils

import (
	"strings"

	svcsdktypes "github.com/aws/aws-sdk-go-v2/service/rds/types"

	svcapitypes "github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/pointer"
	"github.com/swisscom/crossplane-provider-aws-v2/internal/utils/tags"
)

// AnnotationManagedTagKeys records, comma-separated, the spec tag keys the
// provider manages. Only these keys are ever removed.
const AnnotationManagedTagKeys = "rds.aws2.crossplane.io/managed-tag-keys"

// ShouldIgnore returns true if key matches any supplied rule. A rule may be:
//   - exact key match (e.g. "c7n:policy")
//   - prefix* glob where * is only allowed as the last character (e.g. "c7n:*" or "prefix*")
//
// No other wildcard forms are supported.
func ShouldIgnore(key string, rules []string) bool { // intentionally simple, no regex
	for _, r := range rules {
		if r == "" { // skip empty
			continue
		}
		if strings.HasSuffix(r, "*") {
			prefix := strings.TrimSuffix(r, "*")
			if prefix == "" { // guard against blanket "*" wildcard
				continue
			}
			if strings.HasPrefix(key, prefix) {
				return true
			}
			continue
		}
		if key == r { // exact
			return true
		}
	}
	return false
}

// DiffTags returns the spec tags that are missing or have another value in
// current, and the managed keys that are in current but not in spec. Keys
// that are not managed are never removed.
func DiffTags(spec []*svcapitypes.Tag, current []svcsdktypes.Tag, managed []string) (addTags []svcsdktypes.Tag, removeTags []string) {
	currentMap := make(map[string]string, len(current))
	for _, t := range current {
		currentMap[pointer.StringValue(t.Key)] = pointer.StringValue(t.Value)
	}

	// A key listed twice in the spec counts once, with its last value.
	specMap := make(map[string]string, len(spec))
	var keys []string
	for _, t := range spec {
		key := pointer.StringValue(t.Key)
		if _, dup := specMap[key]; !dup {
			keys = append(keys, key)
		}
		specMap[key] = pointer.StringValue(t.Value)
	}

	for _, key := range keys {
		val := specMap[key]
		// AddTagsToResource overwrites a changed value, so the key must not be removed too.
		if currentVal, exists := currentMap[key]; exists && currentVal == val {
			continue
		}
		addTags = append(addTags, svcsdktypes.Tag{
			Key:   pointer.ToOrNilIfZeroValue(key),
			Value: pointer.ToOrNilIfZeroValue(val),
		})
	}

	return addTags, removals(managed, currentMap, specMap)
}

// removals returns the managed keys that are in AWS but no longer in the spec.
func removals(managed []string, current, spec map[string]string) []string {
	var out []string
	for _, key := range managed {
		_, inAWS := current[key]
		_, inSpec := spec[key]
		if inAWS && !inSpec && !tags.IsAWS(key) {
			out = append(out, key)
		}
	}
	return out
}

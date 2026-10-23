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

// Package tags records, in an annotation of a managed resource, the AWS tag
// keys the provider set, so that it removes only its own tags (ADR 0015).
package tags

import (
	"slices"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"k8s.io/apimachinery/pkg/util/sets"
)

// awsPrefix marks tags that only AWS may set or remove.
const awsPrefix = "aws:"

// IsAWS reports whether key is reserved by AWS.
func IsAWS(key string) bool {
	return strings.HasPrefix(key, awsPrefix)
}

// Record sets the record of mg, kept in annotation, to the desired keys plus
// the recorded keys still in current, and reports whether it changed. The
// caller must get a change persisted, e.g. by reporting the resource as
// late-initialized. Record does nothing when the runtime would not persist it.
func Record(mg resource.Managed, annotation string, desired, current []string) bool {
	if !canRecord(mg) {
		return false
	}
	want := sets.New[string]()
	for _, k := range desired {
		if k != "" && !IsAWS(k) {
			want.Insert(k)
		}
	}
	recorded := recordedKeys(mg, annotation)
	// Keep a recorded key until it is gone from AWS, so it is removed even if
	// the provider restarts in between.
	next := want.Union(recorded.Intersection(sets.New(current...)))
	if next.Len() == 0 {
		meta.RemoveAnnotations(mg, annotation)
	} else {
		meta.AddAnnotations(mg, map[string]string{annotation: strings.Join(sets.List(next), ",")})
	}
	return !next.Equal(recorded)
}

// Removable returns the sorted recorded keys of mg; none when the runtime
// would not persist the record, so it can't be trusted.
func Removable(mg resource.Managed, annotation string) []string {
	if !canRecord(mg) {
		return nil
	}
	return sets.List(recordedKeys(mg, annotation))
}

// canRecord reports whether the runtime persists the annotation: it does so
// only with the late-init update, which some management policies disable.
func canRecord(mg resource.Managed) bool {
	p := mg.GetManagementPolicies()
	return len(p) == 0 || slices.Contains(p, xpv2.ManagementActionAll) || slices.Contains(p, xpv2.ManagementActionLateInitialize)
}

func recordedKeys(mg resource.Managed, annotation string) sets.Set[string] {
	v := mg.GetAnnotations()[annotation]
	if v == "" {
		return sets.New[string]()
	}
	return sets.New(strings.Split(v, ",")...)
}

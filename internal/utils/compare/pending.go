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

import "slices"

// Pending replaces observed with a copy of pending when AWS has a pending
// change, so a value that is already on its way is not changed again.
func Pending[T any](observed **T, pending *T) {
	if observed == nil || pending == nil {
		return
	}
	v := *pending
	*observed = &v
}

// PendingSlice replaces observed with a copy of pending when AWS has a pending
// change. An empty list means nothing is pending.
func PendingSlice[T any](observed *[]T, pending []T) {
	if observed == nil || len(pending) == 0 {
		return
	}
	*observed = slices.Clone(pending)
}

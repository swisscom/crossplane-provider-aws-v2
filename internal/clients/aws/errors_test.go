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

import (
	"errors"
	"testing"

	"github.com/aws/smithy-go"
)

func TestWrap(t *testing.T) {
	if Wrap(nil, "msg") != nil {
		t.Fatal("Wrap(nil) must be nil")
	}
	err := Wrap(&smithy.GenericAPIError{Code: "NotFound", Message: "gone"}, "describe")
	if got, want := err.Error(), "describe: NotFound: gone"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got, want := Wrap(errors.New("boom"), "x").Error(), "x: boom"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

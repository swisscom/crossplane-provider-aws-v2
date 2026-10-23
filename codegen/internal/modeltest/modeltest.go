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

// Package modeltest decides what a test does when the AWS API model or SDK
// module it needs can't be downloaded: skip on a dev machine, fail in CI.
package modeltest

import (
	"os"
	"strings"
	"testing"
)

// RequireEnv makes a missing model fail the test even outside CI.
const RequireEnv = "CODEGEN_REQUIRE_MODELS"

// fetchErr appears in the generator's error when the model download fails.
const fetchErr = "cannot fetch AWS API model"

// Required reports whether a missing model must fail the test. GitHub Actions
// sets CI=true, so the safety tests can't pass there by skipping.
func Required() bool {
	return isSet(os.Getenv("CI")) || isSet(os.Getenv(RequireEnv))
}

func isSet(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// Unavailable skips t, or fails it when Required, because a model or SDK
// module could not be downloaded.
func Unavailable(t testing.TB, format string, args ...any) {
	t.Helper()
	if Required() {
		t.Fatalf(format+" (CI or "+RequireEnv+" is set, so this fails instead of skipping)", args...)
		return
	}
	t.Skipf(format, args...)
}

// SkipOnFetchError calls Unavailable if err is a model download error. It
// does nothing for nil or any other error, which the caller still checks.
func SkipOnFetchError(t testing.TB, err error) {
	t.Helper()
	if err != nil && strings.Contains(err.Error(), fetchErr) {
		Unavailable(t, "AWS API model not available: %v", err)
	}
}

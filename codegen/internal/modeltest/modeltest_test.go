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

package modeltest

import (
	"errors"
	"testing"
)

// recorder records Skipf and Fatalf instead of stopping the test.
type recorder struct {
	testing.TB
	skipped, failed bool
}

func (r *recorder) Helper()               {}
func (r *recorder) Skipf(string, ...any)  { r.skipped = true }
func (r *recorder) Fatalf(string, ...any) { r.failed = true }

func TestSkipOnFetchError(t *testing.T) {
	fetchErr := errors.New("cannot fetch AWS API model: dial tcp: no such host")

	cases := map[string]struct {
		ci, require string
		err         error
		wantSkip    bool
		wantFail    bool
	}{
		"NoError":              {err: nil},
		"OtherError":           {err: errors.New("cannot load generator config"), ci: "true"},
		"OfflineSkips":         {err: fetchErr, wantSkip: true},
		"CIFails":              {err: fetchErr, ci: "true", wantFail: true},
		"RequireFails":         {err: fetchErr, require: "1", wantFail: true},
		"CIFalseSkips":         {err: fetchErr, ci: "false", wantSkip: true},
		"RequireZeroSkips":     {err: fetchErr, require: "0", wantSkip: true},
		"RequireWinsOverCIOff": {err: fetchErr, ci: "false", require: "true", wantFail: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("CI", tc.ci)
			t.Setenv(RequireEnv, tc.require)
			r := &recorder{TB: t}
			SkipOnFetchError(r, tc.err)
			if r.skipped != tc.wantSkip || r.failed != tc.wantFail {
				t.Errorf("skipped=%v failed=%v, want skipped=%v failed=%v", r.skipped, r.failed, tc.wantSkip, tc.wantFail)
			}
		})
	}
}

func TestUnavailable(t *testing.T) {
	cases := map[string]struct {
		ci, require string
		wantSkip    bool
		wantFail    bool
	}{
		"OfflineSkips": {wantSkip: true},
		"CIFails":      {ci: "true", wantFail: true},
		"RequireFails": {require: "1", wantFail: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("CI", tc.ci)
			t.Setenv(RequireEnv, tc.require)
			r := &recorder{TB: t}
			Unavailable(r, "docdb SDK module not available: %v", errors.New("boom"))
			if r.skipped != tc.wantSkip || r.failed != tc.wantFail {
				t.Errorf("skipped=%v failed=%v, want skipped=%v failed=%v", r.skipped, r.failed, tc.wantSkip, tc.wantFail)
			}
		})
	}
}

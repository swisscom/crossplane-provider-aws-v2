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

package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/swisscom/crossplane-provider-aws-v2/codegen/internal/importer"
)

func TestExitCode(t *testing.T) {
	blocking := &importer.Report{}
	blocking.Block("Controller", "port me")
	notice := &importer.Report{}
	notice.Add("Controller", "fine")

	cases := map[string]struct {
		r    *importer.Report
		err  error
		want int
	}{
		"Clean":            {r: notice, want: 0},
		"Blocking":         {r: blocking, want: 3},
		"ErrorWins":        {r: blocking, err: errors.New("boom"), want: 1},
		"ErrorWithoutRepo": {err: errors.New("boom"), want: 1},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := exitCode(tc.r, tc.err); got != tc.want {
				t.Errorf("exitCode() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRunRejectsInvalidFlags(t *testing.T) {
	out := t.TempDir()
	cases := map[string]struct {
		args []string
		want string
	}{
		"NoFlags":         {args: nil, want: "--source, --service and --kinds are required"},
		"ServiceEscapes":  {args: []string{"--source", out, "--service", "../x", "--kinds", "Foo", "--output", out}, want: `--service "../x"`},
		"BadKind":         {args: []string{"--source", out, "--service", "rds", "--kinds", "foo/bar", "--output", out}, want: `--kinds: "foo/bar" is not a kind name`},
		"BadAPIVersion":   {args: []string{"--source", out, "--service", "rds", "--kinds", "Foo", "--output", out, "--api-version", "../v1"}, want: `--api-version "../v1"`},
		"UnknownFlagIs2":  {args: []string{"--nope"}, want: "flag provided but not defined"},
		"ExtraArgsAreBad": {args: []string{"--source", out, "--service", "rds", "--kinds", "Foo", "x"}, want: "unexpected arguments"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(tc.args, &stdout, &stderr); got != 2 {
				t.Errorf("run() = %d, want 2; stderr:\n%s", got, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr does not contain %q:\n%s", tc.want, stderr.String())
			}
			if !strings.Contains(stderr.String(), "Usage") {
				t.Errorf("stderr has no usage:\n%s", stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
		})
	}
	if e, err := os.ReadDir(out); err != nil || len(e) != 0 {
		t.Errorf("output dir after invalid runs: %v %v, want empty", e, err)
	}
}

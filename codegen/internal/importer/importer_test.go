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

package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/swisscom/crossplane-provider-aws-v2/codegen/internal/generator"
)

func TestRunAlwaysReturnsAReport(t *testing.T) {
	cases := map[string]struct {
		o    Options
		want string
	}{
		"Invalid": {
			o:    Options{Source: t.TempDir(), Kinds: []string{fooKind}, Options: generator.Options{Service: "../x", APIVersion: "v1alpha1", OutputDir: t.TempDir()}},
			want: `--service "../x"`,
		},
		"NoSourceConfig": {
			o:    Options{Source: t.TempDir(), Kinds: []string{fooKind}, Options: generator.Options{Service: "svc", APIVersion: "v1alpha1", OutputDir: t.TempDir()}},
			want: "source has no generator config for svc",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r, err := Run(context.Background(), tc.o)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Run() error = %v, want it to contain %q", err, tc.want)
			}
			if r == nil {
				t.Fatal("Run() returned a nil report together with an error")
			}
			if r.Service == "" {
				t.Errorf("report has no service: %+v", r)
			}
		})
	}
}

func TestOptionsValidate(t *testing.T) {
	ok := Options{Source: "/src", Kinds: []string{"DBInstance"}, Options: generator.Options{Service: "RDS", APIVersion: "v1beta2"}}
	if err := ok.Validate(); err != nil {
		t.Errorf("Validate() of valid options = %v", err)
	}
	cases := map[string]struct {
		mod  func(*Options)
		want string
	}{
		"NoSource":       {func(o *Options) { o.Source = "" }, "are required"},
		"NoKinds":        {func(o *Options) { o.Kinds = nil }, "are required"},
		"ServiceDot":     {func(o *Options) { o.Service = "rds.x" }, `--service "rds.x"`},
		"ServiceSlash":   {func(o *Options) { o.Service = "a/b" }, `--service "a/b"`},
		"KindLower":      {func(o *Options) { o.Kinds = []string{"DBInstance", "dbCluster"} }, `--kinds: "dbCluster"`},
		"KindTraversal":  {func(o *Options) { o.Kinds = []string{"../X"} }, `--kinds: "../X"`},
		"APIVersionBad":  {func(o *Options) { o.APIVersion = "v1/../x" }, `--api-version "v1/../x"`},
		"APIVersionNone": {func(o *Options) { o.APIVersion = "" }, `--api-version ""`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			o := ok
			o.Kinds = append([]string(nil), ok.Kinds...)
			tc.mod(&o)
			if err := o.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate() = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

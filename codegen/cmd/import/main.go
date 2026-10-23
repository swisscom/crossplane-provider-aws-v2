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

// Command import imports managed resource kinds from
// crossplane-contrib/provider-aws into provider-aws-v2.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/swisscom/crossplane-provider-aws-v2/codegen/internal/importer"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses args, runs the import and returns the exit code: 0 done,
// 1 error, 2 invalid usage (nothing written), 3 blocking findings left.
func run(args []string, stdout, stderr io.Writer) int {
	var o importer.Options
	var kinds string
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.Source, "source", "", "Root of a crossplane-contrib/provider-aws checkout (required)")
	fs.StringVar(&o.Service, "service", "", "aws-sdk-go-v2 service package, e.g. rds (required)")
	fs.StringVar(&kinds, "kinds", "", "Comma-separated kinds to import, e.g. DBInstance (required)")
	fs.StringVar(&o.OutputDir, "output", "..", "Root directory of the provider repository")
	fs.StringVar(&o.APIVersion, "api-version", "v1alpha1", "Kubernetes API version of the generated types")
	fs.StringVar(&o.CacheDir, "cache-dir", defaultCacheDir(), "Directory for cached AWS API models")
	fs.StringVar(&o.TemplateDir, "template-dir", "templates", "Directory with the code templates")
	usage := func(err error) int {
		_, _ = fmt.Fprintln(stderr, err)
		_, _ = fmt.Fprintln(stderr, "Usage of import:")
		fs.PrintDefaults()
		return 2
	}
	if err := fs.Parse(args); err != nil {
		// The flag package has printed the error and the usage already.
		_, _ = fmt.Fprintln(stderr, "Usage: see above")
		return 2
	}
	if fs.NArg() > 0 {
		// e.g. "--kinds A B" would otherwise import A only.
		return usage(fmt.Errorf("unexpected arguments %q; separate kinds with commas", fs.Args()))
	}
	for _, k := range strings.Split(kinds, ",") {
		if k = strings.TrimSpace(k); k != "" {
			o.Kinds = append(o.Kinds, k)
		}
	}
	if err := o.Validate(); err != nil {
		return usage(err)
	}
	r, err := importer.Run(context.Background(), o)
	_, _ = fmt.Fprint(stdout, r.Markdown())
	code := exitCode(r, err)
	switch code {
	case 1:
		_, _ = fmt.Fprintln(stderr, "error:", err)
	case 3:
		_, _ = fmt.Fprintf(stderr, "import: %d blocking finding(s), see the report; port them and run make check-import-todo\n", r.Blocking())
	}
	return code
}

// exitCode maps the result of importer.Run to the exit code.
func exitCode(r *importer.Report, err error) int {
	switch {
	case err != nil:
		return 1
	case r != nil && r.Blocking() > 0:
		return 3
	}
	return 0
}

func defaultCacheDir() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ".cache"
	}
	return filepath.Join(d, "provider-aws-v2-codegen")
}

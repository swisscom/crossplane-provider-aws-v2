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
	"os"
	"path/filepath"
	"strings"

	"github.com/teeverr/provider-aws-v2/codegen/internal/importer"
)

func main() {
	var o importer.Options
	var kinds string
	flag.StringVar(&o.Source, "source", "", "Root of a crossplane-contrib/provider-aws checkout (required)")
	flag.StringVar(&o.Service, "service", "", "aws-sdk-go-v2 service package, e.g. rds (required)")
	flag.StringVar(&kinds, "kinds", "", "Comma-separated kinds to import, e.g. DBInstance (required)")
	flag.StringVar(&o.OutputDir, "output", "..", "Root directory of the provider repository")
	flag.StringVar(&o.APIVersion, "api-version", "v1alpha1", "Kubernetes API version of the generated types")
	flag.StringVar(&o.CacheDir, "cache-dir", defaultCacheDir(), "Directory for cached AWS API models")
	flag.StringVar(&o.TemplateDir, "template-dir", "templates", "Directory with the code templates")
	flag.Parse()

	for _, k := range strings.Split(kinds, ",") {
		if k = strings.TrimSpace(k); k != "" {
			o.Kinds = append(o.Kinds, k)
		}
	}
	if o.Source == "" || o.Service == "" || len(o.Kinds) == 0 {
		fmt.Fprintln(os.Stderr, "--source, --service and --kinds are required")
		flag.Usage()
		os.Exit(2)
	}
	r, err := importer.Run(context.Background(), o)
	if r != nil {
		fmt.Print(r.Markdown())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func defaultCacheDir() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ".cache"
	}
	return filepath.Join(d, "provider-aws-v2-codegen")
}

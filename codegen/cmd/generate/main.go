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

// Command generate creates the API types and controllers of one AWS service
// from the AWS SDK v2 API model, using the ACK code-generator as a library.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swisscom/crossplane-provider-aws-v2/codegen/internal/generator"
)

func main() {
	var o generator.Options
	flag.StringVar(&o.Service, "service", "", "aws-sdk-go-v2 service package, e.g. servicecatalog (required)")
	flag.StringVar(&o.OutputDir, "output", "..", "Root directory of the provider repository")
	flag.StringVar(&o.GeneratorConfig, "generator-config", "", "Path to generator-config.yaml (default <output>/apis/<service>/generator-config.yaml)")
	flag.StringVar(&o.APIVersion, "api-version", "v1alpha1", "Kubernetes API version of the generated types")
	flag.StringVar(&o.CacheDir, "cache-dir", defaultCacheDir(), "Directory for cached AWS API models")
	flag.StringVar(&o.TemplateDir, "template-dir", "templates", "Directory with the code templates")
	registriesOnly := flag.Bool("registries-only", false, "Only regenerate apis/zz_services.go and internal/controller/zz_services.go, e.g. after removing a service")
	flag.Parse()

	if *registriesOnly {
		if err := generator.WriteRegistries(o.OutputDir); err != nil {
			fail(err)
		}
		return
	}
	if o.Service == "" {
		fmt.Fprintln(os.Stderr, "--service is required")
		flag.Usage()
		os.Exit(2)
	}
	o.Service = strings.ToLower(o.Service)
	if err := generator.ResolveServiceModule(&o); err != nil {
		fail(err)
	}
	if err := generator.Run(context.Background(), o); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func defaultCacheDir() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ".cache"
	}
	return filepath.Join(d, "provider-aws-v2-codegen")
}

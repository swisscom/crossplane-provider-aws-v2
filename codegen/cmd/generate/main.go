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
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/teeverr/provider-aws-v2/codegen/internal/generator"
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
	if err := resolveServiceModule(&o); err != nil {
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

// resolveServiceModule makes sure the provider's go.mod requires the
// aws-sdk-go-v2 service module, then uses its version to fetch the matching
// API model, so the generated code and the SDK it compiles against agree.
func resolveServiceModule(o *generator.Options) error {
	mod := "github.com/aws/aws-sdk-go-v2/service/" + o.Service
	if !requires(filepath.Join(o.OutputDir, "go.mod"), mod) {
		fmt.Fprintf(os.Stderr, "adding %s to go.mod\n", mod)
		cmd := exec.Command("go", "get", mod+"@latest") //nolint:gosec // module path built from a flag
		cmd.Dir = o.OutputDir
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("cannot add %s to go.mod: %w", mod, err)
		}
	}
	cmd := exec.Command("go", "list", "-m", "-json", mod) //nolint:gosec // module path built from a flag
	cmd.Dir = o.OutputDir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("cannot resolve %s: %w", mod, err)
	}
	var m struct{ Version, Dir string }
	if err := json.Unmarshal(out, &m); err != nil {
		return err
	}
	if m.Dir == "" {
		dl := exec.Command("go", "mod", "download", mod) //nolint:gosec // module path built from a flag
		dl.Dir = o.OutputDir
		if err := dl.Run(); err != nil {
			return fmt.Errorf("cannot download %s: %w", mod, err)
		}
		return resolveServiceModule(o)
	}
	o.ServiceSDKVersion = m.Version
	o.ModelName, err = modelName(m.Dir)
	return err
}

func requires(gomod, mod string) bool {
	f, err := os.Open(gomod) //nolint:gosec // path comes from a flag
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck // read only
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) >= 2 && (fields[0] == mod || (fields[0] == "require" && fields[1] == mod)) {
			return true
		}
	}
	return false
}

var serviceIDRe = regexp.MustCompile(`const ServiceID = "([^"]+)"`)

// modelName returns the API model file name of a service module: its
// ServiceID, lower-cased and hyphenated (e.g. "Service Catalog" ->
// "service-catalog").
func modelName(moduleDir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(moduleDir, "api_client.go")) //nolint:gosec // module cache path
	if err != nil {
		return "", err
	}
	m := serviceIDRe.FindSubmatch(b)
	if m == nil {
		return "", fmt.Errorf("no ServiceID in %s/api_client.go", moduleDir)
	}
	return strings.ReplaceAll(strings.ToLower(string(m[1])), " ", "-"), nil
}

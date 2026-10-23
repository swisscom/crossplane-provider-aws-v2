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

package generator

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swisscom/crossplane-provider-aws-v2/codegen/internal/modeltest"
)

// fixture is a service whose generator config lives in testdata/<service>.
type fixture struct {
	service    string
	sdkVersion string
	model      string
}

var (
	docdbFixture          = fixture{service: "docdb", sdkVersion: "v1.57.3", model: "docdb"}
	servicecatalogFixture = fixture{service: "servicecatalog", sdkVersion: "v1.47.1", model: "service-catalog"}
)

// options returns the Run options of the fixture with the given generator
// config, rendering into out. Models are fetched once and cached.
func (f fixture) options(t *testing.T, out, generatorConfig string) Options {
	t.Helper()
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = t.TempDir()
	}
	return Options{
		Service:           f.service,
		OutputDir:         out,
		GeneratorConfig:   generatorConfig,
		APIVersion:        "v1alpha1",
		ServiceSDKVersion: f.sdkVersion,
		ModelName:         f.model,
		CacheDir:          filepath.Join(cacheDir, "provider-aws-v2-codegen"),
		TemplateDir:       filepath.Join("..", "..", "templates"),
	}
}

// run generates the fixture into a new directory. edit, if set, changes the
// generator config first; codegenYAML, if set, is written as the
// service's codegen.yaml. It returns the output directory and Run's error.
func (f fixture) run(t *testing.T, edit func(string) string, codegenYAML string) (string, error) {
	t.Helper()
	out := t.TempDir()
	return out, f.runIn(t, out, edit, codegenYAML)
}

func (f fixture) runIn(t *testing.T, out string, edit func(string) string, codegenYAML string) error {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", f.service, "generator-config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := string(b)
	if edit != nil {
		cfg = edit(cfg)
	}
	cfgPath := filepath.Join(t.TempDir(), "generator-config.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if codegenYAML != "" {
		p := ServiceConfigPath(out, f.service)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(codegenYAML), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err = Run(context.Background(), f.options(t, out, cfgPath))
	modeltest.SkipOnFetchError(t, err)
	return err
}

// mustRun is run for configs that must generate.
func (f fixture) mustRun(t *testing.T, edit func(string) string, codegenYAML string) string {
	t.Helper()
	out, err := f.run(t, edit, codegenYAML)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out
}

// addIgnores appends entries to ignore.field_paths of a fixture config.
func addIgnores(paths ...string) func(string) string {
	return func(cfg string) string {
		var b strings.Builder
		for _, p := range paths {
			b.WriteString("    - " + p + "\n")
		}
		return strings.Replace(cfg, "  field_paths:\n", "  field_paths:\n"+b.String(), 1)
	}
}

// removeLine removes a line, which must exist, from a fixture config.
func removeLine(t *testing.T, line string) func(string) string {
	t.Helper()
	return func(cfg string) string {
		if !strings.Contains(cfg, line+"\n") {
			t.Fatalf("fixture has no line %q", line)
		}
		return strings.Replace(cfg, line+"\n", "", 1)
	}
}

// readOut returns a generated file.
func readOut(t *testing.T, out, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(out, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// goFiles returns the .go files below dir.
func goFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".go") {
			files = append(files, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// errContains fails the test unless err contains every want.
func errContains(t *testing.T, err error, want ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got no error, want one containing %q", want)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error does not contain %q:\n%v", w, err)
		}
	}
}

// contains fails the test unless s contains every want.
func contains(t *testing.T, name, s string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(s, w) {
			t.Errorf("%s does not contain %q", name, w)
		}
	}
}

// notContains fails the test if s contains any of unwanted.
func notContains(t *testing.T, name, s string, unwanted ...string) {
	t.Helper()
	for _, w := range unwanted {
		if strings.Contains(s, w) {
			t.Errorf("%s contains %q", name, w)
		}
	}
}

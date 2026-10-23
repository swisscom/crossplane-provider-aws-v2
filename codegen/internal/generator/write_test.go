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
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// snapshot returns the content of every file below dir, keyed by path.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p) //nolint:gosec // test directory
		files[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// copyTemplates copies the templates into a new directory and applies edit
// to one of them.
func copyTemplates(t *testing.T, name string, edit func(string) string) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS(filepath.Join("..", "..", "templates"))); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dst, name)
	b, err := os.ReadFile(p) //nolint:gosec // test directory
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(edit(string(b))), 0o600); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestRunIsAllOrNothing(t *testing.T) {
	out := docdbFixture.mustRun(t, nil, "")
	before := snapshot(t, out)
	broken := copyTemplates(t, "controller/conversions.go.tpl", func(s string) string { return s + "\nfunc broken( {\n" })

	o := docdbFixture.options(t, out, filepath.Join("..", "..", "testdata", "docdb", "generator-config.yaml"))
	o.TemplateDir = broken
	err := Run(context.Background(), o)

	errContains(t, err, "cannot format generated code (see *.broken files)")
	after := snapshot(t, out)
	var brokenFiles []string
	for p := range after {
		if strings.HasSuffix(p, ".broken") {
			brokenFiles = append(brokenFiles, p)
			delete(after, p)
		}
	}
	if len(brokenFiles) == 0 {
		t.Error("no .broken file written")
	}
	if !maps.Equal(before, after) {
		t.Error("a failed Run changed generated files")
	}
}

func TestRunRemovesStaleFiles(t *testing.T) {
	staleAPI := filepath.Join("apis", "docdb", "v1alpha1", "zz_old_kind.go")
	staleCtrl := filepath.Join("internal", "controller", "docdb", "oldkind", "zz_controller.go")
	kept := filepath.Join("apis", "docdb", "v1alpha1", "zz_generated.deepcopy.go")
	seed := func(t *testing.T, out string, paths ...string) {
		t.Helper()
		for _, p := range paths {
			if err := os.MkdirAll(filepath.Join(out, filepath.Dir(p)), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, p), []byte("package x\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	exists := func(t *testing.T, path string) bool {
		t.Helper()
		_, err := os.Stat(path)
		return err == nil
	}

	t.Run("removed", func(t *testing.T) {
		out := t.TempDir()
		seed(t, out, staleAPI, staleCtrl, kept)

		if err := docdbFixture.runIn(t, out, nil, ""); err != nil {
			t.Fatalf("Run: %v", err)
		}

		for _, p := range []string{staleAPI, staleCtrl, filepath.Dir(staleCtrl)} {
			if exists(t, filepath.Join(out, p)) {
				t.Errorf("%s still exists", p)
			}
		}
		if !exists(t, filepath.Join(out, kept)) {
			t.Errorf("%s was removed", kept)
		}
	})
	t.Run("hand-written files", func(t *testing.T) {
		out := t.TempDir()
		handWritten := filepath.Join("internal", "controller", "docdb", "oldkind", "setup.go")
		seed(t, out, staleCtrl, handWritten)
		before := snapshot(t, out)

		err := docdbFixture.runIn(t, out, nil, "")

		errContains(t, err, "internal/controller/docdb/oldkind: kind is no longer generated but has hand-written files (setup.go); delete the directory or stop ignoring the kind")
		if !maps.Equal(before, snapshot(t, out)) {
			t.Error("a failed Run changed files")
		}
	})
}

func TestRunLeavesNoTempFiles(t *testing.T) {
	out := docdbFixture.mustRun(t, nil, "")

	for p := range snapshot(t, out) {
		if strings.HasSuffix(p, ".tmp") || strings.HasSuffix(p, ".broken") {
			t.Errorf("left behind %s", p)
		}
	}
}

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
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gateScript is hack/check-import-todo.sh of this repository.
var gateScript = filepath.Join("..", "..", "..", "hack", "check-import-todo.sh")

func runGate(t *testing.T, root string) (stdout, stderr string, code int) {
	t.Helper()
	script, err := filepath.Abs(gateScript)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", script, root)
	var o, e strings.Builder
	cmd.Stdout, cmd.Stderr = &o, &e
	err = cmd.Run()
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		code = ee.ExitCode()
	case err != nil:
		t.Fatal(err)
	}
	return o.String(), e.String(), code
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func gitAdd(t *testing.T, dir string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", dir, "add", "-A").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
}

func TestCheckImportTodoScript(t *testing.T) {
	for _, tool := range []string{"bash", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
	cases := map[string]struct {
		files      map[string]string
		untracked  map[string]string
		wantCode   int
		wantStdout []string
	}{
		"Clean": {
			files:    map[string]string{"internal/x/a.go": "package x\n", "README.md": "# x\n"},
			wantCode: 0,
		},
		"QuarantinedFile": {
			files:      map[string]string{"internal/x/a_importtodo.go": "//go:build importtodo\n\npackage x\n"},
			wantCode:   1,
			wantStdout: []string{"internal/x/a_importtodo.go: quarantined by codegen/cmd/import (build tag importtodo)"},
		},
		"QuarantinedTest": {
			files:      map[string]string{"internal/x/a_importtodo_test.go": "//go:build importtodo\n\npackage x\n"},
			wantCode:   1,
			wantStdout: []string{"internal/x/a_importtodo_test.go: quarantined by codegen/cmd/import"},
		},
		"Marker": {
			files:      map[string]string{"internal/x/a.go": "package x\n\nfunc F() {\n\tpanic(\"TODO(import): F does not compile yet\")\n}\n"},
			wantCode:   1,
			wantStdout: []string{`internal/x/a.go:4: panic("TODO(import): F does not compile yet")`},
		},
		"CodegenIsExcluded": {
			files:    map[string]string{"codegen/internal/importer/q.go": "package importer // TODO(import)\n", "codegen/x_importtodo.go": "package x\n"},
			wantCode: 0,
		},
		"DocsAreExcluded": {
			files:    map[string]string{"docs/x.md": "Remove each TODO(import) marker.\n"},
			wantCode: 0,
		},
		"UntrackedCounts": {
			untracked:  map[string]string{"internal/y/b.go": "package y // TODO(import): port me\n"},
			wantCode:   1,
			wantStdout: []string{"internal/y/b.go:1: package y // TODO(import): port me"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			gitInit(t, root)
			for p, c := range tc.files {
				writeFile(t, filepath.Join(root, p), c)
			}
			gitAdd(t, root)
			for p, c := range tc.untracked {
				writeFile(t, filepath.Join(root, p), c)
			}
			stdout, stderr, code := runGate(t, root)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tc.wantCode, stdout, stderr)
			}
			for _, w := range tc.wantStdout {
				if !strings.Contains(stdout, w) {
					t.Errorf("stdout does not contain %q:\n%s", w, stdout)
				}
			}
			if tc.wantCode == 0 && stdout+stderr != "" {
				t.Errorf("clean tree printed output:\n%s%s", stdout, stderr)
			}
			if tc.wantCode == 1 && !strings.Contains(stderr, "check-import-todo: code left by codegen/cmd/import still needs porting") {
				t.Errorf("stderr:\n%s", stderr)
			}
		})
	}

	// A file grep can't read must fail the gate, not pass it unchecked.
	t.Run("UnreadableFile", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root reads any file")
		}
		root := t.TempDir()
		gitInit(t, root)
		f := filepath.Join(root, "internal", "x", "a.go")
		writeFile(t, f, "package x // TODO(import)\n")
		gitAdd(t, root)
		if err := os.Chmod(f, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(f, 0o600) })
		_, stderr, code := runGate(t, root)
		if code != 2 || !strings.Contains(stderr, "cannot read") {
			t.Errorf("exit code = %d, stderr = %q; want 2 and cannot read", code, stderr)
		}
	})

	t.Run("NotARepo", func(t *testing.T) {
		root := t.TempDir()
		_, stderr, code := runGate(t, root)
		if code != 2 || !strings.Contains(stderr, "is not a git checkout") {
			t.Errorf("exit code = %d, stderr = %q; want 2 and not a git checkout", code, stderr)
		}
	})
}

// The gate catches what the real quarantine leaves behind.
func TestCheckImportTodoCatchesQuarantine(t *testing.T) {
	for _, tool := range []string{"bash", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
	root := t.TempDir()
	gitInit(t, root)
	file := filepath.Join(root, "internal", "controller", "svc", "kind", "setup.go")
	writeFile(t, file, overrideFixture)
	im := testImporter(t, root, portedFile{path: file, kind: testKind})
	if _, _, code := runGate(t, root); code != 0 {
		t.Fatalf("gate before the import: exit %d, want 0", code)
	}
	if err := im.typecheck(); err != nil {
		t.Fatal(err)
	}
	stdout, _, code := runGate(t, root)
	if code != 1 {
		t.Errorf("gate after quarantine: exit %d, want 1", code)
	}
	for _, want := range []string{
		"internal/controller/svc/kind/setup_importtodo.go: quarantined by codegen/cmd/import",
		"internal/controller/svc/kind/setup.go:12: panic(\"TODO(import): customExternal.Create does not compile yet",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("gate output does not contain %q:\n%s", want, stdout)
		}
	}
}

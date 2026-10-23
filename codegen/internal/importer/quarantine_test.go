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
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	testModule = "example.com/p"
	testKind   = "Kind"
	fooKind    = "Foo"
)

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p) //nolint:gosec // test file
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// testImporter returns an importer for a fresh module in a temp dir, which
// treats the given files as written by this import.
func testImporter(t *testing.T, root string, ported ...portedFile) *importer {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		writeFile(t, filepath.Join(root, "go.mod"), "module "+testModule+"\n\ngo 1.22\n")
	}
	return &importer{
		ctx: context.Background(), tgt: &target{root: root, module: testModule},
		rw:     newRewriter(testModule, func(string) bool { return false }),
		report: &Report{}, newDirs: map[string]bool{}, todo: map[string]int{}, ported: ported,
	}
}

// goRun runs a main package that calls into the fixture and returns its
// combined output and whether it failed.
func goRun(t *testing.T, root, pkg, call string) (string, bool) {
	t.Helper()
	writeFile(t, filepath.Join(root, "cmd", "m", "main.go"), "package main\n\nimport (\n\t\"fmt\"\n\n\t\""+testModule+"/"+pkg+"\"\n)\n\nfunc main() { fmt.Print("+call+") }\n")
	cmd := exec.Command("go", "run", "./cmd/m")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err != nil
}

// An error without a position, like a package name clash, cannot be
// quarantined, so it must fail the import instead of being swallowed.
func TestTypecheckFailsOnPositionlessError(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "internal", "clients", "svc")
	writeFile(t, filepath.Join(dir, "existing.go"), "package svc\n\nfunc A() {}\n")
	file := filepath.Join(dir, "ported.go")
	writeFile(t, file, "// license\n\npackage dbinstance\n\nfunc B() {}\n")
	im := testImporter(t, root, portedFile{path: file})

	err := im.typecheck()
	if err == nil || !strings.Contains(err.Error(), "found packages") {
		t.Fatalf("typecheck() = %v, want an error with the package clash", err)
	}
	if _, serr := os.Stat(filepath.Join(dir, "ported_importtodo.go")); serr == nil {
		t.Error("a positionless error was quarantined into the license header")
	}
}

// A type check that needs more rounds than allowed fails and lists what is
// left, instead of exiting 0 with a tree that does not build.
func TestTypecheckFailsIfItDoesNotConverge(t *testing.T) {
	old := maxRounds
	maxRounds = 1
	t.Cleanup(func() { maxRounds = old })

	root := t.TempDir()
	file := filepath.Join(root, "internal", "x", "x.go")
	// Round 1 moves T, round 2 then moves F, whose signature uses T.
	writeFile(t, file, "package x\n\ntype T struct{ X Undefined }\n\nfunc F(t T) {}\n")
	im := testImporter(t, root, portedFile{path: file})

	err := im.typecheck()
	if err == nil || !strings.Contains(err.Error(), "did not converge after 1 rounds") || !strings.Contains(err.Error(), "internal/x/x.go:") || !strings.Contains(err.Error(), "undefined: T") {
		t.Fatalf("typecheck() = %v, want did not converge with the remaining error", err)
	}

	maxRounds = old
	if err := im.typecheck(); err != nil {
		t.Errorf("typecheck() with enough rounds = %v", err)
	}
}

// Hand-written code is never quarantined: an error in a file this import
// did not write fails the import and leaves the file alone.
func TestTypecheckNeverQuarantinesHandWrittenFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "internal", "controller", "svc", "kind")
	handWritten := `package kind

// isUpToDate was reviewed and hand-ported (v2 semantics).
func isUpToDate() bool { return compareAllUpdatableFields() }

func compareAllUpdatableFields() bool { return false }

func Check() bool { return isUpToDate() }
`
	writeFile(t, filepath.Join(dir, "setup.go"), handWritten)
	// A new upstream file, written by this import.
	newFile := filepath.Join(dir, "hooks.go")
	writeFile(t, newFile, "package kind\n\n// isUpToDate of a newer upstream revision.\nfunc isUpToDate() bool { return true }\n")
	im := testImporter(t, root, portedFile{path: newFile, kind: testKind})

	err := im.typecheck()
	if err == nil || !strings.Contains(err.Error(), "internal/controller/svc/kind/setup.go:") || !strings.Contains(err.Error(), "(not written by this import)") {
		t.Fatalf("typecheck() = %v, want an error naming setup.go as not written by this import", err)
	}
	if got := readFile(t, filepath.Join(dir, "setup.go")); got != handWritten {
		t.Errorf("hand-written setup.go changed:\n%s", got)
	}
	if _, serr := os.Stat(filepath.Join(dir, "setup_importtodo.go")); serr == nil {
		t.Error("hand-written code was quarantined to setup_importtodo.go")
	}
}

const overrideFixture = `package kind

type external struct{}

// Create of the generated client: creates an EMPTY cluster.
func (external) Create() string { return "CreateDBCluster (empty)" }

type customExternal struct{ external }

// Create restores from a snapshot instead.
func (customExternal) Create() string { return restoreFromSnapshot() + undefinedV1Helper() }

func restoreFromSnapshot() string { return "RestoreDBClusterFromSnapshot" }

// Which operation runs.
func Which() string { return customExternal{}.Create() }
`

// A quarantined override stays as a stub that panics, so the embedded
// generated method can never run in its place.
func TestTypecheckStubsOverridingMethod(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "internal", "controller", "svc", "kind")
	file := filepath.Join(dir, "setup.go")
	writeFile(t, file, overrideFixture)
	im := testImporter(t, root, portedFile{path: file, kind: testKind})

	if err := im.typecheck(); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, file)
	if !strings.Contains(got, "// Create restores from a snapshot instead.\nfunc (customExternal) Create() string {\n\tpanic(\"TODO(import): customExternal.Create does not compile yet; port it from setup_importtodo.go\")\n}") {
		t.Errorf("setup.go has no stub for customExternal.Create:\n%s", got)
	}
	todo := readFile(t, filepath.Join(dir, "setup_importtodo.go"))
	if !strings.HasPrefix(todo, "//go:build importtodo") || !strings.Contains(todo, "restoreFromSnapshot() + undefinedV1Helper()") {
		t.Errorf("setup_importtodo.go does not hold the original:\n%s", todo)
	}
	if n := im.report.Blocking(); n != 1 {
		t.Errorf("Blocking() = %d, want 1: %v", n, findings(im.report, topicTodo))
	}
	if !hasFinding(im.report, topicTodo, "body moved to `setup_importtodo.go`; the stub panics until it is ported: undefined: undefinedV1Helper") {
		t.Errorf("no stub finding: %v", findings(im.report, topicTodo))
	}

	out, failed := goRun(t, root, "internal/controller/svc/kind", "kind.Which()")
	if !failed || !strings.Contains(out, "TODO(import): customExternal.Create does not compile yet") || strings.Contains(out, "CreateDBCluster (empty)") {
		t.Errorf("Which() must panic with the TODO instead of calling the embedded Create; got failed=%v:\n%s", failed, out)
	}
}

// A broken hook no longer takes configure<Kind> and init with it, so the
// controller cannot silently run on generated defaults.
func TestTypecheckKeepsInitAndConfigure(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "internal", "controller", "svc", "kind")
	writeFile(t, filepath.Join(dir, "zz_controller.go"), `package kind

var configure func() []string

func Hooks() []string {
	if configure == nil {
		return []string{"generated defaults only"}
	}
	return configure()
}
`)
	file := filepath.Join(dir, "setup.go")
	writeFile(t, file, `package kind

func preDelete() string { return "set SkipFinalSnapshot" + awserrThing() }

func configureKind() []string { return []string{preDelete()} }

func init() {
	configure = configureKind
}
`)
	im := testImporter(t, root, portedFile{path: file, kind: testKind})
	if err := im.typecheck(); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, file)
	for _, want := range []string{
		"func configureKind() []string { return []string{preDelete()} }",
		"func init() {\n\tconfigure = configureKind\n}",
		"func preDelete() string {\n\tpanic(\"TODO(import): preDelete does not compile yet; port it from setup_importtodo.go\")\n}",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("setup.go does not contain %q:\n%s", want, got)
		}
	}
	if n := im.report.Blocking(); n != 1 {
		t.Errorf("Blocking() = %d, want 1: %v", n, findings(im.report, topicTodo))
	}
	out, failed := goRun(t, root, "internal/controller/svc/kind", "kind.Hooks()")
	if !failed || strings.Contains(out, "generated defaults only") || !strings.Contains(out, "TODO(import): preDelete") {
		t.Errorf("Hooks() must panic in preDelete; got failed=%v:\n%s", failed, out)
	}
}

// If an override's signature does not compile, the override cannot stay as
// a stub. Moving it alone would promote the embedded method, so the
// receiver type and its methods go too, and callers are stubbed.
func TestTypecheckMovesEmbeddingReceiver(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "internal", "controller", "svc", "kind")
	file := filepath.Join(dir, "setup.go")
	writeFile(t, file, `package kind

type creator interface{ Create() string }

type external struct{}

func (external) Create() string { return "CreateDBCluster (empty)" }

type customExternal struct {
	external
	snapshot string
}

// Create restores from a snapshot instead.
func (c customExternal) Create(opts ...v1RequestOption) string { return "restore " + c.snapshot }

func (c customExternal) Snapshot() string { return c.snapshot }

// Which operation runs.
func Which() string {
	var c creator = customExternal{snapshot: "s"}
	return c.Create()
}
`)
	im := testImporter(t, root, portedFile{path: file, kind: testKind})
	if err := im.typecheck(); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, file)
	for _, gone := range []string{"type customExternal", "func (c customExternal)"} {
		if strings.Contains(got, gone) {
			t.Errorf("setup.go still contains %q:\n%s", gone, got)
		}
	}
	todo := readFile(t, filepath.Join(dir, "setup_importtodo.go"))
	for _, want := range []string{"type customExternal struct", "func (c customExternal) Create(", "func (c customExternal) Snapshot()"} {
		if !strings.Contains(todo, want) {
			t.Errorf("setup_importtodo.go does not contain %q:\n%s", want, todo)
		}
	}
	if !hasFinding(im.report, topicTodo, "type `customExternal` and its methods moved to `setup_importtodo.go`: its method `Create` does not compile and `customExternal` embeds `external`, so the embedded `Create` would run instead") {
		t.Errorf("no receiver finding: %v", findings(im.report, topicTodo))
	}
	out, failed := goRun(t, root, "internal/controller/svc/kind", "kind.Which()")
	if !failed || strings.Contains(out, "CreateDBCluster (empty)") {
		t.Errorf("Which() must not reach the embedded Create; got failed=%v:\n%s", failed, out)
	}
}

// The receiver type may live in another file of the import.
func TestTypecheckMovesEmbeddingReceiverOfOtherFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "internal", "controller", "svc", "kind")
	types := filepath.Join(dir, "types.go")
	writeFile(t, types, `package kind

type creator interface{ Create() string }

type external struct{}

func (external) Create() string { return "CreateDBCluster (empty)" }

type customExternal struct{ external }
`)
	file := filepath.Join(dir, "setup.go")
	writeFile(t, file, `package kind

func (customExternal) Create(opts ...v1RequestOption) string { return "restore" }

func Which() string {
	var c creator = customExternal{}
	return c.Create()
}
`)
	im := testImporter(t, root, portedFile{path: types, kind: testKind}, portedFile{path: file, kind: testKind})
	if err := im.typecheck(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, types); strings.Contains(got, "type customExternal") {
		t.Errorf("types.go still declares customExternal:\n%s", got)
	}
	out, failed := goRun(t, root, "internal/controller/svc/kind", "kind.Which()")
	if !failed || strings.Contains(out, "CreateDBCluster (empty)") {
		t.Errorf("Which() must not reach the embedded Create; got failed=%v:\n%s", failed, out)
	}
}

// Removing a test cannot change run-time behaviour, so tests move whole.
func TestTypecheckMovesTestsWhole(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "internal", "x")
	writeFile(t, filepath.Join(dir, "x.go"), "package x\n")
	file := filepath.Join(dir, "x_test.go")
	writeFile(t, file, "package x\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) { v1Helper(t) }\n")
	im := testImporter(t, root, portedFile{path: file})
	if err := im.typecheck(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, file); strings.Contains(got, "TestX") {
		t.Errorf("x_test.go still has TestX:\n%s", got)
	}
	todo := readFile(t, filepath.Join(dir, "x_importtodo_test.go"))
	if !strings.Contains(todo, "func TestX(t *testing.T) { v1Helper(t) }") {
		t.Errorf("x_importtodo_test.go:\n%s", todo)
	}
	if !hasFinding(im.report, topicTodo, "`internal/x/x_test.go`: `func TestX(t *testing.T)` moved to `x_importtodo_test.go`: undefined: v1Helper") {
		t.Errorf("findings: %v", findings(im.report, topicTodo))
	}
}

// The message of a declaration with several errors is its first error,
// whatever order the errors come in.
func TestQuarantinePicksEarliestError(t *testing.T) {
	const src = "package x\n\nfunc F() {\n\ta()\n\tb()\n\tc()\n}\n"
	errs := []loadError{
		{line: 6, col: 2, msg: "undefined: c"},
		{line: 4, col: 2, msg: "undefined: a"},
		{line: 5, col: 2, msg: "undefined: b"},
	}
	for i := range 20 {
		root := t.TempDir()
		file := filepath.Join(root, "x.go")
		writeFile(t, file, src)
		im := testImporter(t, root, portedFile{path: file})
		order := slices.Clone(errs)
		for j := range order {
			order[j].file = file
		}
		// Rotate, so every error comes first at some point.
		order = append(order[i%3:], order[:i%3]...)
		if _, err := im.quarantine(file, order); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(readFile(t, filepath.Join(root, "x_importtodo.go")), "// TODO(import): undefined: a\n") {
			t.Fatalf("run %d: TODO message is not the first error:\n%s", i, readFile(t, filepath.Join(root, "x_importtodo.go")))
		}
	}
}

// A re-import that ports no file has nothing to type check.
func TestTypecheckWithoutPortedFiles(t *testing.T) {
	im := testImporter(t, t.TempDir())
	if err := im.typecheck(); err != nil {
		t.Errorf("typecheck() = %v, want nil", err)
	}
}

// Code using an import that cannot be resolved (e.g. SDK v1 awserr) is
// stubbed if only its body uses it, and the import is dropped.
func TestTypecheckStubsUsersOfABadImport(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "internal", "x", "x.go")
	writeFile(t, file, `package x

import "example.com/missing/awserr"

func IsNotFound(err error) bool {
	_, ok := err.(awserr.Error)
	return ok
}

func Other() string { return "ok" }
`)
	im := testImporter(t, root, portedFile{path: file})
	if err := im.typecheck(); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, file)
	if strings.Contains(got, "example.com/missing/awserr") || !strings.Contains(got, `panic("TODO(import): IsNotFound does not compile yet`) || !strings.Contains(got, `func Other() string { return "ok" }`) {
		t.Errorf("x.go:\n%s", got)
	}
	if !hasFinding(im.report, topicTodo, "cannot import \"example.com/missing/awserr\"") && !hasFinding(im.report, topicTodo, "example.com/missing/awserr") {
		t.Errorf("findings: %v", findings(im.report, topicTodo))
	}
}

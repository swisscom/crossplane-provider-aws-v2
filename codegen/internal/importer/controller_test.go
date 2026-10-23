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
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// findings returns the texts of the report's findings of a topic.
func findings(r *Report, topic string) []string {
	out := make([]string, 0, len(r.Items[topic]))
	for _, f := range r.Items[topic] {
		out = append(out, f.Text)
	}
	return out
}

func hasFinding(r *Report, topic, substr string) bool {
	for _, f := range findings(r, topic) {
		if strings.Contains(f, substr) {
			return true
		}
	}
	return false
}

// A controller that was ported before is hand-maintained: new upstream files
// are listed, not copied next to the reviewed code.
func TestPortPackageSkipsNewFilesOfPortedController(t *testing.T) {
	src, root := t.TempDir(), t.TempDir()
	srcDir := filepath.Join(src, "pkg", "controller", "svc", "kind")
	writeFile(t, filepath.Join(srcDir, "setup.go"), "package kind\n")
	writeFile(t, filepath.Join(srcDir, "hooks.go"), "package kind\n\nfunc isUpToDate() bool { return true }\n")
	dstDir := filepath.Join(root, "internal", "controller", "svc", "kind")
	writeFile(t, filepath.Join(dstDir, "setup.go"), "package kind\n")
	im := testImporter(t, root)
	im.src = src

	if err := im.portPackage(srcDir, dstDir, testModule+"/internal/controller/svc/kind", "Kind"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "hooks.go")); err == nil {
		t.Error("hooks.go was ported into a controller that was ported before")
	}
	if len(im.ported) != 0 {
		t.Errorf("ported = %v, want none", im.ported)
	}
	if !hasFinding(im.report, topicController, "`pkg/controller/svc/kind/hooks.go` has no counterpart in `internal/controller/svc/kind`, which was ported before; not ported") {
		t.Errorf("no notice about hooks.go: %v", findings(im.report, topicController))
	}
	if im.report.Blocking() != 0 {
		t.Errorf("Blocking() = %d, want 0 (nothing in the target is lost)", im.report.Blocking())
	}
}

// convert parses src as srcRel, runs convertSetup and returns the printed
// file.
func convert(t *testing.T, src, kind string, r *Report) string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "setup.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if !convertSetup(fset, f, "pkg/controller/svc/foo/setup.go", "internal/controller/svc/foo/setup.go", kind, r) {
		t.Fatal("no Setup function")
	}
	b, err := formatFile(fset, f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const setupWithElseAndWatches = `package foo

func SetupFoo(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(svcapitypes.FooGroupKind)
	reconcilerOpts := []managed.ReconcilerOption{
		managed.WithPollInterval(o.PollInterval),
	}
	if o.Features.Enabled(features.EnableAlphaManagementPolicies) {
		reconcilerOpts = append(reconcilerOpts, managed.WithManagementPolicies())
	} else {
		reconcilerOpts = append(reconcilerOpts, managed.WithInitializers(importantInitializer()))
	}
	r := managed.NewReconciler(mgr, resource.ManagedKind(svcapitypes.FooGroupVersionKind), reconcilerOpts...)
	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&svcapitypes.Foo{}).
		Watches(&corev1.Secret{}, secretToFoo()).
		Complete(r)
}
`

// Setup code the generated Setup does not reproduce is kept as a marker
// with its source line, and blocks the import.
func TestConvertSetupKeepsDroppedCodeAsMarker(t *testing.T) {
	r := &Report{}
	out := convert(t, setupWithElseAndWatches, fooKind, r)
	for _, want := range []string{
		"// configureFoo returns options for the generated Setup function.\n//\n// TODO(import): SetupFoo did things the generated Setup does not. Port them by hand, then remove this comment:\n",
		"//   - pkg/controller/svc/foo/setup.go:8: if o.Features.Enabled(features.EnableAlphaManagementPolicies) {",
		"importantInitializer()",
		"(has an else branch)",
		"//   - pkg/controller/svc/foo/setup.go:17: Watches(&corev1.Secret{}, secretToFoo())",
		"func configureFoo(",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
	if n := r.Blocking(); n != 2 {
		t.Errorf("Blocking() = %d, want 2: %v", n, findings(r, topicController))
	}
	if !hasFinding(r, topicController, "`internal/controller/svc/foo/setup.go`: `SetupFoo` (pkg/controller/svc/foo/setup.go:17): dropped `Watches(&corev1.Secret{}, secretToFoo())`") {
		t.Errorf("no Watches finding: %v", findings(r, topicController))
	}
}

func TestConvertSetupRules(t *testing.T) {
	const head = "package foo\n\nfunc SetupFoo(mgr ctrl.Manager, o controller.Options) error {\n\tname := managed.ControllerName(svcapitypes.FooGroupKind)\n\treconcilerOpts := []managed.ReconcilerOption{managed.WithPollInterval(o.PollInterval)}\n"
	const tail = "\tr := managed.NewReconciler(mgr, resource.ManagedKind(svcapitypes.FooGroupVersionKind), reconcilerOpts...)\n\treturn ctrl.NewControllerManagedBy(mgr).\n\t\tNamed(name).\n\t\tWithOptions(o.ForControllerRuntime()).\n\t\tWithEventFilter(%s).\n\t\tFor(&svcapitypes.Foo{}).\n\t\tComplete(r)\n}\n"
	cases := map[string]struct {
		body, filter string
		blocking     int
		want         []string
		notWant      []string
	}{
		"PlainFeatureCheck": {
			body:     "\tif o.Features.Enabled(features.EnableAlphaManagementPolicies) {\n\t\treconcilerOpts = append(reconcilerOpts, managed.WithManagementPolicies())\n\t}\n",
			filter:   "resource.DesiredStateChanged()",
			blocking: 0,
			notWant:  []string{"TODO(import)", "Features"},
		},
		"CompoundFeatureCheck": {
			body:     "\tif o.Features.Enabled(features.EnableAlphaManagementPolicies) && extra {\n\t\treconcilerOpts = append(reconcilerOpts, managed.WithManagementPolicies())\n\t}\n",
			filter:   "resource.DesiredStateChanged()",
			blocking: 1,
			want:     []string{"(has a condition besides the feature flag)"},
		},
		"OtherEventFilter": {
			filter:   "other()",
			blocking: 1,
			want:     []string{"//   - pkg/controller/svc/foo/setup.go:10: WithEventFilter(other()) (a controller builder call)"},
		},
		"EarlyReturnNil": {
			body:     "\tif !o.Enabled {\n\t\treturn nil\n\t}\n",
			filter:   "resource.DesiredStateChanged()",
			blocking: 1,
			want:     []string{"return nil (returned early without setting up the controller)", "\t\treturn nil, nil, nil\n"},
		},
		"ReturnErrIsFine": {
			body:     "\tif err := check(); err != nil {\n\t\treturn err\n\t}\n",
			filter:   "resource.DesiredStateChanged()",
			blocking: 0,
			want:     []string{"\t\treturn nil, nil, err\n"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := &Report{}
			out := convert(t, head+tc.body+strings.Replace(tail, "%s", tc.filter, 1), fooKind, r)
			if got := r.Blocking(); got != tc.blocking {
				t.Errorf("Blocking() = %d, want %d: %v\n%s", got, tc.blocking, findings(r, topicController), out)
			}
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("output does not contain %q:\n%s", w, out)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(out, w) {
					t.Errorf("output contains %q:\n%s", w, out)
				}
			}
		})
	}
}

// The Setup functions of the kinds imported so far (copied from
// provider-aws ad8ac939) convert without a blocking finding.
func TestConvertSetupOfImportedKinds(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "setup", "*.go.txt"))
	if err != nil || len(files) != 5 {
		t.Fatalf("testdata: %v %v", files, err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			src := readFile(t, file)
			kind := src[strings.Index(src, "func Setup")+len("func Setup") : strings.Index(src, "(mgr")]
			r := &Report{}
			out := convert(t, src, kind, r)
			if n := r.Blocking(); n != 0 || strings.Contains(out, "TODO(import)") {
				t.Errorf("Blocking() = %d: %v\n%s", n, findings(r, topicController), out)
			}
		})
	}
}

// Even if an unclean helper path got recorded, nothing is read or written
// outside the roots.
func TestPortHelpersStaysInsideRoots(t *testing.T) {
	root := t.TempDir()
	im := testImporter(t, root)
	im.src = t.TempDir()
	im.rw.helperPkgs["pkg/../../escape"] = true
	err := im.portHelpers()
	if err == nil || !strings.Contains(err.Error(), `helper package "pkg/../../escape" resolves to`) {
		t.Errorf("portHelpers() = %v, want a containment error", err)
	}
}

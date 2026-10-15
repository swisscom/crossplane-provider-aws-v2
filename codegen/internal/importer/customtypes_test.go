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
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAPIUses(t *testing.T) {
	dir := t.TempDir()
	src := `package dbinstance

import (
	svcapitypes "example.com/apis/docdb/v1alpha1"
	other "example.com/apis/rds/v1alpha1"
)

func f(cr *svcapitypes.DBInstance) bool {
	_ = other.Ignored
	return cr.Status.AtProvider.DBInstanceStatus == svcapitypes.DocDBInstanceStateAvailable
}
`
	if err := os.WriteFile(filepath.Join(dir, "setup.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := apiUses([]string{dir, filepath.Join(dir, "missing")}, "example.com/apis/docdb/v1alpha1")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"DBInstance": true, "DocDBInstanceStateAvailable": true}
	if !maps.Equal(want, got) {
		t.Errorf("apiUses() = %v, want %v", got, want)
	}
}

// A re-import adds the Ref and Selector fields of references to kinds that
// are imported now to an existing struct, and does not declare methods
// again that exist in hand-written files.
func TestReimportMergesFieldsAndSkipsExisting(t *testing.T) {
	srcDir, tgtDir := t.TempDir(), t.TempDir()
	src := `package v1alpha1

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// CustomFooParameters for Foo
type CustomFooParameters struct {
	// Kept is hand-written in the target.
	Kept *string ` + "`json:\"kept,omitempty\"`" + `

	BarNameRef      *xpv1.Reference ` + "`json:\"barNameRef,omitempty\"`" + `
	BarNameSelector *xpv1.Selector  ` + "`json:\"barNameSelector,omitempty\"`" + `

	Last *string ` + "`json:\"last,omitempty\"`" + `
}

func (mg *Foo) ResolveReferences(ctx context.Context) error { return nil }

func (mg *Bar) ResolveReferences(ctx context.Context) error { return nil }
`
	tgt := `/*
header
*/

package v1alpha1

// CustomFooParameters for Foo
type CustomFooParameters struct {
	// Kept is hand-written in the target.
	Kept *string ` + "`json:\"kept,omitempty\"`" + `

	Last *string ` + "`json:\"last,omitempty\"`" + `
}
`
	existing := "package v1alpha1\n\nfunc (mg *Bar) ResolveReferences() error { return nil }\n"
	for p, c := range map[string]string{
		filepath.Join(srcDir, "custom_types.go"): src,
		filepath.Join(tgtDir, "custom_types.go"): tgt,
		filepath.Join(tgtDir, "bar.go"):          existing,
	} {
		if err := os.WriteFile(p, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pkg, err := parseAPIPackage(srcDir, SourceModule+"/apis/svc/v1alpha1")
	if err != nil {
		t.Fatal(err)
	}
	r := &Report{}
	c := &customTypes{
		src: pkg, svcVer: "svc/v1alpha1", kinds: []string{"Bar", "Foo"},
		tgt:    &target{root: tgtDir, module: "example.com/p"},
		rw:     newRewriter("example.com/p", func(string) bool { return true }),
		report: r, keep: map[string]bool{"CustomFooParameters": true},
	}
	merged, fields, err := c.mergeFields([]byte(tgt), pkg.decls["CustomFooParameters"])
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"BarNameRef", "BarNameSelector"}; !slices.Equal(fields, want) {
		t.Errorf("mergeFields() added %v, want %v", fields, want)
	}
	for _, s := range []string{
		"Kept *string",
		"BarNameRef *xpv2.NamespacedReference",
		"BarNameSelector *xpv2.NamespacedSelector",
		`xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"`,
	} {
		if !strings.Contains(strings.Join(strings.Fields(string(merged)), " "), strings.Join(strings.Fields(s), " ")) {
			t.Errorf("merged file lacks %q:\n%s", s, merged)
		}
	}
	if bytes.Index(merged, []byte("BarNameRef")) > bytes.Index(merged, []byte("Last")) {
		t.Errorf("BarNameRef must be inserted before Last, after Kept:\n%s", merged)
	}

	// Target fields in another order than the source, with line comments.
	reordered := `package v1alpha1

type CustomFooParameters struct {
	Last *string ` + "`json:\"last,omitempty\"`" + ` // last
	Kept *string ` + "`json:\"kept,omitempty\"`" + ` // kept
}
`
	merged, fields, err = c.mergeFields([]byte(reordered), pkg.decls["CustomFooParameters"])
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || !strings.Contains(string(merged), "// kept\n\n\tBarNameRef") {
		t.Errorf("reordered target: added %v:\n%s", fields, merged)
	}

	// writeOther writes Foo's resolver to foo.go and skips Bar's.
	c.tgt.root = t.TempDir()
	apis := filepath.Join(c.tgt.root, "apis", "svc", "v1alpha1")
	if err := os.MkdirAll(apis, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"custom_types.go", "bar.go"} {
		b, _ := os.ReadFile(filepath.Join(tgtDir, n))
		if err := os.WriteFile(filepath.Join(apis, n), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	c.header = []byte("/*\nheader\n*/\n\n")
	c.other = []*decl{pkg.decls["Foo.ResolveReferences"], pkg.decls["Bar.ResolveReferences"]}
	if err := c.writeOther(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(apis, "foo.go"))
	if err != nil {
		t.Fatalf("foo.go not written: %v", err)
	}
	if strings.Contains(string(b), "Bar") || !strings.Contains(string(b), "func (mg *Foo) ResolveReferences") {
		t.Errorf("foo.go:\n%s", b)
	}
}

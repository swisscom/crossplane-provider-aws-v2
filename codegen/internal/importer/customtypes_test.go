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

const customTypesFile = "custom_types.go"

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
		src: pkg, svcVer: "svc/v1alpha1", kinds: []string{"Bar", fooKind},
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

// A re-import keeps the declaration order of custom_types.go: stubs are
// filled in place and only new types go to the end.
func TestWriteTypesKeepsOrder(t *testing.T) {
	srcDir, dir := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(srcDir, customTypesFile), `package v1alpha1

// CustomFooObservation of Foo.
type CustomFooObservation struct{}

// NewType is new.
type NewType struct {
	A *string `+"`json:\"a,omitempty\"`"+`
}
`)
	tgt := `/*
header
*/

package v1alpha1

// CustomFooParameters of Foo.
type CustomFooParameters struct {
	A *string ` + "`json:\"a,omitempty\"`" + `
}

// CustomFooObservation of Foo.
type CustomFooObservation struct{}

// Last was declared last by hand.
type Last struct{}
`
	writeFile(t, filepath.Join(dir, customTypesFile), tgt)
	pkg, err := parseAPIPackage(srcDir, SourceModule+"/apis/svc/v1alpha1")
	if err != nil {
		t.Fatal(err)
	}
	c := &customTypes{
		src: pkg, svcVer: "svc/v1alpha1", kinds: []string{fooKind},
		tgt: &target{root: dir, module: testModule}, rw: newRewriter(testModule, func(string) bool { return true }),
		report: &Report{}, keep: map[string]bool{"CustomFooObservation": true, "NewType": true},
	}
	if err := c.writeTypes(dir); err != nil {
		t.Fatal(err)
	}
	first := readFile(t, filepath.Join(dir, customTypesFile))
	order := []string{"type CustomFooParameters", "type CustomFooObservation", "type Last", "type NewType"}
	last := -1
	for _, s := range order {
		i := strings.Index(first, s)
		if i < last {
			t.Errorf("%q is out of order:\n%s", s, first)
		}
		last = i
	}
	if strings.Count(first, "type CustomFooObservation") != 1 {
		t.Errorf("CustomFooObservation declared %d times:\n%s", strings.Count(first, "type CustomFooObservation"), first)
	}
	if err := c.writeTypes(dir); err != nil {
		t.Fatal(err)
	}
	if second := readFile(t, filepath.Join(dir, customTypesFile)); second != first {
		t.Errorf("second run changed custom_types.go:\n%s\nfirst:\n%s", second, first)
	}
}

func TestResolverGaps(t *testing.T) {
	const zz = `package v1alpha1

type Foo struct {
	Spec FooSpec ` + "`json:\"spec\"`" + `
}

type FooSpec struct {
	ForProvider FooParameters ` + "`json:\"forProvider\"`" + `
}

type FooParameters struct {
	CustomFooParameters ` + "`json:\",inline\"`" + `
	Tags []*Tag ` + "`json:\"tags,omitempty\"`" + `
}

type Tag struct{ Key *string }
`
	const custom = `package v1alpha1

type CustomFooParameters struct {
	// +crossplane:generate:reference:type=Bar
	BarID *string ` + "`json:\"barID,omitempty\"`" + `

	Nested *Nested
}

type Nested struct {
	// +crossplane:generate:reference:type=Baz
	BazID *string
}
`
	const resolver = `package v1alpha1

// ResolveReferences of Foo.
func (mg *Foo) ResolveReferences() error {
	_ = mg.Spec.ForProvider.Nested.BazID
	return nil
}
`
	cases := map[string]struct {
		files map[string]string
		want  []resolverGap
	}{
		"Gap": {
			files: map[string]string{"zz_foo.go": zz, customTypesFile: custom, "foo.go": resolver},
			want:  []resolverGap{{Kind: fooKind, Method: "foo.go:4", Field: "BarID", Marker: "custom_types.go:5"}},
		},
		"Resolved": {
			files: map[string]string{"zz_foo.go": zz, customTypesFile: custom, "foo.go": strings.Replace(resolver, "_ = mg", "_ = mg.Spec.ForProvider.BarID\n\t_ = mg", 1)},
		},
		"NoHandWrittenResolver": {
			files: map[string]string{"zz_foo.go": zz, customTypesFile: custom},
		},
		"GeneratedResolver": {
			files: map[string]string{"zz_foo.go": zz, customTypesFile: custom, "zz_generated.resolvers.go": resolver},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for n, c := range tc.files {
				writeFile(t, filepath.Join(dir, n), c)
			}
			got, err := resolverGaps(dir)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("resolverGaps() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestReportResolverGaps(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, customTypesFile), "package v1alpha1\n\ntype Foo struct {\n\t// +crossplane:generate:reference:type=Bar\n\tBarID *string\n}\n")
	written := filepath.Join(dir, "foo.go")
	writeFile(t, written, "package v1alpha1\n\n// ResolveReferences of Foo.\nfunc (mg *Foo) ResolveReferences() error { return nil }\n")

	r := &Report{}
	if err := reportResolverGaps(dir, written, r); err != nil {
		t.Fatal(err)
	}
	if !hasFinding(r, topicReferences, "`Foo` has a hand-written `ResolveReferences` (foo.go:4), so angryjet generates no resolver for it: the marker reference of `BarID` (custom_types.go:5) is never resolved") || r.Blocking() != 1 {
		t.Errorf("findings: %v", findings(r, topicReferences))
	}
	want := "// ResolveReferences of Foo.\n// TODO(import): resolve BarID here; angryjet skips kinds with a hand-written ResolveReferences.\nfunc (mg *Foo) ResolveReferences()"
	if got := readFile(t, written); !strings.Contains(got, want) {
		t.Errorf("foo.go:\n%s", got)
	}

	// A file that was not written by this import is left alone.
	before := readFile(t, written)
	if err := reportResolverGaps(dir, "", &Report{}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, written) != before {
		t.Error("hand-written file changed")
	}
}

// A field renamed by hand keeps its json name; adding the source field
// next to it would make encoding/json drop both.
func TestMergeFieldsMatchesJSONNames(t *testing.T) {
	srcDir := t.TempDir()
	writeFile(t, filepath.Join(srcDir, customTypesFile), `package v1alpha1

type CustomDBSubnetGroupParameters struct {
	SubnetIDs []*string `+"`json:\"subnetIDs,omitempty\"`"+`

	SUbnetIDsSelector *string `+"`json:\"subnetIDsSelector,omitempty\"`"+`

	New *string `+"`json:\"new,omitempty\"`"+`
}
`)
	tgt := `package v1alpha1

type CustomDBSubnetGroupParameters struct {
	SubnetIDs []*string ` + "`json:\"subnetIDs,omitempty\"`" + `

	// renamed by hand (typo fix), same JSON name
	SubnetIDsSelector *string ` + "`json:\"subnetIDsSelector,omitempty\"`" + `
}
`
	pkg, err := parseAPIPackage(srcDir, SourceModule+"/apis/docdb/v1alpha1")
	if err != nil {
		t.Fatal(err)
	}
	r := &Report{}
	c := &customTypes{src: pkg, svcVer: "docdb/v1alpha1", kinds: []string{"DBSubnetGroup"},
		tgt: &target{root: t.TempDir(), module: testModule}, rw: newRewriter(testModule, func(string) bool { return true }),
		report: r, keep: map[string]bool{}}
	merged, fields, err := c.mergeFields([]byte(tgt), pkg.decls["CustomDBSubnetGroupParameters"])
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fields, []string{"New"}) || strings.Contains(string(merged), "SUbnetIDsSelector") {
		t.Errorf("mergeFields() added %v:\n%s", fields, merged)
	}
	if !hasFinding(r, topicTypes, "`CustomDBSubnetGroupParameters`: source field `SUbnetIDsSelector` not added: its json name `subnetIDsSelector` is already used by `SubnetIDsSelector`") {
		t.Errorf("findings: %v", findings(r, topicTypes))
	}
}

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
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/swisscom/crossplane-provider-aws-v2/codegen/internal/generator"
	"github.com/swisscom/crossplane-provider-aws-v2/codegen/internal/modeltest"
)

// ignore.resource_names matches API model names, which differ from the kind
// for initialisms (Vpc → VPC).
func TestWriteGeneratorConfigIgnoresByModelName(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src.yaml"), filepath.Join(dir, "dst.yaml")
	if err := os.WriteFile(src, []byte("ignore:\n    resource_names:\n        - Snapshot\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	all := []generator.Kind{{Name: "SecurityGroup", ModelName: "SecurityGroup"}, {Name: "VPCEndpoint", ModelName: "VpcEndpoint"}}
	if err := writeGeneratorConfig(src, dst, all, &Manifest{Kinds: []string{"SecurityGroup"}}, nil, &Report{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{"- Snapshot", "- VpcEndpoint"} {
		if !strings.Contains(got, want) {
			t.Errorf("generator config lacks %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"VPCEndpoint", "- SecurityGroup"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("generator config contains %q:\n%s", unwanted, got)
		}
	}
}

// kinds returns kinds whose model name equals the kind name.
func kinds(names ...string) []generator.Kind {
	ks := make([]generator.Kind, 0, len(names))
	for _, n := range names {
		ks = append(ks, generator.Kind{Name: n, ModelName: n})
	}
	return ks
}

func TestSourceRevision(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	src := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", src, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q")
	run("remote", "add", "origin", "https://example.com/provider-aws.git")
	write("pkg/controller/docdb/dbcluster/setup.go", "package dbcluster\n")
	write("pkg/controller/docdb/dbinstance/setup.go", "package dbinstance\n")
	run("add", ".")
	run("commit", "-q", "-m", "init")
	commit := run("rev-parse", "HEAD")
	commit = commit[:len(commit)-1]

	// Changes of another kind don't make this kind dirty.
	write("pkg/controller/docdb/dbinstance/setup.go", "package dbinstance // changed\n")
	got, err := sourceRevision(src, kindPaths("docdb", "DBCluster"))
	if err != nil {
		t.Fatal(err)
	}
	want := Revision{Repository: "https://example.com/provider-aws.git", Commit: commit}
	if got != want {
		t.Errorf("sourceRevision(DBCluster) = %+v, want %+v", got, want)
	}

	got, err = sourceRevision(src, kindPaths("docdb", "DBInstance"))
	if err != nil {
		t.Fatal(err)
	}
	want.Dirty = true
	if got != want {
		t.Errorf("sourceRevision(DBInstance) = %+v, want %+v", got, want)
	}

	if _, err := sourceRevision(t.TempDir(), kindPaths("docdb", "DBCluster")); err == nil {
		t.Error("sourceRevision() of a directory without git: want error")
	}

	// A helper dir that is read is checked too.
	write("pkg/controller/docdb/utils/tags.go", "package utils\n")
	run("add", ".")
	run("commit", "-q", "-m", "utils")
	write("pkg/controller/docdb/utils/tags.go", "package utils // injected\n")
	got, err = sourceRevision(src, append(kindPaths("docdb", "DBCluster"), "pkg/controller/docdb/utils"))
	if err != nil || !got.Dirty {
		t.Errorf("sourceRevision() with a changed helper = %+v, %v; want dirty", got, err)
	}

	// A copy of provider-aws inside another repository is not a checkout.
	inner := filepath.Join(src, "vendor-copy", "provider-aws")
	write("vendor-copy/provider-aws/pkg/controller/docdb/dbcluster/setup.go", "package dbcluster\n")
	if _, err := sourceRevision(inner, kindPaths("docdb", "DBCluster")); err == nil || !strings.Contains(err.Error(), "is not the root of a git checkout") {
		t.Errorf("sourceRevision() inside an enclosing repo = %v, want not the root", err)
	}
}

func kindPaths(svc, kind string) []string {
	return []string{"apis/" + svc, "pkg/controller/" + svc + "/" + strings.ToLower(kind)}
}

const srcGeneratorConfig = `# source config
ignore:
  resource_names:
    - Other
  field_paths:
    - FooInput.Bar
resources:
  Foo:
    fields:
      Name:
        is_primary_key: true
`

// A hand edit of the derived generator-config.yaml is never overwritten.
func TestWriteGeneratorConfigRefusesHandEdits(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.yaml")
	writeFile(t, src, srcGeneratorConfig)
	dst := filepath.Join(dir, "apis", "svc", "generator-config.yaml")
	m := &Manifest{Kinds: []string{fooKind}}
	if err := writeGeneratorConfig(src, dst, kinds(fooKind, "Baz"), m, nil, &Report{}); err != nil {
		t.Fatal(err)
	}
	edited := readFile(t, dst) + "# PROBE hand edit\n"
	writeFile(t, dst, edited)

	err := writeGeneratorConfig(src, dst, kinds(fooKind, "Baz"), m, nil, &Report{})
	if err == nil || !strings.Contains(err.Error(), "apis/svc/generator-config.yaml was edited after the last import") {
		t.Errorf("writeGeneratorConfig() = %v, want a hand edit error", err)
	}
	if got := readFile(t, dst); got != edited {
		t.Errorf("hand-edited file changed:\n%s", got)
	}
}

func TestWriteGeneratorConfig(t *testing.T) {
	overlay := func(t *testing.T, s string) *rawYAML {
		t.Helper()
		var doc yaml.Node
		if err := yaml.Unmarshal([]byte(s), &doc); err != nil {
			t.Fatal(err)
		}
		return (*rawYAML)(doc.Content[0])
	}
	cases := map[string]struct {
		// existing is the content of dst before the run; digest is set to
		// its sha256 if digestOf is true.
		existing, digest string
		existingIsOutput bool
		src              string
		overlay          string
		wantErr          string
		want             []string
	}{
		"Fresh": {
			src:  srcGeneratorConfig,
			want: []string{"# source config", "resource_names:\n        - Other\n        # not imported (codegen/cmd/import)\n        - Baz\n", "- FooInput.Bar\n        # ignoreFieldPaths of import.yaml\n        - FooInput.Qux\n"},
		},
		"DigestMatchesSourceChanged": {
			existing: "old output\n",
			digest:   digest([]byte("old output\n")),
			src:      strings.Replace(srcGeneratorConfig, "is_primary_key: true", "is_primary_key: false", 1),
			want:     []string{"is_primary_key: false"},
		},
		"NoDigestSameOutput": {
			existingIsOutput: true,
			src:              srcGeneratorConfig,
			want:             []string{"- Baz"},
		},
		"NoDigestDifferent": {
			existing: "older import\n",
			src:      srcGeneratorConfig,
			wantErr:  "apis/svc/generator-config.yaml differs from what the importer generates and import.yaml records no generatorConfigSHA256",
		},
		"Overlay": {
			src: srcGeneratorConfig,
			overlay: `resources:
  Foo:
    fields:
      Name:
        is_primary_key: false
        late_initialize: {}
    compare:
      ignore: [Spec.ForProvider.Tags]
ignore:
  field_paths: [FooInput.Bar, FooInput.Extra]
`,
			want: []string{
				"            Name:\n                is_primary_key: false\n                late_initialize: {}\n",
				"compare:\n            ignore: [Spec.ForProvider.Tags]",
				"- FooInput.Bar\n        - FooInput.Extra\n",
			},
		},
		"OverlayNotAMapping": {
			src:     srcGeneratorConfig,
			overlay: "[a, b]\n",
			wantErr: "apis/svc/import.yaml: generatorConfig must be a mapping",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "src.yaml")
			writeFile(t, src, tc.src)
			dst := filepath.Join(dir, "apis", "svc", "generator-config.yaml")
			m := &Manifest{Kinds: []string{fooKind}, IgnoreFieldPaths: []string{"FooInput.Qux"}, GeneratorConfigSHA256: tc.digest}
			if tc.overlay != "" {
				m.GeneratorConfig = overlay(t, tc.overlay)
			}
			if tc.existingIsOutput {
				if err := writeGeneratorConfig(src, dst, kinds(fooKind, "Baz"), &Manifest{Kinds: m.Kinds, IgnoreFieldPaths: m.IgnoreFieldPaths}, nil, &Report{}); err != nil {
					t.Fatal(err)
				}
			} else if tc.existing != "" {
				writeFile(t, dst, tc.existing)
			}
			before, _ := os.ReadFile(dst) //nolint:gosec // test file

			err := writeGeneratorConfig(src, dst, kinds(fooKind, "Baz"), m, nil, &Report{})
			got, _ := os.ReadFile(dst) //nolint:gosec // test file
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("writeGeneratorConfig() = %v, want %q", err, tc.wantErr)
				}
				if !bytes.Equal(got, before) {
					t.Errorf("file changed on error:\n%s", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tc.want {
				if !strings.Contains(string(got), w) {
					t.Errorf("output does not contain %q:\n%s", w, got)
				}
			}
			if m.GeneratorConfigSHA256 != digest(got) {
				t.Errorf("GeneratorConfigSHA256 = %q, want the digest of the file", m.GeneratorConfigSHA256)
			}
		})
	}
}

// Upstream ignore entries that match nothing in the model are dropped; the
// same mistake in import.yaml is a repo typo and fails.
func TestWriteGeneratorConfigDropsUnresolvedSourcePaths(t *testing.T) {
	base := readFile(t, filepath.Join("..", "..", "testdata", "servicecatalog", "generator-config.yaml"))
	src := filepath.Join(t.TempDir(), "src.yaml")
	writeFile(t, src, strings.Replace(base, "  field_paths:\n", "  field_paths:\n    - ProvisionProductInput.ProvisionTokn\n    - DeadShape.Member\n", 1))
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = t.TempDir()
	}
	dst := filepath.Join(t.TempDir(), "apis", "servicecatalog", "generator-config.yaml")
	o := generator.Options{
		Service: "servicecatalog", APIVersion: "v1alpha1", ServiceSDKVersion: "v1.47.1", ModelName: "service-catalog",
		CacheDir: filepath.Join(cacheDir, "provider-aws-v2-codegen"), GeneratorConfig: dst, OutputDir: t.TempDir(),
	}
	unresolved := unresolvedIgnorePaths(context.Background(), o)
	all := kinds("ProvisionedProduct")

	r := &Report{}
	m := &Manifest{Kinds: []string{"ProvisionedProduct"}, IgnoreFieldPaths: []string{"ProvisionProductInput.Tags"}}
	if err := writeGeneratorConfig(src, dst, all, m, unresolved, r); err != nil {
		modeltest.SkipOnFetchError(t, err)
		t.Fatal(err)
	}
	got := readFile(t, dst)
	for _, p := range []string{"ProvisionProductInput.ProvisionTokn", "DeadShape.Member"} {
		if strings.Contains(got, p) {
			t.Errorf("output still has %s:\n%s", p, got)
		}
		if !hasFinding(r, topicGeneratorConfig, "`ignore.field_paths` entry `"+p+"` from the source config matches nothing in the servicecatalog API model; dropped") {
			t.Errorf("no notice for %s: %v", p, findings(r, topicGeneratorConfig))
		}
	}
	if !strings.Contains(got, "- ProvisionProductInput.ProvisionToken\n") || !strings.Contains(got, "- ProvisionProductInput.Tags\n") {
		t.Errorf("valid entries lost:\n%s", got)
	}
	if r.Blocking() != 0 || m.GeneratorConfigSHA256 != digest([]byte(got)) {
		t.Errorf("Blocking() = %d, digest %q", r.Blocking(), m.GeneratorConfigSHA256)
	}
	if entries, _ := os.ReadDir(filepath.Dir(dst)); len(entries) != 1 {
		t.Errorf("temp files left: %v", entries)
	}

	m.IgnoreFieldPaths = append(m.IgnoreFieldPaths, "ProvisionProductInput.Tagz")
	err = writeGeneratorConfig(src, dst, all, m, unresolved, &Report{})
	if err == nil || !strings.Contains(err.Error(), `apis/servicecatalog/import.yaml: ignoreFieldPaths entry "ProvisionProductInput.Tagz" matches nothing in the servicecatalog API model`) {
		t.Errorf("writeGeneratorConfig() with a typo in import.yaml = %v", err)
	}
	if readFile(t, dst) != got {
		t.Error("file changed although import.yaml has a typo")
	}
}

func TestReadManifestIsStrict(t *testing.T) {
	cases := map[string]struct {
		content, want string
	}{
		"TypoKey":         {"source: x\napiVersion: v1alpha1\nkinds: [DBInstance]\nignoreFieldPath:\n  - ModifyAdditionalStorageVolume.SetForDelete\n", "field ignoreFieldPath not found"},
		"BadKind":         {"apiVersion: v1alpha1\nkinds: [dbInstance]\n", `kinds[0] "dbInstance" is not a kind name`},
		"BadAPIVersion":   {"apiVersion: v1/../x\nkinds: [DBInstance]\n", `apiVersion "v1/../x" must look like v1alpha1`},
		"RevisionNoKind":  {"apiVersion: v1alpha1\nkinds: [DBInstance]\nrevisions:\n  DBCluster:\n    commit: abc\n", "revisions.DBCluster: kind is not in kinds"},
		"BadFieldPath":    {"apiVersion: v1alpha1\nkinds: [DBInstance]\nignoreFieldPaths: [NoDot]\n", `ignoreFieldPaths[0] "NoDot" must be <Shape>.<Member>`},
		"OverlayNotAMap":  {"apiVersion: v1alpha1\nkinds: [DBInstance]\ngeneratorConfig: [a]\n", "generatorConfig must be a mapping"},
		"UnknownRevField": {"apiVersion: v1alpha1\nkinds: [DBInstance]\nrevisions:\n  DBInstance:\n    comit: abc\n", "field comit not found"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, manifestPath(root, "rds"), tc.content)
			_, err := readManifest(root, "rds")
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "cannot read apis/rds/import.yaml") {
				t.Errorf("readManifest() = %v, want an error containing %q", err, tc.want)
			}
		})
	}

	root := t.TempDir()
	if m, err := readManifest(root, "rds"); err != nil || m == nil {
		t.Errorf("readManifest() of a missing file = %v, %v; want an empty manifest", m, err)
	}
	writeFile(t, manifestPath(root, "rds"), "")
	if _, err := readManifest(root, "rds"); err != nil {
		t.Errorf("readManifest() of an empty file = %v", err)
	}
}

// Reading and writing the committed manifests changes nothing, so a
// re-import gives no diff in them.
func TestManifestRoundTrip(t *testing.T) {
	for _, svc := range []string{"rds", "docdb", "overlay"} {
		t.Run(svc, func(t *testing.T) {
			want := readFile(t, filepath.Join("testdata", "manifest", svc+".yaml"))
			root := t.TempDir()
			writeFile(t, manifestPath(root, svc), want)
			m, err := readManifest(root, svc)
			if err != nil {
				t.Fatal(err)
			}
			if err := writeManifest(root, svc, m); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, manifestPath(root, svc)); got != want {
				t.Errorf("round trip changed the manifest:\n%s\nwant\n%s", got, want)
			}
		})
	}
}

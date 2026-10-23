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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/swisscom/crossplane-provider-aws-v2/codegen/internal/generator"
)

const topicGeneratorConfig = "Generator config"

// Manifest records which kinds of a service were imported. It lives in
// apis/<service>/import.yaml.
type Manifest struct {
	Source string `yaml:"source"`
	// APIVersion is the version of the imported kinds.
	APIVersion string   `yaml:"apiVersion"`
	Kinds      []string `yaml:"kinds"`
	// IgnoreFieldPaths are added to ignore.field_paths of the source
	// generator config, e.g. for fields the AWS API added since the source
	// was generated.
	IgnoreFieldPaths []string `yaml:"ignoreFieldPaths,omitempty"`
	// GeneratorConfig holds repo-local generator settings. It is merged over
	// the source generator config, because generator-config.yaml itself is
	// derived and rewritten by every import.
	GeneratorConfig *rawYAML `yaml:"generatorConfig,omitempty"`
	// GeneratorConfigSHA256 is the sha256 of the generator-config.yaml the
	// importer last wrote, so hand edits can be detected.
	GeneratorConfigSHA256 string `yaml:"generatorConfigSHA256,omitempty"`
	// LastImport is the source revision of the latest import run.
	LastImport *Revision `yaml:"lastImport,omitempty"`
	// Revisions records, per kind, the source revision its controller was
	// ported from.
	Revisions map[string]Revision `yaml:"revisions,omitempty"`
}

// rawYAML keeps a YAML subtree as is. Strict decoding does not look into
// it, because its keys belong to the generator config, not the manifest.
type rawYAML yaml.Node

// UnmarshalYAML keeps the node.
func (r *rawYAML) UnmarshalYAML(n *yaml.Node) error {
	*r = rawYAML(*n)
	return nil
}

// MarshalYAML writes the node back.
func (r *rawYAML) MarshalYAML() (any, error) {
	n := yaml.Node(*r)
	return &n, nil
}

// A Revision identifies the state of the source checkout a kind was
// imported from.
type Revision struct {
	// Repository is the URL of the origin remote, e.g. a fork.
	Repository string `yaml:"repository,omitempty"`
	Commit     string `yaml:"commit"`
	// Dirty is set if the source files of the kind had uncommitted changes.
	Dirty bool `yaml:"dirty,omitempty"`
	// Backfilled is set on revisions that were recorded by hand after the
	// import.
	Backfilled bool `yaml:"backfilled,omitempty"`
}

func manifestPath(root, svc string) string {
	return filepath.Join(root, "apis", svc, "import.yaml")
}

var fieldPathRe = regexp.MustCompile(`^[A-Za-z0-9]+(\.[A-Za-z0-9]+)+$`)

// readManifest reads import.yaml strictly: an unknown key is an error, so a
// typo is not silently erased by the next write.
func readManifest(root, svc string) (*Manifest, error) {
	b, err := os.ReadFile(manifestPath(root, svc)) //nolint:gosec // path built from flags
	if os.IsNotExist(err) {
		return &Manifest{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("cannot read apis/%s/import.yaml: %w", svc, err)
	}
	if len(doc.Content) == 1 {
		if gc := mapValue(doc.Content[0], "generatorConfig"); gc != nil && gc.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("cannot read apis/%s/import.yaml: generatorConfig must be a mapping", svc)
		}
	}
	m := &Manifest{}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(m); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("cannot read apis/%s/import.yaml: %w", svc, err)
	}
	if err := m.validate(); err != nil {
		return nil, fmt.Errorf("cannot read apis/%s/import.yaml: %w", svc, err)
	}
	return m, nil
}

func (m *Manifest) validate() error {
	for i, k := range m.Kinds {
		if !kindRe.MatchString(k) {
			return fmt.Errorf("kinds[%d] %q is not a kind name (^[A-Z][A-Za-z0-9]*$)", i, k)
		}
	}
	if m.APIVersion != "" && !apiVersionRe.MatchString(m.APIVersion) {
		return fmt.Errorf("apiVersion %q must look like v1alpha1", m.APIVersion)
	}
	for _, k := range slices.Sorted(maps.Keys(m.Revisions)) {
		if !slices.Contains(m.Kinds, k) {
			return fmt.Errorf("revisions.%s: kind is not in kinds", k)
		}
	}
	for i, p := range m.IgnoreFieldPaths {
		if !fieldPathRe.MatchString(p) {
			return fmt.Errorf("ignoreFieldPaths[%d] %q must be <Shape>.<Member>", i, p)
		}
	}
	if m.GeneratorConfig != nil && m.GeneratorConfig.Kind != yaml.MappingNode {
		return errors.New("generatorConfig must be a mapping")
	}
	return nil
}

func writeManifest(root, svc string, m *Manifest) error {
	sort.Strings(m.Kinds)
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	hdr := "# Kinds imported from crossplane-contrib/provider-aws.\n# Managed by codegen/cmd/import; generator-config.yaml ignores all other kinds.\n"
	return os.WriteFile(manifestPath(root, svc), append([]byte(hdr), b...), 0o600)
}

// unresolvedFunc returns the ignore.field_paths entries of a candidate
// generator config that match nothing in the API model.
type unresolvedFunc func(cfg []byte) ([]string, error)

// writeGeneratorConfig derives apis/<svc>/generator-config.yaml: the source
// config, with generatorConfig of import.yaml merged over it, every kind not
// in m.Kinds in ignore.resource_names by its API model name (e.g. Vpc for
// kind VPC) and m.IgnoreFieldPaths in ignore.field_paths. Comments and order
// of the source config are kept. It refuses to overwrite a file that was
// edited by hand, and records the digest of what it writes in m.
func writeGeneratorConfig(src, dst string, all []generator.Kind, m *Manifest, unresolved unresolvedFunc, r *Report) error { //nolint:gocyclo // a sequence of checks
	svc := filepath.Base(filepath.Dir(dst))
	rel := "apis/" + svc + "/generator-config.yaml"
	b, err := os.ReadFile(src) //nolint:gosec // path built from flags
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("cannot parse %s: %w", src, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s: expected a YAML mapping", src)
	}
	root := doc.Content[0]
	fromSource := map[string]bool{}
	for _, n := range mapPath(root, "ignore", "field_paths").Content {
		fromSource[n.Value] = true
	}
	if m.GeneratorConfig != nil {
		if m.GeneratorConfig.Kind != yaml.MappingNode {
			return fmt.Errorf("apis/%s/import.yaml: generatorConfig must be a mapping", svc)
		}
		mergeNode(root, (*yaml.Node)(m.GeneratorConfig))
	}
	keepSet := map[string]bool{}
	for _, k := range m.Kinds {
		keepSet[k] = true
	}
	var ignore []string
	for _, k := range all {
		if !keepSet[k.Name] {
			ignore = append(ignore, k.ModelName)
		}
	}
	names := mapPath(root, "ignore", "resource_names")
	names.Kind = yaml.SequenceNode
	names.Tag = "!!seq"
	for i, k := range ignore {
		n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}
		if i == 0 {
			n.HeadComment = "not imported (codegen/cmd/import)"
		}
		names.Content = append(names.Content, n)
	}
	paths := mapPath(root, "ignore", "field_paths")
	paths.Kind = yaml.SequenceNode
	paths.Tag = "!!seq"
	for i, fp := range m.IgnoreFieldPaths {
		n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: fp}
		if i == 0 {
			n.HeadComment = "ignoreFieldPaths of import.yaml"
		}
		paths.Content = append(paths.Content, n)
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}

	if unresolved != nil {
		bad, err := unresolved(out)
		if err != nil {
			return err
		}
		drop := map[string]bool{}
		for _, p := range bad {
			switch {
			case slices.Contains(m.IgnoreFieldPaths, p):
				return fmt.Errorf("apis/%s/import.yaml: ignoreFieldPaths entry %q matches nothing in the %s API model", svc, p, svc)
			case !fromSource[p]:
				return fmt.Errorf("apis/%s/import.yaml: generatorConfig ignore.field_paths entry %q matches nothing in the %s API model", svc, p, svc)
			}
			// An ignore that matches nothing changes no generated code, and
			// the generator rejects it, so a stale upstream entry is dropped.
			drop[p] = true
			r.Add(topicGeneratorConfig, "`ignore.field_paths` entry `%s` from the source config matches nothing in the %s API model; dropped", p, svc)
		}
		if len(drop) > 0 {
			paths.Content = slices.DeleteFunc(paths.Content, func(n *yaml.Node) bool { return drop[n.Value] })
			if out, err = yaml.Marshal(&doc); err != nil {
				return err
			}
		}
	}

	cur, err := os.ReadFile(dst) //nolint:gosec // path built from flags
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return err
	case m.GeneratorConfigSHA256 != "" && digest(cur) != m.GeneratorConfigSHA256:
		return fmt.Errorf("%s was edited after the last import (its sha256 does not match generatorConfigSHA256 in apis/%s/import.yaml) and would be overwritten. Move the edit to generatorConfig in import.yaml (merged over the source config), delete generator-config.yaml and import again", rel, svc)
	case m.GeneratorConfigSHA256 == "" && !bytes.Equal(cur, out):
		return fmt.Errorf("%s differs from what the importer generates and import.yaml records no generatorConfigSHA256, so a hand edit cannot be told from an older import. Move hand edits to generatorConfig in import.yaml, delete generator-config.yaml and import again", rel)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(dst, out, 0o600); err != nil {
		return err
	}
	m.GeneratorConfigSHA256 = digest(out)
	return nil
}

func digest(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// mergeNode merges overlay into base: mappings per key, sequences by
// appending the items base lacks, anything else is replaced.
func mergeNode(base, overlay *yaml.Node) {
	switch {
	case base.Kind == yaml.MappingNode && overlay.Kind == yaml.MappingNode:
		for i := 0; i+1 < len(overlay.Content); i += 2 {
			k, v := overlay.Content[i], overlay.Content[i+1]
			found := false
			for j := 0; j+1 < len(base.Content); j += 2 {
				if base.Content[j].Value == k.Value {
					mergeNode(base.Content[j+1], v)
					found = true
					break
				}
			}
			if !found {
				base.Content = append(base.Content, k, v)
			}
		}
	case base.Kind == yaml.SequenceNode && overlay.Kind == yaml.SequenceNode:
		for _, item := range overlay.Content {
			dup := item.Kind == yaml.ScalarNode && slices.ContainsFunc(base.Content, func(n *yaml.Node) bool {
				return n.Kind == yaml.ScalarNode && n.Value == item.Value
			})
			if !dup {
				base.Content = append(base.Content, item)
			}
		}
	default:
		*base = *overlay
	}
}

// mapValue returns the value of key in mapping m, or nil.
func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// mapPath returns the node at the given keys below m, creating mappings
// and an empty value node as needed.
func mapPath(m *yaml.Node, keys ...string) *yaml.Node {
	for _, k := range keys {
		var next *yaml.Node
		for i := 0; i+1 < len(m.Content); i += 2 {
			if m.Content[i].Value == k {
				next = m.Content[i+1]
				break
			}
		}
		if next == nil {
			next = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, next)
		}
		if next.Kind == yaml.ScalarNode && next.Value == "" {
			next.Kind = yaml.MappingNode
			next.Tag = "!!map"
		}
		m = next
	}
	return m
}

// sourceRevision returns the git revision of the source checkout src.
// Dirty considers paths, the source dirs the import read.
func sourceRevision(src string, paths []string) (Revision, error) {
	git := func(args ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"-C", src}, args...)...) //nolint:gosec // args built from flags
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	top, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return Revision{}, fmt.Errorf("cannot read the git revision of %s: %w", src, err)
	}
	// A copy inside another repository would record that repository's commit.
	abs, err := filepath.Abs(src)
	if err == nil {
		abs, err = filepath.EvalSymlinks(abs)
	}
	if err != nil {
		return Revision{}, err
	}
	if top != abs {
		return Revision{}, fmt.Errorf("%s is not the root of a git checkout (its checkout root is %s); import from the root of a provider-aws clone", src, top)
	}
	commit, err := git("rev-parse", "HEAD")
	if err != nil {
		return Revision{}, fmt.Errorf("cannot read the git revision of %s: %w", src, err)
	}
	repo, _ := git("remote", "get-url", "origin")
	ps := slices.Compact(slices.Sorted(slices.Values(paths)))
	status, err := git(append([]string{"status", "--porcelain", "--"}, ps...)...)
	if err != nil {
		return Revision{}, fmt.Errorf("cannot read the git status of %s: %w", src, err)
	}
	return Revision{Repository: repo, Commit: commit, Dirty: status != ""}, nil
}

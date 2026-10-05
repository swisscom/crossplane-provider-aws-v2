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
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

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
}

func manifestPath(root, svc string) string {
	return filepath.Join(root, "apis", svc, "import.yaml")
}

func readManifest(root, svc string) (*Manifest, error) {
	b, err := os.ReadFile(manifestPath(root, svc)) //nolint:gosec // path built from flags
	if os.IsNotExist(err) {
		return &Manifest{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := &Manifest{}
	return m, yaml.Unmarshal(b, m)
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

// writeGeneratorConfig copies the source generator config and adds every
// kind that is not in keep to ignore.resource_names. Comments and order of
// the source config are preserved.
func writeGeneratorConfig(src, dst string, all []string, m *Manifest) error {
	keep := m.Kinds
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
	keepSet := map[string]bool{}
	for _, k := range keep {
		keepSet[k] = true
	}
	var ignore []string
	for _, k := range all {
		if !keepSet[k] {
			ignore = append(ignore, k)
		}
	}
	names := mapPath(doc.Content[0], "ignore", "resource_names")
	names.Kind = yaml.SequenceNode
	names.Tag = "!!seq"
	for i, k := range ignore {
		n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}
		if i == 0 {
			n.HeadComment = "not imported (codegen/cmd/import)"
		}
		names.Content = append(names.Content, n)
	}
	paths := mapPath(doc.Content[0], "ignore", "field_paths")
	paths.Kind = yaml.SequenceNode
	paths.Tag = "!!seq"
	for i, fp := range m.IgnoreFieldPaths {
		n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: fp}
		if i == 0 {
			n.HeadComment = "ignoreFieldPaths of import.yaml"
		}
		paths.Content = append(paths.Content, n)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, out, 0o600)
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

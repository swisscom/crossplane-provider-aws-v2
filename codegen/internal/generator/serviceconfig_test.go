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
	"os"
	"path/filepath"
	"testing"
)

func TestReadServiceConfigRejectsUnknownKeys(t *testing.T) {
	cases := map[string]string{
		"top level": "clientOperation:\n  DBInstance: [AddTagsToResource]\n",
		"kind":      "kinds:\n  DBInstance:\n    customObserv: true\n",
	}
	for name, yaml := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			p := ServiceConfigPath(root, "docdb")
			if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := ReadServiceConfig(root, "docdb")
			errContains(t, err, "unknown field")
		})
	}
}

func TestRunRejectsUnknownKindInCodegenYAML(t *testing.T) {
	out, err := docdbFixture.run(t, nil, "kinds:\n  DBInstnace: {}\n")

	errContains(t, err, "codegen.yaml: kinds.DBInstnace is not a generated kind")
	if files := goFiles(t, out); len(files) > 0 {
		t.Errorf("Run wrote files although it failed: %v", files)
	}
}

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
	"os"
	"path/filepath"
	"testing"
)

// Tests of ported packages embed files from testdata.
func TestCopyDirKeepsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	for p, c := range map[string]string{"a.json": "src-a", "sub/b.json": "src-b"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(src, p)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, p), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(dst, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "a.json"), []byte("dst-a"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}

	for p, want := range map[string]string{"a.json": "dst-a", "sub/b.json": "src-b"} {
		got, err := os.ReadFile(filepath.Join(dst, p)) //nolint:gosec // test temp dir
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s: got %q, want %q", p, got, want)
		}
	}
}

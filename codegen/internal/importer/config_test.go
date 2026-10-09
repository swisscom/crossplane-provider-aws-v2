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
	"os/exec"
	"path/filepath"
	"testing"
)

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
	got, err := sourceRevision(src, "docdb", "DBCluster")
	if err != nil {
		t.Fatal(err)
	}
	want := Revision{Repository: "https://example.com/provider-aws.git", Commit: commit}
	if got != want {
		t.Errorf("sourceRevision(DBCluster) = %+v, want %+v", got, want)
	}

	got, err = sourceRevision(src, "docdb", "DBInstance")
	if err != nil {
		t.Fatal(err)
	}
	want.Dirty = true
	if got != want {
		t.Errorf("sourceRevision(DBInstance) = %+v, want %+v", got, want)
	}

	if _, err := sourceRevision(t.TempDir(), "docdb", "DBCluster"); err == nil {
		t.Error("sourceRevision() of a directory without git: want error")
	}
}

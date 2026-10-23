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
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/swisscom/crossplane-provider-aws-v2/codegen/internal/modeltest"
)

func TestRunRejectsUnknownIgnoreFieldPaths(t *testing.T) {
	out, err := docdbFixture.run(t, addIgnores(
		"DescribeDBInstancesInput.DBInstanceIndentifier",
		"NoSuchShape.Field",
		"CreateDBInstanceOutput.DBInstance.NoSuchMember",
		// ACK removes members of structures only, so this would be a no-op.
		"DBInstance.VpcSecurityGroups.Status",
	), "")

	errContains(t, err,
		`ignore.field_paths entry "DescribeDBInstancesInput.DBInstanceIndentifier" matches nothing in the docdb API model (shape DescribeDBInstancesInput has no member DBInstanceIndentifier); fix or remove it`,
		`"NoSuchShape.Field" matches nothing in the docdb API model (unknown shape NoSuchShape)`,
		`"CreateDBInstanceOutput.DBInstance.NoSuchMember" matches nothing in the docdb API model (shape DBInstance has no member NoSuchMember)`,
		`"DBInstance.VpcSecurityGroups.Status" matches nothing in the docdb API model (member VpcSecurityGroups of shape DBInstance is a list, not a structure)`,
	)
	if files := goFiles(t, out); len(files) > 0 {
		t.Errorf("Run wrote files although it failed: %v", files)
	}
}

func TestUnresolvedIgnoreFieldPaths(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "generator-config.yaml")
	base, err := os.ReadFile(filepath.Join("..", "..", "testdata", "docdb", "generator-config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	edited := addIgnores("ZZZ.Unknown", "DescribeDBInstancesInput.DBInstanceIndentifier", "DBInstance.Endpoint.NoSuchMember")(string(base))
	if err := os.WriteFile(cfg, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := UnresolvedIgnoreFieldPaths(context.Background(), docdbFixture.options(t, t.TempDir(), cfg))
	if err != nil {
		modeltest.SkipOnFetchError(t, err)
		t.Fatal(err)
	}

	want := []string{"DBInstance.Endpoint.NoSuchMember", "DescribeDBInstancesInput.DBInstanceIndentifier", "ZZZ.Unknown"}
	if !slices.Equal(got, want) {
		t.Errorf("UnresolvedIgnoreFieldPaths() = %q, want %q", got, want)
	}
}

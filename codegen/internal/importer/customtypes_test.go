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
	"maps"
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

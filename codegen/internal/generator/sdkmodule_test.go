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
	"slices"
	"testing"
)

func TestUnexportedInputFields(t *testing.T) {
	dir := t.TempDir()
	src := `package docdb

type noSmithyDocumentSerde struct{}

type CreateDBClusterInput struct {
	DBClusterIdentifier *string
	destinationRegion   *string

	noSmithyDocumentSerde
}

type CreateDBClusterOutput struct {
	resultMetadata *string
}
`
	if err := os.WriteFile(filepath.Join(dir, "api_op_CreateDBCluster.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := unexportedInputFields(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"CreateDBClusterInput.DestinationRegion"}
	if !slices.Equal(got, want) {
		t.Errorf("unexportedInputFields() = %v, want %v", got, want)
	}
}

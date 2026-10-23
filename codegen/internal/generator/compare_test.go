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
	"regexp"
	"slices"
	"testing"

	"github.com/swisscom/crossplane-provider-aws-v2/codegen/internal/modeltest"
)

// TestDiffUpdatableParametersDocDB generates docdb with the repository's
// generator-config.yaml and checks which fields DiffUpdatableParameters
// compares: fields the update operation sends and the read operation
// returns, but no create-only fields.
func TestDiffUpdatableParametersDocDB(t *testing.T) {
	out := t.TempDir()
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = t.TempDir()
	}
	repo := filepath.Join("..", "..", "..")
	o := Options{
		Service:         "docdb",
		OutputDir:       repo,
		GeneratorConfig: filepath.Join(repo, "apis", "docdb", "generator-config.yaml"),
		APIVersion:      "v1alpha1",
		CacheDir:        filepath.Join(cacheDir, "provider-aws-v2-codegen"),
		TemplateDir:     filepath.Join("..", "..", "templates"),
	}
	// The SDK version and model name come from the provider's go.mod.
	if err := ResolveServiceModule(&o); err != nil {
		modeltest.Unavailable(t, "aws-sdk-go-v2/service/docdb not available: %v", err)
	}
	o.OutputDir = out
	if err := Run(context.Background(), o); err != nil {
		modeltest.SkipOnFetchError(t, err)
		t.Fatal(err)
	}

	cases := map[string]struct {
		want    []string
		notWant []string
	}{
		"dbcluster": {
			want:    []string{"backupRetentionPeriod", "deletionProtection", "engineVersion", "port", "preferredBackupWindow", "serverlessV2ScalingConfiguration"},
			notWant: []string{"availabilityZones", "dbSubnetGroupName", "kmsKeyID", "masterUsername", "storageEncrypted"},
		},
		"dbinstance": {
			want:    []string{"dbInstanceClass", "promotionTier", "preferredMaintenanceWindow"},
			notWant: []string{"availabilityZone", "engine"},
		},
	}
	re := regexp.MustCompile(`diff = append\(diff, "([^"]+)"\)`)
	for kind, tc := range cases {
		t.Run(kind, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join(out, "internal", "controller", "docdb", kind, "zz_conversions.go"))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, m := range re.FindAllStringSubmatch(string(b), -1) {
				got = append(got, m[1])
			}
			for _, f := range tc.want {
				if !slices.Contains(got, f) {
					t.Errorf("DiffUpdatableParameters does not compare %q; compares %v", f, got)
				}
			}
			for _, f := range tc.notWant {
				if slices.Contains(got, f) {
					t.Errorf("DiffUpdatableParameters compares create-only field %q", f)
				}
			}
		})
	}
}

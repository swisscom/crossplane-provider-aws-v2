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
	"strings"
	"testing"
)

// TestDiffUpdatableParameters generates services with the repository's
// generator-config.yaml and checks which fields DiffUpdatableParameters
// compares: fields the update operation sends and the read operation
// returns, but no create-only fields.
func TestDiffUpdatableParameters(t *testing.T) {
	type kind struct {
		want    []string
		notWant []string
	}
	cases := map[string]map[string]kind{
		"docdb": {
			"dbcluster": {
				want:    []string{"backupRetentionPeriod", "deletionProtection", "engineVersion", "port", "preferredBackupWindow", "serverlessV2ScalingConfiguration"},
				notWant: []string{"availabilityZones", "dbSubnetGroupName", "kmsKeyID", "masterUsername", "storageEncrypted"},
			},
			"dbinstance": {
				want:    []string{"dbInstanceClass", "promotionTier", "preferredMaintenanceWindow"},
				notWant: []string{"availabilityZone", "engine"},
			},
		},
		// PutResourcePolicy is the create and the update operation.
		"cloudwatchlogs": {
			"resourcepolicy": {want: []string{"policyDocument"}},
		},
		// ReadOne whose output wraps the resource in its only member
		// (DescribeDomainOutput.DomainStatus).
		"opensearch": {
			"domain": {
				want:    []string{"accessPolicies", "clusterConfig", "ebsOptions", "encryptionAtRestOptions", "vpcOptions"},
				notWant: []string{"engineVersion", "tags"},
			},
		},
	}
	re := regexp.MustCompile(`diff = append\(diff, "([^"]+)"\)`)
	for svc, kinds := range cases {
		t.Run(svc, func(t *testing.T) {
			out := generate(t, svc)
			for k, tc := range kinds {
				t.Run(k, func(t *testing.T) {
					b, err := os.ReadFile(filepath.Join(out, "internal", "controller", svc, k, "zz_conversions.go"))
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
		})
	}
}

// generate generates svc into a temporary directory and returns it. It skips
// the test if the SDK module or the AWS API model isn't available.
func generate(t *testing.T, svc string) string {
	t.Helper()
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = t.TempDir()
	}
	repo := filepath.Join("..", "..", "..")
	o := Options{
		Service:         svc,
		OutputDir:       repo,
		GeneratorConfig: filepath.Join(repo, "apis", svc, "generator-config.yaml"),
		APIVersion:      "v1alpha1",
		CacheDir:        filepath.Join(cacheDir, "provider-aws-v2-codegen"),
		TemplateDir:     filepath.Join("..", "..", "templates"),
	}
	// The SDK version and model name come from the provider's go.mod.
	if err := ResolveServiceModule(&o); err != nil {
		t.Skipf("aws-sdk-go-v2/service/%s not available: %v", svc, err)
	}
	o.OutputDir = t.TempDir()
	if err := Run(context.Background(), o); err != nil {
		if strings.Contains(err.Error(), "fetch") {
			t.Skipf("AWS API model not available: %v", err)
		}
		t.Fatal(err)
	}
	return o.OutputDir
}

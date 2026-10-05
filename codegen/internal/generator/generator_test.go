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
	"strings"
	"testing"
)

// TestRunServiceCatalog generates the servicecatalog fixture and checks the
// shape of the output. The AWS API model is fetched once and cached.
func TestRunServiceCatalog(t *testing.T) {
	out := t.TempDir()
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = t.TempDir()
	}
	o := Options{
		Service:           "servicecatalog",
		OutputDir:         out,
		GeneratorConfig:   filepath.Join("..", "..", "testdata", "servicecatalog", "generator-config.yaml"),
		APIVersion:        "v1alpha1",
		ServiceSDKVersion: "v1.47.1",
		ModelName:         "service-catalog",
		CacheDir:          filepath.Join(cacheDir, "provider-aws-v2-codegen"),
		TemplateDir:       filepath.Join("..", "..", "templates"),
	}
	if err := Run(context.Background(), o); err != nil {
		if strings.Contains(err.Error(), "fetch") {
			t.Skipf("AWS API model not available: %v", err)
		}
		t.Fatal(err)
	}

	want := map[string][]string{
		"apis/servicecatalog/v1alpha1/zz_provisioned_product.go": {
			"type ProvisionedProduct struct",
			"xpv2.ManagedResourceSpec",
			"+kubebuilder:resource:scope=Namespaced",
			"Region string",
		},
		"apis/servicecatalog/v1alpha1/custom_types.go": {"type CustomProvisionedProductParameters struct{}"},
		"internal/controller/servicecatalog/provisionedproduct/zz_controller.go": {
			"func SetupGated(",
			"DescribeProvisionedProduct(context.Context, *svcsdk.DescribeProvisionedProductInput, ...func(*svcsdk.Options))",
			"svcsdk.NewFromConfig(cfg)",
			"configure func(ctrl.Manager, controller.Options) ([]option, []managed.ReconcilerOption, error)",
		},
		"internal/controller/servicecatalog/provisionedproduct/zz_conversions.go": {
			"func GenerateDescribeProvisionedProductInput(",
			"func GenerateProvisionProductInput(",
			"func GenerateUpdateProvisionedProductInput(",
			"func GenerateTerminateProvisionedProductInput(",
			`ae.ErrorCode() == "ResourceNotFoundException"`,
		},
		"internal/controller/servicecatalog/zz_setup.go": {"provisionedproduct.SetupGated"},
		"internal/controller/zz_services.go":             {"servicecatalog.SetupGated"},
		"apis/zz_services.go":                            {"servicecatalogv1alpha1.SchemeBuilder.AddToScheme"},
	}
	for path, snippets := range want {
		b, err := os.ReadFile(filepath.Join(out, path))
		if err != nil {
			t.Errorf("missing %s: %v", path, err)
			continue
		}
		for _, s := range snippets {
			if !strings.Contains(string(b), s) {
				t.Errorf("%s does not contain %q", path, s)
			}
		}
		if strings.Contains(string(b), "github.com/aws/aws-sdk-go/") {
			t.Errorf("%s imports AWS SDK v1", path)
		}
	}
}

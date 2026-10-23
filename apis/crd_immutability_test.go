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

package apis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/yaml"
)

// The API server's CEL limits (k8s.io/apiserver/pkg/apis/cel).
const (
	perCallLimit = 1000000
	costBudget   = 10000000
)

// TestCRDImmutability evaluates the generated CEL transition rules of the
// generated service CRDs like the API server does on create and update.
func TestCRDImmutability(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "package", "crds", "*.aws2.crossplane.io_*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var tested int
	for _, f := range files {
		tested++
		t.Run(filepath.Base(f), func(t *testing.T) {
			v := validator(t, f)
			obj := func(region string) map[string]any {
				return map[string]any{"spec": map[string]any{"forProvider": map[string]any{"region": region}}}
			}

			cases := map[string]struct {
				obj, old map[string]any
				wantErr  string
			}{
				"create":         {obj: obj("eu-central-1")},
				"same region":    {obj: obj("eu-central-1"), old: obj("eu-central-1")},
				"region changed": {obj: obj("eu-west-1"), old: obj("eu-central-1"), wantErr: "region is immutable"},
			}
			for name, tc := range cases {
				t.Run(name, func(t *testing.T) {
					var old any
					if tc.old != nil {
						old = tc.old
					}
					errs, remaining := v.Validate(context.Background(), field.NewPath("root"), nil, tc.obj, old, costBudget)
					if remaining < 0 {
						t.Errorf("CEL cost budget exceeded: %d", remaining)
					}
					switch {
					case tc.wantErr == "" && len(errs) > 0:
						t.Errorf("unexpected errors: %v", errs)
					case tc.wantErr != "" && !strings.Contains(errs.ToAggregate().Error(), tc.wantErr):
						t.Errorf("errors %v do not contain %q", errs, tc.wantErr)
					}
				})
			}
		})
	}
	// rds 1, docdb 4, ec2 3: a new kind must show up here.
	if tested != 8 {
		t.Errorf("tested %d service CRDs, want 8", tested)
	}
}

func validator(t *testing.T, path string) *cel.Validator {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatal(err)
	}
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(b, crd); err != nil {
		t.Fatal(err)
	}
	if len(crd.Spec.Versions) != 1 {
		t.Fatalf("want one version, got %d", len(crd.Spec.Versions))
	}
	internal := &apiextensions.JSONSchemaProps{}
	if err := apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(crd.Spec.Versions[0].Schema.OpenAPIV3Schema, internal, nil); err != nil {
		t.Fatal(err)
	}
	s, err := schema.NewStructural(internal)
	if err != nil {
		t.Fatal(err)
	}
	v := cel.NewValidator(s, true, perCallLimit)
	if v == nil {
		t.Fatal("CRD has no CEL rules")
	}
	return v
}

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

package v1alpha1

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	pcCRD  = "aws2.crossplane.io_providerconfigs.yaml"
	cpcCRD = "aws2.crossplane.io_clusterproviderconfigs.yaml"
)

// crd holds what the API server checks on spec at admission. Its validators
// need cel-go, which this module doesn't depend on, so the tests check the
// rule text and run the URL pattern with Go's regexp (the pattern uses no
// syntax where RE2 and ECMA-262 differ).
type crd struct {
	rules     []string
	urlRegexp *regexp.Regexp
	urlMaxLen int64
}

func loadCRD(t *testing.T, file string) crd {
	t.Helper()
	f, err := os.Open("../../package/crds/" + file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // read-only file
	c := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.NewYAMLOrJSONDecoder(f, 4096).Decode(c); err != nil {
		t.Fatal(err)
	}
	spec := c.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"]
	out := crd{}
	for _, r := range spec.XValidations {
		out.rules = append(out.rules, r.Rule)
	}
	u := spec.Properties["endpoint"].Properties["url"]
	if u.Pattern == "" || u.MaxLength == nil {
		t.Fatalf("%s: endpoint.url has no pattern or maxLength", file)
	}
	out.urlRegexp = regexp.MustCompile(u.Pattern)
	out.urlMaxLen = *u.MaxLength
	return out
}

func (c crd) validURL(u string) bool {
	return int64(len(u)) <= c.urlMaxLen && c.urlRegexp.MatchString(u)
}

// Both kinds share the same rules: a namespaced ProviderConfig may use every
// field a ClusterProviderConfig can.
var specRules = []string{
	"self.credentials.source != 'WebIdentity' || has(self.assumeRoleWithWebIdentity)",
	"self.credentials.source != 'Secret' || has(self.credentials.secretRef)",
}

func TestCRDRules(t *testing.T) {
	for _, file := range []string{pcCRD, cpcCRD} {
		t.Run(file, func(t *testing.T) {
			got := slices.Sorted(slices.Values(loadCRD(t, file).rules))
			w := slices.Sorted(slices.Values(specRules))
			if !slices.Equal(got, w) {
				t.Errorf("spec rules = %q, want %q", got, w)
			}
		})
	}
}

func TestCRDEndpointURL(t *testing.T) {
	cases := map[string]struct {
		url   string
		valid bool
	}{
		"LocalStack":        {url: "http://localstack:4566", valid: true},
		"VPCEndpoint":       {url: "https://vpce-1.rds.eu-central-1.vpce.amazonaws.com", valid: true},
		"IPv6":              {url: "http://[::1]:4566", valid: true},
		"WithPath":          {url: "https://example.com/aws/", valid: true},
		"MaxLength":         {url: "https://x/" + strings.Repeat("a", 2038), valid: true},
		"OtherScheme":       {url: "ftp://x"},
		"UserInfo":          {url: "http://user:pw@x"},
		"Query":             {url: "http://x?a=b"},
		"Fragment":          {url: "http://x#f"},
		"EmptyHost":         {url: "http://"},
		"Empty":             {url: ""},
		"NotAURL":           {url: "not a url"},
		"EmptyHostWithPort": {url: "http://:4566"},
		"DotHost":           {url: "http://."},
		"PercentHost":       {url: "https://%00"},
		"PercentInHost":     {url: "http://a%00"},
		"NonNumericPort":    {url: "http://x:port"},
		"Underscore":        {url: "http://localstack_main:4566", valid: true},
		"ServiceDNS":        {url: "http://localstack.localstack.svc:4566", valid: true},
		"Whitespace":        {url: "http://x y"},
		"TooLong":           {url: "https://x/" + strings.Repeat("a", 2039)},
	}
	for _, file := range []string{pcCRD, cpcCRD} {
		c := loadCRD(t, file)
		for name, tc := range cases {
			t.Run(file+"/"+name, func(t *testing.T) {
				if got := c.validURL(tc.url); got != tc.valid {
					t.Errorf("validURL(%q) = %v, want %v", tc.url, got, tc.valid)
				}
			})
		}
	}
}

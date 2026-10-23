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
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// A helper import path must not lead the importer outside the source and
// target trees.
func TestMapImportRejectsUncleanHelperPaths(t *testing.T) {
	for _, p := range []string{
		SourceModule + "/pkg/../../../../tmp/evil",
		SourceModule + "/pkg/./x",
		SourceModule + "/pkg//x",
		SourceModule + "/pkg/x/..",
		SourceModule + "/pkg/",
	} {
		t.Run(p, func(t *testing.T) {
			rw := newRewriter(testModule, func(string) bool { return false })
			dst, why := rw.mapImport(p)
			if dst != "" || !strings.Contains(why, "import path is not clean") {
				t.Errorf("mapImport() = %q, %q; want no target and a reason", dst, why)
			}
			if len(rw.helperPkgs) != 0 {
				t.Errorf("helperPkgs = %v, want none", rw.helperPkgs)
			}
		})
	}
	rw := newRewriter(testModule, func(string) bool { return false })
	if dst, why := rw.mapImport(SourceModule + "/pkg/controller/rds/utils"); dst != testModule+"/internal/controller/rds/utils" || why != "" || !rw.helperPkgs["pkg/controller/rds/utils"] {
		t.Errorf("mapImport() of a clean helper = %q, %q, %v", dst, why, rw.helperPkgs)
	}
}

func rewriteSrc(t *testing.T, rw *rewriter, src string) string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	rw.rewriteFile(fset, f, fileContext{pkgPath: testModule + "/internal/x", ported: true})
	b, err := formatFile(fset, f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Only AWS SDK operations lose their WithContext suffix.
func TestRewriteWithContextOnlyForSDKOperations(t *testing.T) {
	rw := newRewriter(testModule, func(string) bool { return false })
	rw.sdk["rds"] = &sdkNames{client: map[string]bool{"DescribeDBInstancesInput": true}, types: map[string]bool{}}
	out := rewriteSrc(t, rw, `package x

func f(c *cache, client rdsClient, m *mockClient) {
	c.GetWithContext(ctx, "k")
	client.DescribeDBInstancesWithContext(ctx, in)
	m.MockDescribeDBInstancesWithContext = nil
	m.MockGetWithContext = nil
}
`)
	for _, want := range []string{"c.GetWithContext(ctx, \"k\")", "client.DescribeDBInstances(ctx, in)", "m.MockDescribeDBInstances = nil", "m.MockGetWithContext = nil"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
}

// Imports whose package name differs from the last path element are kept.
func TestRewriteKeepsImportsByPackageName(t *testing.T) {
	rw := newRewriter(testModule, func(string) bool { return false })
	out := rewriteSrc(t, rw, `package x

import (
	"errors"

	"github.com/aws/smithy-go"
	"gopkg.in/yaml.v3"
)

func IsNotFound(err error) bool {
	var ae smithy.APIError
	_ = yaml.Marshal
	return errors.As(err, &ae) && ae.ErrorCode() == "DBInstanceNotFound"
}
`)
	for _, want := range []string{`"github.com/aws/smithy-go"`, `"gopkg.in/yaml.v3"`} {
		if !strings.Contains(out, want) {
			t.Errorf("import %s dropped:\n%s", want, out)
		}
	}
	if strings.Contains(out, "smithy \"github.com") {
		t.Errorf("needless alias:\n%s", out)
	}
}

func TestAssumedName(t *testing.T) {
	for p, want := range map[string]string{
		"gopkg.in/yaml.v3":                         "yaml",
		"github.com/aws/smithy-go":                 "smithy",
		"github.com/google/go-cmp/cmp":             "cmp",
		"github.com/google/go-cmp":                 "cmp",
		"example.com/foo/v2":                       "foo",
		"github.com/aws/aws-sdk-go-v2/service/rds": "rds",
		"k8s.io/api/core/v1":                       "core",
		"github.com/x/foo-bar":                     "foo",
	} {
		if got := assumedName(p); got != want {
			t.Errorf("assumedName(%q) = %q, want %q", p, got, want)
		}
	}
}

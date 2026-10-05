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
	"go/ast"
	"go/token"
	"path"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

const (
	xpv1Path        = "github.com/crossplane/crossplane-runtime/apis/common/v1"
	xpv2Path        = "github.com/crossplane/crossplane/apis/v2/core/v2"
	runtimeV1Prefix = "github.com/crossplane/crossplane-runtime/pkg/"
	runtimeV2Prefix = "github.com/crossplane/crossplane-runtime/v2/pkg/"
	pkgErrorsPath   = "github.com/pkg/errors"
	sdkV1Prefix     = "github.com/aws/aws-sdk-go/"
	sdkV2Prefix     = "github.com/aws/aws-sdk-go-v2/"
)

// symbol is a package-level identifier.
type symbol struct{ path, name string }

// A rewriter maps imports and package-level symbols of provider-aws code to
// provider-aws-v2.
type rewriter struct {
	dstModule string
	// apiAvailable reports whether an apis/<svc>/<ver> package exists in
	// the target.
	apiAvailable func(svcVer string) bool
	// helperPkgs collects source helper packages (relative to the source
	// module) that the rewritten code imports.
	helperPkgs map[string]bool
	// sdk are the exported names of AWS SDK v2 service packages, keyed by
	// service.
	sdk map[string]*sdkNames
	// symbols maps individual source symbols to target symbols.
	symbols map[symbol]symbol
}

func newRewriter(dstModule string, apiAvailable func(string) bool) *rewriter {
	rw := &rewriter{
		dstModule:    dstModule,
		apiAvailable: apiAvailable,
		helperPkgs:   map[string]bool{},
		sdk:          map[string]*sdkNames{},
		symbols: map[symbol]symbol{
			{xpv1Path, "Reference"}:                                      {xpv2Path, "NamespacedReference"},
			{xpv1Path, "Selector"}:                                       {xpv2Path, "NamespacedSelector"},
			{xpv1Path, "SecretKeySelector"}:                              {xpv2Path, "LocalSecretKeySelector"},
			{xpv1Path, "SecretReference"}:                                {xpv2Path, "LocalSecretReference"},
			{SourceModule + "/pkg/utils/errors", "Wrap"}:                 {dstModule + "/internal/clients/aws", "Wrap"},
			{SourceModule + "/pkg/utils/errors", "Wrapf"}:                {dstModule + "/internal/clients/aws", "Wrapf"},
			{sdkV1Prefix + "aws", "StringValue"}:                         {sdkV2Prefix + "aws", "ToString"},
			{sdkV1Prefix + "aws", "Int64Value"}:                          {sdkV2Prefix + "aws", "ToInt64"},
			{sdkV1Prefix + "aws", "BoolValue"}:                           {sdkV2Prefix + "aws", "ToBool"},
			{sdkV1Prefix + "aws", "TimeValue"}:                           {sdkV2Prefix + "aws", "ToTime"},
			{sdkV1Prefix + "aws", "StringValueSlice"}:                    {sdkV2Prefix + "aws", "ToStringSlice"},
			{sdkV1Prefix + "aws", "Float64Value"}:                        {sdkV2Prefix + "aws", "ToFloat64"},
			{sdkV1Prefix + "aws", "Int64ValueSlice"}:                     {sdkV2Prefix + "aws", "ToInt64Slice"},
			{sdkV1Prefix + "aws", "StringSlice"}:                         {sdkV2Prefix + "aws", "StringSlice"},
			{SourceModule + "/pkg/utils/connect/aws", "GetConfig"}:       {dstModule + "/internal/clients/aws", "GetConfig"},
			{SourceModule + "/pkg/utils/connect/aws", "NewUsageTracker"}: {dstModule + "/internal/clients/aws", "NewUsageTracker"},
			{runtimeV1Prefix + "reference", "NewAPIResolver"}:            {runtimeV2Prefix + "reference", "NewAPINamespacedResolver"},
			{runtimeV1Prefix + "reference", "APIResolver"}:               {runtimeV2Prefix + "reference", "APINamespacedResolver"},
			{runtimeV1Prefix + "reference", "ResolutionRequest"}:         {runtimeV2Prefix + "reference", "NamespacedResolutionRequest"},
			{runtimeV1Prefix + "reference", "ResolutionResponse"}:        {runtimeV2Prefix + "reference", "NamespacedResolutionResponse"},
			{runtimeV1Prefix + "reference", "MultiResolutionRequest"}:    {runtimeV2Prefix + "reference", "MultiNamespacedResolutionRequest"},
			{runtimeV1Prefix + "reference", "MultiResolutionResponse"}:   {runtimeV2Prefix + "reference", "MultiNamespacedResolutionResponse"},
		},
	}

	// crossplane-runtime v2 dropped the well-known connection secret keys.
	for _, k := range []string{"Endpoint", "Port", "User", "Password", "CA", "ClientCert", "ClientKey", "Token", "Kubeconfig"} {
		n := "ResourceCredentialsSecret" + k + "Key"
		rw.symbols[symbol{xpv1Path, n}] = symbol{dstModule + "/internal/clients/aws", n}
	}
	return rw
}

// unavailable marks source packages that have no counterpart in the target.
var unavailable = map[string]string{
	SourceModule + "/apis/v1alpha1":         "provider-aws ProviderConfig/StoreConfig types; use apis/v1alpha1 of provider-aws-v2",
	SourceModule + "/pkg/features":          "provider-aws feature flags; use crossplane-runtime/v2/pkg/feature",
	SourceModule + "/pkg/utils/connect/aws": "provider-aws AWS session setup; generated connectors use internal/clients/aws.GetConfig",
	runtimeV1Prefix + "connection":          "external secret stores were removed in crossplane-runtime v2",
	sdkV1Prefix + "aws/awserr":              "SDK v1 errors; use errors.As with smithy.APIError",
	sdkV1Prefix + "aws/request":             "SDK v1 request options; use func(*<svc>.Options)",
	sdkV1Prefix + "aws/session":             "SDK v1 sessions; use aws.Config",
}

// mapImport returns the target import path of a source import path, or
// "" and a reason if there is none.
func (rw *rewriter) mapImport(p string) (string, string) {
	if why, ok := unavailable[p]; ok {
		return "", why
	}
	switch {
	case p == xpv1Path:
		return xpv2Path, ""
	case p == pkgErrorsPath:
		return runtimeV2Prefix + "errors", ""
	case strings.HasPrefix(p, runtimeV1Prefix):
		return runtimeV2Prefix + strings.TrimPrefix(p, runtimeV1Prefix), ""
	case p == sdkV1Prefix+"aws":
		return sdkV2Prefix + "aws", ""
	case strings.HasPrefix(p, sdkV1Prefix+"service/"):
		rest := strings.TrimPrefix(p, sdkV1Prefix+"service/")
		if strings.Contains(rest, "/") { // e.g. rds/rdsiface
			return "", "SDK v1 client interface; use the generated Client interface"
		}
		return sdkV2Prefix + "service/" + rest, ""
	case strings.HasPrefix(p, sdkV1Prefix):
		return "", "AWS SDK v1 package"
	case strings.HasPrefix(p, SourceModule+"/apis/"):
		rel := strings.TrimPrefix(p, SourceModule+"/apis/")
		if rw.apiAvailable(rel) {
			return rw.dstModule + "/apis/" + rel, ""
		}
		return "", "API group not imported yet"
	case strings.HasPrefix(p, SourceModule+"/pkg/"):
		rel := strings.TrimPrefix(p, SourceModule+"/")
		rw.helperPkgs[rel] = true
		return rw.dstModule + "/" + helperDst(rel), ""
	}
	return p, ""
}

// helperDst maps a source helper package to its target directory:
// pkg/x/y -> internal/x/y.
func helperDst(rel string) string {
	return "internal/" + strings.TrimPrefix(rel, "pkg/")
}

// rewriteFile rewrites imports and qualified identifiers of f in place and
// returns findings for the report.
func (rw *rewriter) rewriteFile(fset *token.FileSet, f *ast.File, ctx fileContext) []string { //nolint:gocyclo // one switch per rule
	var findings []string
	imports := map[string]string{} // local name -> source path
	for _, is := range f.Imports {
		p, _ := strconv.Unquote(is.Path.Value)
		name := path.Base(p)
		if is.Name != nil {
			name = is.Name.Name
		}
		imports[name] = p
	}
	needed := map[string]string{} // target path -> local name
	reported := map[string]bool{}

	astutil.Apply(f, func(c *astutil.Cursor) bool {
		switch n := c.Node().(type) {
		case *ast.ImportSpec:
			return false
		case *ast.SelectorExpr:
			x, ok := n.X.(*ast.Ident)
			if !ok || x.Obj != nil {
				// method or field selector, e.g. client.FooWithContext
				if strings.HasSuffix(n.Sel.Name, "WithContext") && ctx.code() {
					n.Sel.Name = strings.TrimSuffix(n.Sel.Name, "WithContext")
				}
				return true
			}
			src, ok := imports[x.Name]
			if !ok {
				if strings.HasSuffix(n.Sel.Name, "WithContext") && ctx.code() {
					n.Sel.Name = strings.TrimSuffix(n.Sel.Name, "WithContext")
				}
				return true
			}
			dst, name := rw.mapSymbol(src, n.Sel.Name, ctx)
			if dst == "" {
				if !reported[src] {
					_, why := rw.mapImport(src)
					findings = append(findings, "uses "+src+": "+why)
					reported[src] = true
				}
				return true
			}
			if dst == "." { // same package as the file
				c.Replace(ast.NewIdent(name))
				return false
			}
			local, ok := needed[dst]
			if !ok {
				local = localName(dst, x.Name)
				needed[dst] = local
			}
			x.Name = local
			n.Sel.Name = name
			return false
		}
		return true
	}, nil)

	// Rewrite import declarations: drop all, add needed ones.
	decls := f.Decls[:0]
	for _, d := range f.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
			continue
		}
		decls = append(decls, d)
	}
	f.Decls, f.Imports = decls, nil
	for _, is := range ctx.keepImports(imports) {
		needed[is.path] = is.name
	}
	for p, name := range needed {
		// Packages of the target module may be named differently than their
		// directory, e.g. internal/clients/rds is package dbinstance.
		if name == path.Base(p) && !strings.HasPrefix(p, rw.dstModule+"/") {
			name = ""
		}
		astutil.AddNamedImport(fset, f, name, p)
	}
	return findings
}

type namedImport struct{ path, name string }

// fileContext carries information about the file being rewritten.
type fileContext struct {
	// pkgPath is the target import path of the file's package.
	pkgPath string
	// controller is true for files of a generated controller package.
	controller bool
	// ported is true for controller and helper code, false for API types.
	ported bool
}

func (c fileContext) code() bool { return c.controller || c.ported }

// keepImports returns imports that are used without a selector, e.g. blank
// and dot imports.
func (fileContext) keepImports(imports map[string]string) []namedImport {
	var out []namedImport
	for name, p := range imports {
		if name == "_" || name == "." {
			out = append(out, namedImport{p, name})
		}
	}
	return out
}

// mapSymbol maps a qualified source symbol. It returns "." as path if the
// symbol lives in the package of the file, and "" if it has no target.
func (rw *rewriter) mapSymbol(src, name string, ctx fileContext) (string, string) {
	if s, ok := rw.symbols[symbol{src, name}]; ok {
		if s.path == ctx.pkgPath {
			return ".", s.name
		}
		return s.path, s.name
	}
	// SDK v1 client interface -> generated Client interface.
	if strings.HasPrefix(src, sdkV1Prefix+"service/") && strings.HasSuffix(src, "iface") {
		if ctx.controller && strings.HasSuffix(name, "API") {
			return ".", "Client"
		}
		return "", ""
	}
	dst, _ := rw.mapImport(src)
	if dst == "" {
		return "", ""
	}
	if dst == ctx.pkgPath {
		return ".", name
	}
	// SDK v1 service package: shapes that are not operation inputs/outputs
	// moved to the types package in v2.
	if strings.HasPrefix(dst, sdkV2Prefix+"service/") {
		svc := path.Base(dst)
		if n := rw.sdk[svc]; n != nil && !n.client[name] && n.types[name] {
			return dst + "/types", name
		}
	}
	return dst, name
}

// localName returns the import name to use for a target path.
func localName(dst, srcLocal string) string {
	switch {
	case dst == xpv2Path:
		return "xpv2"
	case strings.HasPrefix(dst, sdkV2Prefix+"service/") && strings.HasSuffix(dst, "/types"):
		return srcLocal + "types"
	case strings.HasSuffix(dst, "/internal/clients/aws"):
		return "awsclient"
	}
	return srcLocal
}

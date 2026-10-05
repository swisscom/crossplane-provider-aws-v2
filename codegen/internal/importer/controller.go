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
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

const topicController = "Controller"

// standardReconcilerOptions are set by the generated Setup function.
var standardReconcilerOptions = map[string]bool{
	"WithCriticalAnnotationUpdater": true,
	"WithPollInterval":              true,
	"WithLogger":                    true,
	"WithRecorder":                  true,
	"WithConnectionPublishers":      true,
	"WithManagementPolicies":        true,
}

// droppedFeatures guard options that the generated Setup function handles
// or that crossplane-runtime v2 removed.
var droppedFeatures = []string{"EnableAlphaExternalSecretStores", "EnableAlphaManagementPolicies"}

// convertSetup converts the Setup<Kind> function of a provider-aws
// controller into a configure<Kind> function that returns the external
// client options and the non-standard reconciler options, and registers it
// in an init function. It reports whether f had a Setup function.
func convertSetup(f *ast.File, kind string, r *Report) bool { //nolint:gocyclo // a sequence of AST checks
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == "Setup"+kind {
			fn = fd
		}
	}
	if fn == nil {
		return false
	}
	var extOpts ast.Expr = ast.NewIdent("nil")
	var recOpts ast.Expr = ast.NewIdent("nil")
	var body []ast.Stmt
	for _, s := range fn.Body.List {
		switch s := s.(type) {
		case *ast.AssignStmt:
			if lhs := assignedName(s); lhs == "cps" {
				continue
			}
			if lit, ok := s.Rhs[0].(*ast.CompositeLit); ok && isReconcilerOptionSlice(lit.Type) {
				lit.Elts = filterReconcilerOptions(lit.Elts, &extOpts)
				recOpts = s.Lhs[0]
			}
			if call, ok := s.Rhs[0].(*ast.CallExpr); ok && isSel(call.Fun, "managed", "NewReconciler") {
				recOpts = reconcilerArgs(call, &extOpts)
				continue
			}
		case *ast.IfStmt:
			if mentionsAny(s.Cond, droppedFeatures) {
				continue
			}
		case *ast.ReturnStmt:
			if len(s.Results) == 1 && containsSel(s.Results[0], "managed", "NewReconciler") {
				ast.Inspect(s.Results[0], func(n ast.Node) bool {
					if call, ok := n.(*ast.CallExpr); ok && isSel(call.Fun, "managed", "NewReconciler") {
						if id, ok := recOpts.(*ast.Ident); !ok || id.Name == "nil" {
							recOpts = reconcilerArgs(call, &extOpts)
						}
						return false
					}
					return true
				})
				continue
			}
			if len(s.Results) == 1 && containsSel(s.Results[0], "ctrl", "NewControllerManagedBy") {
				continue
			}
		}
		body = append(body, s)
	}
	body = append(body, &ast.ReturnStmt{Results: []ast.Expr{extOpts, recOpts, ast.NewIdent("nil")}})
	// Other returns return an error.
	for _, s := range body[:len(body)-1] {
		ast.Inspect(s, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncLit:
				return false
			case *ast.ReturnStmt:
				if len(n.Results) == 1 {
					n.Results = []ast.Expr{ast.NewIdent("nil"), ast.NewIdent("nil"), n.Results[0]}
				}
			}
			return true
		})
	}
	// Drop "name := managed.ControllerName(...)" if no longer used.
	if len(body) > 0 && assignedName(body[0]) == "name" && !usesIdent(body[1:], "name") {
		body = body[1:]
	}
	fn.Name.Name = "configure" + kind
	fn.Doc = &ast.CommentGroup{List: []*ast.Comment{{Slash: fn.Pos() - 1, Text: fmt.Sprintf("// configure%s returns options for the generated Setup function.", kind)}}}
	fn.Type.Results = &ast.FieldList{List: []*ast.Field{
		{Type: &ast.ArrayType{Elt: ast.NewIdent("option")}},
		{Type: &ast.ArrayType{Elt: &ast.SelectorExpr{X: ast.NewIdent("managed"), Sel: ast.NewIdent("ReconcilerOption")}}},
		{Type: ast.NewIdent("error")},
	}}
	fn.Body.List = body
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name: ast.NewIdent("init"),
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent("configure")},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{ast.NewIdent("configure" + kind)},
		}}},
	})
	r.Add(topicController, "%s: converted `Setup%s` to `configure%s`; the generated Setup sets logger, poll interval, recorder, management policies and the connector", kind, kind, kind)
	return true
}

func assignedName(s ast.Stmt) string {
	a, ok := s.(*ast.AssignStmt)
	if !ok || len(a.Lhs) != 1 {
		return ""
	}
	if id, ok := a.Lhs[0].(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func isReconcilerOptionSlice(t ast.Expr) bool {
	at, ok := t.(*ast.ArrayType)
	return ok && isSel(at.Elt, "managed", "ReconcilerOption")
}

func isSel(e ast.Expr, x, sel string) bool {
	s, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := s.X.(*ast.Ident)
	return ok && id.Name == x && s.Sel.Name == sel
}

func containsSel(n ast.Node, x, sel string) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		if e, ok := n.(ast.Expr); ok && isSel(e, x, sel) {
			found = true
		}
		return !found
	})
	return found
}

func mentionsAny(n ast.Node, names []string) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			for _, name := range names {
				found = found || id.Name == name
			}
		}
		return !found
	})
	return found
}

func usesIdent(stmts []ast.Stmt, name string) bool {
	found := false
	for _, s := range stmts {
		ast.Inspect(s, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == name {
				found = true
			}
			return !found
		})
	}
	return found
}

// filterReconcilerOptions drops options the generated Setup sets. The opts
// of the standard connector become the external client options.
func filterReconcilerOptions(elts []ast.Expr, extOpts *ast.Expr) []ast.Expr {
	var out []ast.Expr
	for _, e := range elts {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			out = append(out, e)
			continue
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			// e.g. managed.WithTypedExternalConnector[*T](...)
			if ix, ok := call.Fun.(*ast.IndexExpr); ok {
				sel, _ = ix.X.(*ast.SelectorExpr)
			}
		}
		if sel != nil && standardReconcilerOptions[sel.Sel.Name] {
			continue
		}
		if sel != nil && (sel.Sel.Name == "WithTypedExternalConnector" || sel.Sel.Name == "WithExternalConnecter") && len(call.Args) == 1 {
			if opts, ok := standardConnectorOpts(call.Args[0]); ok {
				*extOpts = opts
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// standardConnectorOpts returns X of &connector{..., opts: X}.
func standardConnectorOpts(e ast.Expr) (ast.Expr, bool) {
	u, ok := e.(*ast.UnaryExpr)
	if !ok {
		return nil, false
	}
	lit, ok := u.X.(*ast.CompositeLit)
	if !ok {
		return nil, false
	}
	if id, ok := lit.Type.(*ast.Ident); !ok || id.Name != "connector" {
		return nil, false
	}
	if v := keyValue(lit, "opts"); v != nil {
		return v, true
	}
	return ast.NewIdent("nil"), true
}

// reconcilerArgs returns the reconciler options passed to
// managed.NewReconciler(mgr, kind, opts...).
func reconcilerArgs(call *ast.CallExpr, extOpts *ast.Expr) ast.Expr {
	if len(call.Args) < 3 {
		return ast.NewIdent("nil")
	}
	if call.Ellipsis.IsValid() && len(call.Args) == 3 {
		return call.Args[2]
	}
	return &ast.CompositeLit{
		Type: &ast.ArrayType{Elt: &ast.SelectorExpr{X: ast.NewIdent("managed"), Sel: ast.NewIdent("ReconcilerOption")}},
		Elts: filterReconcilerOptions(call.Args[2:], extOpts),
	}
}

// rewriteConnect rewrites connectaws.GetConfigV1(ctx, kube, cr, region) to
// connectaws.GetConfig(ctx, kube, connectaws.NewUsageTracker(kube), cr,
// region); the rewriter maps connectaws to internal/clients/aws.
func rewriteConnect(f *ast.File) {
	name := ""
	for n, p := range fileImports(f) {
		if p == SourceModule+"/pkg/utils/connect/aws" {
			name = n
		}
	}
	if name == "" {
		return
	}
	astutil.Apply(f, func(c *astutil.Cursor) bool {
		call, ok := c.Node().(*ast.CallExpr)
		if !ok || !isSel(call.Fun, name, "GetConfigV1") || len(call.Args) != 4 {
			return true
		}
		call.Fun.(*ast.SelectorExpr).Sel.Name = "GetConfig"
		tracker := &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(name), Sel: ast.NewIdent("NewUsageTracker")}, Args: []ast.Expr{call.Args[1]}}
		call.Args = []ast.Expr{call.Args[0], call.Args[1], tracker, call.Args[2], call.Args[3]}
		return true
	}, nil)
}

// portedFile is a hand-written file copied to the target.
type portedFile struct {
	path string // target path
	kind string // kind of the controller package, if any
}

// portPackage copies and rewrites the hand-written Go files of srcDir to
// dstDir. Existing target files are not overwritten.
func (im *importer) portPackage(srcDir, dstDir, dstPkg, kind string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dstDir, 0o750); err != nil {
		return err
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasPrefix(n, "zz_") {
			continue
		}
		dst := filepath.Join(dstDir, n)
		rel := im.rel(dst)
		if _, err := os.Stat(dst); err == nil {
			im.report.Add(topicController, "%s exists; not overwritten", rel)
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, filepath.Join(srcDir, n), nil, parser.ParseComments)
		if err != nil {
			return err
		}
		if kind != "" {
			convertSetup(f, kind, im.report)
		}
		rewriteConnect(f)
		for _, msg := range im.rw.rewriteFile(fset, f, fileContext{pkgPath: dstPkg, controller: kind != "", ported: true}) {
			im.report.Add(rel, "%s", msg)
		}
		rewriteNewClient(f)
		b, err := formatFile(fset, f)
		if err != nil {
			return fmt.Errorf("cannot format %s: %w", rel, err)
		}
		if err := os.WriteFile(dst, b, 0o600); err != nil {
			return err
		}
		im.ported = append(im.ported, portedFile{path: dst, kind: kind})
	}
	return nil
}

func (im *importer) rel(p string) string {
	if r, err := filepath.Rel(im.tgt.root, p); err == nil {
		return r
	}
	return p
}

// portHelpers ports the source helper packages that ported code imports,
// transitively.
func (im *importer) portHelpers() error {
	done := map[string]bool{}
	for {
		var next string
		for p := range im.rw.helperPkgs {
			if !done[p] {
				next = p
				break
			}
		}
		if next == "" {
			return nil
		}
		done[next] = true
		dstDir := filepath.Join(im.tgt.root, helperDst(next))
		if hasGoFiles(dstDir) && !im.newDirs[dstDir] {
			im.report.Add("Helper packages", "`%s` exists in the target; not updated", helperDst(next))
			continue
		}
		im.newDirs[dstDir] = true
		srcDir := filepath.Join(im.src, next)
		if _, err := os.Stat(srcDir); err != nil {
			im.report.Add("Helper packages", "`%s` not found in the source", next)
			continue
		}
		if err := im.portPackage(srcDir, dstDir, im.tgt.module+"/"+helperDst(next), ""); err != nil {
			return err
		}
		im.report.Add("Helper packages", "ported `%s` to `%s`", next, helperDst(next))
	}
}

func hasGoFiles(dir string) bool {
	m, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	return len(m) > 0
}

// rewriteNewClient replaces SDK v1 svc.New(sess) client constructors with the
// SDK v2 svc.NewFromConfig(cfg).
func rewriteNewClient(f *ast.File) {
	svcs := map[string]bool{}
	for _, is := range f.Imports {
		p := strings.Trim(is.Path.Value, `"`)
		if !strings.HasPrefix(p, sdkV2Prefix+"service/") || strings.Count(p, "/") != strings.Count(sdkV2Prefix+"service/x", "/") {
			continue
		}
		name := path.Base(p)
		if is.Name != nil {
			name = is.Name.Name
		}
		svcs[name] = true
	}
	astutil.Apply(f, func(c *astutil.Cursor) bool {
		call, ok := c.Node().(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "New" {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && svcs[id.Name] {
			sel.Sel.Name = "NewFromConfig"
		}
		return true
	}, nil)
}

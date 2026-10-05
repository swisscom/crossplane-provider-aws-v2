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
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

const (
	markerPrefix    = "+crossplane:generate:reference:"
	topicTypes      = "API types"
	topicReferences = "References"
)

// decl is a top-level declaration of a hand-written source file. Grouped
// declarations are split, one spec per decl.
type decl struct {
	key  string // name, Recv.Name for methods
	node ast.Decl
	file *srcFile
	// deps are package-level identifiers of the same package that the
	// declaration uses.
	deps []string
	// imports are the import paths the declaration uses.
	imports []string
}

type srcFile struct {
	name string
	ast  *ast.File
	// imports maps local import names to paths.
	imports map[string]string
	// keep are the declarations to emit, in source order.
	keep []*decl
	// removed are source ranges whose comments must not be emitted.
	removed [][2]token.Pos
}

// apiPackage is a parsed source API package.
type apiPackage struct {
	fset      *token.FileSet
	path      string // import path, e.g. <source>/apis/rds/v1alpha1
	files     []*srcFile
	decls     map[string]*decl
	order     []*decl
	generated map[string]bool // names declared in zz_ files
}

func parseAPIPackage(dir, importPath string) (*apiPackage, error) {
	p := &apiPackage{fset: token.NewFileSet(), path: importPath, decls: map[string]*decl{}, generated: map[string]bool{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(p.fset, filepath.Join(dir, n), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(n, "zz_") {
			for _, d := range f.Decls {
				for _, name := range declNames(d) {
					p.generated[name] = true
				}
			}
			continue
		}
		sf := &srcFile{name: n, ast: f, imports: fileImports(f)}
		p.files = append(p.files, sf)
		for i, d := range splitDecls(f.Decls) {
			k := declKey(d)
			if k == "" || k == "_" {
				k = fmt.Sprintf("%s#%d", n, i)
			}
			dd := &decl{key: k, node: d, file: sf}
			p.decls[k] = dd
			p.order = append(p.order, dd)
		}
	}
	names := map[string]bool{}
	for n := range p.generated {
		names[n] = true
	}
	for _, d := range p.order {
		for _, n := range declNames(d.node) {
			names[n] = true
		}
	}
	for _, d := range p.order {
		d.deps, d.imports = usedNames(d.node, names, d.file.imports)
	}
	return p, nil
}

func fileImports(f *ast.File) map[string]string {
	m := map[string]string{}
	for _, is := range f.Imports {
		p, _ := strconv.Unquote(is.Path.Value)
		name := path.Base(p)
		if is.Name != nil {
			name = is.Name.Name
		}
		m[name] = p
	}
	return m
}

// splitDecls splits grouped type, var and const declarations so that each
// spec can be kept or dropped on its own. Imports are dropped.
func splitDecls(ds []ast.Decl) []ast.Decl {
	var out []ast.Decl
	for _, d := range ds {
		g, ok := d.(*ast.GenDecl)
		if !ok {
			out = append(out, d)
			continue
		}
		if g.Tok == token.IMPORT {
			continue
		}
		if len(g.Specs) == 1 {
			out = append(out, g)
			continue
		}
		for _, s := range g.Specs {
			ng := &ast.GenDecl{Tok: g.Tok, TokPos: specPos(s), Specs: []ast.Spec{s}}
			if ts, ok := s.(*ast.TypeSpec); ok {
				ng.Doc, ts.Doc = ts.Doc, nil
			}
			if vs, ok := s.(*ast.ValueSpec); ok {
				ng.Doc, vs.Doc = vs.Doc, nil
			}
			out = append(out, ng)
		}
	}
	return out
}

func specPos(s ast.Spec) token.Pos {
	if ts, ok := s.(*ast.TypeSpec); ok {
		return ts.Name.Pos()
	}
	return s.Pos()
}

// declNames returns the package-level names a declaration declares.
func declNames(d ast.Decl) []string {
	switch d := d.(type) {
	case *ast.FuncDecl:
		if d.Recv == nil {
			return []string{d.Name.Name}
		}
	case *ast.GenDecl:
		var out []string
		for _, s := range d.Specs {
			switch s := s.(type) {
			case *ast.TypeSpec:
				out = append(out, s.Name.Name)
			case *ast.ValueSpec:
				for _, n := range s.Names {
					out = append(out, n.Name)
				}
			}
		}
		return out
	}
	return nil
}

func declKey(d ast.Decl) string {
	if f, ok := d.(*ast.FuncDecl); ok && f.Recv != nil {
		return recvType(f) + "." + f.Name.Name
	}
	if n := declNames(d); len(n) == 1 {
		return n[0]
	}
	return ""
}

func recvType(f *ast.FuncDecl) string {
	if f.Recv == nil || len(f.Recv.List) == 0 {
		return ""
	}
	t := f.Recv.List[0].Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// usedNames returns the package-level names in names and the import paths
// that node uses.
func usedNames(node ast.Node, names map[string]bool, imports map[string]string) ([]string, []string) {
	skip := map[*ast.Ident]bool{}
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			skip[n.Name] = true
		case *ast.TypeSpec:
			skip[n.Name] = true
		case *ast.ValueSpec:
			for _, id := range n.Names {
				skip[id] = true
			}
		case *ast.Field:
			for _, id := range n.Names {
				skip[id] = true
			}
		case *ast.SelectorExpr:
			skip[n.Sel] = true
		case *ast.KeyValueExpr:
			if id, ok := n.Key.(*ast.Ident); ok {
				skip[id] = true
			}
		}
		return true
	})
	deps, imps := map[string]bool{}, map[string]bool{}
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := n.X.(*ast.Ident); ok && id.Obj == nil {
				if p, ok := imports[id.Name]; ok {
					imps[p] = true
				}
			}
		case *ast.Ident:
			if !skip[n] && names[n.Name] && (n.Obj == nil || n.Obj.Kind != ast.Var || isTopLevel(n)) {
				deps[n.Name] = true
			}
		}
		return true
	})
	return sortedKeys(deps), sortedKeys(imps)
}

// isTopLevel reports whether a resolved identifier refers to a package-level
// object (the parser only resolves file scope).
func isTopLevel(id *ast.Ident) bool {
	_, ok := id.Obj.Decl.(*ast.ValueSpec)
	return ok
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// target describes the provider-aws-v2 checkout being imported into.
type target struct {
	root   string
	module string
	// kinds are the imported kinds per API package, keyed by
	// <service>/<version>, including the ones of the current import.
	kinds map[string]map[string]bool
}

func (t *target) apiAvailable(svcVer string) bool {
	_, ok := t.kinds[svcVer]
	return ok
}

// kindAvailable reports whether the source kind <pkg>.<kind> was imported.
func (t *target) kindAvailable(srcPkg, kind string) bool {
	rel, ok := strings.CutPrefix(srcPkg, SourceModule+"/apis/")
	return ok && t.kinds[rel][kind]
}

// customTypes imports the hand-written API code of kinds in package
// apis/<svcVer>: their Custom<Kind>Parameters and Custom<Kind>Observation
// types, the types these use, methods on the kinds and other declarations
// that only depend on available code. References to kinds that were not
// imported are dropped.
type customTypes struct {
	src    *apiPackage
	svcVer string
	kinds  []string
	// srcKinds are all kinds of the source package.
	srcKinds []string
	tgt      *target
	rw       *rewriter
	report   *Report

	keep map[string]bool
	// dropFields are Ref and Selector fields of dropped references.
	dropFields map[string]bool
	// resolvers are the ported ResolveReferences methods, keyed by kind.
	resolvers map[string]*decl
	// other are kept declarations other than types; header is the file header.
	other  []*decl
	header []byte
}

func (c *customTypes) run() error {
	targetDir := filepath.Join(c.tgt.root, "apis", c.svcVer)
	tgtNames, err := targetGeneratedNames(targetDir)
	if err != nil {
		return err
	}
	c.keep, c.dropFields, c.resolvers = map[string]bool{}, map[string]bool{}, map[string]*decl{}

	// Seeds: custom types and methods of the kinds.
	var seeds []string
	isKind := map[string]bool{}
	for _, k := range c.kinds {
		isKind[k] = true
		for _, n := range []string{"Custom" + k + "Parameters", "Custom" + k + "Observation"} {
			if _, ok := c.src.decls[n]; ok {
				seeds = append(seeds, n)
			}
		}
	}
	for _, d := range c.src.order {
		if f, ok := d.node.(*ast.FuncDecl); ok && isKind[recvType(f)] {
			if f.Name.Name == "ResolveReferences" {
				c.resolvers[recvType(f)] = d
				continue
			}
			seeds = append(seeds, d.key)
		}
	}
	notImported := map[string]bool{}
	for _, k := range c.srcKinds {
		notImported[k] = !c.tgt.kinds[c.svcVer][k]
	}
	// Kinds that were not imported may exist as plain SDK shapes in the
	// target, e.g. DBCluster; they are not available.
	available := func(name string) bool { return (tgtNames[name] && !notImported[name]) || c.keep[name] }
	for _, s := range seeds {
		c.closure(s, available, true)
	}
	// Other declarations that use a kept declaration and only depend on
	// available code, e.g. interfaces the kinds implement.
	for changed := true; changed; {
		changed = false
		for _, d := range c.src.order {
			if c.keep[d.key] || isResolver(d) || !c.usesKept(d, isKind) || isCustomTypeOfOtherKind(d.key, isKind) {
				continue
			}
			if c.resolvable(d, available, map[string]bool{}) {
				c.closure(d.key, available, false)
				changed = true
			}
		}
	}
	for k, d := range c.resolvers {
		c.portResolver(k, d)
	}
	c.rewriteMarkers()
	c.dropRefFields()
	return c.writeTypes(targetDir)
}

func isResolver(d *decl) bool {
	f, ok := d.node.(*ast.FuncDecl)
	return ok && f.Name.Name == "ResolveReferences"
}

func isCustomTypeOfOtherKind(name string, isKind map[string]bool) bool {
	m := regexp.MustCompile(`^Custom(\w+?)(Parameters|Observation)$`).FindStringSubmatch(name)
	return m != nil && !isKind[m[1]]
}

func (*customTypes) usesKept(d *decl, isKind map[string]bool) bool {
	for _, dep := range d.deps {
		if isKind[dep] {
			return true
		}
	}
	return false
}

// resolvable reports whether d and its hand-written dependencies only use
// available names and imports.
func (c *customTypes) resolvable(d *decl, available func(string) bool, seen map[string]bool) bool {
	if seen[d.key] {
		return true
	}
	seen[d.key] = true
	for _, p := range d.imports {
		if dst, _ := c.rw.mapImport(p); dst == "" {
			return false
		}
	}
	for _, dep := range d.deps {
		if available(dep) {
			continue
		}
		dd, ok := c.src.decls[dep]
		if !ok || !c.resolvable(dd, available, seen) {
			return false
		}
	}
	return true
}

// closure keeps name and the hand-written declarations it depends on.
func (c *customTypes) closure(name string, available func(string) bool, seed bool) {
	d, ok := c.src.decls[name]
	if !ok || c.keep[name] {
		return
	}
	c.keep[name] = true
	for _, dep := range d.deps {
		if available(dep) {
			continue
		}
		if _, ok := c.src.decls[dep]; ok {
			c.closure(dep, available, seed)
			continue
		}
		c.report.Add(topicTypes, "`%s` uses `%s`, which is not generated in the target; port it by hand", name, dep)
	}
}

// portResolver ports a hand-written ResolveReferences method to namespaced
// references. Resolutions of kinds that were not imported are dropped.
func (c *customTypes) portResolver(kind string, d *decl) { //nolint:gocyclo // a sequence of AST checks
	f := d.node.(*ast.FuncDecl)
	stmts := f.Body.List
	type block struct {
		stmts []ast.Stmt
		req   *ast.CompositeLit
	}
	var prologue []ast.Stmt
	var blocks []*block
	var epilogue []ast.Stmt
	for _, s := range stmts {
		if req := resolveRequest(s); req != nil {
			blocks = append(blocks, &block{stmts: []ast.Stmt{s}, req: req})
			continue
		}
		if _, ok := s.(*ast.ReturnStmt); ok || len(blocks) == 0 {
			if len(blocks) == 0 {
				prologue = append(prologue, s)
			} else {
				epilogue = append(epilogue, s)
			}
			continue
		}
		if len(epilogue) > 0 {
			c.report.Add(topicReferences, "`%s.ResolveReferences` has an unexpected structure; port it by hand", kind)
			return
		}
		blocks[len(blocks)-1].stmts = append(blocks[len(blocks)-1].stmts, s)
	}

	var kept []*block
	prev := f.Body.Lbrace
	for _, b := range blocks {
		pkg, tk := resolveTarget(b.req, d.file.imports, c.src.path)
		ok := pkg != "" && c.tgt.kindAvailable(pkg, tk)
		if ok {
			if ex := extractorPkg(b.req, d.file.imports); ex != "" {
				if dst, _ := c.rw.mapImport(ex); dst == "" {
					ok = false
				}
			}
		}
		field := fieldName(b.req, "CurrentValue", "CurrentValues")
		if ok {
			kept = append(kept, b)
			prev = b.stmts[len(b.stmts)-1].End()
			continue
		}
		for _, n := range []string{"Reference", "References", "Selector"} {
			if fn := fieldName(b.req, n); fn != "" {
				c.dropFields[fn] = true
			}
		}
		d.file.removed = append(d.file.removed, [2]token.Pos{prev, b.stmts[len(b.stmts)-1].End()})
		prev = b.stmts[len(b.stmts)-1].End()
		c.report.Add(topicReferences, "%s: dropped reference of `%s` to `%s.%s` (kind not imported)", kind, field, pkg, tk)
	}
	if len(kept) == 0 {
		c.report.Add(topicReferences, "%s: hand-written `ResolveReferences` removed, all its references were dropped; angryjet generates it from `%s` markers", kind, markerPrefix)
		return
	}

	body := append([]ast.Stmt{}, prologue...)
	defined := map[string]bool{}
	for _, b := range kept {
		for _, s := range b.stmts {
			if a, ok := s.(*ast.AssignStmt); ok && (a.Tok == token.DEFINE || a.Tok == token.ASSIGN) {
				a.Tok = token.ASSIGN
				for _, l := range a.Lhs {
					if id, ok := l.(*ast.Ident); ok && id.Name != "_" && !defined[id.Name] {
						defined[id.Name] = true
						a.Tok = token.DEFINE
					}
				}
			}
		}
		body = append(body, b.stmts...)
		b.req.Elts = append(b.req.Elts, &ast.KeyValueExpr{
			Key:   &ast.Ident{Name: "Namespace", NamePos: b.req.Rbrace - 1},
			Value: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(recvName(f)), Sel: ast.NewIdent("GetNamespace")}},
		})
	}
	f.Body.List = append(body, epilogue...)
	c.keep[d.key] = true
	c.report.Add(topicReferences, "%s: ported hand-written `ResolveReferences` to namespaced references. angryjet does not generate resolvers for kinds with a hand-written one, so `%s` markers of %s are ignored", kind, markerPrefix, kind)
}

func recvName(f *ast.FuncDecl) string {
	if n := f.Recv.List[0].Names; len(n) > 0 {
		return n[0].Name
	}
	return "mg"
}

// resolveRequest returns the request literal of a r.Resolve or
// r.ResolveMultiple call assigned by s.
func resolveRequest(s ast.Stmt) *ast.CompositeLit {
	a, ok := s.(*ast.AssignStmt)
	if !ok || len(a.Rhs) != 1 {
		return nil
	}
	call, ok := a.Rhs[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Resolve" && sel.Sel.Name != "ResolveMultiple") {
		return nil
	}
	lit, _ := call.Args[1].(*ast.CompositeLit)
	return lit
}

func keyValue(lit *ast.CompositeLit, key string) ast.Expr {
	for _, e := range lit.Elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == key {
				return kv.Value
			}
		}
	}
	return nil
}

// fieldName returns the last selector of the first present key, e.g.
// DBClusterIdentifierRef for Reference: mg.Spec.ForProvider.DBClusterIdentifierRef.
func fieldName(lit *ast.CompositeLit, keys ...string) string {
	for _, k := range keys {
		v := keyValue(lit, k)
		if call, ok := v.(*ast.CallExpr); ok && len(call.Args) == 1 {
			v = call.Args[0]
		}
		if sel, ok := v.(*ast.SelectorExpr); ok {
			return sel.Sel.Name
		}
	}
	return ""
}

// resolveTarget returns the package and kind of To: reference.To{Managed: &pkg.Kind{}}.
func resolveTarget(lit *ast.CompositeLit, imports map[string]string, self string) (string, string) {
	to, ok := keyValue(lit, "To").(*ast.CompositeLit)
	if !ok {
		return "", ""
	}
	u, ok := keyValue(to, "Managed").(*ast.UnaryExpr)
	if !ok {
		return "", ""
	}
	cl, ok := u.X.(*ast.CompositeLit)
	if !ok {
		return "", ""
	}
	switch t := cl.Type.(type) {
	case *ast.Ident:
		return self, t.Name
	case *ast.SelectorExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return imports[id.Name], t.Sel.Name
		}
	}
	return "", ""
}

// extractorPkg returns the package of Extract: pkg.Fn(), unless it is the
// reference package.
func extractorPkg(lit *ast.CompositeLit, imports map[string]string) string {
	call, ok := keyValue(lit, "Extract").(*ast.CallExpr)
	if !ok {
		return ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name == "reference" {
		return ""
	}
	return imports[id.Name]
}

var markerRe = regexp.MustCompile(`^//\s*\+crossplane:generate:reference:(\w+)=(.*)$`)

// rewriteMarkers rewrites reference markers of kept struct fields to the
// target module, or drops them and their Ref and Selector fields if the
// referenced kind was not imported.
func (c *customTypes) rewriteMarkers() {
	for _, d := range c.src.order {
		if !c.keep[d.key] {
			continue
		}
		ast.Inspect(d.node, func(n ast.Node) bool {
			f, ok := n.(*ast.Field)
			if !ok || f.Doc == nil || len(f.Names) == 0 {
				return true
			}
			m := map[string]string{}
			for _, cm := range f.Doc.List {
				if mm := markerRe.FindStringSubmatch(cm.Text); mm != nil {
					m[mm[1]] = strings.TrimSpace(mm[2])
				}
			}
			if m["type"] == "" {
				return true
			}
			name := f.Names[0].Name
			pkg, kind := c.src.path, m["type"]
			if i := strings.LastIndex(m["type"], "."); i >= 0 {
				pkg, kind = m["type"][:i], m["type"][i+1:]
			}
			keep := c.tgt.kindAvailable(pkg, kind)
			ex := m["extractor"]
			if i := strings.LastIndex(ex, "."); keep && i > 0 && strings.Contains(ex[:i], "/") {
				if dst, _ := c.rw.mapImport(ex[:i]); dst == "" {
					keep = false
				}
			}
			if !keep {
				ref, sel := m["refFieldName"], m["selectorFieldName"]
				if ref == "" {
					ref = name + "Ref"
					if _, ok := f.Type.(*ast.ArrayType); ok {
						ref = name + "Refs"
					}
				}
				if sel == "" {
					sel = name + "Selector"
				}
				c.dropFields[ref], c.dropFields[sel] = true, true
				c.report.Add(topicReferences, "%s: dropped reference of `%s` to `%s.%s` (kind not imported)", d.key, name, pkg, kind)
			}
			var list []*ast.Comment
			for _, cm := range f.Doc.List {
				if mm := markerRe.FindStringSubmatch(cm.Text); mm != nil {
					if !keep {
						continue
					}
					cm.Text = strings.ReplaceAll(cm.Text, SourceModule+"/", c.tgt.module+"/")
				}
				list = append(list, cm)
			}
			f.Doc.List = list
			return true
		})
	}
}

// dropRefFields removes the Ref and Selector fields of dropped references
// from kept struct types.
func (c *customTypes) dropRefFields() {
	for _, d := range c.src.order {
		if !c.keep[d.key] {
			continue
		}
		ast.Inspect(d.node, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok {
				return true
			}
			var fields []*ast.Field
			for _, f := range st.Fields.List {
				if len(f.Names) == 1 && c.dropFields[f.Names[0].Name] {
					start := f.Pos()
					if f.Doc != nil {
						start = f.Doc.Pos()
					}
					d.file.removed = append(d.file.removed, [2]token.Pos{start, f.End()})
					continue
				}
				fields = append(fields, f)
			}
			st.Fields.List = fields
			return true
		})
	}
}

// targetGeneratedNames returns the names declared in zz_ files of dir.
func targetGeneratedNames(dir string) (map[string]bool, error) {
	fset := token.NewFileSet()
	names := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "zz_") || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, d := range f.Decls {
			for _, n := range declNames(d) {
				names[n] = true
			}
		}
	}
	return names, nil
}

// writeTypes emits kept types into custom_types.go, replacing still empty
// generated stubs. Other kept declarations are emitted by writeOther, after
// deepcopy and managed resource methods were generated.
func (c *customTypes) writeTypes(dir string) error {
	ctPath := filepath.Join(dir, "custom_types.go")
	ct, err := os.ReadFile(ctPath)
	if err != nil {
		return err
	}
	header := ct[:bytes.Index(ct, []byte("\npackage "))+1]
	ct, existing, err := removeStubs(ct, c.src.path)
	if err != nil {
		return err
	}
	var types, other []*decl
	for _, d := range c.src.order {
		if !c.keep[d.key] {
			continue
		}
		if existing[d.key] {
			c.report.Add(topicTypes, "`%s` already exists in %s; not overwritten", d.key, ctPath)
			continue
		}
		if g, ok := d.node.(*ast.GenDecl); ok && g.Tok == token.TYPE && !isInterface(g) {
			types = append(types, d)
			continue
		}
		other = append(other, d)
	}
	c.other, c.header = other, header
	if len(types) == 0 {
		return nil
	}
	out, err := c.render(header, types, ct)
	if err != nil {
		return err
	}
	return os.WriteFile(ctPath, out, 0o600)
}

// writeOther emits kept declarations other than types into <kind>.go.
func (c *customTypes) writeOther() error {
	other, header := c.other, c.header
	if len(other) == 0 {
		return nil
	}
	dir := filepath.Join(c.tgt.root, "apis", c.svcVer)
	name := snake(strings.Join(c.kinds, "_")) + ".go"
	p := filepath.Join(dir, name)
	if _, err := os.Stat(p); err == nil {
		c.report.Add(topicTypes, "%s exists; not overwritten. Compare it with the source by hand", p)
		return nil
	}
	out, err := c.render(header, other, nil)
	if err != nil {
		return err
	}
	return os.WriteFile(p, out, 0o600)
}

func isInterface(g *ast.GenDecl) bool {
	ts, ok := g.Specs[0].(*ast.TypeSpec)
	if !ok {
		return false
	}
	_, ok = ts.Type.(*ast.InterfaceType)
	return ok
}

// removeStubs removes empty Custom*Parameters and Custom*Observation
// structs from a target custom_types.go and returns the names of the
// remaining declarations.
func removeStubs(src []byte, _ string) ([]byte, map[string]bool, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "custom_types.go", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, err
	}
	existing := map[string]bool{}
	var cut [][2]int
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.TYPE || len(g.Specs) != 1 {
			for _, n := range declNames(d) {
				existing[n] = true
			}
			continue
		}
		ts := g.Specs[0].(*ast.TypeSpec)
		st, ok := ts.Type.(*ast.StructType)
		if ok && len(st.Fields.List) == 0 && strings.HasPrefix(ts.Name.Name, "Custom") {
			start := g.Pos()
			if g.Doc != nil {
				start = g.Doc.Pos()
			}
			cut = append(cut, [2]int{fset.Position(start).Offset, fset.Position(g.End()).Offset})
			continue
		}
		existing[ts.Name.Name] = true
	}
	for i := len(cut) - 1; i >= 0; i-- {
		src = append(src[:cut[i][0]], src[cut[i][1]:]...)
	}
	return src, existing, nil
}

// render prints decls as a file of the target package. If base is set, the
// declarations are appended to it, otherwise a new file with header is
// created.
func (c *customTypes) render(header []byte, decls []*decl, base []byte) ([]byte, error) {
	var b bytes.Buffer
	if base != nil {
		b.Write(bytes.TrimRight(base, "\n"))
		b.WriteString("\n")
	} else {
		b.Write(header)
		fmt.Fprintf(&b, "package %s\n", path.Base(c.svcVer))
	}
	imports := map[string]string{}
	for _, sf := range c.src.files {
		var ds []*decl
		for _, d := range decls {
			if d.file == sf {
				ds = append(ds, d)
			}
		}
		if len(ds) == 0 {
			continue
		}
		text, imps, err := c.renderDecls(sf, ds)
		if err != nil {
			return nil, err
		}
		b.WriteString("\n")
		b.Write(text)
		for p, n := range imps {
			imports[p] = n
		}
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", b.Bytes(), parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("cannot parse rendered code: %w\n%s", err, b.String())
	}
	for p, n := range imports {
		if n == path.Base(p) {
			n = ""
		}
		astutil.AddNamedImport(fset, f, n, p)
	}
	var out bytes.Buffer
	if err := format.Node(&out, fset, f); err != nil {
		return nil, err
	}
	return format.Source(out.Bytes())
}

// renderDecls rewrites and prints decls of sf without package clause and
// imports. It returns the imports the code needs.
func (c *customTypes) renderDecls(sf *srcFile, ds []*decl) ([]byte, map[string]string, error) {
	f := &ast.File{Name: sf.ast.Name, Package: sf.ast.Package}
	// Imports are needed so that the rewriter can resolve selectors.
	var specs []ast.Spec
	for _, is := range sf.ast.Imports {
		specs = append(specs, is)
		f.Imports = append(f.Imports, is)
	}
	if len(specs) > 0 {
		f.Decls = append(f.Decls, &ast.GenDecl{Tok: token.IMPORT, Specs: specs, Lparen: 1})
	}
	inRange := func(p token.Pos, rs [][2]token.Pos) bool {
		for _, r := range rs {
			if p >= r[0] && p < r[1] {
				return true
			}
		}
		return false
	}
	var keepRanges [][2]token.Pos
	for _, d := range ds {
		f.Decls = append(f.Decls, d.node)
		start := d.node.Pos()
		if doc := docOf(d.node); doc != nil {
			start = doc.Pos()
		}
		keepRanges = append(keepRanges, [2]token.Pos{start, d.node.End() + 1})
	}
	for _, cg := range sf.ast.Comments {
		if len(cg.List) > 0 && inRange(cg.Pos(), keepRanges) && !inRange(cg.Pos(), sf.removed) {
			f.Comments = append(f.Comments, cg)
		}
	}
	dst := c.tgt.module + "/apis/" + c.svcVer
	for _, msg := range c.rw.rewriteFile(c.src.fset, f, fileContext{pkgPath: dst}) {
		c.report.Add(topicTypes, "%s: %s", sf.name, msg)
	}
	imports := map[string]string{}
	for _, is := range f.Imports {
		p, _ := strconv.Unquote(is.Path.Value)
		n := path.Base(p)
		if is.Name != nil {
			n = is.Name.Name
		}
		imports[p] = n
	}
	var b bytes.Buffer
	cfg := &printerConfig
	if err := cfg.Fprint(&b, c.src.fset, f); err != nil {
		return nil, nil, err
	}
	// Drop the package clause and imports.
	out := b.Bytes()
	if i := bytes.Index(out, []byte("\n)\n")); i >= 0 && len(f.Imports) > 0 {
		out = out[i+3:]
	} else if i := bytes.IndexByte(out, '\n'); i >= 0 {
		out = out[i+1:]
	}
	return out, imports, nil
}

func docOf(d ast.Decl) *ast.CommentGroup {
	switch d := d.(type) {
	case *ast.FuncDecl:
		return d.Doc
	case *ast.GenDecl:
		return d.Doc
	}
	return nil
}

func snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 && s[i-1] != '_' && !(s[i-1] >= 'A' && s[i-1] <= 'Z') {
				b.WriteByte('_')
			}
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

var printerConfig = printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}

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
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// resolverGap is a reference marker that angryjet ignores because its kind
// has a hand-written ResolveReferences that does not resolve it.
type resolverGap struct {
	Kind   string
	Method string // file:line of ResolveReferences
	Field  string
	Marker string // file:line of the marked field
}

// resolverGaps finds marker references of kinds with a hand-written
// ResolveReferences in the API package in dir that the method does not
// mention. angryjet generates no resolver for such kinds.
func resolverGaps(dir string) ([]resolverGap, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	types := map[string]*ast.StructType{}
	type method struct {
		fn  *ast.FuncDecl
		pos string
	}
	resolvers := map[string]method{}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.GenDecl:
				for _, s := range d.Specs {
					if ts, ok := s.(*ast.TypeSpec); ok {
						if st, ok := ts.Type.(*ast.StructType); ok {
							types[ts.Name.Name] = st
						}
					}
				}
			case *ast.FuncDecl:
				if d.Name.Name == "ResolveReferences" && d.Recv != nil && !strings.HasPrefix(n, "zz_") {
					resolvers[recvType(d)] = method{fn: d, pos: posString(fset, d.Pos())}
				}
			}
		}
	}

	var gaps []resolverGap
	for _, kind := range slices.Sorted(maps.Keys(resolvers)) {
		m := resolvers[kind]
		used := map[string]bool{}
		ast.Inspect(m.fn.Body, func(n ast.Node) bool {
			if s, ok := n.(*ast.SelectorExpr); ok {
				used[s.Sel.Name] = true
			}
			return true
		})
		seen := map[string]bool{}
		var walk func(name string)
		walk = func(name string) {
			st, ok := types[name]
			if !ok || seen[name] {
				return
			}
			seen[name] = true
			for _, f := range st.Fields.List {
				if f.Doc != nil && strings.Contains(f.Doc.Text(), markerPrefix) {
					for _, id := range f.Names {
						if !used[id.Name] {
							gaps = append(gaps, resolverGap{Kind: kind, Method: m.pos, Field: id.Name, Marker: posString(fset, id.Pos())})
						}
					}
				}
				walk(baseTypeName(f.Type))
			}
		}
		walk(kind)
	}
	return gaps, nil
}

// baseTypeName returns the local type name behind *T, []T and map[K]T.
func baseTypeName(e ast.Expr) string {
	for {
		switch t := e.(type) {
		case *ast.Ident:
			return t.Name
		case *ast.StarExpr:
			e = t.X
		case *ast.ArrayType:
			e = t.Elt
		case *ast.MapType:
			e = t.Value
		default:
			return ""
		}
	}
}

func posString(fset *token.FileSet, p token.Pos) string {
	pos := fset.Position(p)
	return fmt.Sprintf("%s:%d", filepath.Base(pos.Filename), pos.Line)
}

// reportResolverGaps reports every resolver gap in dir as blocking. If the
// method is in written, a file this import wrote, it also gets a
// TODO(import) note so the gate holds it.
func reportResolverGaps(dir, written string, r *Report) error {
	gaps, err := resolverGaps(dir)
	if err != nil {
		return err
	}
	fields := map[string][]string{}
	for _, g := range gaps {
		r.Block(topicReferences, "`%s` has a hand-written `ResolveReferences` (%s), so angryjet generates no resolver for it: the marker reference of `%s` (%s) is never resolved. Resolve it in the hand-written method or remove the method", g.Kind, g.Method, g.Field, g.Marker)
		if written != "" && strings.HasPrefix(g.Method, filepath.Base(written)+":") {
			fields[g.Kind] = append(fields[g.Kind], g.Field)
		}
	}
	if len(fields) == 0 {
		return nil
	}
	src, err := os.ReadFile(written) //nolint:gosec // a file this import wrote
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, written, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	var at []int
	notes := map[int]string{}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "ResolveReferences" || len(fields[recvType(fd)]) == 0 {
			continue
		}
		o := fset.Position(fd.Pos()).Offset
		at = append(at, o)
		notes[o] = fmt.Sprintf("// TODO(import): resolve %s here; angryjet skips kinds with a hand-written ResolveReferences.\n", strings.Join(fields[recvType(fd)], ", "))
	}
	slices.Sort(at)
	for i := len(at) - 1; i >= 0; i-- {
		src = slices.Concat(src[:at[i]], []byte(notes[at[i]]), src[at[i]:])
	}
	return os.WriteFile(written, src, 0o600)
}

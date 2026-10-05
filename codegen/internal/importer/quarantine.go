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
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
	"sigs.k8s.io/yaml"

	"github.com/teeverr/provider-aws-v2/codegen/internal/generator"
)

const (
	topicTodo = "Not compiling yet (importtodo)"
	todoTag   = "importtodo"
	maxRounds = 40
)

// importer ports hand-written code of one service.
type importer struct {
	ctx    context.Context
	o      Options
	src    string // source checkout
	tgt    *target
	rw     *rewriter
	report *Report

	ported  []portedFile
	newDirs map[string]bool
	// todo counts quarantined declarations per target file.
	todo map[string]int
}

func formatFile(fset *token.FileSet, f *ast.File) ([]byte, error) {
	var b bytes.Buffer
	if err := format.Node(&b, fset, f); err != nil {
		return nil, err
	}
	return format.Source(b.Bytes())
}

// typecheck type checks the ported packages and moves declarations that do
// not compile to <file>_importtodo.go files that are excluded from the
// build. Missing SDK operations of a generated Client interface are added to
// codegen.yaml instead.
func (im *importer) typecheck() error { //nolint:gocyclo // a fixpoint loop
	dirs := map[string]bool{}
	for _, f := range im.ported {
		dirs[filepath.Dir(f.path)] = true
	}
	patterns := make([]string, 0, len(dirs))
	for d := range dirs {
		patterns = append(patterns, "./"+im.rel(d))
	}
	sort.Strings(patterns)
	kindOf := map[string]string{}
	for _, f := range im.ported {
		if f.kind != "" {
			kindOf[filepath.Dir(f.path)] = f.kind
		}
	}

	for round := 0; round < maxRounds; round++ {
		errs, err := im.load(patterns)
		if err != nil {
			return err
		}
		if os.Getenv("IMPORT_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "round %d: %d errors\n", round, len(errs))
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "  %s\n", e.String(im))
			}
		}
		if len(errs) == 0 {
			return nil
		}
		failing := map[string]bool{}
		for _, e := range errs {
			failing[filepath.Dir(e.file)] = true
		}
		quarantine := map[string]map[int]string{} // file -> line -> message
		ops := map[string][]string{}
		progress := false
		var stuck []string
		for _, e := range errs {
			if m := regexp.MustCompile(`could not import (\S+)`).FindStringSubmatch(e.msg); m != nil {
				if rel, ok := strings.CutPrefix(m[1], im.tgt.module+"/"); ok && failing[filepath.Join(im.tgt.root, rel)] {
					continue // fixed once the imported package compiles
				}
			}
			if op := im.missingClientOp(e.msg); op != "" && kindOf[filepath.Dir(e.file)] != "" {
				k := kindOf[filepath.Dir(e.file)]
				if !slices.Contains(ops[k], op) {
					ops[k] = append(ops[k], op)
				}
				continue
			}
			if strings.HasPrefix(filepath.Base(e.file), "zz_") || !im.isPorted(e.file) {
				stuck = append(stuck, e.String(im))
				continue
			}
			if quarantine[e.file] == nil {
				quarantine[e.file] = map[int]string{}
			}
			if _, ok := quarantine[e.file][e.line]; !ok {
				quarantine[e.file][e.line] = e.msg
			}
		}
		if len(ops) > 0 {
			if err := im.addClientOps(ops); err != nil {
				return err
			}
			progress = true
		}
		for file, lines := range quarantine {
			n, err := im.quarantine(file, lines)
			if err != nil {
				return err
			}
			progress = progress || n > 0
		}
		if !progress {
			for _, s := range stuck {
				im.report.Add(topicTodo, "unresolved: %s", s)
			}
			return nil
		}
	}
	im.report.Add(topicTodo, "type check did not converge after %d rounds", maxRounds)
	return nil
}

func (im *importer) isPorted(file string) bool {
	for _, f := range im.ported {
		if filepath.Dir(f.path) == filepath.Dir(file) {
			return true
		}
	}
	return false
}

type loadError struct {
	file string
	line int
	msg  string
}

func (e loadError) String(im *importer) string {
	return fmt.Sprintf("%s:%d: %s", im.rel(e.file), e.line, e.msg)
}

func (im *importer) load(patterns []string) ([]loadError, error) {
	cfg := &packages.Config{
		Context: im.ctx,
		Dir:     im.tgt.root,
		Mode:    packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
		Tests:   true,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []loadError
	for _, p := range pkgs {
		for _, e := range p.Errors {
			pos := e.Pos
			if pos == "" || pos == "-" {
				// e.g. a package that cannot be listed
				for _, f := range p.GoFiles {
					pos = f + ":1:1"
					break
				}
			}
			parts := strings.Split(pos, ":")
			if len(parts) < 2 {
				continue
			}
			line, _ := strconv.Atoi(parts[1])
			le := loadError{file: parts[0], line: line, msg: e.Msg}
			k := fmt.Sprintf("%s:%d:%s", le.file, le.line, le.msg)
			if !seen[k] {
				seen[k] = true
				out = append(out, le)
			}
		}
	}
	return out, nil
}

var missingMethodRe = regexp.MustCompile(`\(type Client has no field or method (\w+)\)`)

// missingClientOp returns the SDK operation a Client interface misses.
func (im *importer) missingClientOp(msg string) string {
	m := missingMethodRe.FindStringSubmatch(msg)
	if m == nil {
		return ""
	}
	if n := im.rw.sdk[im.o.Service]; n != nil && n.client[m[1]+"Input"] {
		return m[1]
	}
	return ""
}

func (im *importer) addClientOps(ops map[string][]string) error {
	sc, err := generator.ReadServiceConfig(im.tgt.root, im.o.Service)
	if err != nil {
		return err
	}
	if sc.ClientOperations == nil {
		sc.ClientOperations = map[string][]string{}
	}
	for k, os := range ops {
		for _, op := range os {
			if !slices.Contains(sc.ClientOperations[k], op) {
				sc.ClientOperations[k] = append(sc.ClientOperations[k], op)
				im.report.Add(topicController, "%s: added `%s` to the Client interface (codegen.yaml)", k, op)
			}
		}
		sort.Strings(sc.ClientOperations[k])
	}
	b, err := yaml.Marshal(sc)
	if err != nil {
		return err
	}
	b = append([]byte("# provider-aws-v2 codegen settings of this service, see codegen/README.md.\n"), b...)
	if err := os.WriteFile(generator.ServiceConfigPath(im.tgt.root, im.o.Service), b, 0o600); err != nil {
		return err
	}
	return generator.Run(im.ctx, im.o.Options)
}

// quarantine moves the declarations of file at the given lines to the
// importtodo file. It returns the number of moved declarations.
func (im *importer) quarantine(file string, lines map[int]string) (int, error) { //nolint:gocyclo // AST bookkeeping
	src, err := os.ReadFile(file)
	if err != nil {
		return 0, err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, parser.ParseComments)
	if err != nil {
		return 0, err
	}
	type cut struct {
		start, end int
		msg        string
	}
	var cuts []cut
	var badImports []string
	for _, d := range f.Decls {
		start, end := fset.Position(d.Pos()).Line, fset.Position(d.End()).Line
		var msg string
		for l, m := range lines {
			if l >= start && l <= end && (msg == "" || l < start) {
				msg = m
			}
		}
		if msg == "" {
			continue
		}
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
			for _, s := range g.Specs {
				is := s.(*ast.ImportSpec)
				if l := fset.Position(is.Pos()).Line; lines[l] != "" {
					badImports = append(badImports, is.Path.Value)
				}
			}
			continue
		}
		p := d.Pos()
		if doc := docOf(d); doc != nil {
			p = doc.Pos()
		}
		cuts = append(cuts, cut{start: fset.Position(p).Offset, end: fset.Position(d.End()).Offset, msg: msg})
	}
	// Declarations that use an import that cannot be resolved.
	for _, ip := range badImports {
		name := ""
		for _, is := range f.Imports {
			if is.Path.Value == ip {
				p, _ := strconv.Unquote(ip)
				name = filepath.Base(p)
				if is.Name != nil {
					name = is.Name.Name
				}
			}
		}
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
				continue
			}
			if usesPkg(d, name) {
				p := d.Pos()
				if doc := docOf(d); doc != nil {
					p = doc.Pos()
				}
				cuts = append(cuts, cut{start: fset.Position(p).Offset, end: fset.Position(d.End()).Offset, msg: "cannot import " + ip})
			}
		}
	}
	sort.Slice(cuts, func(i, j int) bool { return cuts[i].start < cuts[j].start })
	// Deduplicate overlapping cuts.
	var uniq []cut
	for _, c := range cuts {
		if len(uniq) > 0 && c.start < uniq[len(uniq)-1].end {
			continue
		}
		uniq = append(uniq, c)
	}
	if len(uniq) == 0 && len(badImports) == 0 {
		return 0, nil
	}

	var moved bytes.Buffer
	for _, c := range uniq {
		fmt.Fprintf(&moved, "\n// TODO(import): %s\n%s\n", oneLine(c.msg), src[c.start:c.end])
		im.report.Add(topicTodo, "%s: `%s`: %s", im.rel(file), declSummary(src[c.start:c.end]), oneLine(c.msg))
	}
	out := src
	for i := len(uniq) - 1; i >= 0; i-- {
		out = append(append([]byte{}, out[:uniq[i].start]...), out[uniq[i].end:]...)
	}
	out, err = pruneImports(file, out, badImports)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(file, out, 0o600); err != nil {
		return 0, err
	}
	im.todo[file] += len(uniq)
	return len(uniq) + len(badImports), im.appendTodo(file, src, f, fset, moved.Bytes())
}

func usesPkg(n ast.Node, name string) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		if s, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := s.X.(*ast.Ident); ok && id.Name == name {
				found = true
			}
		}
		return !found
	})
	return found
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func declSummary(b []byte) string {
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "//") {
			return strings.TrimSuffix(strings.TrimSuffix(l, "{"), " ")
		}
	}
	return ""
}

// pruneImports removes imports that are no longer used, and bad ones.
func pruneImports(name string, src []byte, bad []string) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	for _, is := range slices.Clone(f.Imports) {
		p, _ := strconv.Unquote(is.Path.Value)
		local := filepath.Base(p)
		if is.Name != nil {
			local = is.Name.Name
		}
		if local == "_" || local == "." {
			continue
		}
		if slices.Contains(bad, is.Path.Value) || !usesPkg(f, local) {
			n := ""
			if is.Name != nil {
				n = is.Name.Name
			}
			astutil.DeleteNamedImport(fset, f, n, p)
		}
	}
	return formatFile(fset, f)
}

// appendTodo appends moved declarations to the importtodo file of file.
func (im *importer) appendTodo(file string, src []byte, f *ast.File, fset *token.FileSet, moved []byte) error {
	base := strings.TrimSuffix(filepath.Base(file), ".go")
	test := strings.HasSuffix(base, "_test")
	base = strings.TrimSuffix(base, "_test") + "_" + todoTag
	if test {
		base += "_test"
	}
	todo := filepath.Join(filepath.Dir(file), base+".go")
	existing, err := os.ReadFile(todo)
	if os.IsNotExist(err) {
		var b bytes.Buffer
		header := src[:fset.Position(f.Package).Offset]
		// Keep the license header, drop build constraints.
		for _, l := range strings.SplitAfter(string(header), "\n") {
			if !strings.HasPrefix(l, "//go:build") {
				b.WriteString(l)
			}
		}
		hdr := strings.TrimLeft(b.String(), "\n")
		b.Reset()
		fmt.Fprintf(&b, "//go:build %s\n\n%s", todoTag, hdr)
		fmt.Fprintf(&b, "// Code imported from provider-aws that does not compile yet. Port it, move\n// it to %s and delete this file.\n\n", filepath.Base(file))
		end := fset.Position(f.Name.End()).Offset
		b.Write(src[fset.Position(f.Package).Offset:end])
		b.WriteString("\n")
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
				b.WriteString("\n")
				b.Write(src[fset.Position(g.Pos()).Offset:fset.Position(g.End()).Offset])
				b.WriteString("\n")
			}
		}
		existing = b.Bytes()
	} else if err != nil {
		return err
	}
	return os.WriteFile(todo, append(existing, moved...), 0o600)
}

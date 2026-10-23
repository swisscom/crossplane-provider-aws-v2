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
	"go/printer"
	"go/token"
	"go/types"
	"maps"
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

	"github.com/swisscom/crossplane-provider-aws-v2/codegen/internal/generator"
)

const (
	topicTodo = "Not compiling yet (importtodo)"
	todoTag   = "importtodo"
)

// maxRounds bounds the type check loop; a var so tests can lower it.
var maxRounds = 40

var couldNotImportRe = regexp.MustCompile(`could not import (\S+)`)

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
	// helpers are the source helper packages ported in this run.
	helpers []string
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

// typecheck type checks the ported packages and quarantines declarations
// of files written by this import that do not compile. Missing SDK
// operations of a generated Client interface are added to codegen.yaml
// instead. Any error it cannot quarantine fails the import.
func (im *importer) typecheck() error { //nolint:gocyclo // a fixpoint loop
	dirs := map[string]bool{}
	for _, f := range im.ported {
		dirs[filepath.Dir(f.path)] = true
	}
	if len(dirs) == 0 {
		return nil // nothing was ported, e.g. on a re-import
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

	for round := 0; ; round++ {
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
		if round == maxRounds {
			return fmt.Errorf("type check did not converge after %d rounds; remaining errors:\n\t%s", maxRounds, im.errorList(errs))
		}
		failing := map[string]bool{}
		for _, e := range errs {
			failing[e.dir] = true
		}
		quarantine := map[string][]loadError{}
		ops := map[string][]string{}
		var left []loadError
		for _, e := range errs {
			if m := couldNotImportRe.FindStringSubmatch(e.msg); m != nil {
				if rel, ok := strings.CutPrefix(m[1], im.tgt.module+"/"); ok && failing[filepath.Join(im.tgt.root, rel)] {
					continue // fixed once the imported package compiles
				}
			}
			if op := im.missingClientOp(e.msg); op != "" && kindOf[e.dir] != "" {
				k := kindOf[e.dir]
				if !slices.Contains(ops[k], op) {
					ops[k] = append(ops[k], op)
				}
				continue
			}
			left = append(left, e)
			if e.file == "" || strings.HasPrefix(filepath.Base(e.file), "zz_") || !im.isPorted(e.file) {
				continue
			}
			quarantine[e.file] = append(quarantine[e.file], e)
		}
		progress := false
		if len(ops) > 0 {
			if err := im.addClientOps(ops); err != nil {
				return err
			}
			progress = true
		}
		for _, file := range slices.Sorted(maps.Keys(quarantine)) {
			n, err := im.quarantine(file, quarantine[file])
			if err != nil {
				return err
			}
			progress = progress || n > 0
		}
		if !progress {
			if len(left) == 0 {
				left = errs
			}
			return fmt.Errorf("%d error(s) the importer cannot quarantine:\n\t%s", len(left), im.errorList(left))
		}
	}
}

// errorList renders errors sorted, one per line.
func (im *importer) errorList(errs []loadError) string {
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		out = append(out, e.String(im))
	}
	sort.Strings(out)
	return strings.Join(slices.Compact(out), "\n\t")
}

// isPorted reports whether this import wrote file. Only those files may be
// quarantined; everything else was written or reviewed by a human.
func (im *importer) isPorted(file string) bool {
	for _, f := range im.ported {
		if f.path == file {
			return true
		}
	}
	return false
}

// loadError is a type check error. file is empty for errors without a
// position, e.g. a package name clash.
type loadError struct {
	dir       string
	file      string
	line, col int
	msg       string
}

func (e loadError) String(im *importer) string {
	if e.file == "" {
		return fmt.Sprintf("%s: %s", im.rel(e.dir), e.msg)
	}
	s := fmt.Sprintf("%s:%d:%d: %s", im.rel(e.file), e.line, e.col, e.msg)
	if !strings.HasPrefix(filepath.Base(e.file), "zz_") && !im.isPorted(e.file) {
		s += " (not written by this import)"
	}
	return s
}

var errPosRe = regexp.MustCompile(`^(.+?):(\d+)(?::(\d+))?$`)

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
		dir := p.Dir
		if dir == "" && len(p.GoFiles) > 0 {
			dir = filepath.Dir(p.GoFiles[0])
		}
		for _, e := range p.Errors {
			for _, le := range im.loadErrors(dir, e) {
				k := fmt.Sprintf("%s|%s:%d:%d:%s", le.dir, le.file, le.line, le.col, le.msg)
				if !seen[k] {
					seen[k] = true
					out = append(out, le)
				}
			}
		}
	}
	return out, nil
}

var compilerLineRe = regexp.MustCompile(`^(\S+\.go):(\d+)(?::(\d+))?: (.*)$`)

// loadErrors converts a package error. The go command reports compiler
// output as one error without a position ("# pkg\nfile:line:col: msg"); its
// lines are split into positioned errors so they can be quarantined.
func (im *importer) loadErrors(dir string, e packages.Error) []loadError {
	if strings.HasPrefix(e.Msg, "\t") {
		return nil // a note of the previous error, e.g. "other declaration of"
	}
	if m := errPosRe.FindStringSubmatch(e.Pos); m != nil {
		le := loadError{dir: filepath.Dir(m[1]), file: m[1], msg: e.Msg}
		le.line, _ = strconv.Atoi(m[2])
		le.col, _ = strconv.Atoi(m[3])
		return []loadError{le}
	}
	if strings.HasPrefix(e.Msg, "# ") {
		var out []loadError
		for _, l := range strings.Split(e.Msg, "\n")[1:] {
			if strings.HasPrefix(l, "\t") {
				continue
			}
			m := compilerLineRe.FindStringSubmatch(l)
			if m == nil {
				out = nil
				break
			}
			f := m[1]
			if !filepath.IsAbs(f) {
				f = filepath.Join(im.tgt.root, f)
			}
			le := loadError{dir: filepath.Dir(f), file: f, msg: m[4]}
			le.line, _ = strconv.Atoi(m[2])
			le.col, _ = strconv.Atoi(m[3])
			out = append(out, le)
		}
		if len(out) > 0 {
			return out
		}
	}
	return []loadError{{dir: dir, msg: e.Msg}}
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
	for _, k := range slices.Sorted(maps.Keys(ops)) {
		for _, op := range ops[k] {
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
	b = append([]byte("# provider-aws-v2 codegen settings of this service, see docs/architecture.md.\n"), b...)
	if err := os.WriteFile(generator.ServiceConfigPath(im.tgt.root, im.o.Service), b, 0o600); err != nil {
		return err
	}
	return generator.Run(im.ctx, im.o.Options)
}

// quarantine handles the declarations of file that have errors. A function
// or method whose errors are all inside its body keeps its signature and
// gets a body that panics, so callers and overrides stay in place and fail
// loudly. Everything else is moved. The originals go to the importtodo file.
// It returns the number of changes.
func (im *importer) quarantine(file string, errs []loadError) (int, error) { //nolint:gocyclo // AST bookkeeping
	src, err := os.ReadFile(file) //nolint:gosec // a file this import wrote
	if err != nil {
		return 0, err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, parser.ParseComments)
	if err != nil {
		return 0, err
	}
	off := func(p token.Pos) int { return fset.Position(p).Offset }
	todo := filepath.Base(todoPath(file))
	test := strings.HasSuffix(file, "_test.go")

	problems := map[ast.Decl][]loadError{}
	var badImports []string
	for _, d := range f.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
			for _, s := range g.Specs {
				is := s.(*ast.ImportSpec)
				l := fset.Position(is.Pos()).Line
				if slices.ContainsFunc(errs, func(e loadError) bool { return e.line == l }) {
					badImports = append(badImports, is.Path.Value)
				}
			}
			continue
		}
		from, to := fset.Position(d.Pos()), fset.Position(d.End())
		for _, e := range errs {
			if e.within(from, to) {
				problems[d] = append(problems[d], e)
			}
		}
	}
	// Uses of an import that cannot be resolved don't compile either.
	for _, ip := range badImports {
		names := importNamesOf(f, ip)
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
				continue
			}
			if p := firstUse(d, names...); p.IsValid() {
				pos := fset.Position(p)
				problems[d] = append(problems[d], loadError{file: file, line: pos.Line, col: pos.Column, msg: "cannot import " + ip})
			}
		}
	}

	stubbable := func(d ast.Decl) bool {
		fd, ok := d.(*ast.FuncDecl)
		return ok && !test && fd.Body != nil && inBody(fset, fd.Body, problems[d])
	}
	// A moved method of a type that embeds another would let the embedded
	// method run in its place, so the type and its methods move with it.
	withRecv := map[ast.Decl]string{}
	var elsewhere [][2]string // receiver type, method
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || len(problems[d]) == 0 || stubbable(d) {
			continue
		}
		t := recvType(fd)
		td, emb := embeddingType(f, t)
		if td == nil {
			elsewhere = append(elsewhere, [2]string{t, fd.Name.Name})
			continue
		}
		why := fmt.Sprintf("its method `%s` does not compile and `%s` embeds `%s`, so the embedded `%s` would run instead: %s", fd.Name.Name, t, emb, fd.Name.Name, oneLine(earliest(problems[d]).msg))
		if _, done := withRecv[td]; !done {
			im.report.Block(topicTodo, "`%s`: type `%s` and its methods moved to `%s`: %s", im.rel(file), t, todo, why)
		}
		withRecv[td] = why
		for _, m := range f.Decls {
			if md, ok := m.(*ast.FuncDecl); ok && m != d && recvType(md) == t {
				withRecv[m] = why
			}
		}
	}

	type edit struct {
		start, end int
		repl       string
	}
	var edits []edit
	var moved bytes.Buffer
	for _, d := range f.Decls {
		ps := problems[d]
		why, forced := withRecv[d]
		if len(ps) == 0 && !forced {
			continue
		}
		start := d.Pos()
		if doc := docOf(d); doc != nil {
			start = doc.Pos()
		}
		if len(ps) == 0 {
			fmt.Fprintf(&moved, "\n// TODO(import): moved with its type: %s\n%s\n", why, src[off(start):off(d.End())])
			edits = append(edits, edit{start: off(start), end: off(d.End())})
			continue
		}
		e := earliest(ps)
		fmt.Fprintf(&moved, "\n// TODO(import): %s\n%s\n", oneLine(e.msg), src[off(start):off(d.End())])
		sum := declSummaryOf(fset, d, src[off(d.Pos()):off(d.End())])
		if fd, ok := d.(*ast.FuncDecl); ok && !forced && stubbable(d) {
			msg := fmt.Sprintf("TODO(import): %s does not compile yet; port it from %s", declKey(fd), todo)
			edits = append(edits, edit{start: off(fd.Body.Lbrace), end: off(fd.Body.Rbrace) + 1, repl: fmt.Sprintf("{\n\tpanic(%q)\n}", msg)})
			im.report.Block(topicTodo, "`%s`: `%s`: body moved to `%s`; the stub panics until it is ported: %s", im.rel(file), sum, todo, oneLine(e.msg))
			continue
		}
		edits = append(edits, edit{start: off(start), end: off(d.End())})
		im.report.Block(topicTodo, "`%s`: `%s` moved to `%s`: %s", im.rel(file), sum, todo, oneLine(e.msg))
	}
	if len(edits) == 0 && len(badImports) == 0 {
		return 0, nil
	}
	out := src
	for i := len(edits) - 1; i >= 0; i-- {
		out = slices.Concat(out[:edits[i].start], []byte(edits[i].repl), out[edits[i].end:])
	}
	out, err = pruneImports(file, out, badImports)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(file, out, 0o600); err != nil {
		return 0, err
	}
	im.todo[file] += len(edits)
	if err := im.appendTodo(file, src, f, fset, moved.Bytes()); err != nil {
		return 0, err
	}
	n := len(edits) + len(badImports)
	for _, rm := range elsewhere {
		m, err := im.moveEmbeddingType(file, rm[0], rm[1])
		if err != nil {
			return 0, err
		}
		n += m
	}
	return n, nil
}

// moveEmbeddingType moves type t from another file of this import in the
// same package, if it embeds a type, so its embedded method can't be
// promoted in place of the moved method.
func (im *importer) moveEmbeddingType(from, t, method string) (int, error) {
	for _, pf := range im.ported {
		if pf.path == from || filepath.Dir(pf.path) != filepath.Dir(from) {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, pf.path, nil, parser.SkipObjectResolution)
		if err != nil {
			return 0, err
		}
		td, emb := embeddingType(f, t)
		if td == nil {
			continue
		}
		why := fmt.Sprintf("its method `%s` (in `%s`) does not compile and `%s` embeds `%s`, so the embedded `%s` would run instead", method, filepath.Base(from), t, emb, method)
		return im.quarantine(pf.path, []loadError{{file: pf.path, line: fset.Position(td.Pos()).Line, msg: why}})
	}
	return 0, nil
}

// embeddingType returns the declaration of struct type name in f and the
// first type it embeds, or nil if f does not declare such a type.
func embeddingType(f *ast.File, name string) (ast.Decl, string) {
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.TYPE {
			continue
		}
		for _, s := range g.Specs {
			ts := s.(*ast.TypeSpec)
			st, ok := ts.Type.(*ast.StructType)
			if !ok || ts.Name.Name != name {
				continue
			}
			for _, fl := range st.Fields.List {
				if len(fl.Names) == 0 {
					return d, types.ExprString(fl.Type)
				}
			}
		}
	}
	return nil, ""
}

// within reports whether e lies between from and to. Without a column,
// the line decides.
func (e loadError) within(from, to token.Position) bool {
	if e.line < from.Line || e.line > to.Line {
		return false
	}
	if e.col == 0 {
		return true
	}
	return (e.line > from.Line || e.col >= from.Column) && (e.line < to.Line || e.col <= to.Column)
}

// inBody reports whether every error lies inside body. An error without a
// column counts as outside, so its declaration is moved, not stubbed.
func inBody(fset *token.FileSet, body *ast.BlockStmt, errs []loadError) bool {
	l, r := fset.Position(body.Lbrace), fset.Position(body.Rbrace)
	for _, e := range errs {
		if e.col == 0 || !e.within(l, r) || (e.line == l.Line && e.col == l.Column) || (e.line == r.Line && e.col == r.Column) {
			return false
		}
	}
	return true
}

// earliest returns the error with the smallest position, so the message of
// a declaration does not depend on map order.
func earliest(errs []loadError) loadError {
	return slices.MinFunc(errs, func(a, b loadError) int {
		if a.line != b.line {
			return a.line - b.line
		}
		if a.col != b.col {
			return a.col - b.col
		}
		return strings.Compare(a.msg, b.msg)
	})
}

func importNamesOf(f *ast.File, quoted string) []string {
	for _, is := range f.Imports {
		if is.Path.Value == quoted {
			return importNames(is)
		}
	}
	return nil
}

// firstUse returns the position of the first pkg.X selector in n.
func firstUse(n ast.Node, pkgs ...string) token.Pos {
	var pos token.Pos
	ast.Inspect(n, func(n ast.Node) bool {
		if s, ok := n.(*ast.SelectorExpr); ok && !pos.IsValid() {
			if id, ok := s.X.(*ast.Ident); ok && slices.Contains(pkgs, id.Name) {
				pos = s.Pos()
			}
		}
		return !pos.IsValid()
	})
	return pos
}

// declSummaryOf returns the signature of a function, or the first line of
// another declaration.
func declSummaryOf(fset *token.FileSet, d ast.Decl, b []byte) string {
	if fd, ok := d.(*ast.FuncDecl); ok {
		c := *fd
		c.Doc, c.Body = nil, nil
		var s strings.Builder
		if err := printer.Fprint(&s, fset, &c); err == nil {
			return oneLine(s.String())
		}
	}
	return declSummary(b)
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
		names := importNames(is)
		if names[0] == "_" || names[0] == "." {
			continue
		}
		if slices.Contains(bad, is.Path.Value) || !slices.ContainsFunc(names, func(n string) bool { return usesPkg(f, n) }) {
			n := ""
			if is.Name != nil {
				n = is.Name.Name
			}
			astutil.DeleteNamedImport(fset, f, n, p)
		}
	}
	return formatFile(fset, f)
}

// todoPath returns the importtodo file of file: x.go -> x_importtodo.go,
// x_test.go -> x_importtodo_test.go.
func todoPath(file string) string {
	base := strings.TrimSuffix(filepath.Base(file), ".go")
	test := strings.HasSuffix(base, "_test")
	base = strings.TrimSuffix(base, "_test") + "_" + todoTag
	if test {
		base += "_test"
	}
	return filepath.Join(filepath.Dir(file), base+".go")
}

// appendTodo appends moved declarations to the importtodo file of file.
func (im *importer) appendTodo(file string, src []byte, f *ast.File, fset *token.FileSet, moved []byte) error {
	todo := todoPath(file)
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

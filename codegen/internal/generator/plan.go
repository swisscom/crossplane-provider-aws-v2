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
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	awssdkmodel "github.com/aws-controllers-k8s/code-generator/pkg/api"
	ackmodel "github.com/aws-controllers-k8s/code-generator/pkg/model"
)

// kindPlan is what the templates need to emit safe code for one kind. It is
// computed and checked before anything is rendered.
type kindPlan struct {
	Config KindConfig
	// Identifier is the member that carries the external name; required for
	// ReadMany kinds, optional (configured) for ReadOne kinds.
	Identifier *identifierPlan
	// ListMember is the list of a ReadMany output that holds the resources.
	ListMember string
	// ReadWrapper and CreateWrapper are the member paths ACK reads a
	// ReadOne or Create output from, e.g. [DBInstance].
	ReadWrapper   []string
	CreateWrapper []string
	// DeleteDefault is the default delete hook of a kind without a delete
	// operation; IsUpToDateHookRequired marks GetAttributes kinds that can update.
	DeleteDefault          string
	IsUpToDateHookRequired bool
	// ImmutableFields are the spec fields (ACK names) CEL keeps unchanged;
	// OptionalImmutable are the JSON names of those that are optional.
	ImmutableFields   map[string]bool
	OptionalImmutable []string
}

// identifierPlan says where the external name goes in the operation inputs
// and where it shows up in the read output.
type identifierPlan struct {
	ElemMember   string // member of the read output element, e.g. DBInstanceIdentifier
	InputMember  string // member of the read input set from the external name
	InputIsList  bool   // the read input takes a list, e.g. VpcIds
	UpdateMember string // "" if the update input has no such member
	DeleteMember string // "" if the delete input has no such member
	SpecField    string // spec field with the same name, if any
}

// planKinds checks every kind against codegen.yaml and returns the plans,
// keyed by kind. It reports all problems at once, so one run lists every
// config line to write.
func planKinds(m *ackmodel.Model, raw rawMembers, sc *ServiceConfig, genCfg, scPath string) (map[string]*kindPlan, error) {
	crds, err := m.GetCRDs()
	if err != nil {
		return nil, err
	}
	tds, err := m.GetTypeDefs()
	if err != nil {
		return nil, err
	}
	names := newJSONNames(tds)
	var errs []error
	plans := map[string]*kindPlan{}
	for _, crd := range crds {
		kc := sc.Kinds[crd.Kind]
		if kindErrs := checkKind(factsOf(crd), kc, genCfg); len(kindErrs) > 0 {
			// The plan of an unsafe kind would be unusable; report why only.
			errs = append(errs, kindErrs...)
			plans[crd.Kind] = &kindPlan{Config: kc}
			continue
		}
		p, err := planKind(crd, raw, kc, scPath)
		if err != nil {
			errs = append(errs, err)
		}
		errs = append(errs, planImmutable(crd, p)...)
		plans[crd.Kind] = p
		errs = append(errs, checkSensitiveFields(crd, names, raw, kc, scPath)...)
	}
	for _, k := range slices.Sorted(maps.Keys(sc.Kinds)) {
		if plans[k] == nil {
			errs = append(errs, fmt.Errorf("%s: kinds.%s is not a generated kind", scPath, k))
		}
	}
	return plans, errors.Join(errs...)
}

func planKind(crd *ackmodel.CRD, raw rawMembers, kc KindConfig, scPath string) (*kindPlan, error) {
	p := &kindPlan{Config: kc, CreateWrapper: outputWrapper(crd, crd.Ops.Create)}
	switch {
	case kc.OrphanOnDelete:
		p.DeleteDefault = "orphanDelete"
	case kc.CustomDelete:
		p.DeleteDefault = "notImplementedDelete"
	}
	p.IsUpToDateHookRequired = kc.CustomIsUpToDate
	op, readMany := readOperation(crd)
	if op != nil && !readMany {
		p.ReadWrapper = outputWrapper(crd, op)
	}
	if op == nil || (!readMany && kc.Identifier == "") {
		return p, nil
	}
	var elem *awssdkmodel.Shape
	if readMany {
		member, e, err := readManyList(op, crd.GetOutputWrapperFieldPath(op))
		if err != nil {
			return p, fmt.Errorf("%s: %w", crd.Kind, err)
		}
		p.ListMember, elem = member, e
	} else {
		e, err := crd.GetOutputShape(op)
		if err != nil {
			return p, fmt.Errorf("%s: %w", crd.Kind, err)
		}
		elem = e
	}
	id, err := resolveIdentifier(crd.Names.Original, kc, rawShape{op.InputRef.Shape.ShapeName, raw[op.InputRef.Shape.ShapeName]},
		rawShape{elem.ShapeName, raw[elem.ShapeName]}, inputMembers(raw, crd.Ops.Update), inputMembers(raw, crd.Ops.Delete))
	if err != nil {
		return p, fmt.Errorf("%s: %w; set kinds.%s.identifier in %s", crd.Kind, err, crd.Kind, scPath)
	}
	if _, ok := crd.SpecFields[id.ElemMember]; ok {
		id.SpecField = id.ElemMember
	}
	p.Identifier = id
	return p, nil
}

// rawShape is a shape's name and its members before ignore rules.
type rawShape struct {
	name    string
	members map[string]*awssdkmodel.ShapeRef
}

func inputMembers(raw rawMembers, op *awssdkmodel.Operation) map[string]*awssdkmodel.ShapeRef {
	if op == nil || op.InputRef.Shape == nil {
		return nil
	}
	return raw[op.InputRef.Shape.ShapeName]
}

// resolveIdentifier finds the member that holds the external name: a string
// member of the read output that the read input can filter by. Members are
// taken before ignore rules, because identifiers are usually ignored inputs.
func resolveIdentifier(kind string, kc KindConfig, readIn, elem rawShape, updIn, delIn map[string]*awssdkmodel.ShapeRef) (*identifierPlan, error) {
	candidates := []string{kind + "Identifier", kind + "Id", kind + "Name"}
	if kc.Identifier != "" {
		candidates = []string{kc.Identifier}
	}
	var found []*identifierPlan
	for _, c := range candidates {
		if !isStringRef(elem.members[c]) {
			continue
		}
		in, isList, ok := identifierInput(readIn.members, c, kc.IdentifierInput)
		if !ok {
			continue
		}
		id := &identifierPlan{ElemMember: c, InputMember: in, InputIsList: isList}
		if isStringRef(updIn[c]) {
			id.UpdateMember = c
		}
		if isStringRef(delIn[c]) {
			id.DeleteMember = c
		}
		found = append(found, id)
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return nil, fmt.Errorf("cannot tell which field holds the external name: none of %s is a string member of both %s and %s",
			strings.Join(candidates, ", "), elem.name, readIn.name)
	}
	var names []string
	for _, f := range found {
		names = append(names, f.ElemMember)
	}
	return nil, fmt.Errorf("ambiguous identifier (%s all match)", strings.Join(names, ", "))
}

// identifierInput returns the read input member that takes the identifier
// c: the configured one, c itself, or the list c+"s".
func identifierInput(in map[string]*awssdkmodel.ShapeRef, c, configured string) (string, bool, bool) {
	names := []string{c, c + "s"}
	if configured != "" {
		names = []string{configured}
	}
	for _, n := range names {
		switch {
		case isStringRef(in[n]):
			return n, false, true
		case in[n] != nil && in[n].Shape != nil && in[n].Shape.Type == shapeList && isStringRef(&in[n].Shape.MemberRef):
			return n, true, true
		}
	}
	return "", false, false
}

func isStringRef(r *awssdkmodel.ShapeRef) bool {
	return r != nil && r.Shape != nil && r.Shape.Type == shapeString
}

// readManyList returns the list member of a ReadMany output that holds the
// resources, and its element shape. A configured output wrapper wins;
// otherwise the output must have exactly one list.
func readManyList(op *awssdkmodel.Operation, w *string) (string, *awssdkmodel.Shape, error) {
	out := op.OutputRef.Shape
	if out == nil {
		return "", nil, fmt.Errorf("%s has no output", op.ExportedName)
	}
	if w != nil {
		ref, ok := out.MemberRefs[*w]
		if !ok || ref.Shape.Type != shapeList {
			return "", nil, fmt.Errorf("output_wrapper_field_path %s of %s is not a list member", *w, op.ExportedName)
		}
		return *w, ref.Shape.MemberRef.Shape, nil
	}
	var lists []string
	for _, m := range out.MemberNames() {
		if out.MemberRefs[m].Shape.Type == shapeList {
			lists = append(lists, m)
		}
	}
	if len(lists) == 0 {
		return "", nil, fmt.Errorf("the output of %s has no list", op.ExportedName)
	}
	return lists[0], out.MemberRefs[lists[0]].Shape.MemberRef.Shape, nil
}

// outputWrapper returns the member path ACK's SetResource reads an output
// from: the configured wrapper, or the single structure member of the output.
func outputWrapper(crd *ackmodel.CRD, op *awssdkmodel.Operation) []string {
	if op == nil || op.OutputRef.Shape == nil {
		return nil
	}
	out := op.OutputRef.Shape
	if w := crd.GetOutputWrapperFieldPath(op); w != nil {
		var path []string
		s := out
		for _, m := range strings.Split(*w, ".") {
			ref, ok := s.MemberRefs[m]
			if !ok || ref.Shape.Type != shapeStructure {
				// Lists are looped over, which is nil-safe.
				return path
			}
			path, s = append(path, m), ref.Shape
		}
		return path
	}
	if out.UsedAsOutput && len(out.MemberRefs) == 1 {
		for m, ref := range out.MemberRefs {
			if ref.Shape.Type == shapeStructure {
				return []string{m}
			}
		}
	}
	return nil
}

// nilCheck returns a Go condition that is true when v or any member of the
// wrapper path is nil (or, with notNil, when none is).
func nilCheck(v string, wrapper []string, notNil bool) string {
	op, join := " == nil", " || "
	if notNil {
		op, join = " != nil", " && "
	}
	conds := make([]string, 0, 1+len(wrapper))
	conds = append(conds, v+op)
	for _, m := range wrapper {
		v += "." + m
		conds = append(conds, v+op)
	}
	return strings.Join(conds, join)
}

// planImmutable collects the identifier spec field and the configured
// immutableFields of a kind.
func planImmutable(crd *ackmodel.CRD, p *kindPlan) []error {
	byJSON := map[string]string{}
	for name, f := range crd.SpecFields {
		byJSON[f.Names.CamelLower] = name
	}
	fields := map[string]bool{}
	if p.Identifier != nil && p.Identifier.SpecField != "" {
		fields[p.Identifier.SpecField] = true
	}
	var errs []error
	for _, j := range p.Config.ImmutableFields {
		name, ok := byJSON[j]
		if !ok {
			errs = append(errs, fmt.Errorf("%s: immutableFields entry %s is not a spec.forProvider field", crd.Kind, j))
			continue
		}
		fields[name] = true
	}
	p.ImmutableFields = fields
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		if f := crd.SpecFields[name]; !f.IsRequired() {
			p.OptionalImmutable = append(p.OptionalImmutable, f.Names.CamelLower)
		}
	}
	return errs
}

var (
	readManyLoop  = regexp.MustCompile(`(?m)^(\t*)for _, elem := range (\S+) \{\n`)
	readManyBreak = regexp.MustCompile(`(?m)^(\t*)found = true\n\t*break\n`)
)

// firstElementOnly rewrites ACK's ReadMany mapping, a loop that always
// breaks after the first element, into an if, which staticcheck accepts.
// An unexpected shape fails generation instead of being skipped silently.
func firstElementOnly(kind, snippet string) (string, error) {
	if len(readManyLoop.FindAllString(snippet, -1)) != 1 || len(readManyBreak.FindAllString(snippet, -1)) != 1 {
		return "", fmt.Errorf("unexpected ACK ReadMany code for %s; update firstElementOnly", kind)
	}
	snippet = readManyLoop.ReplaceAllString(snippet, "${1}if len($2) > 0 {\n${1}\telem := $2[0]\n")
	return readManyBreak.ReplaceAllString(snippet, "${1}found = true\n"), nil
}

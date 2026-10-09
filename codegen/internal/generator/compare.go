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
	"slices"
	"strings"

	awssdkmodel "github.com/aws-controllers-k8s/code-generator/pkg/api"
	ackmodel "github.com/aws-controllers-k8s/code-generator/pkg/model"
)

// UpdatableField is a spec field that the update operation sends and that
// Generate<Kind> sets from the read operation, so desired and observed
// values can be compared.
type UpdatableField struct {
	GoName   string
	JSONName string
}

// readOperation returns the operation Generate<Kind> uses (same order as
// the controller template), or nil if it can't be compared field by field.
func readOperation(r *ackmodel.CRD) (*awssdkmodel.Operation, bool) {
	switch {
	case r.Ops.ReadOne != nil:
		return r.Ops.ReadOne, false
	case r.Ops.GetAttributes != nil:
		return nil, false
	case r.Ops.ReadMany != nil:
		return r.Ops.ReadMany, true
	}
	return nil, false
}

// HasUpdatableFields returns whether DiffUpdatableParameters is generated
// for the CRD.
func HasUpdatableFields(r *ackmodel.CRD) bool {
	op, _ := readOperation(r)
	return r.Ops.Update != nil && op != nil
}

// UpdatableFields returns the spec fields compared by the generated
// DiffUpdatableParameters: members of the update input that the read
// output also has (after renames), minus fields with compare.is_ignored or
// listed in resources.<Kind>.compare.ignore of generator-config.yaml.
func UpdatableFields(r *ackmodel.CRD) ([]UpdatableField, error) {
	readOp, readMany := readOperation(r)
	if r.Ops.Update == nil || readOp == nil {
		return nil, nil
	}
	in, err := r.GetInputShape(r.Ops.Update)
	if err != nil {
		return nil, err
	}
	out, err := readOutputShape(r, readOp, readMany)
	if err != nil || out == nil {
		return nil, err
	}
	cfg := r.Config()
	updated := map[string]bool{}
	for _, m := range in.MemberNames() {
		updated[cfg.GetResourceFieldName(r.Names.Original, r.Ops.Update.ExportedName, m)] = true
	}
	ignored := r.CompareIgnoredFields()

	var fields []UpdatableField
	for _, m := range out.MemberNames() {
		name := cfg.GetResourceFieldName(r.Names.Original, readOp.ExportedName, m)
		f, ok := r.SpecFields[name]
		if !ok || !updated[name] {
			continue
		}
		if f.FieldConfig != nil && f.FieldConfig.Compare != nil && f.FieldConfig.Compare.IsIgnored {
			continue
		}
		if slices.Contains(ignored, name) || slices.Contains(ignored, "Spec."+name) {
			continue
		}
		fields = append(fields, UpdatableField{GoName: f.Names.Camel, JSONName: f.Names.CamelLower})
	}
	slices.SortFunc(fields, func(a, b UpdatableField) int { return strings.Compare(a.GoName, b.GoName) })
	return fields, nil
}

// readOutputShape returns the shape Generate<Kind> reads fields from: the
// (unwrapped) output of a ReadOne operation, or the list element of a
// ReadMany operation, found like ACK's SetResource does.
func readOutputShape(r *ackmodel.CRD, op *awssdkmodel.Operation, readMany bool) (*awssdkmodel.Shape, error) {
	if !readMany {
		return r.GetOutputShape(op)
	}
	if r.GetOutputWrapperFieldPath(op) != nil {
		s, err := r.GetOutputShape(op)
		if err != nil {
			return nil, err
		}
		if s.Type == "list" {
			return s.MemberRef.Shape, nil
		}
		return s, nil
	}
	if op.OutputRef.Shape == nil {
		return nil, nil
	}
	for _, m := range op.OutputRef.Shape.MemberNames() {
		if ref := op.OutputRef.Shape.MemberRefs[m]; ref.Shape.Type == "list" {
			return ref.Shape.MemberRef.Shape, nil
		}
	}
	return nil, nil
}

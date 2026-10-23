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
	"fmt"
	"strings"

	ackmodel "github.com/aws-controllers-k8s/code-generator/pkg/model"
)

type readKind int

const (
	readNone readKind = iota
	readOne
	readMany
	readGetAttributes
)

// kindFacts is what checkKind needs to know about a kind's operations.
type kindFacts struct {
	Kind                       string
	Read                       readKind
	ReadOp, UpdateOp, DeleteOp string
	NotFoundCode               string
	ReadManyLists              []string
	ReadManyWrapper            *string
}

func factsOf(crd *ackmodel.CRD) kindFacts {
	f := kindFacts{Kind: crd.Kind, NotFoundCode: crd.ExceptionCode(404)}
	switch {
	case crd.Ops.ReadOne != nil:
		f.Read, f.ReadOp = readOne, crd.Ops.ReadOne.ExportedName
	case crd.Ops.GetAttributes != nil:
		f.Read, f.ReadOp = readGetAttributes, crd.Ops.GetAttributes.ExportedName
	case crd.Ops.ReadMany != nil:
		op := crd.Ops.ReadMany
		f.Read, f.ReadOp = readMany, op.ExportedName
		f.ReadManyWrapper = crd.GetOutputWrapperFieldPath(op)
		if out := op.OutputRef.Shape; out != nil {
			for _, m := range out.MemberNames() {
				if out.MemberRefs[m].Shape.Type == shapeList {
					f.ReadManyLists = append(f.ReadManyLists, m)
				}
			}
		}
	}
	if crd.Ops.Update != nil {
		f.UpdateOp = crd.Ops.Update.ExportedName
	}
	if crd.Ops.Delete != nil {
		f.DeleteOp = crd.Ops.Delete.ExportedName
	}
	return f
}

// checkKind fails for operation sets whose generated defaults would be
// unsafe, unless codegen.yaml opts in explicitly. genCfg is the generator
// config named in the messages.
func checkKind(f kindFacts, kc KindConfig, genCfg string) []error {
	k := f.Kind
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(k+": "+format, a...)) }

	switch {
	case f.Read == readNone && !kc.CustomObserve:
		add("the API model has no read operation, so the generated Observe can't tell whether the resource exists; add one in generator-config.yaml or set kinds.%s.customObserve: true and write an observe hook", k)
	case f.Read != readNone && kc.CustomObserve:
		add("kinds.%s.customObserve is set but %s is a read operation; remove it", k, f.ReadOp)
	}

	switch {
	case kc.CustomDelete && kc.OrphanOnDelete, f.DeleteOp != "" && (kc.CustomDelete || kc.OrphanOnDelete):
		add("kinds.%s: customDelete and orphanOnDelete are exclusive and need a kind without a delete operation", k)
	case f.DeleteOp == "" && !kc.CustomDelete && !kc.OrphanOnDelete:
		add("the API model has no delete operation, so deleting the managed resource would leave the AWS resource behind; set kinds.%s.customDelete: true (and write a delete hook) or kinds.%s.orphanOnDelete: true", k, k)
	}

	if f.Read != readNone && f.NotFoundCode == "UNKNOWN" {
		add(`no "not found" error code: neither the API model nor generator-config.yaml has one; set resources.%s.exceptions.errors.404.code in %s`, k, genCfg)
	}

	if f.Read == readMany {
		switch {
		case f.ReadManyWrapper != nil && strings.Contains(*f.ReadManyWrapper, "."):
			add("nested output_wrapper_field_path %s is not supported", *f.ReadManyWrapper)
		case f.ReadManyWrapper == nil && len(f.ReadManyLists) > 1:
			add("the output of %s has several lists (%s); set operations.%s.output_wrapper_field_path in %s", f.ReadOp, strings.Join(f.ReadManyLists, ", "), f.ReadOp, genCfg)
		}
	}

	needsHook := f.Read == readGetAttributes && f.UpdateOp != ""
	switch {
	case needsHook && !kc.CustomIsUpToDate:
		add("%s is a GetAttributes operation, so no drift check is generated although %s can update the resource; set kinds.%s.customIsUpToDate: true and write an isUpToDate hook", f.ReadOp, f.UpdateOp, k)
	case !needsHook && kc.CustomIsUpToDate:
		add("kinds.%s.customIsUpToDate is only for GetAttributes kinds with an update operation; remove it", k)
	}
	return errs
}

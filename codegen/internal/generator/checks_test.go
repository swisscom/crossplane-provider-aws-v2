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
	"strings"
	"testing"
)

// ignoreOperations adds ignore.operations to a fixture config.
func ignoreOperations(ops ...string) func(string) string {
	return func(cfg string) string {
		return strings.Replace(cfg, "ignore:\n", "ignore:\n  operations:\n    - "+strings.Join(ops, "\n    - ")+"\n", 1)
	}
}

const subnetGroupDir = "internal/controller/docdb/dbsubnetgroup/"

func TestRunKindOperationChecks(t *testing.T) {
	t.Run("no read operation", func(t *testing.T) {
		_, err := docdbFixture.run(t, ignoreOperations("DescribeDBSubnetGroups"), "")

		errContains(t, err, "DBSubnetGroup: the API model has no read operation, so the generated Observe can't tell whether the resource exists; add one in generator-config.yaml or set kinds.DBSubnetGroup.customObserve: true and write an observe hook")
	})
	t.Run("customObserve", func(t *testing.T) {
		out := docdbFixture.mustRun(t, ignoreOperations("DescribeDBSubnetGroups"), "kinds:\n  DBSubnetGroup:\n    customObserve: true\n")
		ctrl := readOut(t, out, subnetGroupDir+"zz_controller.go")

		contains(t, "zz_controller.go", ctrl, "notImplementedObserve,", `errors.New("Observe is not implemented for DBSubnetGroup: set the observe hook")`)
	})
	t.Run("no delete operation", func(t *testing.T) {
		_, err := docdbFixture.run(t, ignoreOperations("DeleteDBSubnetGroup"), "")

		errContains(t, err, "DBSubnetGroup: the API model has no delete operation, so deleting the managed resource would leave the AWS resource behind; set kinds.DBSubnetGroup.customDelete: true (and write a delete hook) or kinds.DBSubnetGroup.orphanOnDelete: true")
	})
	t.Run("orphanOnDelete", func(t *testing.T) {
		out := docdbFixture.mustRun(t, ignoreOperations("DeleteDBSubnetGroup"), "kinds:\n  DBSubnetGroup:\n    orphanOnDelete: true\n")
		ctrl := readOut(t, out, subnetGroupDir+"zz_controller.go")

		contains(t, "zz_controller.go", ctrl, "orphanDelete,", "func orphanDelete(context.Context, *svcapitypes.DBSubnetGroup) (managed.ExternalDelete, error) {\n\treturn managed.ExternalDelete{}, nil")
	})
	t.Run("customDelete", func(t *testing.T) {
		out := docdbFixture.mustRun(t, ignoreOperations("DeleteDBSubnetGroup"), "kinds:\n  DBSubnetGroup:\n    customDelete: true\n")
		ctrl := readOut(t, out, subnetGroupDir+"zz_controller.go")

		contains(t, "zz_controller.go", ctrl, "notImplementedDelete,", `errors.New("Delete is not implemented for DBSubnetGroup: set the delete hook")`)
	})
	t.Run("no not-found code", func(t *testing.T) {
		withoutExceptions := func(cfg string) string {
			return strings.Replace(cfg, "    exceptions:\n      errors:\n        404:\n          code: ResourceNotFoundException\n", "", 1)
		}
		_, err := servicecatalogFixture.run(t, withoutExceptions, "")

		errContains(t, err, `ProvisionedProduct: no "not found" error code: neither the API model nor generator-config.yaml has one; set resources.ProvisionedProduct.exceptions.errors.404.code in`)
	})
}

func TestCheckKind(t *testing.T) {
	safe := kindFacts{Kind: "W", Read: readMany, ReadOp: "DescribeWs", UpdateOp: "ModifyW", DeleteOp: "DeleteW", NotFoundCode: "WNotFound", ReadManyLists: []string{"Ws"}}
	with := func(edit func(*kindFacts)) kindFacts {
		f := safe
		edit(&f)
		return f
	}
	nested := "A.B"
	cases := map[string]struct {
		f       kindFacts
		kc      KindConfig
		wantErr string // "" means no error
	}{
		"safe":                         {f: safe},
		"no read op":                   {f: with(func(f *kindFacts) { f.Read, f.ReadOp = readNone, "" }), wantErr: "no read operation"},
		"no read op, customObserve":    {f: with(func(f *kindFacts) { f.Read, f.ReadOp = readNone, "" }), kc: KindConfig{CustomObserve: true}},
		"customObserve with read op":   {f: safe, kc: KindConfig{CustomObserve: true}, wantErr: "W: kinds.W.customObserve is set but DescribeWs is a read operation; remove it"},
		"no delete op":                 {f: with(func(f *kindFacts) { f.DeleteOp = "" }), wantErr: "no delete operation"},
		"no delete op, customDelete":   {f: with(func(f *kindFacts) { f.DeleteOp = "" }), kc: KindConfig{CustomDelete: true}},
		"no delete op, orphanOnDelete": {f: with(func(f *kindFacts) { f.DeleteOp = "" }), kc: KindConfig{OrphanOnDelete: true}},
		"both delete opt-ins": {
			f: with(func(f *kindFacts) { f.DeleteOp = "" }), kc: KindConfig{CustomDelete: true, OrphanOnDelete: true},
			wantErr: "W: kinds.W: customDelete and orphanOnDelete are exclusive",
		},
		"delete opt-in with delete op": {f: safe, kc: KindConfig{OrphanOnDelete: true}, wantErr: "are exclusive and need a kind without a delete operation"},
		"no 404 code":                  {f: with(func(f *kindFacts) { f.NotFoundCode = "UNKNOWN" }), wantErr: `no "not found" error code`},
		"several lists": {
			f:       with(func(f *kindFacts) { f.ReadManyLists = []string{"As", "Bs"} }),
			wantErr: "W: the output of DescribeWs has several lists (As, Bs); set operations.DescribeWs.output_wrapper_field_path in generator-config.yaml",
		},
		"several lists, wrapper": {f: with(func(f *kindFacts) { f.ReadManyLists = []string{"As", "Bs"}; f.ReadManyWrapper = &f.ReadManyLists[0] })},
		"nested wrapper":         {f: with(func(f *kindFacts) { f.ReadManyWrapper = &nested }), wantErr: "nested output_wrapper_field_path A.B is not supported"},
		"GetAttributes with update": {
			f:       with(func(f *kindFacts) { f.Read, f.ReadOp = readGetAttributes, "GetWAttributes" }),
			wantErr: "W: GetWAttributes is a GetAttributes operation, so no drift check is generated although ModifyW can update the resource; set kinds.W.customIsUpToDate: true",
		},
		"GetAttributes with update, customIsUpToDate": {f: with(func(f *kindFacts) { f.Read, f.ReadOp = readGetAttributes, "GetWAttributes" }), kc: KindConfig{CustomIsUpToDate: true}},
		"GetAttributes without update":                {f: with(func(f *kindFacts) { f.Read, f.ReadOp, f.UpdateOp = readGetAttributes, "GetWAttributes", "" })},
		"customIsUpToDate elsewhere":                  {f: safe, kc: KindConfig{CustomIsUpToDate: true}, wantErr: "kinds.W.customIsUpToDate is only for GetAttributes kinds"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			errs := checkKind(tc.f, tc.kc, "generator-config.yaml")
			if tc.wantErr == "" {
				if len(errs) > 0 {
					t.Fatalf("unexpected errors: %v", errs)
				}
				return
			}
			if len(errs) != 1 {
				t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
			}
			errContains(t, errs[0], tc.wantErr)
		})
	}
}

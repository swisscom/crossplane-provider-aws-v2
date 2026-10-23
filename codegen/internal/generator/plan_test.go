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
	"regexp"
	"strings"
	"testing"

	awssdkmodel "github.com/aws-controllers-k8s/code-generator/pkg/api"
)

const docdbInstanceDir = "internal/controller/docdb/dbinstance/"

// fn returns the source of the generated function name, up to the next one.
func fn(t *testing.T, src, name string) string {
	t.Helper()
	i := strings.Index(src, "func "+name+"(")
	if i < 0 {
		t.Fatalf("no func %s", name)
	}
	rest := src[i+1:]
	if j := strings.Index(rest, "\nfunc "); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

func TestRunReadManyUsesExternalName(t *testing.T) {
	out := docdbFixture.mustRun(t, nil, "")
	conv := readOut(t, out, docdbInstanceDir+"zz_conversions.go")
	ctrl := readOut(t, out, docdbInstanceDir+"zz_controller.go")

	setFromName := "if name := meta.GetExternalName(cr); name != \"\" {\n\t\tres.DBInstanceIdentifier = aws.String(name)"
	for _, f := range []string{"GenerateDescribeDBInstancesInput", "GenerateModifyDBInstanceInput", "GenerateDeleteDBInstanceInput"} {
		contains(t, f, fn(t, conv, f), setFromName)
	}
	contains(t, "zz_controller.go", ctrl,
		"filterList:     filterByExternalName,",
		"if aws.ToString(elem.DBInstanceIdentifier) == name {",
		`errMultipleFound = "found %d DBInstance resources with external name %q, expected one"`,
		"switch n := len(resp.DBInstances); {",
	)
	notContains(t, "zz_controller.go", ctrl, "nopFilterList")
}

func members(kv ...any) map[string]*awssdkmodel.ShapeRef {
	m := map[string]*awssdkmodel.ShapeRef{}
	for i := 0; i < len(kv); i += 2 {
		m[kv[i].(string)] = &awssdkmodel.ShapeRef{Shape: kv[i+1].(*awssdkmodel.Shape)}
	}
	return m
}

var (
	str     = &awssdkmodel.Shape{Type: shapeString}
	strList = &awssdkmodel.Shape{Type: shapeList, MemberRef: awssdkmodel.ShapeRef{Shape: str}}
	boolean = &awssdkmodel.Shape{Type: "boolean"}
)

func TestResolveIdentifier(t *testing.T) {
	cases := map[string]struct {
		kc      KindConfig
		readIn  map[string]*awssdkmodel.ShapeRef
		elem    map[string]*awssdkmodel.ShapeRef
		upd     map[string]*awssdkmodel.ShapeRef
		want    *identifierPlan
		wantErr string
	}{
		"auto": {
			readIn: members("WidgetIdentifier", str, "Marker", str),
			elem:   members("WidgetIdentifier", str, "WidgetArn", str),
			upd:    members("WidgetIdentifier", str),
			want:   &identifierPlan{ElemMember: "WidgetIdentifier", InputMember: "WidgetIdentifier", UpdateMember: "WidgetIdentifier"},
		},
		"configured": {
			kc:     KindConfig{Identifier: "Label"},
			readIn: members("Label", str, "WidgetName", str),
			elem:   members("Label", str, "WidgetName", str),
			want:   &identifierPlan{ElemMember: "Label", InputMember: "Label"},
		},
		"list input": {
			readIn: members("WidgetIds", strList),
			elem:   members("WidgetId", str),
			want:   &identifierPlan{ElemMember: "WidgetId", InputMember: "WidgetIds", InputIsList: true},
		},
		"configured input": {
			kc:     KindConfig{IdentifierInput: "Names"},
			readIn: members("Names", strList),
			elem:   members("WidgetName", str),
			want:   &identifierPlan{ElemMember: "WidgetName", InputMember: "Names", InputIsList: true},
		},
		"none": {
			readIn:  members("Filters", strList),
			elem:    members("WidgetName", str, "WidgetId", boolean),
			wantErr: "cannot tell which field holds the external name: none of WidgetIdentifier, WidgetId, WidgetName is a string member of both Elem and In",
		},
		"configured does not resolve": {
			kc:      KindConfig{Identifier: "Label"},
			readIn:  members("WidgetName", str),
			elem:    members("WidgetName", str),
			wantErr: "none of Label is a string member",
		},
		"ambiguous": {
			readIn:  members("WidgetId", str, "WidgetName", str),
			elem:    members("WidgetId", str, "WidgetName", str),
			wantErr: "ambiguous identifier (WidgetId, WidgetName all match)",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := resolveIdentifier("Widget", tc.kc, rawShape{"In", tc.readIn}, rawShape{"Elem", tc.elem}, tc.upd, nil)
			if tc.wantErr != "" {
				errContains(t, err, tc.wantErr)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if *got != *tc.want {
				t.Errorf("resolveIdentifier() = %+v, want %+v", *got, *tc.want)
			}
		})
	}
}

func TestRunReportsUnresolvedIdentifier(t *testing.T) {
	_, err := docdbFixture.run(t, nil, "kinds:\n  DBSubnetGroup:\n    identifier: Label\n")

	errContains(t, err, "DBSubnetGroup: cannot tell which field holds the external name: none of Label is a string member of both DBSubnetGroup and DescribeDBSubnetGroupsInput; set kinds.DBSubnetGroup.identifier in apis/docdb/codegen.yaml")
}

func TestReadManyList(t *testing.T) {
	elemA := &awssdkmodel.Shape{ShapeName: "A", Type: shapeStructure}
	elemB := &awssdkmodel.Shape{ShapeName: "B", Type: shapeStructure}
	listOf := func(e *awssdkmodel.Shape) *awssdkmodel.Shape {
		return &awssdkmodel.Shape{Type: shapeList, MemberRef: awssdkmodel.ShapeRef{Shape: e}}
	}
	op := func(m map[string]*awssdkmodel.ShapeRef) *awssdkmodel.Operation {
		return &awssdkmodel.Operation{ExportedName: "DescribeWidgets", OutputRef: awssdkmodel.ShapeRef{Shape: &awssdkmodel.Shape{Type: shapeStructure, MemberRefs: m}}}
	}
	wrapper := func(s string) *string { return &s }
	cases := map[string]struct {
		op       *awssdkmodel.Operation
		wrapper  *string
		want     string
		wantElem *awssdkmodel.Shape
		wantErr  string
	}{
		"one list": {
			op:   op(members("Marker", str, "Widgets", listOf(elemA))),
			want: "Widgets", wantElem: elemA,
		},
		"wrapper picks the list": {
			op:      op(members("Widgets", listOf(elemA), "Gadgets", listOf(elemB))),
			wrapper: wrapper("Widgets"),
			want:    "Widgets", wantElem: elemA,
		},
		"wrapper is not a list": {
			op:      op(members("Marker", str, "Widgets", listOf(elemA))),
			wrapper: wrapper("Marker"),
			wantErr: "output_wrapper_field_path Marker of DescribeWidgets is not a list member",
		},
		"no list": {
			op:      op(members("Marker", str)),
			wantErr: "the output of DescribeWidgets has no list",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, elem, err := readManyList(tc.op, tc.wrapper)
			if tc.wantErr != "" {
				errContains(t, err, tc.wantErr)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want || elem != tc.wantElem {
				t.Errorf("readManyList() = %s, %v; want %s, %v", got, elem.ShapeName, tc.want, tc.wantElem.ShapeName)
			}
		})
	}
}

// method returns the source of a generated method of external.
func method(t *testing.T, src, name string) string {
	t.Helper()
	return fn(t, strings.Replace(src, "func (e *external) "+name+"(", "func "+name+"(", 1), name)
}

var specAssignment = regexp.MustCompile(`\bcr\.Spec\.ForProvider\.\w+ = `)

func TestRunCreateWritesStatusOnly(t *testing.T) {
	t.Run("docdb", func(t *testing.T) {
		out := docdbFixture.mustRun(t, nil, "")
		create := method(t, readOut(t, out, docdbInstanceDir+"zz_controller.go"), "Create")

		contains(t, "Create", create,
			"if resp != nil && resp.DBInstance != nil {",
			"ko := cr.DeepCopy()",
			"ko.Status.AtProvider.DeepCopyInto(&cr.Status.AtProvider)",
		)
		if m := specAssignment.FindString(create); m != "" {
			t.Errorf("Create writes spec: %q", m)
		}
	})
	t.Run("servicecatalog read wrapper", func(t *testing.T) {
		withWrapper := func(cfg string) string {
			return strings.Replace(cfg, "  DescribeProvisionedProduct:\n", "  DescribeProvisionedProduct:\n    output_wrapper_field_path: ProvisionedProductDetail\n", 1)
		}
		out := servicecatalogFixture.mustRun(t, withWrapper, "")
		dir := "internal/controller/servicecatalog/provisionedproduct/"
		conv := readOut(t, out, dir+"zz_conversions.go")
		ctrl := readOut(t, out, dir+"zz_controller.go")

		contains(t, "GenerateProvisionedProduct", fn(t, conv, "GenerateProvisionedProduct"),
			"if resp == nil || resp.ProvisionedProductDetail == nil {\n\t\treturn cr\n\t}",
			"cr.Status.AtProvider.ARN = resp.ProvisionedProductDetail.Arn")
		contains(t, "Observe", method(t, ctrl, "Observe"),
			"if resp == nil || resp.ProvisionedProductDetail == nil {",
			"return managed.ExternalObservation{}, errors.New(errEmptyDescribe)")
		contains(t, "zz_controller.go", ctrl, `errEmptyDescribe = "AWS returned an empty ProvisionedProduct description"`)
	})
}

func TestRunEmitsImmutabilityMarkers(t *testing.T) {
	const rule = `// +kubebuilder:validation:XValidation:rule="self == oldSelf",message=`
	t.Run("region and immutableFields", func(t *testing.T) {
		out := docdbFixture.mustRun(t, nil, "kinds:\n  DBInstance:\n    immutableFields: [availabilityZone, dbInstanceClass]\n")
		types := readOut(t, out, "apis/docdb/v1alpha1/zz_db_instance.go")

		contains(t, "zz_db_instance.go", types,
			rule+`"region is immutable"`+"\n\tRegion string",
			rule+`"availabilityZone is immutable"`+"\n\tAvailabilityZone *string",
			rule+`"dbInstanceClass is immutable"`+"\n\t// +kubebuilder:validation:Required\n\tDBInstanceClass *string",
			`// +kubebuilder:validation:XValidation:rule="!has(oldSelf.availabilityZone) || has(self.availabilityZone)",message="availabilityZone cannot be removed once set"`+"\ntype DBInstanceParameters struct",
		)
		// Required fields can't be removed, so they need no removal rule.
		notContains(t, "zz_db_instance.go", types, "dbInstanceClass cannot be removed")
		notContains(t, "zz_db_subnet_group.go", readOut(t, out, "apis/docdb/v1alpha1/zz_db_subnet_group.go"), "cannot be removed once set")
	})
	t.Run("unknown field", func(t *testing.T) {
		_, err := docdbFixture.run(t, nil, "kinds:\n  DBInstance:\n    immutableFields: [noSuchField]\n")

		errContains(t, err, "DBInstance: immutableFields entry noSuchField is not a spec.forProvider field")
	})
}

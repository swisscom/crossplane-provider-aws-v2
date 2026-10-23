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
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
	ttpl "text/template"

	ackmodel "github.com/aws-controllers-k8s/code-generator/pkg/model"
)

func TestEnumDefQuotesValues(t *testing.T) {
	tpl, err := ttpl.ParseFiles(filepath.Join("..", "..", "templates", "apis", "enum_def.go.tpl"))
	if err != nil {
		t.Fatal(err)
	}
	e, err := ackmodel.NewEnumDef(ackmodel.EnumDef{}.Names, []string{`a"b\c`, "plain"})
	if err != nil {
		t.Fatal(err)
	}
	e.Names.Camel = "Widget"
	var b strings.Builder
	b.WriteString("package x\n\n")
	if err := tpl.ExecuteTemplate(&b, "enum_def", e); err != nil {
		t.Fatal(err)
	}

	if _, err := parser.ParseFile(token.NewFileSet(), "enums.go", b.String(), 0); err != nil {
		t.Errorf("rendered enum does not parse: %v\n%s", err, b.String())
	}
	contains(t, "enum", b.String(), `Widget_a_b_c Widget = "a\"b\\c"`)
}

func TestFirstElementOnly(t *testing.T) {
	ack := "\n\tfound := false\n" +
		"\tfor _, elem := range resp.Widgets {\n" +
		"\t\tif elem.Name != nil {\n\t\t\tcr.Status.AtProvider.Name = elem.Name\n\t\t}\n" +
		"\t\tfor _, f0iter := range elem.Tags {\n\t\t\t_ = f0iter\n\t\t}\n" +
		"\t\tfound = true\n" +
		"\t\tbreak\n" +
		"\t}\n" +
		"\tif !found {\n\t\treturn cr\n\t}\n"
	want := "\n\tfound := false\n" +
		"\tif len(resp.Widgets) > 0 {\n" +
		"\t\telem := resp.Widgets[0]\n" +
		"\t\tif elem.Name != nil {\n\t\t\tcr.Status.AtProvider.Name = elem.Name\n\t\t}\n" +
		"\t\tfor _, f0iter := range elem.Tags {\n\t\t\t_ = f0iter\n\t\t}\n" +
		"\t\tfound = true\n" +
		"\t}\n" +
		"\tif !found {\n\t\treturn cr\n\t}\n"

	got, err := firstElementOnly("Widget", ack)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("firstElementOnly() =\n%s\nwant\n%s", got, want)
	}

	_, err = firstElementOnly("Widget", strings.Replace(ack, "\t\tbreak\n", "", 1))
	errContains(t, err, "unexpected ACK ReadMany code for Widget; update firstElementOnly")
}

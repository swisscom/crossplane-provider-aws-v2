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
	"regexp"
	"strings"

	awssdkmodel "github.com/aws-controllers-k8s/code-generator/pkg/api"
	ackmodel "github.com/aws-controllers-k8s/code-generator/pkg/model"
)

var (
	secretWords = regexp.MustCompile(`(?i)(password|passwd|passphrase|secret|token|credential|privatekey|apikey|accesskey)`)
	// Names ending like this refer to a secret instead of holding it.
	referenceSuffix = regexp.MustCompile(`(Arn|ARN|KmsKeyId|KMSKeyID|Status|Id|ID)$`)
)

// isSensitiveLeaf returns whether a member looks like it holds a secret
// value. Only strings and blobs can; bools, numbers, enums and structs can't.
func isSensitiveLeaf(member string, s *awssdkmodel.Shape) bool {
	if s == nil || (s.Type != shapeString && s.Type != shapeBlob) || s.IsEnum() {
		return false
	}
	return secretWords.MatchString(member) && !referenceSuffix.MatchString(member)
}

// sensitiveLeaf is a secret-looking field of a CRD.
type sensitiveLeaf struct {
	path   string // JSON path, e.g. spec.forProvider.masterUserPassword
	owner  string // shape that has the member, "" for top-level fields
	member string
}

// jsonNames maps shape → member → JSON name, as the generated types use.
type jsonNames map[string]map[string]string

func newJSONNames(tds []*ackmodel.TypeDef) jsonNames {
	n := jsonNames{}
	for _, td := range tds {
		if td.Shape == nil {
			continue
		}
		m := map[string]string{}
		for name, a := range td.Attrs {
			m[name] = a.Names.CamelLower
		}
		n[td.Shape.ShapeName] = m
	}
	return n
}

func (n jsonNames) get(shape, member string) string {
	if j, ok := n[shape][member]; ok {
		return j
	}
	return member
}

// sensitiveLeaves returns the secret-looking string leaves reachable from
// ref. Lists and maps are looked through; seen stops recursive shapes.
func sensitiveLeaves(n jsonNames, path, owner, member string, ref *awssdkmodel.ShapeRef, seen map[string]bool) []sensitiveLeaf {
	s := ref.Shape
	if s == nil {
		return nil
	}
	switch s.Type {
	case shapeStructure:
		if seen[s.ShapeName] {
			return nil
		}
		seen[s.ShapeName] = true
		defer delete(seen, s.ShapeName)
		var res []sensitiveLeaf
		for _, m := range s.MemberNames() {
			res = append(res, sensitiveLeaves(n, path+"."+n.get(s.ShapeName, m), s.ShapeName, m, s.MemberRefs[m], seen)...)
		}
		return res
	case shapeList:
		return sensitiveLeaves(n, path, owner, member, &s.MemberRef, seen)
	case shapeMap:
		return sensitiveLeaves(n, path, owner, member, &s.ValueRef, seen)
	}
	if isSensitiveLeaf(member, s) {
		return []sensitiveLeaf{{path: path, owner: owner, member: member}}
	}
	return nil
}

// checkSensitiveFields fails for every secret-looking field that would be a
// plain API field, unless codegen.yaml allows it with a reason.
func checkSensitiveFields(crd *ackmodel.CRD, n jsonNames, raw rawMembers, kc KindConfig, scPath string) []error {
	var leaves []sensitiveLeaf
	for _, name := range crd.SpecFieldNames() {
		f := crd.SpecFields[name]
		if f.ShapeRef != nil {
			leaves = append(leaves, sensitiveLeaves(n, "spec.forProvider."+f.Names.CamelLower, "", f.Names.Original, f.ShapeRef, map[string]bool{})...)
		}
	}
	for _, name := range crd.StatusFieldNames() {
		f := crd.StatusFields[name]
		if f.ShapeRef != nil {
			leaves = append(leaves, sensitiveLeaves(n, "status.atProvider."+f.Names.CamelLower, "", f.Names.Original, f.ShapeRef, map[string]bool{})...)
		}
	}

	allowed := map[string]bool{}
	var errs []error
	for _, a := range kc.AllowSensitiveFields {
		allowed[a.Path] = true
		if strings.TrimSpace(a.Reason) == "" {
			errs = append(errs, fmt.Errorf("%s: allowSensitiveFields entry %s needs a reason", crd.Kind, a.Path))
		}
	}
	found := map[string]bool{}
	for _, l := range leaves {
		found[l.path] = true
		if allowed[l.path] {
			continue
		}
		errs = append(errs, fmt.Errorf("%s: %s looks like a secret but would be a plain API field; ignore it (ignore.field_paths: %s) and add a LocalSecretKeySelector field in custom_types.go, or allow it in %s kinds.%s.allowSensitiveFields with a reason",
			crd.Kind, l.path, strings.Join(ignoreSuggestions(crd, raw, l), ", "), scPath, crd.Kind))
	}
	for _, a := range kc.AllowSensitiveFields {
		if !found[a.Path] {
			errs = append(errs, fmt.Errorf("%s: allowSensitiveFields entry %s matches no sensitive field; remove it", crd.Kind, a.Path))
		}
	}
	return errs
}

// ignoreSuggestions returns the ignore.field_paths entries that would drop
// the leaf: on its owner shape, or on the operation shapes of a top-level field.
func ignoreSuggestions(crd *ackmodel.CRD, raw rawMembers, l sensitiveLeaf) []string {
	if l.owner != "" {
		return []string{l.owner + "." + l.member}
	}
	var shapes []string
	if strings.HasPrefix(l.path, "spec.") {
		for _, op := range []*awssdkmodel.Operation{crd.Ops.Create, crd.Ops.Update} {
			if op != nil && op.InputRef.Shape != nil {
				shapes = append(shapes, op.InputRef.Shape.ShapeName)
			}
		}
	} else if op, readMany := readOperation(crd); op != nil {
		if s, err := readOutputShape(crd, op, readMany); err == nil && s != nil {
			shapes = append(shapes, s.ShapeName)
		}
	}
	var res []string
	for _, s := range shapes {
		if _, ok := raw[s][l.member]; ok {
			res = append(res, s+"."+l.member)
		}
	}
	if len(res) == 0 {
		res = append(res, "<Shape>."+l.member)
	}
	return res
}

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
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	awssdkmodel "github.com/aws-controllers-k8s/code-generator/pkg/api"
	ackmodel "github.com/aws-controllers-k8s/code-generator/pkg/model"
)

// UnresolvedIgnoreFieldPaths returns the ignore.field_paths entries of
// o.GeneratorConfig that match nothing in the API model, sorted.
func UnresolvedIgnoreFieldPaths(ctx context.Context, o Options) ([]string, error) {
	svc := strings.ToLower(o.Service)
	cfg, err := loadConfig(o)
	if err != nil {
		return nil, err
	}
	api, err := loadAPI(ctx, svc, o, cfg)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, u := range unresolvedIgnorePaths(api, cfg.Ignore.FieldPaths) {
		paths = append(paths, u.path)
	}
	return paths, nil
}

// AWS API model shape types.
const (
	shapeStructure = "structure"
	shapeList      = "list"
	shapeMap       = "map"
	shapeString    = "string"
	shapeBlob      = "blob"
)

type unresolvedPath struct{ path, reason string }

// unresolvedIgnorePaths checks ignore paths against the API model before ACK
// applies them, because ACK silently skips members it can't find.
func unresolvedIgnorePaths(api *ackmodel.SDKAPI, paths []string) []unresolvedPath {
	var res []unresolvedPath
	for _, p := range paths {
		if reason := resolveIgnorePath(api.API.Shapes, p); reason != "" {
			res = append(res, unresolvedPath{path: p, reason: reason})
		}
	}
	slices.SortFunc(res, func(a, b unresolvedPath) int { return strings.Compare(a.path, b.path) })
	return res
}

// resolveIgnorePath returns why path matches nothing, or "" if it resolves.
func resolveIgnorePath(shapes map[string]*awssdkmodel.Shape, path string) string {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return "not of the form Shape.Member"
	}
	s, ok := shapes[parts[0]]
	if !ok {
		return "unknown shape " + parts[0]
	}
	name := parts[0]
	for i, m := range parts[1:] {
		ref, ok := s.MemberRefs[m]
		if !ok {
			return fmt.Sprintf("shape %s has no member %s", name, m)
		}
		if i == len(parts)-2 {
			return ""
		}
		// ACK removes the member from its parent structure only.
		if ref.Shape.Type != shapeStructure {
			return fmt.Sprintf("member %s of shape %s is a %s, not a structure", m, name, ref.Shape.Type)
		}
		s, name = ref.Shape, ref.ShapeName
	}
	return ""
}

// checkIgnorePaths returns one error per ignore path that matches nothing.
func checkIgnorePaths(file, svc string, api *ackmodel.SDKAPI, paths []string) error {
	unresolved := unresolvedIgnorePaths(api, paths)
	errs := make([]error, 0, len(unresolved))
	for _, u := range unresolved {
		errs = append(errs, fmt.Errorf("%s: ignore.field_paths entry %q matches nothing in the %s API model (%s); fix or remove it", file, u.path, svc, u.reason))
	}
	return errors.Join(errs...)
}

// rawMembers is every shape's members before ACK applies the ignore rules.
// The SDK Go structs keep ignored members, so generated code may still set
// them (e.g. the identifier of a read input).
type rawMembers map[string]map[string]*awssdkmodel.ShapeRef

func snapshotMembers(api *ackmodel.SDKAPI) rawMembers {
	raw := rawMembers{}
	for name, s := range api.API.Shapes {
		m := make(map[string]*awssdkmodel.ShapeRef, len(s.MemberRefs))
		for k, v := range s.MemberRefs {
			m[k] = v
		}
		raw[name] = m
	}
	return raw
}

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
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// sdkNames are the exported package-level names of an AWS SDK v2 service.
type sdkNames struct {
	client map[string]bool // github.com/aws/aws-sdk-go-v2/service/<svc>
	types  map[string]bool // github.com/aws/aws-sdk-go-v2/service/<svc>/types
}

// loadSDKNames indexes the AWS SDK v2 package of svc as resolved by the go
// module in dir.
func loadSDKNames(dir, svc string) (*sdkNames, error) {
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", "github.com/aws/aws-sdk-go-v2/service/"+svc)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrapf(err, "cannot locate AWS SDK v2 package of service %s", svc)
	}
	pkgDir := strings.TrimSpace(string(out))
	n := &sdkNames{}
	if n.client, err = exportedNames(pkgDir); err != nil {
		return nil, err
	}
	if n.types, err = exportedNames(filepath.Join(pkgDir, "types")); err != nil {
		return nil, err
	}
	return n, nil
}

func exportedNames(dir string) (map[string]bool, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.SkipObjectResolution)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot parse %s", dir)
	}
	names := map[string]bool{}
	for _, p := range pkgs {
		for _, f := range p.Files {
			for _, d := range f.Decls {
				for _, name := range declNames(d) {
					if token.IsExported(name) {
						names[name] = true
					}
				}
			}
		}
	}
	return names, nil
}

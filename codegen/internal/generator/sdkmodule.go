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
	"bufio"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	ackgenconfig "github.com/aws-controllers-k8s/code-generator/pkg/config"
)

// ResolveServiceModule makes sure the provider's go.mod requires the
// aws-sdk-go-v2 service module, then uses its version to fetch the matching
// API model, so the generated code and the SDK it compiles against agree.
func ResolveServiceModule(o *Options) error {
	mod := "github.com/aws/aws-sdk-go-v2/service/" + o.Service
	if !requires(filepath.Join(o.OutputDir, "go.mod"), mod) {
		fmt.Fprintf(os.Stderr, "adding %s to go.mod\n", mod)
		cmd := exec.Command("go", "get", mod+"@latest") //nolint:gosec // module path built from a flag
		cmd.Dir = o.OutputDir
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("cannot add %s to go.mod: %w", mod, err)
		}
	}
	cmd := exec.Command("go", "list", "-m", "-json", mod) //nolint:gosec // module path built from a flag
	cmd.Dir = o.OutputDir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("cannot resolve %s: %w", mod, err)
	}
	var m struct{ Version, Dir string }
	if err := json.Unmarshal(out, &m); err != nil {
		return err
	}
	if m.Dir == "" {
		dl := exec.Command("go", "mod", "download", mod) //nolint:gosec // module path built from a flag
		dl.Dir = o.OutputDir
		if err := dl.Run(); err != nil {
			return fmt.Errorf("cannot download %s: %w", mod, err)
		}
		return ResolveServiceModule(o)
	}
	o.ServiceSDKVersion = m.Version
	o.ServiceSDKDir = m.Dir
	o.ModelName, err = modelName(m.Dir)
	return err
}

func requires(gomod, mod string) bool {
	f, err := os.Open(gomod) //nolint:gosec // path comes from a flag
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck // read only
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) >= 2 && (fields[0] == mod || (fields[0] == "require" && fields[1] == mod)) {
			return true
		}
	}
	return false
}

var serviceIDRe = regexp.MustCompile(`const ServiceID = "([^"]+)"`)

// modelName returns the API model file name of a service module: its
// ServiceID, lower-cased and hyphenated (e.g. "Service Catalog" ->
// "service-catalog").
func modelName(moduleDir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(moduleDir, "api_client.go")) //nolint:gosec // module cache path
	if err != nil {
		return "", err
	}
	m := serviceIDRe.FindSubmatch(b)
	if m == nil {
		return "", fmt.Errorf("no ServiceID in %s/api_client.go", moduleDir)
	}
	return strings.ReplaceAll(strings.ToLower(string(m[1])), " ", "-"), nil
}

// unexportedInputFields returns the members of operation inputs that the
// SDK keeps unexported (e.g. DestinationRegion of the presigned URL
// customization of rds and docdb) as ACK field paths, e.g.
// CreateDBClusterInput.DestinationRegion. Generated code cannot set them.
func unexportedInputFields(moduleDir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(moduleDir, "api_op_*.go"))
	if err != nil {
		return nil, err
	}
	var paths []string
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, sp := range gd.Specs {
				ts := sp.(*ast.TypeSpec) //nolint:forcetypeassert // type declarations hold type specs
				st, ok := ts.Type.(*ast.StructType)
				if !ok || !strings.HasSuffix(ts.Name.Name, "Input") {
					continue
				}
				for _, fld := range st.Fields.List {
					for _, n := range fld.Names {
						if !n.IsExported() {
							paths = append(paths, ts.Name.Name+"."+strings.ToUpper(n.Name[:1])+n.Name[1:])
						}
					}
				}
			}
		}
	}
	return paths, nil
}

func ignoreUnexportedInputs(cfg *ackgenconfig.Config, moduleDir string) error {
	if moduleDir == "" {
		return nil
	}
	paths, err := unexportedInputFields(moduleDir)
	if err != nil {
		return fmt.Errorf("cannot read inputs of %s: %w", moduleDir, err)
	}
	for _, p := range paths {
		if !slices.Contains(cfg.Ignore.FieldPaths, p) {
			cfg.Ignore.FieldPaths = append(cfg.Ignore.FieldPaths, p)
		}
	}
	return nil
}

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

// Package generator generates provider-aws-v2 API types and controllers for an
// AWS service. It uses the ACK code-generator to load and interpret the AWS
// SDK v2 API model and its generator-config.yaml, and renders our own
// templates (crossplane-runtime v2, namespaced MRs, AWS SDK v2).
package generator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	ttpl "text/template"

	awssdkmodel "github.com/aws-controllers-k8s/code-generator/pkg/api"
	ackgenconfig "github.com/aws-controllers-k8s/code-generator/pkg/config"
	"github.com/aws-controllers-k8s/code-generator/pkg/generate/code"
	"github.com/aws-controllers-k8s/code-generator/pkg/generate/templateset"
	ackmodel "github.com/aws-controllers-k8s/code-generator/pkg/model"
	acksdk "github.com/aws-controllers-k8s/code-generator/pkg/sdk"
	"github.com/iancoleman/strcase"
	"golang.org/x/tools/imports"
	"sigs.k8s.io/yaml"
)

const (
	// ModulePath is the Go module of the provider.
	ModulePath = "github.com/teeverr/provider-aws-v2"
	// GroupSuffix is appended to the service name to form the API group.
	GroupSuffix = "aws2.crossplane.io"
)

// Options of a generator run.
type Options struct {
	Service         string
	OutputDir       string
	GeneratorConfig string
	APIVersion      string
	// ServiceSDKVersion is the version of the aws-sdk-go-v2 service module;
	// its tag holds the API model.
	ServiceSDKVersion string
	// ModelName is the API model file name, e.g. service-catalog.
	ModelName   string
	CacheDir    string
	TemplateDir string
}

// defaultConfig makes ACK's code emitters read and write
// cr.Spec.ForProvider / cr.Status.AtProvider.
var defaultConfig = ackgenconfig.Config{
	PrefixConfig: ackgenconfig.PrefixConfig{
		SpecField:   ".Spec.ForProvider",
		StatusField: ".Status.AtProvider",
	},
	IncludeACKMetadata:             false,
	SetManyOutputNotFoundErrReturn: "return cr",
}

var (
	apiTemplates = []string{
		"apis/doc.go.tpl",
		"apis/enums.go.tpl",
		"apis/groupversion_info.go.tpl",
		"apis/types.go.tpl",
	}
	includeTemplates = []string{
		"boilerplate.go.tpl",
		"apis/enum_def.go.tpl",
		"apis/type_def.go.tpl",
		"controller/sdk_find_read_one.go.tpl",
		"controller/sdk_find_read_many.go.tpl",
		"controller/sdk_find_get_attributes.go.tpl",
	}
)

// Run generates the code of one service.
func Run(ctx context.Context, o Options) error {
	svc := strings.ToLower(o.Service)
	if o.GeneratorConfig == "" {
		o.GeneratorConfig = filepath.Join(o.OutputDir, "apis", svc, "generator-config.yaml")
	}
	cfg, err := ackgenconfig.New(o.GeneratorConfig, defaultConfig)
	if err != nil {
		return fmt.Errorf("cannot load generator config: %w", err)
	}
	normalizeOperationTypes(&cfg)

	m, err := loadModel(ctx, svc, o, cfg)
	if err != nil {
		return err
	}
	files, err := render(m, o)
	if err != nil {
		return err
	}
	for path, content := range files {
		if err := writeGo(filepath.Join(o.OutputDir, path), content); err != nil {
			return err
		}
	}
	if err := writeCustomTypes(m, o); err != nil {
		return err
	}
	return WriteRegistries(o.OutputDir)
}

// ListKinds returns the kinds the generator config of o yields, before
// any resource is ignored by callers.
func ListKinds(ctx context.Context, o Options) ([]string, error) {
	cfg, err := ackgenconfig.New(o.GeneratorConfig, defaultConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot load generator config: %w", err)
	}
	normalizeOperationTypes(&cfg)
	m, err := loadModel(ctx, strings.ToLower(o.Service), o, cfg)
	if err != nil {
		return nil, err
	}
	crds, err := m.GetCRDs()
	if err != nil {
		return nil, err
	}
	kinds := make([]string, 0, len(crds))
	for _, c := range crds {
		kinds = append(kinds, c.Kind)
	}
	sort.Strings(kinds)
	return kinds, nil
}

func loadModel(ctx context.Context, svc string, o Options, cfg ackgenconfig.Config) (*ackmodel.Model, error) {
	modelName := o.ModelName
	if cfg.SDKNames.Model != "" {
		modelName = strings.ToLower(cfg.SDKNames.Model)
	}
	basePath, err := acksdk.EnsureModel(ctx, o.CacheDir, "", svc, modelName, o.ServiceSDKVersion)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch AWS API model: %w", err)
	}
	api, err := acksdk.NewHelper(basePath, cfg).API(modelName)
	if err != nil {
		return nil, fmt.Errorf("cannot load AWS API model: %w", err)
	}
	api.APIGroupSuffix = GroupSuffix
	docCfg, err := ackgenconfig.NewDocumentationConfig("")
	if err != nil {
		return nil, err
	}
	return ackmodel.New(api, svc, o.APIVersion, cfg, docCfg)
}

type apiVars struct {
	templateset.MetaVars
	ModulePath string
	EnumDefs   []*ackmodel.EnumDef
	TypeDefs   []*ackmodel.TypeDef
}

type crdVars struct {
	templateset.MetaVars
	ModulePath string
	CRD        *ackmodel.CRD
	// ClientOps are additional operations of the Client interface.
	ClientOps []*awssdkmodel.Operation
}

// ServiceConfig is the optional apis/<service>/codegen.yaml. It configures
// provider-aws-v2 specifics that the ACK generator config does not cover.
type ServiceConfig struct {
	// ClientOperations are SDK operations added to the Client interface of
	// a kind's controller, keyed by kind, e.g. for hand-written code.
	ClientOperations map[string][]string `json:"clientOperations,omitempty"`
}

// ServiceConfigPath returns the path of the codegen.yaml of a service.
func ServiceConfigPath(root, svc string) string {
	return filepath.Join(root, "apis", svc, "codegen.yaml")
}

// ReadServiceConfig reads the codegen.yaml of a service, if any.
func ReadServiceConfig(root, svc string) (*ServiceConfig, error) {
	c := &ServiceConfig{}
	b, err := os.ReadFile(ServiceConfigPath(root, svc))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	return c, yaml.Unmarshal(b, c)
}

type setupVars struct {
	templateset.MetaVars
	ModulePath string
	CRDs       []*ackmodel.CRD
}

func render(m *ackmodel.Model, o Options) (map[string][]byte, error) { //nolint:gocyclo // linear list of templates
	enumDefs, err := m.GetEnumDefs()
	if err != nil {
		return nil, err
	}
	typeDefs, err := m.GetTypeDefs()
	if err != nil {
		return nil, err
	}
	crds, err := m.GetCRDs()
	if err != nil {
		return nil, err
	}
	sc, err := ReadServiceConfig(o.OutputDir, strings.ToLower(o.Service))
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", ServiceConfigPath(o.OutputDir, o.Service), err)
	}

	ts := templateset.New([]string{o.TemplateDir}, includeTemplates, nil, funcMap())
	mv := m.MetaVars()
	svc := mv.ServicePackageName

	versions := map[string]bool{}
	for _, crd := range crds {
		v, err := crd.GetStorageVersion(mv.APIVersion)
		if err != nil {
			return nil, err
		}
		versions[v] = true
	}
	for v := range versions {
		for _, tpl := range apiTemplates {
			vars := &apiVars{MetaVars: mv, ModulePath: ModulePath, EnumDefs: enumDefs, TypeDefs: typeDefs}
			vars.APIVersion = v
			out := filepath.Join("apis", svc, v, "zz_"+strings.TrimSuffix(filepath.Base(tpl), ".tpl"))
			if err := ts.Add(out, tpl, vars); err != nil {
				return nil, err
			}
		}
	}

	for _, crd := range crds {
		v, err := crd.GetStorageVersion(mv.APIVersion)
		if err != nil {
			return nil, err
		}
		vars := &crdVars{MetaVars: mv, ModulePath: ModulePath, CRD: crd}
		vars.APIVersion = v
		for _, name := range sc.ClientOperations[crd.Kind] {
			op, ok := m.SDKAPI.API.Operations[name]
			if !ok {
				return nil, fmt.Errorf("clientOperations of %s: unknown operation %s", crd.Kind, name)
			}
			vars.ClientOps = append(vars.ClientOps, op)
		}
		files := map[string]string{
			filepath.Join("apis", svc, v, "zz_"+strcase.ToSnake(crd.Kind)+".go"):               "apis/crd.go.tpl",
			filepath.Join("internal", "controller", svc, crd.Names.Lower, "zz_controller.go"):  "controller/controller.go.tpl",
			filepath.Join("internal", "controller", svc, crd.Names.Lower, "zz_conversions.go"): "controller/conversions.go.tpl",
		}
		for out, tpl := range files {
			if err := ts.Add(out, tpl, vars); err != nil {
				return nil, err
			}
		}
	}
	sv := &setupVars{MetaVars: mv, ModulePath: ModulePath, CRDs: crds}
	if err := ts.Add(filepath.Join("internal", "controller", svc, "zz_setup.go"), "controller/setup.go.tpl", sv); err != nil {
		return nil, err
	}

	if err := ts.Execute(); err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for path, buf := range ts.Executed() {
		out[path] = buf.Bytes()
	}
	return out, nil
}

func funcMap() ttpl.FuncMap {
	return ttpl.FuncMap{
		"ToLower": strings.ToLower,
		"ResourceExceptionCode": func(r *ackmodel.CRD, httpStatusCode int) string {
			return r.ExceptionCode(httpStatusCode)
		},
		"GoCodeSetReadOneOutput": func(r *ackmodel.CRD, src, dst string, indent int) (string, error) {
			return code.SetResource(r.Config(), r, ackmodel.OpTypeGet, src, dst, indent)
		},
		"GoCodeSetReadOneInput": func(r *ackmodel.CRD, src, dst string, indent int) (string, error) {
			return code.SetSDK(r.Config(), r, ackmodel.OpTypeGet, src, dst, indent)
		},
		"GoCodeSetReadManyOutput": func(r *ackmodel.CRD, src, dst string, indent int) (string, error) {
			return code.SetResource(r.Config(), r, ackmodel.OpTypeList, src, dst, indent)
		},
		"GoCodeSetReadManyInput": func(r *ackmodel.CRD, src, dst string, indent int) (string, error) {
			return code.SetSDK(r.Config(), r, ackmodel.OpTypeList, src, dst, indent)
		},
		"ListMemberNameInReadManyOutput": code.ListMemberNameInReadManyOutput,
		"GoCodeGetAttributesSetInput": func(r *ackmodel.CRD, src, dst string, indent int) (string, error) {
			return code.SetSDKGetAttributes(r.Config(), r, src, dst, indent)
		},
		"GoCodeGetAttributesSetOutput": func(r *ackmodel.CRD, src, dst string, indent int) (string, error) {
			return code.SetResourceGetAttributes(r.Config(), r, src, dst, indent)
		},
		"GoCodeSetCreateOutput": func(r *ackmodel.CRD, src, dst string, indent int) (string, error) {
			return code.SetResource(r.Config(), r, ackmodel.OpTypeCreate, src, dst, indent)
		},
		"GoCodeSetCreateInput": func(r *ackmodel.CRD, src, dst string, indent int) (string, error) {
			return code.SetSDK(r.Config(), r, ackmodel.OpTypeCreate, src, dst, indent)
		},
		"GoCodeSetUpdateInput": func(r *ackmodel.CRD, src, dst string, indent int) (string, error) {
			return code.SetSDK(r.Config(), r, ackmodel.OpTypeUpdate, src, dst, indent)
		},
		"GoCodeSetDeleteInput": func(r *ackmodel.CRD, src, dst string, indent int) (string, error) {
			return code.SetSDK(r.Config(), r, ackmodel.OpTypeDelete, src, dst, indent)
		},
		// IAM is a global service; its resources have no region field.
		"IsGlobalService": func(group string) bool {
			return strings.HasPrefix(group, "iam.")
		},
	}
}

// unkeyedTime matches the unkeyed metav1.Time literals ACK emits.
var unkeyedTime = regexp.MustCompile(`metav1\.Time\{([^}:]+)\}`)

func writeGo(path string, src []byte) error {
	src = unkeyedTime.ReplaceAll(src, []byte("metav1.Time{Time: $1}"))
	out, err := imports.Process(path, src, nil)
	if err != nil {
		// Keep the unformatted source to make template errors debuggable.
		_ = writeFile(path, src)
		return fmt.Errorf("cannot format %s: %w", path, err)
	}
	return writeFile(path, out)
}

func writeFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// writeCustomTypes creates custom_types.go with empty Custom<Kind>Parameters
// and Custom<Kind>Observation structs, unless the file exists. The file is hand-written afterwards: it is
// where additional fields such as references are added.
func writeCustomTypes(m *ackmodel.Model, o Options) error {
	crds, err := m.GetCRDs()
	if err != nil {
		return err
	}
	byVersion := map[string][]string{}
	for _, crd := range crds {
		v, err := crd.GetStorageVersion(o.APIVersion)
		if err != nil {
			return err
		}
		byVersion[v] = append(byVersion[v], crd.Kind)
	}
	for v, kinds := range byVersion {
		path := filepath.Join(o.OutputDir, "apis", m.MetaVars().ServicePackageName, v, "custom_types.go")
		if _, err := os.Stat(path); err == nil {
			continue
		}
		sort.Strings(kinds)
		var b strings.Builder
		b.WriteString(boilerplate)
		fmt.Fprintf(&b, "\npackage %s\n", v)
		for _, k := range kinds {
			fmt.Fprintf(&b, "\n// Custom%[1]sParameters contains the hand-written fields of %[1]sParameters.\ntype Custom%[1]sParameters struct{}\n", k)
			fmt.Fprintf(&b, "\n// Custom%[1]sObservation contains the hand-written fields of %[1]sObservation.\ntype Custom%[1]sObservation struct{}\n", k)
		}
		if err := writeGo(path, []byte(b.String())); err != nil {
			return err
		}
	}
	return nil
}

// legacyOperationTypes maps operation types used by provider-aws generator
// configs (older ACK versions) to the names current ACK understands.
var legacyOperationTypes = map[string]string{
	"read": "READ_ONE",
}

// normalizeOperationTypes rewrites legacy operation types, so generator
// configs can be imported from provider-aws unchanged.
func normalizeOperationTypes(cfg *ackgenconfig.Config) {
	for name, op := range cfg.Operations {
		for i, t := range op.OperationType {
			if n, ok := legacyOperationTypes[strings.ToLower(t)]; ok {
				op.OperationType[i] = n
			}
		}
		cfg.Operations[name] = op
	}
}

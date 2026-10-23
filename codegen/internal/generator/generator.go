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
	"maps"
	"os"
	"path/filepath"
	"slices"
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
	"sigs.k8s.io/yaml"
)

const (
	// ModulePath is the Go module of the provider.
	ModulePath = "github.com/swisscom/crossplane-provider-aws-v2"
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
	ModelName string
	// ServiceSDKDir is the directory of the aws-sdk-go-v2 service module.
	// If set, input members the SDK does not export are ignored.
	ServiceSDKDir string
	CacheDir      string
	TemplateDir   string
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

// Run generates the code of one service. It checks the generator config and
// the API model first and writes nothing if any check fails.
func Run(ctx context.Context, o Options) error {
	svc := strings.ToLower(o.Service)
	if o.GeneratorConfig == "" {
		o.GeneratorConfig = filepath.Join(o.OutputDir, "apis", svc, "generator-config.yaml")
	}
	cfg, err := loadConfig(o)
	if err != nil {
		return err
	}
	// Only the entries a person wrote are checked, not the derived ones.
	userPaths := slices.Clone(cfg.Ignore.FieldPaths)
	if err := ignoreUnexportedInputs(&cfg, o.ServiceSDKDir); err != nil {
		return err
	}
	api, err := loadAPI(ctx, svc, o, cfg)
	if err != nil {
		return err
	}
	if err := checkIgnorePaths(o.GeneratorConfig, svc, api, userPaths); err != nil {
		return err
	}
	raw := snapshotMembers(api)
	m, err := newModel(api, svc, o, cfg)
	if err != nil {
		return err
	}
	sc, err := ReadServiceConfig(o.OutputDir, svc)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", ServiceConfigPath(o.OutputDir, svc), err)
	}
	plans, err := planKinds(m, raw, sc, o.GeneratorConfig, ServiceConfigPath("", svc))
	if err != nil {
		return err
	}
	files, err := render(m, o, sc, plans)
	if err != nil {
		return err
	}
	custom, err := customTypes(m, o)
	if err != nil {
		return err
	}
	maps.Copy(files, custom)
	// Format everything before writing anything, so a failure leaves the
	// previous output intact.
	formatted, err := formatAll(o.OutputDir, files)
	if err != nil {
		return err
	}
	stale, err := staleFiles(o.OutputDir, m.MetaVars().ServicePackageName, formatted)
	if err != nil {
		return err
	}
	if err := writeFiles(o.OutputDir, formatted); err != nil {
		return err
	}
	if err := removeStale(o.OutputDir, stale); err != nil {
		return err
	}
	return WriteRegistries(o.OutputDir)
}

// Kind is a kind the generator yields.
type Kind struct {
	// Name is the Kubernetes kind, e.g. VPCEndpoint.
	Name string
	// ModelName is the resource name of the AWS API model, e.g. VpcEndpoint.
	// ignore.resource_names of the generator config matches it.
	ModelName string
}

// ListKinds returns the kinds the generator config of o yields, sorted by
// name, before any resource is ignored by callers.
func ListKinds(ctx context.Context, o Options) ([]Kind, error) {
	cfg, err := loadConfig(o)
	if err != nil {
		return nil, err
	}
	if err := ignoreUnexportedInputs(&cfg, o.ServiceSDKDir); err != nil {
		return nil, err
	}
	svc := strings.ToLower(o.Service)
	api, err := loadAPI(ctx, svc, o, cfg)
	if err != nil {
		return nil, err
	}
	m, err := newModel(api, svc, o, cfg)
	if err != nil {
		return nil, err
	}
	crds, err := m.GetCRDs()
	if err != nil {
		return nil, err
	}
	kinds := make([]Kind, 0, len(crds))
	for _, c := range crds {
		kinds = append(kinds, Kind{Name: c.Kind, ModelName: c.Names.Original})
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i].Name < kinds[j].Name })
	return kinds, nil
}

func loadConfig(o Options) (ackgenconfig.Config, error) {
	cfg, err := ackgenconfig.New(o.GeneratorConfig, defaultConfig)
	if err != nil {
		return cfg, fmt.Errorf("cannot load generator config: %w", err)
	}
	normalizeOperationTypes(&cfg)
	return cfg, nil
}

func loadAPI(ctx context.Context, svc string, o Options, cfg ackgenconfig.Config) (*ackmodel.SDKAPI, error) {
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
	return api, nil
}

// newModel applies the ignore rules of cfg to api, in place.
func newModel(api *ackmodel.SDKAPI, svc string, o Options, cfg ackgenconfig.Config) (*ackmodel.Model, error) {
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
	// Plan holds the checked, kind-specific choices of the templates.
	Plan *kindPlan
}

// ServiceConfig is the optional apis/<service>/codegen.yaml. It configures
// provider-aws-v2 specifics that the ACK generator config does not cover.
type ServiceConfig struct {
	// ClientOperations are SDK operations added to the Client interface of
	// a kind's controller, keyed by kind, e.g. for hand-written code.
	ClientOperations map[string][]string `json:"clientOperations,omitempty"`
	// Kinds holds per-kind settings, keyed by kind.
	Kinds map[string]KindConfig `json:"kinds,omitempty"`
}

// KindConfig holds the codegen.yaml settings of one kind.
type KindConfig struct {
	// Identifier is the read output member that equals the external name,
	// if the generator can't find it.
	Identifier string `json:"identifier,omitempty"`
	// IdentifierInput is the read input member that takes the external
	// name, if it is neither Identifier nor Identifier+"s".
	IdentifierInput string `json:"identifierInput,omitempty"`
	// CustomObserve: the kind has no read operation and a hand-written
	// observe hook decides whether the resource exists.
	CustomObserve bool `json:"customObserve,omitempty"`
	// CustomDelete: the kind has no delete operation and a hand-written
	// delete hook deletes the resource.
	CustomDelete bool `json:"customDelete,omitempty"`
	// OrphanOnDelete: the kind has no delete operation and deleting the
	// managed resource deliberately leaves the AWS resource in place.
	OrphanOnDelete bool `json:"orphanOnDelete,omitempty"`
	// CustomIsUpToDate: the kind reads with GetAttributes, can be updated
	// and a hand-written isUpToDate hook detects drift.
	CustomIsUpToDate bool `json:"customIsUpToDate,omitempty"`
	// ImmutableFields are JSON names of spec.forProvider fields that can't
	// change once set, e.g. create-only fields.
	ImmutableFields []string `json:"immutableFields,omitempty"`
	// AllowSensitiveFields lists secret-looking fields that are knowingly
	// plain API fields.
	AllowSensitiveFields []SensitiveAllowance `json:"allowSensitiveFields,omitempty"`
}

// SensitiveAllowance allows one secret-looking field, for a reason.
type SensitiveAllowance struct {
	// Path is the JSON path, e.g. spec.forProvider.clientToken.
	Path string `json:"path"`
	// Reason says why the value is not a secret; it is required.
	Reason string `json:"reason"`
}

// ServiceConfigPath returns the path of the codegen.yaml of a service.
func ServiceConfigPath(root, svc string) string {
	return filepath.Join(root, "apis", svc, "codegen.yaml")
}

// ReadServiceConfig reads the codegen.yaml of a service, if any. Unknown keys
// are errors, so a typo can't silently drop a setting.
func ReadServiceConfig(root, svc string) (*ServiceConfig, error) {
	c := &ServiceConfig{}
	b, err := os.ReadFile(ServiceConfigPath(root, svc))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	return c, yaml.UnmarshalStrict(b, c)
}

type setupVars struct {
	templateset.MetaVars
	ModulePath string
	CRDs       []*ackmodel.CRD
}

func render(m *ackmodel.Model, o Options, sc *ServiceConfig, plans map[string]*kindPlan) (map[string][]byte, error) { //nolint:gocyclo // linear list of templates
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
		vars := &crdVars{MetaVars: mv, ModulePath: ModulePath, CRD: crd, Plan: plans[crd.Kind]}
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
			s, err := code.SetResource(r.Config(), r, ackmodel.OpTypeList, src, dst, indent)
			if err != nil {
				return "", err
			}
			return firstElementOnly(r.Kind, s)
		},
		"GoCodeSetReadManyInput": func(r *ackmodel.CRD, src, dst string, indent int) (string, error) {
			return code.SetSDK(r.Config(), r, ackmodel.OpTypeList, src, dst, indent)
		},
		"GoCodeGetAttributesSetInput": func(r *ackmodel.CRD, src, dst string, indent int) (string, error) {
			return code.SetSDKGetAttributes(r.Config(), r, src, dst, indent)
		},
		"GoCodeGetAttributesSetOutput": func(r *ackmodel.CRD, src, dst string, indent int) (string, error) {
			return code.SetResourceGetAttributes(r.Config(), r, src, dst, indent)
		},
		"GoCodeSetCreateOutput": func(r *ackmodel.CRD, src, dst string, indent int) (string, error) {
			s, err := code.SetResource(r.Config(), r, ackmodel.OpTypeCreate, src, dst, indent)
			if strings.TrimSpace(s) == "" {
				return "", err
			}
			return s, err
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
		"NotNil":             func(v string, wrapper []string) string { return nilCheck(v, wrapper, true) },
		"AnyNil":             func(v string, wrapper []string) string { return nilCheck(v, wrapper, false) },
		"HasUpdatableFields": HasUpdatableFields,
		"UpdatableFields":    UpdatableFields,
		// IAM is a global service; its resources have no region field.
		"IsGlobalService": func(group string) bool {
			return strings.HasPrefix(group, "iam.")
		},
	}
}

// customTypes returns custom_types.go with empty Custom<Kind>Parameters
// and Custom<Kind>Observation structs for each API version that has none
// yet. The file is hand-written afterwards: it is where additional fields
// such as references are added.
func customTypes(m *ackmodel.Model, o Options) (map[string][]byte, error) {
	crds, err := m.GetCRDs()
	if err != nil {
		return nil, err
	}
	byVersion := map[string][]string{}
	for _, crd := range crds {
		v, err := crd.GetStorageVersion(o.APIVersion)
		if err != nil {
			return nil, err
		}
		byVersion[v] = append(byVersion[v], crd.Kind)
	}
	files := map[string][]byte{}
	for v, kinds := range byVersion {
		path := filepath.Join("apis", m.MetaVars().ServicePackageName, v, "custom_types.go")
		if _, err := os.Stat(filepath.Join(o.OutputDir, path)); err == nil {
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
		files[path] = []byte(b.String())
	}
	return files, nil
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

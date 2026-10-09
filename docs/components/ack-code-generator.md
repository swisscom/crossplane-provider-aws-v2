# ACK code-generator

## What it is

[aws-controllers-k8s/code-generator](https://github.com/aws-controllers-k8s/code-generator)
is the generator behind the AWS Controllers for Kubernetes (ACK) project.
It reads an AWS API model plus a `generator-config.yaml` and builds a
**model of Kubernetes resources**: which kinds exist, which operations
create/read/update/delete them, which fields go to spec and status, and Go
snippets that copy fields between the Kubernetes object and SDK
inputs/outputs. ACK uses it with its own templates and its own runtime.

## Why

- Inferring kinds, operations and field mappings from AWS models is the hard
  part of a generator, and ACK has solved it for hundreds of services.
- provider-aws (v1) was generated with the same tool, so its
  `generator-config.yaml` files can be reused unchanged
  ([ADR 0003](../adr/0003-ack-code-generator-as-a-library.md)).
- Recent versions emit **AWS SDK v2** code.

## What we use and what we don't

| Used | Not used |
|------|----------|
| Model loading (`pkg/sdk`, `pkg/api`) | ACK runtime (`aws-controllers-k8s/runtime`) |
| | ACK's Crossplane target (`ack-generate crossplane`): runtime v1, SDK v1, cluster-scoped; our templates replace it |
| `generator-config.yaml` format (`pkg/config`) | ACK templates, Helm charts, RBAC, release artifacts |
| CRD model: kinds, spec/status fields, operations (`pkg/model`) | ACK metadata/status fields (`IncludeACKMetadata: false`) |
| Code snippets `code.SetSDK`, `code.SetResource`, … (`pkg/generate/code`) | ACK's `compare` delta code (we generate our own diff) |
| Template set rendering (`pkg/generate/templateset`) | |

It is a library dependency of the separate `codegen/` Go module only
(`codegen/go.mod`, currently v0.64.0); the provider binary doesn't depend on
it.

## How it is used

`codegen/internal/generator/generator.go`:

1. `ackgenconfig.New(generator-config.yaml, defaultConfig)`. `defaultConfig`
   points ACK's snippets at Crossplane's layout:
   `.Spec.ForProvider` / `.Status.AtProvider`.
2. Small adaptations before loading the model: legacy operation types of
   provider-aws configs (`read` → `READ_ONE`), unexported SDK members added
   to `ignore.field_paths`.
3. `ackmodel.New(…)` → `m.GetCRDs()`, `GetTypeDefs()`, `GetEnumDefs()`.
4. Our templates call ACK snippet functions through the `funcMap`
   (`GoCodeSetCreateInput`, `GoCodeSetReadOneOutput`, …).
5. Post-processing: unkeyed `metav1.Time{…}` literals are keyed, then
   `goimports`.

## `generator-config.yaml` essentials

The format is ACK's ([reference](https://github.com/aws-controllers-k8s/code-generator/blob/main/pkg/config/config.go)).
Settings used most here:

```yaml
ignore:
  resource_names: [GlobalCluster]       # kinds not to generate
  field_paths:                          # members not to map
    - CreateDBClusterInput.DBClusterIdentifier   # external name instead
    - CreateDBClusterInput.MasterUserPassword    # secret ref instead
operations:
  DescribeDBClusters:
    operation_type: [READ_MANY]         # override inferred operation types
    resource_name: DBCluster
resources:
  DBCluster:
    fields:
      AllowMajorVersionUpgrade:         # field that only an update sends
        from: {operation: ModifyDBCluster, path: AllowMajorVersionUpgrade}
      EngineVersion:                    # observed value in status
        is_read_only: true
        from: {operation: DescribeDBClusters, path: DBClusters.EngineVersion}
    exceptions:
      errors:
        404: {code: DBClusterNotFoundFault}   # -> IsNotFound
    compare:
      ignore: [Tags]                    # excluded from DiffUpdatableParameters
```

`apis/<svc>/generator-config.yaml` of an imported service is written by the
importer (source config + all other kinds ignored); edit it like any other
config afterwards.

## Pitfalls

- **Operation inference** is name based (`Create<Kind>`, `Describe<Kind>s`,
  …). Irregular APIs need `operations:` overrides.
- **Renames** (`renames.operations`) change the spec field name, not the SDK
  name; hand-written code must use the spec name.
- **ReadMany vs ReadOne**: with `Describe…s` (ReadMany) the generated
  observe gets a list and calls `filterList`; most AWS "describe" calls are
  ReadMany.
- ACK snippets are SDK-v2 code, but types in the API stay ACK-style
  (`*int64`, `[]*string`); see [aws-sdk.md](aws-sdk.md#types-api-vs-sdk).

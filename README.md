# provider-aws-v2

A native (no Terraform) [Crossplane](https://crossplane.io/) provider for AWS.

Status: **proof of concept**.

- crossplane-runtime v2, Crossplane v2 (safe-start)
- AWS SDK for Go v2 only
- **namespaced managed resources only**; credentials from a namespaced
  `ProviderConfig` or a cluster-wide `ClusterProviderConfig`
- API groups: `aws2.crossplane.io` (provider configs), `<service>.aws2.crossplane.io` (MRs)

## Project layout

```
apis/
  aws2.go                     # registers all API groups in the scheme
  generate.go                 # controller-gen (deepcopy, CRDs) + angryjet (MR methods)
  v1alpha1/                   # ProviderConfig, ClusterProviderConfig, ProviderConfigUsage
  <service>/<version>/        # one package per AWS service and API version
cmd/provider/                 # provider binary
internal/
  clients/aws/                # AWS SDK v2 config from a (Cluster)ProviderConfig
  controller/
    aws2.go                   # registers all controllers (safe-start gated)
    config/                   # ProviderConfig / ClusterProviderConfig controllers
    <service>/<kind>/         # one controller per MR kind
  version/
package/                      # crossplane.yaml + generated CRDs
examples/                     # example manifests (also used by e2e tests)
cluster/                      # image build and local integration test
hack/                         # boilerplate header
build/                        # crossplane/build submodule (make machinery)
codegen/                      # code generator (own go.mod), see below
```

Planned:
- `importer/` (own `go.mod`): tool to port resources from
  [crossplane-contrib/provider-aws](https://github.com/crossplane-contrib/provider-aws).
- `test/e2e/`: [uptest](https://github.com/crossplane/uptest) tests against real AWS.

## Authentication

`spec.credentials.source` of a `ProviderConfig` / `ClusterProviderConfig`:

| Source | Credentials |
|---|---|
| `Secret` | AWS shared credentials file (`[default]` profile) in a Secret. A namespaced `ProviderConfig` may only reference a Secret in its own namespace. |
| `IRSA`, `PodIdentity` | Provider pod identity via the AWS SDK default credential chain. |
| `WebIdentity` | `sts:AssumeRoleWithWebIdentity` with `spec.assumeRoleWithWebIdentity`. |

Optional `spec.assumeRole` assumes a role on top of any source.
Optional `spec.endpoint.url` overrides the AWS endpoint (e.g. LocalStack).
See [examples/provider/config.yaml](examples/provider/config.yaml).

## Developing

```shell
make submodules      # once: fetch the build submodule
make generate        # regenerate deepcopy, MR methods and CRDs
make reviewable      # generate + lint + test
make build           # binary, image and xpkg
make dev             # kind cluster + CRDs + run the provider locally
```

`make` pins `GOTOOLCHAIN` to the Go version in `go.mod` and downloads it on
first use.

## Generating a service

`codegen/` uses the [ACK code-generator](https://github.com/aws-controllers-k8s/code-generator)
as a library to read the AWS SDK v2 API model and a `generator-config.yaml`,
and renders its own templates (crossplane-runtime v2, namespaced MRs, AWS SDK
v2). The `generator-config.yaml` format is ACK's, so the configs of
crossplane-contrib/provider-aws can be reused (legacy `operation_type: Read`
is accepted).

```shell
# 1. write apis/<service>/generator-config.yaml
# 2. generate types and controllers (adds the SDK service module to go.mod)
make services SERVICES=servicecatalog
# 3. deepcopy, MR methods, CRDs
make generate
```

The API model is fetched from the `service/<service>/<version>` tag of
aws-sdk-go-v2 that matches `go.mod`, and is cached in the user cache directory.

Generated per service:

| File | Content |
|---|---|
| `apis/<svc>/v1alpha1/zz_*.go` | types, enums, one file per kind |
| `apis/<svc>/v1alpha1/custom_types.go` | created once, then hand-written: `Custom<Kind>Parameters`/`Observation` (inlined) |
| `internal/controller/<svc>/<kind>/zz_controller.go` | `Setup`/`SetupGated`, SDK `Client` interface, Observe/Create/Update/Delete with hooks |
| `internal/controller/<svc>/<kind>/zz_conversions.go` | `Generate<Op>Input`, `Generate<Kind>`, `IsNotFound` |
| `internal/controller/<svc>/zz_setup.go` | service `SetupGated` |
| `apis/zz_services.go`, `internal/controller/zz_services.go` | registry of all services |

Generated controllers only map fields. Resource-specific behavior lives in
hand-written files next to `zz_controller.go`, which append to
`externalOptions` (hooks: `preObserve`, `postObserve`, `lateInitialize`,
`isUpToDate`, `preCreate`, `postCreate`, `preUpdate`, `postUpdate`,
`preDelete`, `postDelete`) and `reconcilerOptions` (e.g. initializers) in
`init()`. By default every observed resource is up to date and its status is
not set to Available: these hooks are required for a working controller.

To remove a service, delete its `apis/<svc>` and `internal/controller/<svc>`
directories and CRDs, then run
`cd codegen && go run ./cmd/generate --registries-only --output ..`.

Run `make codegen.test` to test the generator.

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
```

Planned:
- `codegen/` (own `go.mod`): generator that uses the ACK code-generator as a
  library to create types and controllers from the AWS API model.
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

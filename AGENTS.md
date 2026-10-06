# AGENTS.md

Instructions for AI coding agents (and humans) working in this repository.
Keep this file short and current: update it in the same PR that changes a
workflow it describes.

## What this is

`provider-aws-v2` ("provider-aws-v2-native") is a native Crossplane v2 AWS
provider: crossplane-runtime v2, **namespaced-only** managed resources, API
group `*.aws2.crossplane.io`, AWS SDK Go v2, code generated with the
aws-controllers-k8s (ACK) code-generator. Kinds are generated from AWS API
models and, where needed, imported from
[crossplane-contrib/provider-aws](https://github.com/crossplane-contrib/provider-aws)
with `codegen/cmd/import`.

Status: proof of concept. Imported so far: `rds/DBInstance`.

## Layout

| Path | Content |
|------|---------|
| `apis/<svc>/generator-config.yaml` | ACK generator config of a service (hand-written) |
| `apis/<svc>/codegen.yaml` | Extra SDK operations per kind for the generated `Client` interface |
| `apis/<svc>/import.yaml` | Kinds imported from provider-aws (managed by the importer) |
| `apis/<svc>/v1alpha1/zz_*.go` | Generated API types — never edit |
| `apis/<svc>/v1alpha1/*.go` | Hand-written custom types, extra methods |
| `internal/controller/<svc>/<kind>/zz_*.go` | Generated controller — never edit |
| `internal/controller/<svc>/<kind>/setup.go` | Hand-written hooks, wired via `configure`/`wrapExternal` in `init()` |
| `internal/clients/aws` | ProviderConfig → `aws.Config`, usage tracker, error helpers, connection secret keys |
| `internal/clients/<svc>`, `internal/utils/*` | Hand-written helpers (mostly imported) |
| `codegen/` | Separate Go module: generator (`cmd/generate`), importer (`cmd/import`), templates |
| `package/crds` | Generated CRDs |
| `build/` | Crossplane build submodule (`make submodules`) — don't edit |

Design and reasoning: `docs/architecture.md`. Known pitfalls: `docs/porting-notes.md`.

## Commands

```sh
make services SERVICES="rds"   # generate types + controllers from generator-config.yaml
make generate                  # deepcopy, managed methods, reference resolvers, CRDs
go build ./... && go vet ./... && go test ./...   # root module
make codegen.test              # tests of the codegen module
```

Order matters: `make services` → `make generate` → build. Hand-written code
that references generated methods (e.g. `var _ Iface = (*Kind)(nil)`) only
compiles after `make generate`.

## Conventions

- Managed resources are namespaced. Secret refs are `xpv2.LocalSecretKeySelector`
  / `LocalSecretReference` (no namespace field): always resolve them in
  `mg.GetNamespace()`. Never read secrets from other namespaces.
- References use `xpv2.NamespacedReference`/`NamespacedSelector` and the
  `+crossplane:generate:reference` markers; angryjet generates the resolvers.
  Don't hand-write `ResolveReferences` unless the marker can't express it.
- Customize controllers only through the hooks in `zz_controller.go`
  (`configure`, `wrapExternal`, `pre*/post*`, `isUpToDate`, `lateInitialize`).
  If a hook is missing, change the template in `codegen/templates`, regenerate,
  don't edit `zz_` files.
- Need another SDK operation in the controller? Add it to
  `apis/<svc>/codegen.yaml` and run `make services`.
- API types keep ACK's `*int64`/`[]*string`; the SDK v2 uses `*int32`/`[]string`.
  Convert with `internal/utils/pointer` (`Int32`, `Int64`) and
  `aws.ToStringSlice`/`aws.StringSlice`.
- Errors: wrap with `crossplane-runtime/v2/pkg/errors`; AWS errors with
  `awsclient.Wrap`.
- Files start with the Apache 2.0 license header (see `codegen/templates/boilerplate.go.tpl`).

## Don'ts

- Don't edit `zz_*` files, `package/crds`, or `build/`.
- Don't commit `*_importtodo.go` files — they are importer output that still
  needs porting (build tag `importtodo`, excluded from the build).
- Don't add cluster-scoped managed resources or the `publishConnectionDetailsTo`
  / external secret store API — both are gone in Crossplane v2.

## Skills

Task workflows live in `.agents/skills/` (Agent Skills format):

- `import-kind` — import a kind from provider-aws and port it.
- `add-service` — generate a new service/kind from scratch.

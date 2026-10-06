# Architecture

How provider-aws-v2 is built and why. For day-to-day commands see
[`AGENTS.md`](../AGENTS.md) and the [README](../README.md).

## Goals

- Native (non-Terraform) AWS provider for **Crossplane v2**: crossplane-runtime
  v2, namespaced managed resources only, no external secret stores.
- Generated from AWS API models (AWS SDK Go v2 + ACK code-generator), so new
  kinds and API changes cost little.
- Reuse the battle-tested hand-written logic of
  [crossplane-contrib/provider-aws](https://github.com/crossplane-contrib/provider-aws)
  per kind, instead of upgrading that repository in place. Only kinds that are
  needed get imported.
- API group `*.aws2.crossplane.io`, so it can run next to provider-aws during
  migration.

## Pipeline

```
AWS API model (aws-sdk-go-v2)          provider-aws (optional)
        │                                      │
        ▼                                      ▼
 codegen/cmd/generate  ◄── generator-config ── codegen/cmd/import
 (ACK model + our templates)                   (copies + rewrites hand-written code,
        │                                       quarantines what does not compile)
        ▼
 apis/<svc>/v1alpha1/zz_*.go
 internal/controller/<svc>/<kind>/zz_*.go
        │
        ▼
 make generate  (controller-gen, angryjet: deepcopy, managed methods,
                 reference resolvers, CRDs)
```

### Code generator (`codegen/`)

A separate Go module, so its dependencies (ACK code-generator, aws-sdk-go v1
model loader) don't leak into the provider.

- Uses ACK's `code-generator` as a library for the model (CRDs, fields,
  operations, SDK ↔ CR conversion snippets) and renders **our own templates**
  (`codegen/templates`) for crossplane-runtime v2.
- Inputs per service: `apis/<svc>/generator-config.yaml` (ACK config) and
  `apis/<svc>/codegen.yaml` (extra SDK operations per kind).
- AWS API models are downloaded per service and cached
  (`~/Library/Caches/provider-aws-v2-codegen` on macOS).

### Generated controller

Per kind, `zz_controller.go` contains a `Client` interface (only the SDK
operations used), a connector and an `external` client with hooks:

| Hook | Default |
|------|---------|
| `preObserve`, `postObserve`, `filterList` | no-op |
| `isUpToDate` | always up to date |
| `lateInitialize` | no-op |
| `preCreate`/`postCreate`, `preUpdate`/`postUpdate`, `preDelete`/`postDelete` | no-op |

Hand-written code sets two package variables in `init()`:

- `configure(mgr, opts) ([]option, []managed.ReconcilerOption, error)`: hooks
  for the external client, plus reconciler options appended after the defaults
  (so they override them, e.g. a custom connector).
- `wrapExternal(*external) TypedExternalClient`: wraps the client, e.g. to
  override `Create`.

The generated `Setup` already wires logger, poll interval, recorder,
management policies, change logs and MR metrics.

### Importer (`codegen/cmd/import`)

Ports one or more kinds of a provider-aws service:

1. Generator config + `import.yaml`; runs the generator.
2. Custom types closure → `apis/<svc>/v1alpha1`, with xpv1 → namespaced xpv2
   types. Reference markers are rewritten; references to kinds that are not
   imported are dropped (with their fields).
3. Controller `setup.go` and the transitive closure of helper packages are
   copied and rewritten: imports, runtime v1 → v2 symbols, SDK v1 → v2 symbols,
   `Setup` → `configure`, custom connectors to `awsclient.GetConfig` +
   `NewFromConfig`.
4. A type-check loop (go/packages): missing SDK operations are added to
   `codegen.yaml` and the generator reruns; declarations that still fail move
   to `<file>_importtodo.go` (build tag `importtodo`) with the compiler error
   as `TODO(import)`. The repository always builds.

What stays manual is listed in [`porting-notes.md`](porting-notes.md).
Mechanical fixes found during manual ports should be taught to the importer.

## Namespaced resources: decisions

- **Secrets**: all secret refs are local (`LocalSecretKeySelector`) and
  resolved in the MR namespace. Provider-owned helper secrets (e.g. the RDS
  password/restore cache) are also stored in the MR namespace, so tenants are
  isolated and cleanup follows the namespace.
- **Connection details**: only `writeConnectionSecretToRef` (local). The
  well-known keys (`username`, `password`, `endpoint`, `port`, ...) are
  constants in `internal/clients/aws`, as runtime v2 dropped them.
- **References**: `NamespacedReference`/`NamespacedSelector`, resolved in the
  MR namespace by angryjet-generated resolvers.
- **ProviderConfig**: namespaced `ProviderConfig` plus cluster-scoped
  `ClusterProviderConfig` (runtime v2 `ProviderConfigRef` with `kind`).

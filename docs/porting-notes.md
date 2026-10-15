# Porting notes

Pitfalls found while porting kinds from provider-aws (crossplane-runtime v1,
AWS SDK Go v1, cluster-scoped) to this provider. Add an entry when you hit
something new; if the fix is mechanical, teach the importer too.

## AWS SDK Go v1 → v2

| v1 | v2 |
|----|----|
| `*int64` counts/sizes | mostly `*int32` (API types keep `*int64`: use `pointer.Int32`/`pointer.Int64`) |
| `[]*string` | `[]string` (`aws.ToStringSlice`, `aws.StringSlice`) |
| `[]*types.T` | `[]types.T`; `out.Items[0]` is a value |
| `input.SetFoo(x)` | struct literal / field assignment |
| `svc.New(sess)` | `svc.NewFromConfig(cfg)` |
| `client.FooWithContext(ctx, in, ...request.Option)` | `client.Foo(ctx, in, ...func(*svc.Options))` |
| `<svc>iface.<Svc>API` | generated `Client` interface or a small local interface |
| `*string` enums | typed string enums, zero value `""` |
| `awserr.Error` / `Code()` | `errors.As` with typed errors / `smithy.APIError` |

## crossplane-runtime v1 → v2

- `xpv1.SecretKeySelector`/`SecretReference` → `xpv2.LocalSecretKeySelector`/
  `LocalSecretReference`: no namespace field; use `mg.GetNamespace()`.
  Struct literal: `LocalSecretKeySelector{LocalSecretReference: ..., Key: ...}`.
- `xpv1.Reference`/`Selector` → `xpv2.NamespacedReference`/`NamespacedSelector`;
  resolvers use `NewAPINamespacedResolver` and `NamespacedResolutionRequest{..., Namespace: mg.GetNamespace()}`.
- `xpv1.ResourceCredentialsSecret*Key` are gone → `awsclient.ResourceCredentialsSecret*Key`.
- No external secret stores (`publishConnectionDetailsTo`, `ConnectionPublishers`, `StoreConfig`).
- Typed external clients: `managed.TypedExternalConnector[*Kind]`,
  `managed.WithTypedExternalConnector[*Kind](...)` (type parameter may need to be explicit).

## AWS API changes since provider-aws

The importer generates from the current SDK v2 model, not from the model
provider-aws was generated with. New fields come for free, unless they break
generation:

- New fields are not in the hand-written `lateInitialize` of the source.
  `isUpToDate` hooks of provider-aws compare a hand-picked subset of fields;
  base them on the generated `DiffUpdatableParameters` instead.
- `float64` fields are allowed (`crd:allowDangerousTypes=true` in
  `apis/generate.go`); Kubernetes discourages floats because of rounding in
  other languages, the AWS API uses them anyway (e.g. capacity units).
- Input members that SDK v2 keeps unexported (`DestinationRegion`) are
  ignored by the generator automatically.

## Things the compiler does not catch

- **Removed fields in strings**: `cmpopts.IgnoreFields(T{}, "FooRef")` with a
  field that no longer exists panics at runtime. Grep for removed field names
  after dropping references.
- **Hand-built `external{...}`**: every hook field must be set; nil hooks panic.
- **Restore in `preCreate`**: provider-aws restored snapshots in `preCreate`
  and then still ran the create operation. Override `Create` via
  `wrapExternal` instead (done for docdb/DBCluster; rds/DBInstance still
  has the old pattern).
- **Connection details publish order**: the reconciler publishes Observe's
  connection details before Update. A `postObserve` that publishes the
  desired password makes "password changed" checks against the connection
  secret always false; publish it in `postUpdate`.
- **Cache/secret location**: code that used the provider namespace
  (`GetProviderNamespace`) compiles but is wrong for namespaced MRs.
- **nil vs. empty slices**: `aws.StringSlice(nil)` returns an empty, non-nil
  slice. Late-initializing `x = aws.StringSlice(out.X)` turns a nil spec field
  into `[]` — guard with `out.X != nil`. Likewise a test helper
  `f(values ...string) []string { return values }` returns nil for `f()`.

## Importing more kinds of a service

- Re-run the importer with all kinds of the service; old kinds keep their
  ported code. References that were dropped because the target kind was
  missing come back: the importer adds the `*Ref`/`*Selector` fields to the
  existing struct and ports the resolver (docdb `DBCluster` →
  `DBSubnetGroup`/`DBClusterParameterGroup`). Update the README of the old
  kind ("Differences from provider-aws") and its examples.
- Extending a shared SDK fake (`internal/clients/<svc>/fake`): add the
  `Mock<Op>` field, the `Called` slice and the method for every new
  operation, or tests of the new kinds don't compile.
- `aws.ToStringSlice(nil)` returns `[]string{}`; use
  `pointer.SlicePtrToValue` where a nil spec field must stay nil in the
  request (tests compare inputs).

## Tests

- `cmp.Diff` on SDK v2 structs panics on the unexported `noSmithyDocumentSerde`
  field. Use an option that ignores unexported fields (`ignoreUnexported` in
  `internal/controller/rds/dbinstance/setup_test.go`) or
  `cmpopts.IgnoreUnexported(types.T{})`.
- gomock-based kube mocks from provider-aws are not ported; use the
  controller-runtime fake client or `crossplane-runtime/v2/pkg/test.MockClient`.
- SDK fakes: implement the generated `Client` interface with `Mock<Op>`
  function fields; unset mocks return empty outputs
  (`internal/clients/rds/fake`).

## Generation order

`make services` → `make generate` → build. angryjet loads the API package, so
hand-written API files that need generated methods (`var _ I = (*Kind)(nil)`)
break `make generate` if they are written before it ran; the importer
writes them in a second step for this reason.

## Per kind

What was ported, dropped or changed for each kind is in its README
(`internal/controller/<svc>/<kind>/README.md`, "Differences from
provider-aws").

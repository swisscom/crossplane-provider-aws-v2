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

## Things the compiler does not catch

- **Removed fields in strings**: `cmpopts.IgnoreFields(T{}, "FooRef")` with a
  field that no longer exists panics at runtime. Grep for removed field names
  after dropping references.
- **Hand-built `external{...}`**: every hook field must be set; nil hooks panic.
- **Cache/secret location**: code that used the provider namespace
  (`GetProviderNamespace`) compiles but is wrong for namespaced MRs.

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

### rds/DBInstance (commit `75acc8b`)

- ~16 controller and ~40 helper declarations quarantined; all ported.
- `SourceDBClusterIDRef`/`Selector` dropped (DBCluster not imported);
  `SourceDBClusterID` (plain string) kept.
- Password/restore cache secret moved to the MR namespace.
- Only instance parts of `pending_modified_values` ported (cluster parts come with DBCluster).

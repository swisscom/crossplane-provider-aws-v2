---
name: import-kind
description: Import a managed resource kind (e.g. rds/DBInstance) from crossplane-contrib/provider-aws into provider-aws-v2 and port the hand-written code to crossplane-runtime v2, namespaced resources and AWS SDK Go v2. Use when asked to import, migrate or port a kind or service from provider-aws.
---

# Import a kind from provider-aws

The importer does the mechanical part; you port what it could not. Reference
port: commit `75acc8b` (rds/DBInstance). Pitfalls: `docs/porting-notes.md`.

## 1. Prerequisites

- A checkout of `crossplane-contrib/provider-aws` (e.g. `../provider-aws`).
- Clean working tree for the paths the importer writes (it does not overwrite
  existing hand-written files).
- Kinds whose source has a reference to another kind: import the referenced
  kind in the same run (or accept that the reference is dropped).

## 2. Run the importer

```sh
cd codegen
go run ./cmd/import --source ../../provider-aws --service <svc> --kinds <Kind>[,<Kind>] --output .. > /tmp/<svc>-report.md
```

`<svc>` is the aws-sdk-go-v2 service package name (e.g. `rds`, `ec2`). The
importer:

1. Writes `apis/<svc>/import.yaml` and a filtered `generator-config.yaml`,
   runs the generator.
2. Copies custom types (xpv1 → namespaced xpv2 types), rewrites reference
   markers, drops refs to kinds that are not imported.
3. Copies controller `setup.go` (Setup → `configure` hook) and the closure of
   helper packages, rewrites imports/symbols to runtime v2 and SDK v2.
4. Type-checks in a loop: missing SDK operations go to `apis/<svc>/codegen.yaml`;
   every declaration that still does not compile is moved to
   `<file>_importtodo.go` with a `// TODO(import): <compiler error>` comment.

Read the report. The repo builds at this point; the `*_importtodo.go` files
are excluded by the `importtodo` build tag.

If the run fails or the output looks wrong, fix the importer
(`codegen/internal/importer`) rather than patching its output by hand — the
next kind will need the same fix. To start over for a new service, remove
what the importer wrote (check `git status` first — don't discard unrelated
work), keep `apis/<svc>/import.yaml`, and refresh the registries:

```sh
git status --short
rm -rf apis/<svc>/v1alpha1 internal/controller/<svc> internal/clients/<svc> package/crds/<svc>.aws2.crossplane.io_*.yaml
cd codegen && go run ./cmd/generate --registries-only --output ..
```

Helper packages under `internal/utils` and `internal/clients` are shared
between services; only remove the ones `git status` shows as new.

## 3. Port the quarantined code

```sh
find . -name '*_importtodo*.go' -not -path './codegen/*'
grep -rn 'TODO(import)' --include='*_importtodo*.go' .
```

Work bottom-up: `internal/utils/*` → `internal/clients/<svc>` →
`internal/controller/<svc>/utils` → controller `setup.go` → tests. For each
file, move the declarations back into the non-todo file, drop the
`TODO(import)` comments, fix, then delete the `_importtodo` file.

Mechanical fixes (follow the compiler, `go build ./... ` / `go vet ./...`):

- `*int64` ↔ `*int32`: `pointer.Int32(x)` / `pointer.Int64(x)`; in tests
  `aws.Int64(n)` → `aws.Int32(n)` for SDK structs only.
- `[]*string` → `[]string`: `aws.ToStringSlice`; spec fields of custom types
  that are already `[]string` can be assigned directly.
- `[]*svcsdktypes.T` → `[]svcsdktypes.T`; `out.Items[0]` is a value — take
  `&out.Items[0]` where the code needs a pointer.
- SDK v1 `Set*` setters → struct literals.
- `<svc>iface.<Svc>API` → the generated `Client` interface, or a small local
  interface with just the needed methods (e.g. `TagClient`).
- `request.Option` → `func(*svcsdk.Options)`; `Mock*WithContext` → `Mock*`.
- Enums are typed strings in SDK v2 (`types.Foo("x")`, compare with `""`).
- `awserr` codes → `errors.As` with smithy/typed errors.

Semantic fixes (think, don't just make it compile):

- **Secrets**: `LocalSecretKeySelector` has no namespace. Read it from
  `cr.GetNamespace()`. Provider-owned cache secrets live in the MR namespace,
  not the provider namespace.
- **Dropped references**: grep for removed `*Ref`/`*Selector` field names,
  including strings in `cmpopts.IgnoreFields(...)` — they compile but panic
  at runtime.
- **Connection details**: `xpv1.ResourceCredentialsSecret*Key` →
  `awsclient.ResourceCredentialsSecret*Key`. No `publishConnectionDetailsTo`.
- **Connector**: custom connectors use `awsclient.GetConfig(ctx, kube, usage, cr, region)`
  and `svcsdk.NewFromConfig(cfg)`; create the usage tracker once in `configure`.
- **Hooks**: when building `external{...}` by hand, set every hook field
  (nil hooks panic) — compare with `newExternal` in `zz_controller.go`.
- Reconciler options from the old Setup (poll interval, logger, recorder,
  management policies, connection publishers) are provided by the generated
  Setup — don't re-add them.

Tests:

- Port table tests as they are; they are the best evidence the port is right.
- `cmp.Diff` on SDK v2 structs panics on `noSmithyDocumentSerde`: pass an
  option that ignores unexported fields (see `ignoreUnexported` in
  `internal/controller/rds/dbinstance/setup_test.go`).
- gomock kube mocks are not ported: use
  `sigs.k8s.io/controller-runtime/pkg/client/fake` or `crossplane-runtime/v2/pkg/test.MockClient`.
- Fakes of SDK clients: implement the generated `Client` interface with
  `Mock<Op>` function fields (see `internal/clients/rds/fake`).

## 4. Verify

```sh
find . -name '*_importtodo*' -not -path './codegen/*'   # must be empty
grep -rn 'TODO(import)' --include=*.go apis internal    # must be empty
go mod tidy && go build ./... && go vet ./... && go test ./...
```

Optionally create the CRD in envtest (`sigs.k8s.io/controller-runtime/pkg/envtest`,
`CRDDirectoryPaths: package/crds`) with a namespaced object that sets the
secret refs and references, to check the schema keeps those fields.

## 5. Finish

- Add examples under `examples/<svc>/` (namespaced, `metadata.namespace` set)
  covering as many fields as possible; split into several files when fields
  are mutually exclusive or engine-specific (see `examples/rds/`). Validate
  them with `kubectl apply --server-side --dry-run=server --validate=strict`
  against an envtest apiserver that has `package/crds` installed.
- Commit the import and the port together (one commit per kind is fine).
- Add a short entry for anything new you learned to `docs/porting-notes.md`,
  and teach the importer if the fix was mechanical.
- Update the "Imported so far" line in `AGENTS.md`.

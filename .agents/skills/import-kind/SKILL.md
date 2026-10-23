---
name: import-kind
description: Import a managed resource kind (e.g. rds/DBInstance) from crossplane-contrib/provider-aws into provider-aws-v2 and port the hand-written code to crossplane-runtime v2, namespaced resources and AWS SDK Go v2. Use when asked to import, migrate or port a kind or service from provider-aws.
---

# Import a kind from provider-aws

The importer does the mechanical part; you port what it could not. Reference
port: docdb `DBCluster`/`DBInstance` (commits `c501153`, `b74da45`).
Pitfalls: `docs/porting-notes.md`. Background: `docs/components/importer.md`.

## 1. Prerequisites

- A checkout of `crossplane-contrib/provider-aws` (e.g. `../provider-aws`).
- Clean working tree for the paths the importer writes (it does not overwrite
  existing hand-written files).
- Kinds whose source has a reference to another kind: import the referenced
  kind in the same run (or accept that the reference is dropped).
- Adding kinds to a service that has imported kinds: pass all kinds, old and
  new, in `--kinds`. Existing hand-written files are kept; existing custom
  structs get the fields they lack (e.g. `*Ref`/`*Selector` of references
  to the new kinds) — check them in the report
  ([importer.md](../../../docs/components/importer.md#importing-more-kinds-of-a-service)).

## 2. Run the importer

```sh
cd codegen
go run ./cmd/import --source ../../provider-aws --service <svc> --kinds <Kind>[,<Kind>] --output .. > /tmp/<svc>-report.md
```

`<svc>` is the aws-sdk-go-v2 service package name (e.g. `rds`, `ec2`).
`--source` must be the root of a git checkout. Exit codes: 0 done, 1 error
(read stderr), 2 invalid flags (nothing written), 3 written with blocking
findings to port. `go run` prints `exit status 3` but returns 1; the last
stderr line says which case it is. The importer:

1. Derives `apis/<svc>/generator-config.yaml` (never edit it; put repo-local
   settings under `generatorConfig` in `import.yaml`, see
   [importer.md](../../../docs/components/importer.md#derived-generator-configyaml)),
   writes `apis/<svc>/import.yaml` and runs the generator. If
   `generator-config.yaml` was edited by hand, it stops: move the edit,
   delete the file and run again. Prefer importing from a clean checkout.
2. Copies custom types (xpv1 → namespaced xpv2 types) and the declarations
   the source controllers use (e.g. state constants), rewrites reference
   markers, drops refs to kinds that are not imported.
3. Copies controller `setup.go` (Setup → `configure` hook) and the closure of
   helper packages, rewrites imports/symbols to runtime v2 and SDK v2.
4. Type-checks in a loop: missing SDK operations go to `apis/<svc>/codegen.yaml`.
   A function or method whose body does not compile keeps its signature and
   gets `panic("TODO(import): …")`; the original goes to
   `<file>_importtodo.go` with a `// TODO(import): <compiler error>` comment.
   Other declarations that don't compile are moved there whole.
5. Records the source revisions (`revisions` for fresh kinds, `lastImport`).

Read the report: it starts with the number of blocking findings. The repo
builds at this point, but **stubs panic at run time**, including a stubbed
`init` (every test of that package panics). The `*_importtodo.go` files are
excluded by the `importtodo` build tag.

The AWS API model is the one of the current aws-sdk-go-v2 service module, so
it may have fields that provider-aws never had. They come for free, but the
hand-written `lateInitialize` of the source doesn't know them. The source's
`isUpToDate` usually compares a hand-picked subset of fields: rewrite it on
top of the generated `DiffUpdatableParameters` (all updatable fields) and
only add what it can't compare — renamed/reshaped fields, passwords, tags
(see docdb `DBCluster`/`DBInstance`). Remove fields the update operation
rejects with `compare.Without`. `float64` fields are fine (CRDs are
generated with `crd:allowDangerousTypes=true`). If a field breaks
generation, list it under `ignoreFieldPaths` in `apis/<svc>/import.yaml` and
run the importer again (see "start over" below); ignore the input members
and the output shape member, e.g.:

```yaml
ignoreFieldPaths:
    - ModifyAdditionalStorageVolume.SetForDelete
```

Input members that SDK v2 keeps unexported (e.g. `DestinationRegion`) are
ignored by the generator automatically.

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
make check-import-todo   # lists every *_importtodo*.go file and TODO(import) marker
```

Work bottom-up: `internal/utils/*` → `internal/clients/<svc>` →
`internal/controller/<svc>/utils` → controller `setup.go` → tests. For each
file, replace each stub's body with the ported body from the
`_importtodo` file (and move the other declarations back), drop the
`TODO(import)` comments, fix, then delete the `_importtodo` file.

Also port the blocking findings that are notes, not stubs:

- `configure<Kind>` doc: Setup code the generated Setup doesn't do (an
  `else` branch, `Watches`/`Owns`/predicates, an early `return nil`). Port
  it into `configure<Kind>` or decide it's not needed, then remove the note.
- `ResolveReferences` doc: marker references the hand-written method
  doesn't resolve. Resolve them there, or drop the method so angryjet
  generates it.

Mechanical fixes (follow the compiler, `go build ./... ` / `go vet ./...`):

- `*int64` ↔ `*int32`: `pointer.Int32(x)` / `pointer.Int64(x)`; in tests
  `aws.Int64(n)` → `aws.Int32(n)` for SDK structs only.
- `[]*string` → `[]string`: `aws.ToStringSlice`; spec fields of custom types
  that are already `[]string` can be assigned directly.
- `[]*svcsdktypes.T` → `[]svcsdktypes.T`; `out.Items[0]` is a value — take
  `&out.Items[0]` where the code needs a pointer.
- SDK v1 `Set*` setters → struct literals.
- `<svc>iface.<Svc>API` → the generated `Client` interface, or a small local
  interface with just the needed methods (e.g. `TagClient`). Operations the
  importer could not see (code quarantined for `<svc>iface`, e.g. tag
  helpers) must be added to `apis/<svc>/codegen.yaml` by hand, then
  `make services SERVICES=<svc>`.
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
- **Alternative create operations** (restore from snapshot): override
  `Create` with `wrapExternal` instead of calling them in `preCreate`, or the
  generated create runs afterwards.
- **Passwords in connection details**: the reconciler publishes Observe's
  connection details before it calls Update. Don't publish a changed
  password in `postObserve`; publish it in `postUpdate`.
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
make check-import-todo    # must pass
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
- Write `internal/controller/<svc>/<kind>/README.md` following the template
  in `docs/development.md#kind-readme`: origin (revision from
  `import.yaml`), behavior and hooks, differences from provider-aws, fields
  AWS can't change after creation, known limitations, history row.
- Add the kind under "Added" in `CHANGELOG.md`.
- Commit the import and the port together (one commit per kind is fine),
  subject `<svc>/<kind>: import and port from provider-aws`; then add the
  commit to the README history in a follow-up or amend.
- Add a short entry for anything new you learned to `docs/porting-notes.md`,
  and teach the importer if the fix was mechanical.
- Update the "Imported so far" line in `AGENTS.md`.

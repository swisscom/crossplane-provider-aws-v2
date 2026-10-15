# Testing

## Layers

| Layer | What | Where | Run |
|-------|------|-------|-----|
| Unit: hooks | Table tests of `setup.go` hooks and the generated controller with a fake SDK client | `internal/controller/<svc>/<kind>/*_test.go` | `go test ./...` |
| Unit: helpers | `internal/utils/*`, `internal/clients/*` | next to the code | `go test ./...` |
| Generator/importer | Generate a fixture service, check output; importer units | `codegen/internal/*` | `make codegen.test` (part of `make test`) |
| Schema | Install `package/crds` in envtest, apply examples with `--dry-run=server --validate=strict` | ad hoc | see below |
| End to end | Real AWS (planned: [uptest](https://github.com/crossplane/uptest)) | — | — |

## Unit tests of a controller

- **SDK fakes** implement the generated `Client` interface with
  `Mock<Op>` function fields; unset mocks return empty outputs
  (`internal/clients/<svc>/fake`). The docdb fake also records calls
  (`Called`), so tests can assert that an operation was *not* called. Add a field when
  `clientOperations` grows.
- **Kubernetes client**: `crossplane-runtime/v2/pkg/test.MockClient` or
  `sigs.k8s.io/controller-runtime/pkg/client/fake`. gomock is not used.
- Drive the real `external` through `newExternal(kube, fakeClient, opts)`
  (and `wrapExternal` if the kind sets it), so tests cover generated and
  hand-written code together.
- **`cmp.Diff` on SDK v2 structs** panics on the unexported
  `noSmithyDocumentSerde` field: pass an option that ignores unexported
  fields (`ignoreUnexported` in the controller tests) or
  `cmpopts.IgnoreUnexported(T{})`.
- Assert `managed.ExternalObservation.Diff`, not only `ResourceUpToDate`:
  it tells *which* field drifted.

Test a flow, not only a function, where the reconciler's order matters
(e.g. `TestPasswordChange` in docdb DBCluster: isUpToDate → postObserve →
preUpdate → postUpdate).

## Schema checks with envtest

```sh
export KUBEBUILDER_ASSETS="$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@latest use -p path)"
```

Start an envtest API server with `CRDDirectoryPaths: package/crds`, then
`kubectl apply --server-side --dry-run=server --validate=strict -f examples/<svc>/`.
This catches dropped fields (references, secret refs) and invalid
examples. Kill leftover `etcd`/`kube-apiserver` processes afterwards.

## Generator tests

`codegen/internal/generator` generates `servicecatalog` (fixture config in
`codegen/testdata`) and docdb (the repository's config) into a temp dir and
checks snippets of the output, e.g. which fields `DiffUpdatableParameters`
compares. They download the API model on first run and skip if it is not
available.

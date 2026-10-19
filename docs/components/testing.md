# Testing

## Layers

| Layer | What | Where | Run |
|-------|------|-------|-----|
| Unit: hooks | Table tests of `setup.go` hooks and the generated controller with a fake SDK client | `internal/controller/<svc>/<kind>/*_test.go` | `go test ./...` |
| Unit: helpers | `internal/utils/*`, `internal/clients/*` | next to the code | `go test ./...` |
| Generator/importer | Generate a fixture service, check output; importer units | `codegen/internal/*` | `make codegen.test` (part of `make test`) |
| Schema | Install `package/crds` in envtest, apply examples with `--dry-run=server --validate=strict` | ad hoc | see below |
| End to end | Real AWS in a dev cluster; manual for now (planned: [uptest](https://github.com/crossplane/uptest)) | — | [see below](#manual-validation-against-aws) |

## Unit tests of a controller

- **SDK fakes** implement the generated `Client` interface with
  `Mock<Op>` function fields; unset mocks return empty outputs
  (`internal/clients/<svc>/fake`). The docdb fake records calls in
  `Called`, the ec2 fake records every input in `Calls[<operation>]`, so
  tests can assert how often an operation was called, with what input, or
  that it was *not* called. Add a field when `clientOperations` grows.
- **Events**: give the code under test a small recorder that appends
  `event.Event`s to a slice and compare it with `cmp.Diff`
  (`TestAdoptExistingByName` in ec2 SecurityGroup).
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

Check that a new test fails without the change (revert the fix, or
replace the condition with `false`): a test that passes either way tests
nothing.

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

## Manual validation against AWS

Before a kind is released, run it against real AWS in a dev cluster with
the provider installed. This found bugs that unit tests didn't (an update
loop on every poll, wrong docs). Procedure:

1. **Plan the cases** from the kind README: create, no-op reconcile
   (no update loop), each updatable field, immutable fields, references,
   delete, and special behavior (adoption, orphaning with
   `managementPolicies`, dependency errors).
2. **Name and label everything** (`<prefix>-…`, a label like
   `validation=run1`) so that it can be found and deleted in bulk.
3. **Check both sides** after each step: the resource (`Synced`, `Ready`,
   events, `status.atProvider`) and AWS (CLI, console). For "did the
   provider call X?", use CloudTrail (`lookup-events` by resource name);
   it lags a few minutes.
4. **Watch for loops**: an `Update` on every poll shows as repeated
   `UpdatedExternalResource` events or repeated modify calls in CloudTrail.
5. **Clean up and prove it**: delete by label, then check AWS. For an ID
   you no longer have a resource for, an Observe-only resource
   (`managementPolicies: [Observe]`, the ID as external name) reports
   whether it still exists, without touching it.
6. **Write a short report**: what was tested, results, findings, what was
   not tested. Fix findings with a test that reproduces them.

Use read-only AWS calls outside the test resources; the provider, not
you, should change AWS.

## Generator tests

`codegen/internal/generator` generates `servicecatalog` (fixture config in
`codegen/testdata`) and docdb (the repository's config) into a temp dir and
checks snippets of the output, e.g. which fields `DiffUpdatableParameters`
compares. They download the API model on first run and skip if it is not
available.

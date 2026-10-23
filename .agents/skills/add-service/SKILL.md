---
name: add-service
description: Generate a new AWS service or kind from scratch (without provider-aws code) in provider-aws-v2 using the ACK-based code generator. Use when asked to add a new AWS service or resource that does not exist in crossplane-contrib/provider-aws, or should be generated fresh.
---

# Add a service or kind from scratch

For kinds that exist in crossplane-contrib/provider-aws and have meaningful
hand-written logic, prefer the `import-kind` skill.
Background: `docs/components/code-generator.md` (templates, hooks, drift
detection) and `docs/components/ack-code-generator.md` (`generator-config.yaml`).

## 1. Write the generator config

Create `apis/<svc>/generator-config.yaml` (`<svc>` = aws-sdk-go-v2 service
package name). It is an ACK generator config; see the
[ACK docs](https://aws-controllers-k8s.github.io/community/docs/contributor-docs/code-generator-config/)
and `apis/rds/generator-config.yaml` as an example. Typically:

- `ignore.resource_names`: every resource of the service you don't want,
  by AWS model name (`Vpc`, not `VPC`). Before adding ignores, list all
  kinds with `cd codegen && go run ./cmd/generate --list-kinds --service <svc>`.
- `ignore.field_paths`: fields that break generation or must not be exposed
  (input members, e.g. `CreateDBClusterInput.MasterUserPassword`, and output
  shape members). Every entry must match a member of a structure in the API
  model, or generation fails. Ignore the identifier inputs (e.g.
  `CreateDBClusterInput.DBClusterIdentifier`). The generated read, update
  and delete inputs set them from the external name; the create input does
  **not**, so set it in a `preCreate` hook (step 4).
  `float64` fields need no ignore (CRDs are generated with
  `crd:allowDangerousTypes=true`). Members the SDK v2 keeps unexported are
  ignored automatically.
- `resources.<Kind>`: renames, `is_read_only`, custom field config, and
  `exceptions.errors.404.code` (otherwise `IsNotFound` never matches).

Look at the AWS API model of the service to choose Create/Read/Update/Delete
operations; ACK infers them from operation names (`Create<Kind>`, `Describe<Kind>s`, ...).

## 2. Answer the generator's questions

The generator fails closed: if it can't emit a safe default it stops,
writes nothing, and names the line to add. Expect some of these, in
`apis/<svc>/codegen.yaml` under `kinds.<Kind>` unless noted
(full list: `docs/components/code-generator.md#safety-checks-fail-closed`):

- `identifier`: which read output member equals the external name, if it
  isn't `<Kind>Identifier`/`Id`/`Name`.
- A secret-looking field: ignore it in `ignore.field_paths` and add a
  `xpv2.LocalSecretKeySelector` field in `custom_types.go`, resolved in the
  MR's namespace. Use `allowSensitiveFields` (with a reason) only for
  values that aren't secret, such as idempotency tokens.
- No read or delete operation: `customObserve`, `customDelete` or
  `orphanOnDelete`, and write the hook.
- No 404 code: `resources.<Kind>.exceptions.errors.404.code` in
  `generator-config.yaml`.
- Create-only fields: `immutableFields` (JSON names) makes them immutable
  with CEL. `region` is immutable already.

Never silence a check by allowing something you haven't understood.

## 3. Generate

```sh
make services SERVICES="<svc>"   # types, controller, adds the SDK module to go.mod
make generate                    # deepcopy, managed methods, resolvers, CRDs
go build ./...
```

Generated: `apis/<svc>/v1alpha1/zz_*.go`, `internal/controller/<svc>/<kind>/zz_*.go`,
`package/crds/<svc>.aws2.crossplane.io_*.yaml`, and the service registries
`apis/zz_services.go`, `internal/controller/zz_services.go`.

## 4. Customize (only if needed)

The generated `isUpToDate` compares the spec fields that the update
operation sends and the read operation returns (`DiffUpdatableParameters`
in `zz_conversions.go`). Exclude a field with `compare.is_ignored` or
`resources.<Kind>.compare.ignore` in `generator-config.yaml`. Fields that
are named or shaped differently in the read output, and tags, need a
hand-written `isUpToDate` that starts from `DiffUpdatableParameters`
(see `internal/controller/docdb/dbcluster/setup.go`). There is no late
initialization by default. Create writes the AWS response to
`status.atProvider` only, so values AWS fills in reach `spec` only through
a `lateInitialize` hook. If the API normalises identifiers (e.g.
lowercases them), write a `filterList` hook: the default keeps exact
matches only. Add behavior in a hand-written
`internal/controller/<svc>/<kind>/setup.go`:

```go
func init() {
	configure = func(mgr ctrl.Manager, o controller.Options) ([]option, []managed.ReconcilerOption, error) {
		return []option{func(e *external) {
			e.isUpToDate = isUpToDate
			e.preCreate = preCreate
		}}, nil, nil
	}
}
```

- Name in the create input: if AWS takes the resource's name on create
  (most kinds), set it from the external name in `preCreate`. Without it,
  AWS rejects Create or picks its own name, which Observe then never finds.
  From `internal/controller/docdb/dbclusterparametergroup/setup.go`:

  ```go
  func preCreate(_ context.Context, cr *svcapitypes.DBClusterParameterGroup, obj *svcsdk.CreateDBClusterParameterGroupInput) error {
  	obj.DBClusterParameterGroupName = pointer.ToOrNilIfZeroValue(meta.GetExternalName(cr))
  	return nil
  }
  ```
- Extra SDK operations: list them under the kind in `apis/<svc>/codegen.yaml`
  (`clientOperations`) and rerun `make services`.
- Custom spec fields: hand-written `apis/<svc>/v1alpha1/custom_types.go`
  referenced from the generator config.
- References to other kinds: `+crossplane:generate:reference` markers on
  custom fields; namespaced types only (see `AGENTS.md`). To reference a
  generated field, ignore it and re-add it as a custom field.
- IDs assigned by AWS: set the external name in `postCreate` and pass
  `managed.WithInitializers()` (see `internal/controller/ec2/securitygroup`).
- More pitfalls: `docs/components/code-generator.md#pitfalls`,
  `docs/components/ack-code-generator.md#pitfalls`.

## 5. Verify

```sh
go build ./... && go vet ./... && go test ./...
make codegen.test codegen.lint   # if you touched codegen/
```

## 6. Finish

- Add an example manifest under `examples/<svc>/` (namespaced, with
  `metadata.namespace`).
- Write `internal/controller/<svc>/<kind>/README.md` following
  `docs/development.md#kind-readme` (origin: "generated from scratch").
- Add the kind under "Added" in `CHANGELOG.md`.
- Check the definition of done in `docs/development.md`.

## Removing a service

Delete `apis/<svc>`, `internal/controller/<svc>` and its CRDs, then refresh the
registries:

```sh
cd codegen && go run ./cmd/generate --registries-only --output ..
```

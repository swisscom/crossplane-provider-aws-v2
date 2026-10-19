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
  shape members). `float64` fields need no ignore (CRDs are generated with
  `crd:allowDangerousTypes=true`). Members the SDK v2 keeps unexported are
  ignored automatically.
- `resources.<Kind>`: renames, `is_read_only`, custom field config, and
  `exceptions.errors.404.code` (otherwise `IsNotFound` never matches).

Look at the AWS API model of the service to choose Create/Read/Update/Delete
operations; ACK infers them from operation names (`Create<Kind>`, `Describe<Kind>s`, ...).

## 2. Generate

```sh
make services SERVICES="<svc>"   # types, controller, adds the SDK module to go.mod
make generate                    # deepcopy, managed methods, resolvers, CRDs
go build ./...
```

Generated: `apis/<svc>/v1alpha1/zz_*.go`, `internal/controller/<svc>/<kind>/zz_*.go`,
`package/crds/<svc>.aws2.crossplane.io_*.yaml`, and the service registries
`apis/zz_services.go`, `internal/controller/zz_services.go`.

## 3. Customize (only if needed)

The generated `isUpToDate` compares the spec fields that the update
operation sends and the read operation returns (`DiffUpdatableParameters`
in `zz_conversions.go`). Exclude a field with `compare.is_ignored` or
`resources.<Kind>.compare.ignore` in `generator-config.yaml`. Fields that
are named or shaped differently in the read output, and tags, need a
hand-written `isUpToDate` that starts from `DiffUpdatableParameters`
(see `internal/controller/docdb/dbcluster/setup.go`). There is no late
initialization. Add behavior in a hand-written
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

## 4. Verify

```sh
go build ./... && go vet ./... && go test ./...
make codegen.test codegen.lint   # if you touched codegen/
```

## 5. Finish

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

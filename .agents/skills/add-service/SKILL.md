---
name: add-service
description: Generate a new AWS service or kind from scratch (without provider-aws code) in provider-aws-v2 using the ACK-based code generator. Use when asked to add a new AWS service or resource that does not exist in crossplane-contrib/provider-aws, or should be generated fresh.
---

# Add a service or kind from scratch

For kinds that exist in crossplane-contrib/provider-aws and have meaningful
hand-written logic, prefer the `import-kind` skill.

## 1. Write the generator config

Create `apis/<svc>/generator-config.yaml` (`<svc>` = aws-sdk-go-v2 service
package name). It is an ACK generator config; see the
[ACK docs](https://aws-controllers-k8s.github.io/community/docs/contributor-docs/code-generator-config/)
and `apis/rds/generator-config.yaml` as an example. Typically:

- `ignore.resource_names`: every resource of the service you don't want.
- `ignore.field_paths`: fields that break generation or must not be exposed.
- `resources.<Kind>`: renames, `is_read_only`, custom field config.

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

The generated controller has no `isUpToDate` logic (always up to date) and no
late initialization. Add behavior in a hand-written
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
  custom fields; namespaced types only (see `AGENTS.md`).

## 4. Verify

```sh
go build ./... && go vet ./... && go test ./...
make codegen.test   # if you touched codegen/
```

Add an example manifest under `examples/` (namespaced, with `metadata.namespace`).

## Removing a service

Delete `apis/<svc>`, `internal/controller/<svc>` and its CRDs, then refresh the
registries:

```sh
cd codegen && go run ./cmd/generate --registries-only --output ..
```

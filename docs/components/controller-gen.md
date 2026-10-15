# controller-gen (kubebuilder markers)

## What it is

[controller-gen](https://github.com/kubernetes-sigs/controller-tools) from the
Kubebuilder project reads Go types and `// +kubebuilder:…` marker comments
and generates DeepCopy methods and CustomResourceDefinition (CRD) YAML.

## Why

It is the standard way to define Kubernetes APIs in Go; crossplane-runtime,
controller-runtime and every Crossplane provider use it. The Go types are
the single definition; CRDs are never written by hand.

## How it is used

`apis/generate.go` runs it during `make generate`:

```go
//go:generate go run -tags generate sigs.k8s.io/controller-tools/cmd/controller-gen object:headerFile=../hack/boilerplate.go.txt paths=./... crd:crdVersions=v1,allowDangerousTypes=true output:artifacts:config=../package/crds
```

- `object` → `zz_generated.deepcopy.go` in each API package.
- `crd` → `package/crds/<group>_<plural>.yaml`, packaged into the xpkg.
- `allowDangerousTypes=true` permits `float64` fields
  ([ADR 0006](../adr/0006-allow-float-fields.md)).

### Where markers come from

Markers are plain `// +…` comments in the Go API types. Nobody runs a tool
to add them; they are either rendered by our templates or written by hand.

| Source | Markers | In |
|--------|---------|----|
| Our templates, fixed text | `+kubebuilder:object:root`, `resource:scope=Namespaced`, `subresource:status`, `printcolumn`, `storageversion` (`crd.go.tpl`); `+kubebuilder:object:generate`, `+groupName`, `+versionName` (`doc.go.tpl`); `+kubebuilder:skipversion` (`type_def.go.tpl`) | `zz_*.go` |
| Our templates, from the ACK model | `+kubebuilder:validation:Required` when the field is `required` in the AWS model (`$field.IsRequired`) | `zz_<kind>.go` |
| ACK model | Field doc comments (AWS API documentation, `ShapeRef.Documentation`); controller-gen turns them into CRD `description`s | `zz_*.go` |
| Hand-written (or copied by the importer from provider-aws and rewritten) | `+optional`, `+kubebuilder:validation:…`, `+crossplane:generate:reference:…` | `custom_types.go` |

To change a generated marker, change the template and regenerate; to add one
to a single kind, add a field or rule in `custom_types.go`. Markers are read
by controller-gen (`+kubebuilder:…`, `+groupName`) and
[angryjet](angryjet.md) (`+crossplane:generate:reference:…`), both run by
`make generate` from the `//go:generate` lines in `apis/generate.go`.

Common markers:

| Marker | Effect |
|--------|--------|
| `+kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,aws2}` | namespaced CRD, `kubectl get managed`, `kubectl get aws2` |
| `+kubebuilder:subresource:status` | status subresource |
| `+kubebuilder:printcolumn:…` | `READY`, `SYNCED`, `EXTERNAL-NAME`, `AGE` columns |
| `+kubebuilder:validation:Required` / `Optional` | required fields |
| `+kubebuilder:validation:XValidation:rule=…` | CEL rules, e.g. immutability (`self == oldSelf`) |

## Pitfalls

- `package/crds` is deleted and regenerated on every run; never edit it.
- Required fields come from the AWS model (`required` members of the create
  input). Fields replaced by references or secrets must be optional, or the
  reference can never fill them.
- Floats are serialized as JSON numbers; clients in other languages may
  round large values. AWS uses floats only for small values (capacities).

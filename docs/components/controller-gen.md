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

Markers come from the generator templates (`codegen/templates/apis/crd.go.tpl`)
and from hand-written types:

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

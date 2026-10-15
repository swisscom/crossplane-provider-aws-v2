# angryjet (crossplane-tools)

## What it is

[angryjet](https://github.com/crossplane/crossplane-tools) generates the
methods every Crossplane managed resource needs, and the code that resolves
cross-resource references.

## Why

crossplane-runtime works with any managed resource through interfaces
(`resource.Managed`: `GetCondition`, `GetProviderConfigReference`,
`GetManagementPolicies`, …). Writing those accessors per kind is boilerplate;
angryjet derives them from the struct layout (`xpv2.ManagedResourceSpec`
embedded in the spec).

## How it is used

`apis/generate.go`, after controller-gen:

```go
//go:generate go run -tags generate github.com/crossplane/crossplane-tools/cmd/angryjet generate-methodsets --header-file=../hack/boilerplate.go.txt ./...
```

Outputs per API package:

- `zz_generated.managed.go`, `zz_generated.managedlist.go`: interface methods.
- `zz_generated.resolvers.go`: `ResolveReferences` for fields with
  reference markers.

### References

A reference lets a user point at another managed resource instead of
copying its ID. In a hand-written type (`apis/rds/v1alpha1/custom_types.go`):

```go
// +crossplane:generate:reference:type=github.com/swisscom/crossplane-provider-aws-v2/apis/rds/v1alpha1.DBInstance
SourceDBInstanceID *string `json:"sourceDBInstanceID,omitempty"`

// +optional
SourceDBInstanceIDRef *xpv2.NamespacedReference `json:"sourceDBInstanceIDRef,omitempty"`

// +optional
SourceDBInstanceIDSelector *xpv2.NamespacedSelector `json:"sourceDBInstanceIDSelector,omitempty"`
```

The generated resolver looks the target up **in the namespace of the
managed resource** and copies its external name (or the value of an
`extractor`) into the field. The managed reconciler calls it before
every Observe (its default reference resolver); nothing to wire.

## Pitfalls

- angryjet loads the API package: hand-written files that use not-yet
  generated methods break it. Generate first, then add such files (the
  importer does this in two steps).
- Don't hand-write `ResolveReferences` unless the marker can't express the
  reference. One case: the referencing field is generated (in a `zz_`
  file), so it can't carry a marker; docdb `DBInstance.dbClusterIdentifier`
  has a hand-written resolver in `apis/docdb/v1alpha1/dbcluster_dbinstance.go`
  for this reason.
- References to kinds that don't exist in this provider can't be generated;
  keep the plain field.

# docdb/DBInstance

Manages an instance of an Amazon DocumentDB cluster
(`DBInstance.docdb.aws2.crossplane.io`).

## Origin

Imported from provider-aws `swisscom/provider-aws@ad8ac93` (clean;
`apis/docdb/import.yaml`), ported by hand in `c501153`.

## Behavior

- **External name**: `metadata.name` by default; used as
  `DBInstanceIdentifier`. Matched case-insensitively, because AWS stores
  identifiers in lowercase.
- **Engine**: the DocumentDB API also returns RDS and Neptune instances. An
  instance whose engine is not `docdb` is refused before anything is copied
  into spec or status:
  `DBInstance "orders-1" has engine "postgres", not "docdb"; refusing to manage it`
  (shown as `late-init failed: ...`). Deleting such an MR removes the
  finalizer without calling AWS. No Describe filter is sent: AWS documents
  the engine filter only for `DescribeDBClusters`.
- **Cluster**: `dbClusterIdentifier`, or a reference to a `docdb/DBCluster`
  (`dbClusterIdentifierRef`/`Selector`), resolved in the namespace of the
  referencing resource unless `namespace` is set on the reference
  (hand-written resolver in `apis/docdb/v1alpha1/dbcluster_dbinstance.go`).
- **Drift**: generated `DiffUpdatableParameters`, minus
  `autoMinorVersionUpgrade`; plus `enablePerformanceInsights`, the
  Performance Insights KMS key (key ID equals key ARN),
  `preferredMaintenanceWindow` (case-insensitive; AWS returns it in
  lowercase) and tags. A `dbInstanceClass` or `caCertificateIdentifier`
  that AWS holds in `pendingModifiedValues` counts as applied, so the
  instance is not modified again on every poll until the maintenance window.
- **Tags**: as for `docdb/DBCluster`: spec tags are added or updated, only
  keys the provider recorded in `docdb.aws2.crossplane.io/managed-tag-keys`
  are removed (ADR 0015).
- **Update**: `ModifyDBInstance` without `AutoMinorVersionUpgrade`, then tags.
- **Delete**: skipped while the instance is already `deleting`.
- **Connection details**: `endpoint`, `port` (credentials are on the cluster).
- **Conditions**: from the instance status.

| Hook | Why |
|------|-----|
| `preObserve`, `preCreate`, `preDelete`, `filterList` | identifiers from the external name, foreign instance dropped when deleting |
| `lateInitialize` | engine guard first; AZ, CA certificate, maintenance window, promotion tier |
| `isUpToDate` | see Drift |
| `postObserve`, `postCreate` | conditions, connection details, persisting the managed tag keys |
| `preUpdate`, `postUpdate` | drop unsupported fields, tags |

## Differences from provider-aws

- All updatable fields are compared.
- Instances of other engines are not adopted, modified or deleted.
- Pending changes are not re-sent; tags not set by the provider are kept.
- Secrets/references resolved in the MR's namespace.

## Limitations

- `autoMinorVersionUpgrade` can't be changed after creation (DocumentDB's
  ModifyDBInstance rejects it).
- Not compared because AWS can't change them: `availabilityZone`, `engine`.
- `caCertificateIdentifier` exists in both generated and custom parameters;
  the generated field wins and the custom one is dead.
- Only `dbInstanceClass` and `caCertificateIdentifier` are read from
  `pendingModifiedValues`; other pending fields can't be set in the spec.
- Tag keys are recorded only with the `LateInitialize` (or `*`) management
  policy. Without it the provider never removes tags.

## Examples

[`examples/docdb/dbinstance.yaml`](../../../../examples/docdb/dbinstance.yaml).

## History

| Date | Commit | Change |
|------|--------|--------|
| 2026-10-16 | unreleased | Engine guard; pending `dbInstanceClass`/`caCertificateIdentifier` not re-sent; case-insensitive identifier and maintenance window; tags removed only if the provider set them |
| 2026-10-16 | unreleased | Create no longer writes `spec.forProvider` (status only) |
| 2026-10-16 | unreleased | `region` is immutable |
| 2026-10-16 | unreleased | `pendingModifiedValues.masterUserPassword` removed from status |
| 2026-10-09 | `b74da45` | Compare all updatable fields |
| 2026-10-09 | `c501153` | Imported from provider-aws and ported to runtime v2, SDK v2, namespaced secrets |

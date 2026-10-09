# docdb/DBInstance

Manages an instance of an Amazon DocumentDB cluster
(`DBInstance.docdb.aws2.crossplane.io`).

## Origin

Imported from provider-aws `swisscom/provider-aws@ad8ac93` (clean;
`apis/docdb/import.yaml`), ported by hand in `2362ae0`.

## Behavior

- **External name**: `metadata.name` by default; used as
  `DBInstanceIdentifier`.
- **Cluster**: `dbClusterIdentifier`, or a reference to a `docdb/DBCluster`
  in the same namespace (`dbClusterIdentifierRef`/`Selector`; hand-written
  resolver in `apis/docdb/v1alpha1/dbcluster_dbinstance.go`).
- **Drift**: generated `DiffUpdatableParameters`, minus
  `autoMinorVersionUpgrade`; plus `enablePerformanceInsights`, the
  Performance Insights KMS key (key ID equals key ARN) and tags.
- **Update**: `ModifyDBInstance` without `AutoMinorVersionUpgrade`, then tags.
- **Delete**: skipped while the instance is already `deleting`.
- **Connection details**: `endpoint`, `port` (credentials are on the cluster).
- **Conditions**: from the instance status.

| Hook | Why |
|------|-----|
| `preObserve`, `preCreate`, `preDelete`, `filterList` | identifiers from the external name |
| `lateInitialize` | AZ, CA certificate, maintenance window, promotion tier |
| `isUpToDate` | see Drift |
| `postObserve`, `postCreate` | conditions, connection details |
| `preUpdate`, `postUpdate` | drop unsupported fields, tags |

## Differences from provider-aws

- All updatable fields are compared.
- Secrets/references resolved in the MR's namespace.

## Limitations

- `autoMinorVersionUpgrade` can't be changed after creation (DocumentDB's
  ModifyDBInstance rejects it).
- Not compared because AWS can't change them: `availabilityZone`, `engine`.
- `caCertificateIdentifier` exists in both generated and custom parameters;
  the generated field wins and the custom one is dead.

## Examples

[`examples/docdb/dbinstance.yaml`](../../../../examples/docdb/dbinstance.yaml).

## History

| Date | Commit | Change |
|------|--------|--------|
| 2026-10-09 | `1a8d835` | Compare all updatable fields |
| 2026-10-09 | `2362ae0` | Imported from provider-aws and ported to runtime v2, SDK v2, namespaced secrets |

# rds/DBInstance

Manages an Amazon RDS DB instance (`DBInstance.rds.aws2.crossplane.io`):
standalone, read replica, restored, or a member of an Aurora cluster.

## Origin

Imported from provider-aws (`swisscom/provider-aws`, revision not recorded;
the importer records revisions since docdb), ported by hand in `75acc8b`.

## Behavior

- **External name**: `metadata.name` by default; `DBInstanceIdentifier`.
- **Create**: `CreateDBInstance` with a password from
  `masterUserPasswordSecretRef` or `autogeneratePassword`. Read replicas
  (`sourceDBInstanceID`/`Ref`/`Selector`, `sourceDBClusterID`) use
  `CreateDBInstanceReadReplica` (overridden `Create`).
- **Restore**: `restoreFrom.source` `S3`, `Snapshot` or `PointInTime`.
- **Password**: a provider-owned cache secret in the MR's namespace stores
  the last applied password and the restore state; changes are detected
  against it.
- **Drift**: hand-written; builds a JSON patch between desired spec and the
  observed state (`createPatch`) with special rules for engine version,
  option/parameter groups, VPC security groups, backup window and retention,
  storage (gp3 thresholds), additional storage volumes, KMS key,
  `monitoringInterval` (0 matches monitoring off) and SDK-v2-only fields. No update while `modifying`, `upgrading`, `rebooting`,
  `creating` or `deleting`.
- **Delete**: final snapshot options, `deleteAutomatedBackups`; the cache
  secret is deleted afterwards.
- **Status**: `databaseRole` (standalone, primary, read replica, cluster
  writer/reader); conditions from the instance status.
- **Connection details**: `username`, `password`, `endpoint`, `port`.

## Differences from provider-aws

- Password/restore cache secret lives in the MR's namespace, not the
  provider's.
- Fields that only exist in SDK v2 are compared (see history).
- `sourceDBClusterIDRef`/`Selector` dropped (rds/DBCluster not imported);
  `sourceDBClusterID` kept.
- Only the instance part of `pending_modified_values` is ported.

## Limitations

- Drift detection doesn't use the generated `DiffUpdatableParameters` yet;
  new updatable fields from SDK updates need hand-written handling.
- Restores run in `preCreate`, after which the generated `Create` still calls
  `CreateDBInstance` (the same pattern was fixed for docdb/DBCluster by
  overriding `Create`). To be verified against AWS and fixed.

## Examples

[`examples/rds/`](../../../../examples/rds/): standalone, Oracle, read
replica, restore.

## History

| Date | Commit | Change |
|------|--------|--------|
| 2026-10-19 | | Fix update loop with `monitoringInterval: 0` (monitoring off) |
| 2026-10-07 | `6a1364d` | Drift detection for fields new in SDK v2 (database insights, network type, …); additional storage volumes; managed password toggling |
| 2026-10-06 | `68972b4` | Comprehensive examples |
| 2026-10-05 | `75acc8b` | Imported from provider-aws and ported to runtime v2, SDK v2, namespaced secrets |

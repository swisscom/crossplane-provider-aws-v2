# rds/DBInstance

Manages an Amazon RDS DB instance (`DBInstance.rds.aws2.crossplane.io`):
standalone, read replica, restored, or a member of an Aurora cluster.

## Origin

Imported from provider-aws `swisscom/provider-aws@ad8ac93` (backfilled in `apis/rds/import.yaml`: the import did not record its revision; the rds source is identical from `5750cb1` to `ad8ac93`, the port has `5750cb1`'s DBPortNumber fix and not the later `f68427c`), ported by hand in `e43afb1`.

## Behavior

- **External name**: `metadata.name` by default; `DBInstanceIdentifier`.
  Matched case-insensitively (AWS stores identifiers in lowercase).
- **Foreign engines**: `DescribeDBInstances` also returns DocumentDB and
  Neptune instances. An instance with engine `docdb` or `neptune` is never
  managed: Observe fails with "refusing to manage it", and a deleted MR whose
  name matches one is released without an AWS call.
- **Create**: `CreateDBInstance` with a password from
  `masterUserPasswordSecretRef` or `autogeneratePassword` (generated once; a
  retry reuses the cached one). With `manageMasterUserPassword: true` no
  password is sent; combining it with a ref or `autogeneratePassword` is an
  error. A ref to a missing Secret or an empty value is an error. Read
  replicas (`sourceDBInstanceID`/`Ref`/`Selector`, `sourceDBClusterID`) use
  `CreateDBInstanceReadReplica`. Create writes `status.atProvider` only; the
  first Observe late-initializes unset spec fields.
- **Restore**: `restoreFrom.source` `S3`, `Snapshot` or `PointInTime` replaces
  `CreateDBInstance` (overridden `Create`). The password is cached before the
  restore call. S3 restores send it; snapshot and point-in-time restores keep
  the source's password until the instance is available, then the provider
  applies the cached or referenced password with `ModifyDBInstance`.
- **Password**: a provider-owned cache Secret `dbinstance.<uid>` in the MR's
  namespace holds the password AWS has, and a "not applied yet" flag. It is
  owned by the MR (controller ownerReference, not blocking deletion), so
  garbage collection removes it. A stored password is never replaced by an
  empty one. A changed `masterUserPasswordSecretRef` value is detected
  against the cache and applied with `ModifyDBInstance`. Every modify that
  sends a password first caches it with the flag set; postUpdate clears the
  flag. After a failed modify, a lost response or a failed cache write, the
  next round re-sends the same password and publishes nothing new until it
  succeeded (the connection secret keeps its last value). So after a lost
  modify response or a failed cache write, the connection secret can briefly
  keep the replaced password until the re-send succeeds (usually one error
  backoff); it heals by itself. AWS applies a new
  master password asynchronously, as soon as possible, regardless of
  `applyImmediately`; the connection secret switches after
  `ModifyDBInstance` succeeded. While AWS manages the password
  (`manageMasterUserPassword`), the ref is not compared. Turning management
  off needs `masterUserPasswordSecretRef` or `autogeneratePassword`; a
  generated password is reused until AWS has confirmed it.
- **TDE**: `tdeCredentialPasswordSecretRef` (Secret in the MR's namespace) is
  sent on create and with every modify, not on restores. A changed value is
  not detected as drift; it goes out with the next modify.
- **Drift**: hand-written; builds a JSON patch between desired spec and the
  observed state (`createPatch`) with special rules for engine version,
  option/parameter groups, VPC security groups (order ignored), backup window
  and retention, storage (gp3 thresholds), additional storage volumes,
  CloudWatch log exports (as a set), `monitoringInterval` (0 matches
  monitoring off) and SDK-v2-only fields. AWS's form counts
  as the user's: `engine`, `dbInstanceClass`, `storageType`, `licenseModel`
  and `caCertificateIdentifier` compare case-insensitively; `kmsKeyID` and
  `performanceInsightsKMSKeyID` match a key ID against its key ARN; an alias
  matches any key (an alias ARN any key in its partition, region and
  account), see Limitations. No update while `modifying`, `upgrading`,
  `rebooting`, `creating` or `deleting`.
- **Tags** ([ADR 0015](../../../../docs/adr/0015-docdb-tag-ownership.md)):
  spec tags are added, a changed value is overwritten with
  `AddTagsToResource`. Only keys the provider recorded in the annotation
  `rds.aws2.crossplane.io/managed-tag-keys` and that left the spec (or all of
  them when `tags` is unset) are removed; tags set by others (AWS Backup,
  tag policies, the console) are kept. `aws:` keys and `tagsIgnore` matches
  are never touched. Only removes under management policies with
  `LateInitialize` or `*`, which persist the record.
- **Delete**: if the spec sets `deletionProtection: false` and AWS may still
  have it on, Delete first sends only `ModifyDBInstance{deletionProtection:
  false, applyImmediately: true}` and deletes in the next round; its error is
  reported. Then final snapshot options and `deleteAutomatedBackups`; the
  cache Secret is deleted afterwards.
- **Status**: `databaseRole` (standalone, primary, read replica, cluster
  writer/reader; unset if the cluster does not list the instance); conditions
  from the instance status.
- **Connection details**: `username`, `password`, `endpoint`, `port`. The
  password is the one AWS has: absent while a restore has not applied it,
  empty while AWS manages it. Observe never writes a Secret.

## Differences from provider-aws

- Password/restore cache secret lives in the MR's namespace, not the
  provider's, and is owned by the MR.
- Fields that only exist in SDK v2 are compared (see history).
- `sourceDBClusterIDRef`/`Selector` dropped (rds/DBCluster not imported);
  `sourceDBClusterID` kept.
- Only the instance part of `pending_modified_values` is ported.
- `tdeCredentialPassword` is a Secret reference.

## Limitations

- Drift detection doesn't use the generated `DiffUpdatableParameters` yet;
  new updatable fields from SDK updates need hand-written handling.
- KMS aliases are not resolved: an alias in `kmsKeyID` or
  `performanceInsightsKMSKeyID` matches any observed key ARN (an alias ARN
  any key in the same partition, region and account). An instance encrypted
  with a different key is reported up to date, and changing the alias is not
  detected as drift.
- The cache Secret holds the password in plaintext, like the connection
  Secret next to it; see ADR 0013.
- MRs whose cache was emptied by the old restore bug and use only
  `autogeneratePassword` have no known password; set
  `masterUserPasswordSecretRef` to give the instance one.

## Examples

[`examples/rds/`](../../../../examples/rds/): standalone, Oracle (incl. TDE),
read replica, restore, Secrets Manager managed password.

## History

| Date | Commit | Change |
|------|--------|--------|
| 2026-10-23 | — | Removes only the tags it recorded (`rds.aws2.crossplane.io/managed-tag-keys`, BREAKING); AWS errors without request IDs |
| 2026-10-19 | `bea5e9f` | Fix update loop with `monitoringInterval: 0` (monitoring off) |
| 2026-10-16 | — | Password lifecycle (ADR 0013): publish only applied passwords, restore overrides Create, `manageMasterUserPassword` create/toggle, cache Secret owned by the MR and never emptied |
| 2026-10-16 | — | Delete lifts only `deletionProtection`; tag value changes keep the tag; nil-safety; spec security groups no longer reordered; drift no longer printed to stderr |
| 2026-10-16 | — | Engine guard for DocumentDB/Neptune instances; case-insensitive external name; normalised values (case, KMS key ID/ARN/alias) |
| 2026-10-16 | — | `tdeCredentialPassword` removed, `tdeCredentialPasswordSecretRef` added (BREAKING) |
| 2026-10-16 | — | Generator: Create no longer writes `spec.forProvider`; `region` is immutable; `pendingModifiedValues.masterUserPassword` removed from status |
| 2026-10-07 | `9e2ab6c` | Drift detection for fields new in SDK v2 (database insights, network type, …); additional storage volumes; managed password toggling |
| 2026-10-06 | `344c433` | Comprehensive examples |
| 2026-10-05 | `e43afb1` | Imported from provider-aws and ported to runtime v2, SDK v2, namespaced secrets |

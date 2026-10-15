# docdb/DBCluster

Manages an Amazon DocumentDB cluster (`DBCluster.docdb.aws2.crossplane.io`).
Instances of the cluster are separate `docdb/DBInstance` resources.

## Origin

Imported from provider-aws `swisscom/provider-aws@ad8ac93` (clean;
`apis/docdb/import.yaml`), ported by hand in `2362ae0`.

## Behavior

- **External name**: `metadata.name` by default; used as
  `DBClusterIdentifier` in every call.
- **References**: `dbSubnetGroupNameRef`/`Selector` to a
  `docdb/DBSubnetGroup` and `dbClusterParameterGroupNameRef`/`Selector` to a
  `docdb/DBClusterParameterGroup` in the same namespace (hand-written
  resolver in `apis/docdb/v1alpha1/dbcluster.go`).
- **Create**: `CreateDBCluster` with the master password from
  `masterUserPasswordSecretRef`, or generated with `autogeneratePassword`
  and written to that secret. With `manageMasterUserPassword` AWS manages it
  in Secrets Manager.
- **Restore**: with `restoreFrom.source` `Snapshot` or `PointInTime`,
  `Create` is overridden (`wrapExternal`) and calls
  `RestoreDBClusterFromSnapshot` / `RestoreDBClusterToPointInTime` instead
  of `CreateDBCluster`. The password is set by the first update afterwards.
- **Drift**: generated `DiffUpdatableParameters` (all updatable fields),
  plus: engine version (upgrades only), parameter group, CloudWatch log
  exports, VPC security groups, `manageMasterUserPassword`, master password,
  tags. No update while the cluster is `modifying` or `upgrading`.
- **Update**: `ModifyDBCluster`; then a second call for engine version /
  parameter group if still needed, and tag changes.
- **Delete**: `finalDBSnapshotIdentifier` / `skipFinalSnapshot` are passed.
- **Connection details**: `username`, `endpoint`, `port`, `readerEndpoint`,
  `password`. A changed password is published after the update that applied
  it, not by Observe.
- **Conditions**: from the cluster status (`available` → Available).

| Hook | Why |
|------|-----|
| `preObserve`, `preDelete`, `filterList` | identifier from the external name; final snapshot |
| `lateInitialize` | AWS defaults (backup window, port, ServerlessV2 capacity, …) |
| `isUpToDate` | fields the generated diff can't compare (see Drift) |
| `postObserve` | conditions, connection details |
| `preCreate`, `customExternal.Create` | password, restore paths |
| `preUpdate`, `postUpdate` | log exports, password, managed password toggling, engine/param group, tags |

## Differences from provider-aws

- All updatable fields are compared (provider-aws compared a hand-picked
  subset).
- Restore no longer calls `CreateDBCluster` after the restore.
- A changed password is actually sent to AWS (provider-aws published it to
  the connection secret during Observe, so the change was never detected).
- `serverlessV2ScalingConfiguration` (new in the SDK v2 model) is supported.
- Secrets are read from the MR's namespace.
- References to kms/Key and ec2/SecurityGroup were dropped (kinds not
  imported); the plain fields stay.

## Limitations

- Fields AWS can't change after creation are not compared, so changing them
  has no effect: `availabilityZones`, `dbSubnetGroupName`, `engine`,
  `globalClusterIdentifier`, `kmsKeyID`, `masterUsername`,
  `storageEncrypted`, `preSignedURL`, `sourceRegion`.
- `masterUserSecretKMSKeyID` can't be changed for an existing managed secret.
- Password changes are detected by comparing with the connection secret;
  without `writeConnectionSecretToRef` they are not detected. (rds/DBInstance
  uses a provider-owned cache secret instead; the same approach would fix
  this.)

## Examples

[`examples/docdb/dbcluster.yaml`](../../../../examples/docdb/dbcluster.yaml),
[`examples/docdb/dbcluster-restore.yaml`](../../../../examples/docdb/dbcluster-restore.yaml).

## History

| Date | Commit | Change |
|------|--------|--------|
| 2026-10-15 | `0bc1dd7` | References to DBSubnetGroup and DBClusterParameterGroup |
| 2026-10-09 | `1a8d835` | Compare all updatable fields; restore without CreateDBCluster; password change applied before it is published |
| 2026-10-09 | `3adf7eb` | `serverlessV2ScalingConfiguration` supported (float fields allowed) |
| 2026-10-09 | `2362ae0` | Imported from provider-aws and ported to runtime v2, SDK v2, namespaced secrets |

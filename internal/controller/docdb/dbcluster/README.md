# docdb/DBCluster

Manages an Amazon DocumentDB cluster (`DBCluster.docdb.aws2.crossplane.io`).
Instances of the cluster are separate `docdb/DBInstance` resources.

## Origin

Imported from provider-aws `swisscom/provider-aws@ad8ac93` (clean;
`apis/docdb/import.yaml`), ported by hand in `c501153`.

## Behavior

- **External name**: `metadata.name` by default; used as
  `DBClusterIdentifier` in every call. Matched case-insensitively, because
  AWS stores identifiers in lowercase.
- **Engine**: the DocumentDB API also returns RDS and Neptune clusters, which
  share the identifier name space. `DescribeDBClusters` is sent with the
  AWS-documented filter `engine=docdb`, and a cluster whose engine is not
  `docdb` is refused before anything is copied into spec or status:
  `DBCluster "orders" has engine "aurora-mysql", not "docdb"; refusing to manage it`
  (shown as `late-init failed: ...`). Deleting such an MR removes the
  finalizer without calling AWS.
- **References**: `dbSubnetGroupNameRef`/`Selector` to a
  `docdb/DBSubnetGroup` and `dbClusterParameterGroupNameRef`/`Selector` to a
  `docdb/DBClusterParameterGroup`, resolved in the namespace of the
  referencing resource unless `namespace` is set on the reference
  (hand-written resolver in `apis/docdb/v1alpha1/dbcluster.go`).
- **Create**: `CreateDBCluster` with the master password from
  `masterUserPasswordSecretRef`, or generated with `autogeneratePassword`
  and written to that secret. With `manageMasterUserPassword` AWS manages it
  in Secrets Manager.
- **Restore**: with `restoreFrom.source` `Snapshot` or `PointInTime`,
  `Create` is overridden (`wrapExternal`) and calls
  `RestoreDBClusterFromSnapshot` / `RestoreDBClusterToPointInTime` instead
  of `CreateDBCluster`. The restored cluster has the backup's password; the
  referenced (or generated) one is applied by the first update once the
  cluster is available.
- **Master password**: after AWS accepted a password (Create or Modify), the
  provider records its SHA-256 digest, never the password, in the Secret
  `docdb-dbcluster.<metadata.uid>.password-state` in the MR's namespace. The
  Secret has a controller owner reference to the DBCluster
  (`blockOwnerDeletion: false`) and is garbage collected with it. A
  referenced password whose digest differs is a diff
  (`masterUserPasswordSecretRef`), with or without
  `writeConnectionSecretToRef`. Without a state Secret (MRs from before it),
  a connection secret that already has the password counts as applied;
  otherwise the referenced password is applied once. A changed password is
  always sent with `ApplyImmediately=true`. While `manageMasterUserPassword`
  is true, the referenced password is not compared, sent or published. See
  ADR 0014.
- **Drift**: generated `DiffUpdatableParameters` (all updatable fields),
  plus: engine version (upgrades only), parameter group name and
  `preferredMaintenanceWindow` (both case-insensitive; AWS returns them in
  lowercase), CloudWatch log exports, VPC security groups,
  `manageMasterUserPassword`, master password, tags. While the cluster is
  `creating`, `modifying`, `upgrading`, `backing-up`,
  `resetting-master-credentials`, `rebooting`, `renaming`, `starting`,
  `stopping`, `migrating` or `maintenance` it is reported up to date (AWS
  rejects a Modify then); `failed` and unknown states still show their diff.
- **Tags**: every tag of `spec.tags` is added or updated in place
  (`AddTagsToResource` overwrites the value). The provider removes only keys
  it set itself and that left the spec; it records them in the annotation
  `docdb.aws2.crossplane.io/managed-tag-keys`, which the runtime persists
  with the late-initialization update. Tags from tag policies, AWS Backup,
  the console or `aws:` tags are never removed. Unset `tags` adds nothing
  but still removes recorded keys. See ADR 0015.
- **Update**: `ModifyDBCluster`; then a second call for engine version /
  parameter group if still needed, and tag changes. The password digest is
  recorded right after the first call, so a failing second call does not
  resend it.
- **Delete**: `finalDBSnapshotIdentifier` / `skipFinalSnapshot` are passed;
  skipped while the cluster is already `deleting`.
- **Connection details**: `username`, `endpoint`, `port`, `readerEndpoint`,
  `password`. `username` and `port` come from the observed cluster (spec as
  fallback), so restored clusters publish them too; `port` is left out while
  unknown. A changed password is published after the update that applied
  it, not by Observe; an update that sent no password publishes none.
- **Conditions**: from the cluster status (`available` → Available).

| Hook | Why |
|------|-----|
| `preObserve`, `preDelete`, `filterList` | identifier from the external name, engine filter, foreign cluster dropped when deleting, final snapshot, no second delete |
| `lateInitialize` | engine guard first; AWS defaults (backup window, port, ServerlessV2 capacity, …) |
| `isUpToDate` | transitional states, fields the generated diff can't compare (see Drift), password state, tags |
| `postObserve` | conditions, connection details, persisting the managed tag keys |
| `preCreate`, `postCreate`, `customExternal.Create` | password and its state, restore paths |
| `preUpdate`, `postUpdate` | log exports, password and its state, managed password toggling, engine/param group, tags |

## Differences from provider-aws

- All updatable fields are compared (provider-aws compared a hand-picked
  subset).
- Restore no longer calls `CreateDBCluster` after the restore.
- A changed password is actually sent to AWS, immediately (provider-aws
  published it to the connection secret during Observe, so the change was
  never detected).
- Clusters of other engines are not adopted, modified or deleted.
- Tags not set by the provider are kept.
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
- Password changes are always applied immediately; a change for the
  maintenance window can't be expressed.
- If the name of a not-deleted DBCluster MR is used by a non-DocumentDB
  cluster, Observe finds nothing and Create fails with
  `DBClusterAlreadyExistsFault`. Nothing is written to the other cluster;
  change the MR's external name.
- Tag keys are recorded only with the `LateInitialize` (or `*`) management
  policy. Without it the provider never removes tags.
- The provider writes the password state Secret in the MR's namespace (it
  already writes the connection secret and generated passwords there).

## Examples

[`examples/docdb/dbcluster.yaml`](../../../../examples/docdb/dbcluster.yaml),
[`examples/docdb/dbcluster-restore.yaml`](../../../../examples/docdb/dbcluster-restore.yaml).

## History

| Date | Commit | Change |
|------|--------|--------|
| 2026-10-16 | unreleased | Engine guard and `engine=docdb` filter; password applied-state Secret, `ApplyImmediately` with a password, rotation without connection secret, generated password applied after restore; no Modify in transitional states, no second delete; username/port from the observed cluster; tags removed only if the provider set them; case-insensitive identifier, window and parameter group name |
| 2026-10-16 | unreleased | Create no longer writes `spec.forProvider` (status only) |
| 2026-10-16 | unreleased | `region` is immutable |
| 2026-10-15 | `0bc1dd7` | References to DBSubnetGroup and DBClusterParameterGroup |
| 2026-10-09 | `b74da45` | Compare all updatable fields; restore without CreateDBCluster; password change applied before it is published |
| 2026-10-09 | `11ef368` | `serverlessV2ScalingConfiguration` supported (float fields allowed) |
| 2026-10-09 | `c501153` | Imported from provider-aws and ported to runtime v2, SDK v2, namespaced secrets |

# docdb/DBSubnetGroup

Manages an Amazon DocumentDB subnet group
(`DBSubnetGroup.docdb.aws2.crossplane.io`).

## Origin

Imported from provider-aws `swisscom/provider-aws@ad8ac93` (clean;
`apis/docdb/import.yaml`), ported by hand in `0bc1dd7`.

## Behavior

- **External name**: `metadata.name` by default; used as
  `DBSubnetGroupName` in every call. Matched case-insensitively, because AWS
  stores names in lowercase.
- **Create**: `CreateDBSubnetGroup` with `subnetIDs` and tags.
- **Drift**: generated `DiffUpdatableParameters`
  (`dbSubnetGroupDescription`), plus `subnetIDs` (same set, any order) and
  tags.
- **Tags**: as for `docdb/DBCluster`: spec tags are added or updated, only
  keys the provider recorded in `docdb.aws2.crossplane.io/managed-tag-keys`
  are removed (ADR 0015).
- **Update**: `ModifyDBSubnetGroup` with description and subnets, then tag
  changes (`AddTagsToResource` / `RemoveTagsFromResource`).
- **Conditions**: Available once observed.

| Hook | Why |
|------|-----|
| `preObserve`, `preCreate`, `preDelete`, `filterList` | name from the external name |
| `preCreate`, `preUpdate` | `subnetIDs` is a custom field (`SubnetIds` is ignored in the generator config) |
| `isUpToDate` | subnets and tags, which the generated diff can't compare |
| `postObserve`, `postUpdate` | condition, persisting the managed tag keys, tags |

## Differences from provider-aws

- Drift detection reports which field differs.
- Tags not set by the provider are kept.
- The reference of `subnetIDs` to ec2 `Subnet` (`subnetIDsRefs`,
  `subnetIDsSelector`) was dropped: ec2 is not imported. Set `subnetIDs`.
- New in the SDK v2 model: `status.atProvider.supportedNetworkTypes`.

## Limitations

- **Shared name space, no engine check.** RDS, Neptune and DocumentDB subnet
  groups are the same AWS objects, and a subnet group has no engine
  attribute. An MR whose name matches an existing RDS or Neptune subnet group
  adopts it, and a change of `subnetIDs` or the description then changes
  that group for its RDS/Neptune databases. Create and Delete are safe (AWS
  rejects a duplicate name and deleting a group in use). Use names that
  cannot collide with RDS groups, or adopt an existing group with
  `managementPolicies: [Observe]` first.
- Tag keys are recorded only with the `LateInitialize` (or `*`) management
  policy. Without it the provider never removes tags.

## Examples

[`examples/docdb/dbsubnetgroup.yaml`](../../../../examples/docdb/dbsubnetgroup.yaml).

## History

| Date | Commit | Change |
|------|--------|--------|
| 2026-10-16 | unreleased | Case-insensitive name; tags removed only if the provider set them; shared name space documented |
| 2026-10-16 | unreleased | Create no longer writes `spec.forProvider` (status only) |
| 2026-10-16 | unreleased | `region` is immutable |
| 2026-10-15 | `0bc1dd7` | Imported from provider-aws and ported to runtime v2, SDK v2, namespaced |

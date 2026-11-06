# docdb/DBSubnetGroup

Manages an Amazon DocumentDB subnet group
(`DBSubnetGroup.docdb.aws2.crossplane.io`).

## Origin

Imported from provider-aws `crossplane-contrib/provider-aws@ad8ac93` (clean;
`apis/docdb/import.yaml`), ported by hand in `0bc1dd7`.

## Behavior

- **External name**: `metadata.name` by default; used as
  `DBSubnetGroupName` in every call.
- **Create**: `CreateDBSubnetGroup` with `subnetIDs` and tags.
- **Drift**: generated `DiffUpdatableParameters`
  (`dbSubnetGroupDescription`), plus `subnetIDs` (same set, any order) and
  tags.
- **Update**: `ModifyDBSubnetGroup` with description and subnets, then tag
  changes (`AddTagsToResource` / `RemoveTagsFromResource`).
- **Conditions**: Available once observed.

| Hook | Why |
|------|-----|
| `preObserve`, `preCreate`, `preDelete`, `filterList` | name from the external name |
| `preCreate`, `preUpdate` | `subnetIDs` is a custom field (`SubnetIds` is ignored in the generator config) |
| `isUpToDate` | subnets and tags, which the generated diff can't compare |
| `postObserve`, `postUpdate` | condition, tags |

## Differences from provider-aws

- Drift detection reports which field differs.
- The reference of `subnetIDs` to ec2 `Subnet` (`subnetIDsRefs`,
  `subnetIDsSelector`) was dropped: ec2 is not imported. Set `subnetIDs`.
- New in the SDK v2 model: `status.atProvider.supportedNetworkTypes`.

## Limitations

- None known; all fields of the spec can be changed.

## Examples

[`examples/docdb/dbsubnetgroup.yaml`](../../../../examples/docdb/dbsubnetgroup.yaml).

## History

| Date | PR | Change |
|------|--------|--------|
| 2026-11-04 | [#4](https://github.com/swisscom/crossplane-provider-aws-v2/pull/4) | Create no longer copies the AWS response into `spec.forProvider` (status only) |
| 2026-11-04 | [#4](https://github.com/swisscom/crossplane-provider-aws-v2/pull/4) | Tags starting with `aws:` are ignored instead of being removed ([ADR 0011](../../../../docs/adr/0011-only-spec-tags.md)) |
| 2026-10-15 | `0bc1dd7` | Imported from provider-aws and ported to runtime v2, SDK v2, namespaced |

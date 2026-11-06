# docdb/DBClusterParameterGroup

Manages an Amazon DocumentDB cluster parameter group
(`DBClusterParameterGroup.docdb.aws2.crossplane.io`).

## Origin

Imported from provider-aws `crossplane-contrib/provider-aws@ad8ac93` (clean;
`apis/docdb/import.yaml`), ported by hand in `0bc1dd7`.

## Behavior

- **External name**: `metadata.name` by default; used as
  `DBClusterParameterGroupName` in every call.
- **Create**: `CreateDBClusterParameterGroup` with family, description and
  tags. AWS creates the group with default parameters; the parameters of
  the spec are set by the first update.
- **Late initialization**: parameters of the group that the spec doesn't
  list are added to `parameters` with their current values
  (`DescribeDBClusterParameters`, all pages).
- **Drift**: generated `DiffUpdatableParameters` (no fields:
  `ModifyDBClusterParameterGroup` only takes parameters), plus the values of
  `parameters` and tags. A changed `dbParameterGroupFamily` or `description`
  is reported as an error (AWS can't change them).
- **Update**: `ModifyDBClusterParameterGroup` with all parameters of the
  spec, then tag changes.
- **Conditions**: Available once observed.

| Hook | Why |
|------|-----|
| `preObserve`, `preCreate`, `preDelete`, `filterList` | name from the external name |
| `lateInitialize` | parameters from `DescribeDBClusterParameters` |
| `isUpToDate` | parameters, tags, immutable family/description |
| `preUpdate`, `postUpdate` | parameters (custom field), tags |
| `postObserve` | condition |

## Differences from provider-aws

- Drift detection reports which field differs.
- `DescribeDBClusterParameters` follows pagination markers.

## Limitations

- `dbParameterGroupFamily` and `description` can't be changed after
  creation; changing them makes the resource fail to sync.
- `applyMethod` is not compared: AWS only changes it together with the
  value.
- Every update sends all parameters of the spec (including late-initialized
  ones). AWS accepts at most 20 parameters per call; DocumentDB families
  have fewer.
- Parameters removed from the spec are not reset to their defaults.

## Examples

[`examples/docdb/dbclusterparametergroup.yaml`](../../../../examples/docdb/dbclusterparametergroup.yaml).

## History

| Date | PR | Change |
|------|--------|--------|
| 2026-11-04 | [#4](https://github.com/swisscom/crossplane-provider-aws-v2/pull/4) | Create no longer copies the AWS response into `spec.forProvider` (status only) |
| 2026-11-04 | [#4](https://github.com/swisscom/crossplane-provider-aws-v2/pull/4) | Tags starting with `aws:` are ignored instead of being removed ([ADR 0011](../../../../docs/adr/0011-only-spec-tags.md)) |
| 2026-10-15 | `0bc1dd7` | Imported from provider-aws and ported to runtime v2, SDK v2, namespaced |

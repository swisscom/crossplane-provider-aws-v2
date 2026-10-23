# docdb/DBClusterParameterGroup

Manages an Amazon DocumentDB cluster parameter group
(`DBClusterParameterGroup.docdb.aws2.crossplane.io`).

## Origin

Imported from provider-aws `swisscom/provider-aws@ad8ac93` (clean;
`apis/docdb/import.yaml`), ported by hand in `0bc1dd7`.

## Behavior

- **External name**: `metadata.name` by default; used as
  `DBClusterParameterGroupName` in every call. Matched case-insensitively,
  because AWS stores names in lowercase.
- **Family**: Aurora and Neptune parameter groups share the name space. A
  group whose family does not start with `docdb` is refused before its
  parameters are read:
  `DBClusterParameterGroup "x" has family "aurora-mysql8.0", not a DocumentDB (docdb*) family; refusing to manage it`.
  Deleting such an MR removes the finalizer without calling AWS.
- **Create**: `CreateDBClusterParameterGroup` with family, description and
  tags. AWS creates the group with default parameters; the parameters of
  the spec are set by the first update.
- **Late initialization**: parameters of the group that the spec doesn't
  list are added to `parameters` with their current values
  (`DescribeDBClusterParameters`, all pages). A missing `applyMethod`
  becomes `pending-reboot` (valid for static and dynamic parameters);
  parameters without a value are skipped (the CRD requires one).
- **Drift**: generated `DiffUpdatableParameters` (no fields:
  `ModifyDBClusterParameterGroup` only takes parameters), plus the values of
  `parameters` and tags. A changed `dbParameterGroupFamily` or `description`
  is reported as an error (AWS can't change them).
- **Update**: `Update` is overridden (`wrapExternal`): only parameters whose
  value differs from AWS are sent, at most 20 per
  `ModifyDBClusterParameterGroup` call (AWS's limit), then tag changes. A
  tags-only change makes no Modify call.
- **Tags**: as for `docdb/DBCluster`: spec tags are added or updated, only
  keys the provider recorded in `docdb.aws2.crossplane.io/managed-tag-keys`
  are removed (ADR 0015).
- **Conditions**: Available once observed.

| Hook | Why |
|------|-----|
| `preObserve`, `preCreate`, `preDelete`, `filterList` | name from the external name, foreign group dropped when deleting |
| `lateInitialize` | family guard first; parameters from `DescribeDBClusterParameters` |
| `isUpToDate` | parameters, tags, immutable family/description |
| `customExternal.Update`, `postUpdate` | changed parameters in batches, tags |
| `postObserve` | condition, persisting the managed tag keys |

## Differences from provider-aws

- Drift detection reports which field differs.
- `DescribeDBClusterParameters` follows pagination markers.
- Only changed parameters are sent, in batches of 20.
- Groups of other engines are not adopted, modified or deleted; tags not
  set by the provider are kept.

## Limitations

- `dbParameterGroupFamily` and `description` can't be changed after
  creation; changing them makes the resource fail to sync.
- `applyMethod` is not compared: AWS only changes it together with the
  value.
- Parameters removed from the spec are not reset to their defaults.
- Tag keys are recorded only with the `LateInitialize` (or `*`) management
  policy. Without it the provider never removes tags.

## Examples

[`examples/docdb/dbclusterparametergroup.yaml`](../../../../examples/docdb/dbclusterparametergroup.yaml).

## History

| Date | Commit | Change |
|------|--------|--------|
| 2026-10-16 | unreleased | Family guard; case-insensitive name; only changed parameters, 20 per call; `applyMethod` default and late-init without values; tags removed only if the provider set them |
| 2026-10-16 | unreleased | Create no longer writes `spec.forProvider` (status only) |
| 2026-10-16 | unreleased | `region` is immutable |
| 2026-10-15 | `0bc1dd7` | Imported from provider-aws and ported to runtime v2, SDK v2, namespaced |

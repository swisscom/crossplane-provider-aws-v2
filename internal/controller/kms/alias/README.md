# kms/Alias

Manages an alias of an AWS KMS key (`Alias.kms.aws2.crossplane.io`).

## Origin

Generated from scratch. provider-aws has an `Alias` kind with a
hand-written controller that the importer can't take over; its behavior
was reimplemented with the generated controller.

## Behavior

- **External name**: `metadata.name` by default. The alias name is
  `alias/<external name>`; names starting with `aws/` are rejected.
- **Target key**: `targetKeyId` (key ID or ARN), or
  `targetKeyIdRef`/`targetKeyIdSelector` to a `Key` in the same namespace.
- **Observe**: `ListAliases` over all pages, then the alias with the name.
  The status has the alias ARN, the dates and the target key ID.
- **Drift**: `targetKeyId` (a key ARN is compared by its key ID).
- **Update**: `UpdateAlias` with the new target key.
- **Delete**: `DeleteAlias`.
- **Conditions**: Available once observed.
- **IAM**: `kms:ListAliases`, `CreateAlias`, `UpdateAlias`, `DeleteAlias`,
  on the alias and on the target keys.

| Hook | Why |
|------|-----|
| client wrapper (`listAllAliases`) | `ListAliases` can't filter by name |
| `filterList` | the alias with the name |
| `preCreate`, `preUpdate`, `preDelete` | alias name from the external name, `targetKeyId` is a custom field with a reference |
| `isUpToDate` | `targetKeyId` is a custom field |
| `postObserve` | condition |

## Differences from provider-aws

- Changing `targetKeyId` updates the alias. provider-aws never detected
  the change; it also looked the alias up by the new target key, didn't
  find it and tried to create it again.

## Limitations

- Every observation lists all aliases of the account and Region (one call
  per 50 aliases).

## Examples

[`examples/kms/alias.yaml`](../../../../examples/kms/alias.yaml).

## History

| Date | PR | Change |
|------|--------|--------|
| 2026-11-16 | | Generated and implemented |

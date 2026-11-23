# kms/Grant

Manages a grant on an AWS KMS key (`Grant.kms.aws2.crossplane.io`).

## Origin

Imported from provider-aws `crossplane-contrib/provider-aws@ad8ac93` (clean;
`apis/kms/import.yaml`) and ported by hand.

## Behavior

- **External name**: the grant ID that AWS assigns, set after create.
- **Key**: `keyId` (key ID or ARN), or `keyIdRef`/`keyIdSelector` to a
  `Key` in the same namespace.
- **Create**: `CreateGrant` with grantee, operations, constraints and
  retiring principal.
- **Observe**: `ListGrants` filtered by key and grant ID.
- **Drift**: none. KMS grants can't be changed; recreate the grant to
  change it.
- **Delete**: `RevokeGrant`; a grant that no longer exists counts as
  deleted.
- **Conditions**: Available once observed.
- **IAM**: `kms:ListGrants`, `CreateGrant`, `RevokeGrant`.

| Hook | Why |
|------|-----|
| `preObserve`, `preCreate` | `keyId` is a custom field with a reference (`KeyId` is ignored in the generator config) |
| `postCreate` | external name from the grant ID |
| `postObserve` | condition |
| `delete` | grants are revoked, KMS has no delete operation |

## Differences from provider-aws

- Revoking a grant that no longer exists no longer fails.
- "Not found" errors are recognized (`NotFoundException`); before, the
  generated check used a code KMS doesn't return.

## Limitations

- All fields are immutable; changes to the spec are not applied.
- Retiring a grant (`RetireGrant`) is not supported.

## Examples

[`examples/kms/grant.yaml`](../../../../examples/kms/grant.yaml).

## History

| Date | PR | Change |
|------|--------|--------|
| 2026-11-16 | | Imported from provider-aws and ported to runtime v2, SDK v2, namespaced |

# kms/Alias

Manages an AWS KMS alias (`Alias.kms.aws2.crossplane.io`): a name that
points to a KMS key.

## Origin

Generated from scratch on 2026-11-20 (`apis/kms/generator-config.yaml`).

## Behavior

- **Spec**: `targetKeyID` (key ID or ARN), or `targetKeyIDRef`/`Selector`
  to a `Key` in the same namespace.
- **External name**: the alias name, by default `metadata.name`. The
  `alias/` prefix is added if missing, so `my-app` and `alias/my-app` both
  mean `alias/my-app`. Use the `crossplane.io/external-name` annotation for
  names `metadata.name` can't hold (e.g. with `/`).
- **Observe**: `ListAliases`, all pages (100 aliases each), then the alias
  with that name; KMS has no operation that reads one alias. Needs only
  `kms:ListAliases`, no permission on the target key.
- **Drift**: `targetKeyID`; a key ID equals its ARN.
- **Create**: `CreateAlias`. **Update**: `UpdateAlias` (points the alias to
  another key). **Delete**: `DeleteAlias`.
- **Status**: `aliasARN`, `targetKeyID`, `creationDate`,
  `lastUpdatedDate`.
- **Conditions**: Available once the alias exists.
- **Changed outside Crossplane**: an alias pointed to another key is
  pointed back; a deleted alias is created again.
- **IAM**: `kms:ListAliases`; `kms:CreateAlias`, `kms:UpdateAlias` and
  `kms:DeleteAlias` on the alias and on the keys involved (key policy).

| Hook | Why |
|------|-----|
| client wrapper `allPages`, `preObserve`, `filterList` | find the alias by name across all pages |
| `postObserve` | Available condition |
| `isUpToDate` | target compared as key ID or ARN |
| `preCreate`, `preUpdate`, `preDelete` | alias name from the external name; target key |

## Limitations

- Alias names starting with `alias/aws/` are reserved by AWS.
- `targetKeyID` must be a key ID or ARN, not another alias.
- An alias can't point to a key that is pending deletion.

## Examples

[`examples/kms/alias.yaml`](../../../../examples/kms/alias.yaml), referencing the
Key of [`examples/kms/key.yaml`](../../../../examples/kms/key.yaml).

## History

| Date | PR | Change |
|------|--------|--------|
| 2026-11-20 | | Generated from scratch |

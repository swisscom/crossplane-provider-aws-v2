# kms/Grant

Manages an AWS KMS grant (`Grant.kms.aws2.crossplane.io`): permission for
a principal to use a KMS key in some operations.

## Origin

Generated from scratch on 2026-11-20 (`apis/kms/generator-config.yaml`;
`RevokeGrant` mapped as the delete operation).

## Behavior

- **Spec**: `keyID` (key ID or ARN), or `keyIDRef`/`Selector` to a `Key`
  in the same namespace; `operations`; `granteePrincipal` or
  `granteeServicePrincipal`; optional `retiringPrincipal` or
  `retiringServicePrincipal`, `constraints`, `name`.
- **External name**: the grant ID, set after creation (AWS assigns it).
- **Create**: `CreateGrant`. `name` defaults to `metadata.name`: with a
  name, KMS returns the existing grant when the same request is repeated,
  so a retried create doesn't make a duplicate.
- **Connection details**: `grantID` and `grantToken` (returned only on
  creation; a token lets the grantee use the grant before it is
  propagated in KMS).
- **Observe**: `ListGrants` filtered by key and grant ID. The key is the
  observed one (`status.atProvider.keyID`), else `spec.forProvider.keyID`.
- **Drift**: grants can't be changed. If `keyID`, `name`, `operations`,
  `constraints` or a principal differ from the grant, Observe returns an
  error naming them; create a new Grant instead. A grantee or retiring
  principal that KMS returns as a service principal instead of an ARN
  isn't compared.
- **Delete**: `RevokeGrant`; a grant that no longer exists is ignored.
- **Status**: `grantID`, `keyID` (key ARN), `creationDate`,
  `issuingAccount`.
- **Conditions**: Available once the grant exists.
- **Changed outside Crossplane**: a revoked or retired grant is created
  again, with a new grant ID.
- **IAM** (key policy): `kms:CreateGrant`, `kms:ListGrants`,
  `kms:RevokeGrant`.

| Hook | Why |
|------|-----|
| `preObserve`, `filterList` | find the grant by key and grant ID |
| `postObserve` | Available condition |
| `isUpToDate` | grants are immutable: differences are errors |
| `preCreate` | key ID; default name |
| `postCreate` | external name; grant token as connection detail |
| `preDelete` | key and grant ID |

## Limitations

- A grant can't be created on a disabled key or a key pending deletion.
- Grant tokens for `CreateGrant` (`grantTokens`) are not supported.
- The grant isn't retired with `RetireGrant`; deletion needs
  `kms:RevokeGrant`.

## Examples

[`examples/kms/grant.yaml`](../../../../examples/kms/grant.yaml), referencing the
Key of [`examples/kms/key.yaml`](../../../../examples/kms/key.yaml).

## History

| Date | PR | Change |
|------|--------|--------|
| 2026-11-20 | | Generated from scratch |

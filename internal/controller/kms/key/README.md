# kms/Key

Manages a customer managed AWS KMS key (`Key.kms.aws2.crossplane.io`),
including its key policy, enabled state, automatic rotation and tags.

## Origin

Generated from scratch on 2026-11-20 (`apis/kms/generator-config.yaml`,
`apis/kms/codegen.yaml`; `ScheduleKeyDeletion` mapped as the delete
operation).

## Behavior

- **External name**: the key ID, set after creation (AWS assigns it). To
  manage an existing key, set the `crossplane.io/external-name` annotation
  to its key ID.
- **Create**: `CreateKey` with `description`, `keySpec`, `keyUsage`,
  `origin`, `multiRegion`, `customKeyStoreID`, `xksKeyID`, `policy`,
  `bypassPolicyLockoutSafetyCheck` and the spec tags (no others are added).
  `enabled: false` and rotation are applied by the first update.
- **Observe**: `DescribeKey`. The other read operations are called only if
  the spec sets their field, so credentials without them can manage keys
  that don't use them: `GetKeyPolicy` (`policy`), `ListResourceTags`
  (`tags`), `GetKeyRotationStatus` (`enableKeyRotation`,
  `rotationPeriodInDays`).
- **Drift**: unset fields aren't managed.
  - `description`.
  - `policy`, compared as JSON (AWS reformats policies).
  - `enabled` (default `true`), while the key is `Enabled`, `Disabled` or
    pending deletion; in other states KMS can't enable or disable it.
  - `enableKeyRotation`; `rotationPeriodInDays` while rotation is on.
  - tags (keys starting with `aws:` are ignored).
  - `keySpec`, `keyUsage`, `origin`, `multiRegion`, `customKeyStoreID` and
    `xksKeyID` can't be changed: a difference is reported as an error.
- **Scheduled for deletion outside Crossplane**: while the resource
  exists, a key in `PendingDeletion` (or `PendingReplicaDeletion`) is
  drift. The update cancels the deletion (`CancelKeyDeletion`; KMS restores
  the key disabled) and then enables it if `enabled` is `true`.
- **Update**, in this order: `CancelKeyDeletion`, `EnableKey`,
  `UpdateKeyDescription`, `PutKeyPolicy` (policy name `default`, with
  `bypassPolicyLockoutSafetyCheck`), `UntagResource`/`TagResource`,
  `EnableKeyRotation` (with `rotationPeriodInDays`) or
  `DisableKeyRotation`, `DisableKey`. Enabling comes first and disabling
  last because KMS can't change the rotation of a disabled key.
- **Delete**: `ScheduleKeyDeletion` with `pendingWindowInDays` (7–30, AWS
  default 30). Once the key is pending deletion, the resource is gone for
  Crossplane; AWS deletes the key after the window. It can be restored
  with `CancelKeyDeletion` until then.
- **Key deleted in AWS** (after the window): `DescribeKey` returns
  `NotFoundException`, and a new key with a new ID is created. Data
  encrypted with the old key can't be decrypted with the new one.
- **Status**: the `KeyMetadata` of `DescribeKey` (`arn`, `keyID`,
  `keyState`, `enabled`, `creationDate`, `deletionDate`, algorithms, …),
  plus `keyRotationEnabled`, `rotationPeriodInDays` and `nextRotationDate`
  when rotation is managed.
- **Conditions** (from `keyState`):

  | Key state | Ready condition |
  |-----------|-----------------|
  | `Enabled`, `Disabled`, `Updating` | Available |
  | `Creating` | Creating |
  | `PendingDeletion`, `PendingReplicaDeletion` | Unavailable ("scheduled for deletion") |
  | `PendingImport` | Unavailable ("import key material") |
  | `Unavailable` | Unavailable (custom key store disconnected) |

- **IAM**: `kms:CreateKey`, `kms:DescribeKey`, `kms:ScheduleKeyDeletion`,
  `kms:TagResource` (for tags on create); and only when the field is used:
  `kms:GetKeyPolicy`/`kms:PutKeyPolicy`, `kms:ListResourceTags`/
  `kms:UntagResource`, `kms:GetKeyRotationStatus`/`kms:EnableKeyRotation`/
  `kms:DisableKeyRotation`, `kms:EnableKey`/`kms:DisableKey`,
  `kms:UpdateKeyDescription`, `kms:CancelKeyDeletion`. All but
  `kms:CreateKey` are evaluated against the key policy: a `policy` must
  keep them for the provider's principal.

| Hook | Why |
|------|-----|
| `preObserve`, `preDelete` | key ID from the external name; pending window |
| `postObserve` | conditions from the key state; pending deletion of a deleted resource means gone |
| `isUpToDate` | KMS has no update operation; compares the fields above with their read operations |
| `update` | one operation per differing field, in a fixed order |
| `postCreate` | external name from the assigned key ID |

## Limitations

- Rotation can't be changed while the key is disabled and stays disabled
  (`enabled: false`): KMS rejects it.
- `rotationPeriodInDays` without `enableKeyRotation` only applies while
  rotation is on.
- Imported key material, multi-Region replicas (`ReplicateKey`),
  on-demand rotation and policies other than `default` are not managed.
- A key created by `CreateKey` whose external name could not be saved
  (e.g. the API server was unreachable) is left behind; CreateKey is not
  idempotent.

## Examples

[`examples/kms/key.yaml`](../../../../examples/kms/key.yaml).

## History

| Date | PR | Change |
|------|--------|--------|
| 2026-11-20 | | Generated from scratch |

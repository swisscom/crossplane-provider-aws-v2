# kms/Key

Manages an AWS KMS key (`Key.kms.aws2.crossplane.io`).

## Origin

Imported from provider-aws `crossplane-contrib/provider-aws@ad8ac93` (clean;
`apis/kms/import.yaml`) and ported by hand.

## Behavior

- **External name**: the key ID that AWS assigns, set after create.
- **Create**: `CreateKey` with the generated spec fields (description, key
  spec and usage, policy, tags, multi-Region, ...).
- **Late initialization**: `policy` (the default policy AWS creates when
  the spec has none) and `enabled`.
- **Drift**: `description` (if set), `enabled`, `policy` (compared as
  parsed JSON, ignoring formatting; statement order counts),
  `enableKeyRotation` and tags. KMS has no update operation for the other
  fields, so there is no generated diff.
- **Update**: only the operations of the fields that differ:
  `UpdateKeyDescription`, `PutKeyPolicy`,
  `EnableKeyRotation`/`DisableKeyRotation`, `TagResource`/`UntagResource`,
  `EnableKey`/`DisableKey`.
- **Delete**: `ScheduleKeyDeletion` with `pendingWindowInDays` (AWS
  default: 30 days); not called again once a deletion date is set.
- **Conditions**: Available when `Enabled` or `Updating`; Creating while
  `Creating`; Unavailable when `Disabled`, `PendingImport` or `Unavailable`.
  A key in `PendingDeletion` or `PendingReplicaDeletion` is reported as not
  existing.
- **Not found**: `NotFoundException`, which KMS returns as HTTP 400; it is
  declared as the 404 code in `apis/kms/generator-config.yaml`.
- **Referenced by**: [`Alias`](../alias/README.md) (`targetKeyIdRef`) and
  [`Grant`](../grant/README.md) (`keyIdRef`), in the same namespace. Other
  kinds can reference the key ARN with the `KMSKeyARN()` extractor
  (`apis/kms/v1alpha1/key_grant.go`).
- **IAM**: `kms:CreateKey`, `DescribeKey`, `GetKeyPolicy`,
  `GetKeyRotationStatus`, `ListResourceTags`, `ScheduleKeyDeletion`, and
  for changes `PutKeyPolicy`, `UpdateKeyDescription`,
  `EnableKey`/`DisableKey`, `EnableKeyRotation`/`DisableKeyRotation`,
  `TagResource`/`UntagResource`. A missing action fails only the change
  that needs it (`Synced=False` with `AccessDeniedException`).

| Hook | Why |
|------|-----|
| `preObserve` | key ID from the external name |
| `postCreate` | external name from the key ID |
| `postObserve` | condition from the key state |
| `lateInitialize` | policy and enabled state from AWS |
| `isUpToDate`, `update` | KMS updates each property with its own operation |
| `delete` | keys can only be scheduled for deletion |

## Differences from provider-aws

- Drift detection reports which field differs.
- An update calls only the operations of the changed fields. provider-aws
  called all of them on every update, so a role without e.g.
  `kms:UpdateKeyDescription` couldn't change any field.
- Tags are not late-initialized from AWS: only the tags in the spec are
  applied, tags starting with `aws:` are ignored
  ([ADR 0012](../../../../docs/adr/0012-only-spec-tags.md)).
- Changing the value of an existing tag only calls `TagResource`. Before,
  the tag was also removed after it was updated.
- The `Creating`, `Updating` and `PendingReplicaDeletion` key states set a
  condition.

## Limitations

- A key that is scheduled for deletion outside Crossplane is reported as
  not existing, and the provider creates a new key.
- `keySpec`, `keyUsage`, `origin`, `multiRegion`, `customKeyStoreID` and
  `xksKeyID` can't be changed after creation.
- `bypassPolicyLockoutSafetyCheck` is only used on create.
- Some roles allow `EnableKey` and `EnableKeyRotation` but not
  `UpdateKeyDescription`, `DisableKey` or `DisableKeyRotation`; with such a
  role, the description can't be changed and the key can't be disabled.
- Not managed: custom key stores, importing key material, multi-Region
  replicas (`ReplicateKey`).

## Examples

[`examples/kms/key.yaml`](../../../../examples/kms/key.yaml).

## History

| Date | PR | Change |
|------|--------|--------|
| 2026-11-17 | | Update calls only the operations of changed fields (found in live validation) |
| 2026-11-16 | | Imported from provider-aws and ported to runtime v2, SDK v2, namespaced |

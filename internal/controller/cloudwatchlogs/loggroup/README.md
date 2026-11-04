# cloudwatchlogs/LogGroup

Manages an Amazon CloudWatch Logs log group
(`LogGroup.cloudwatchlogs.aws2.crossplane.io`).

## Origin

Generated from scratch on 2026-11-04 (`apis/cloudwatchlogs/generator-config.yaml`,
`apis/cloudwatchlogs/codegen.yaml`).

## Behavior

- **External name**: the log group name, by default `metadata.name`. Log
  group names usually contain `/`, which `metadata.name` can't: set the
  `crossplane.io/external-name` annotation.
- **Observe**: `DescribeLogGroups` with the name as prefix, then the log
  group with exactly that name. `ListLogGroups` isn't used: it doesn't return
  the retention.
- **Create**: `CreateLogGroup` with the class, KMS key, deletion protection
  and the spec tags (no others are added). AWS can't set the retention on
  creation; the first update sets it.
- **Drift**: unset fields aren't managed.
  - `retentionInDays`; `0` (keep forever) equals no retention in AWS.
  - `kmsKeyID`: a key ID equals its ARN.
  - `deletionProtectionEnabled`.
  - tags (keys starting with `aws:` are ignored).
- **Update**: one operation per differing field: `PutRetentionPolicy` (or
  `DeleteRetentionPolicy` for `0`), `AssociateKmsKey` (replaces the key),
  `PutLogGroupDeletionProtection`, `UntagResource` / `TagResource`.
- **Delete**: `DeleteLogGroup`.
- **Status**: `arn` (ends with `:*`, for IAM policies), `logGroupARN`
  (without `:*`), `retentionInDays`, `creationTime`, `storedBytes`,
  `metricFilterCount`, `dataProtectionStatus`.
- **Conditions**: Available once the log group exists.

| Hook | Why |
|------|-----|
| `preObserve`, `filterList` | find the log group by its name |
| `postObserve` | Available condition |
| `isUpToDate` | AWS has no update operation for log groups, so nothing is generated; compares the fields above |
| `preCreate`, `preDelete` | log group name from the external name; tags |
| `update` | one AWS operation per differing field |

## Limitations

- `logGroupClass` can't be changed after creation.
- A KMS key can't be removed: unsetting `kmsKeyID` leaves the key.
- With `deletionProtectionEnabled: true`, AWS rejects the delete; set it to
  `false` first.
- Data protection, subscription filters, metric filters and log streams are
  not managed.

## Examples

[`examples/cloudwatchlogs/loggroup.yaml`](../../../../examples/cloudwatchlogs/loggroup.yaml).

## History

| Date | Commit | Change |
|------|--------|--------|
| 2026-11-04 | — | Generated from scratch |

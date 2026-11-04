# cloudwatchlogs/ResourcePolicy

Manages an account-level Amazon CloudWatch Logs resource policy
(`ResourcePolicy.cloudwatchlogs.aws2.crossplane.io`), which lets AWS
services such as OpenSearch Service or Route 53 write to log groups.

## Origin

Generated from scratch on 2026-11-04 (`apis/cloudwatchlogs/generator-config.yaml`).

## Behavior

- **External name**: the policy name, by default `metadata.name`.
- **Observe**: `DescribeResourcePolicies` (one page has all policies: an
  account has at most 10 per region), then the policy with that name.
- **Create, Update**: `PutResourcePolicy`, which creates the policy or
  replaces its document. Creating a policy whose name exists takes it over.
- **Drift**: `policyDocument` compared as JSON; a list with one element
  equals the element.
- **Delete**: `DeleteResourcePolicy`.
- **Status**: `policyScope`, `lastUpdatedTime`.
- **Conditions**: Available once the policy exists.

| Hook | Why |
|------|-----|
| `preObserve`, `filterList` | all policies in one page, then the one with the name |
| `postObserve` | Available condition |
| `isUpToDate` | policy documents as JSON, not as strings |
| `preCreate`, `preUpdate`, `preDelete` | policy name from the external name |

## Limitations

- Only account-level policies. Policies of a single log group
  (`resourceArn`, scope `RESOURCE`) are not supported.

## Examples

[`examples/cloudwatchlogs/resourcepolicy.yaml`](../../../../examples/cloudwatchlogs/resourcepolicy.yaml).

## History

| Date | Commit | Change |
|------|--------|--------|
| 2026-11-04 | — | Generated from scratch |

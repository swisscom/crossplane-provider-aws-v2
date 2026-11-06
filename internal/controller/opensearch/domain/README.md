# opensearch/Domain

Manages an Amazon OpenSearch Service domain
(`Domain.opensearch.aws2.crossplane.io`), including its access policy.

## Origin

Generated from scratch on 2026-11-04 (`apis/opensearch/generator-config.yaml`,
`apis/opensearch/codegen.yaml`).

## Behavior

- **External name**: the domain name, by default `metadata.name` (3 to 28
  characters, lowercase letters, numbers, hyphens). Set the
  `crossplane.io/external-name` annotation to manage an existing domain.
- **Create**: `CreateDomain` with the spec, including its tags (no others
  are added), and the master user's password from
  `masterUserPasswordSecretRef` (a Secret in the namespace of the Domain).
- **Access policy**: `accessPolicies` may be multi-line JSON (e.g. YAML
  `accessPolicies: |`); it's sent to AWS compacted to one line, on create
  and update, because AWS rejects line breaks in it. Invalid JSON is sent
  as is, for AWS to report the error.
- **Drift**: all fields `UpdateDomainConfig` can change, plus:
  - `accessPolicies` compared as JSON; a list with one element equals the
    element, as AWS returns it.
  - `advancedOptions`, `logPublishingOptions`: only the keys in the spec.
  - `advancedSecurityOptions` without the master user (AWS doesn't return
    it).
  - `autoTuneOptions.desiredState` against the Auto-Tune state
    (`ENABLED`/`ENABLE_IN_PROGRESS` vs. the `DISABLED*` states; `ERROR`
    counts as up to date).
  - `encryptionAtRestOptions.kmsKeyID`: a key ID equals its ARN.
  - `engineVersion`, tags (keys starting with `aws:` are ignored), and the
    master password (see limitations).
  - While AWS processes a change (`processing`, `upgradeProcessing`) or the
    domain is being created or deleted, the domain is up to date: AWS
    rejects further changes until then.
- **Update**, one step per reconcile, because AWS processes one change at a
  time:
  1. `RemoveTags` / `AddTags`.
  2. `UpdateDomainConfig` with only the differing fields; AWS applies every
     field it gets. The master user is sent only with a changed password.
  3. Once no other change is pending, `UpgradeDomain` to `engineVersion`.
- **Delete**: `DeleteDomain`; skipped while AWS is deleting it.
- **Connection details**: `endpoint` (the VPC endpoint for VPC domains),
  `port` (443), `username` (`masterUserName`), `password`.
- **Status**: `arn`, `endpoint` (also for VPC domains), `endpoints`,
  `domainID`, `processing`, `changeProgressDetails`, … read only.
- **Conditions**: Creating until the domain is created and has an endpoint;
  Available afterwards, also while changes are processed (blue/green
  deployments keep the domain serving); Deleting while AWS deletes it.

| Hook | Why |
|------|-----|
| `preObserve`, `preDelete` | domain name from the external name; skip delete while deleting |
| `postObserve` | VPC endpoint, conditions, connection details |
| `isUpToDate` | fields compared differently (above), engine version, tags, password; no changes while processing |
| `preCreate` | domain name, tags, Auto-Tune input shape, master password, compact access policy |
| `postCreate`, `postUpdate` | connection details with the (new) password |
| `preUpdate` | send only the differing fields; master user only with its password; compact access policy |
| `wrapExternal` (`domainExternal.Update`) | tags and `UpgradeDomain`, which `UpdateDomainConfig` can't change |

## Limitations

- **Password changes are only detected with `writeConnectionSecretToRef`.**
  The password isn't readable from AWS, so the one in
  `masterUserPasswordSecretRef` is compared with the one in the connection
  secret. To change it:
  1. Keep `writeConnectionSecretToRef` set.
  2. Change the value in the Secret of `masterUserPasswordSecretRef`.
  3. On the next reconcile, the provider sends the master user with the new
     password, and then writes the new password to the connection secret.

  Without a connection secret, changing the Secret does nothing; change the
  password with the OpenSearch security API instead. If the connection
  secret is deleted, the password is sent again (no change in AWS).
- `masterUserName` alone: a change is only sent together with a password
  change; AWS doesn't return the master user.
- `autoTuneOptions.rollbackOnDisable` and `maintenanceSchedules` are sent
  with every change of `autoTuneOptions`, but drift isn't detected: AWS
  doesn't return them.
- Keys removed from `advancedOptions` or `logPublishingOptions` stay in AWS;
  set them to their default or `enabled: false` instead.
- AWS rejects some changes, e.g. moving between public and VPC access,
  changing the KMS key, or a lower `engineVersion`; the error is in the
  `Synced` condition. Upgrades must follow AWS's supported upgrade paths.
- The JSON name of SAML options is `sAMLOptions` (from the AWS model).

## Examples

[`examples/opensearch/domain.yaml`](../../../../examples/opensearch/domain.yaml).

## History

| Date | PR | Change |
|------|--------|--------|
| 2026-11-06 | [#4](https://github.com/swisscom/crossplane-provider-aws-v2/pull/4) | Multi-line `accessPolicies` sent compacted; AWS rejected line breaks |
| 2026-11-04 | [#4](https://github.com/swisscom/crossplane-provider-aws-v2/pull/4) | Generated from scratch |

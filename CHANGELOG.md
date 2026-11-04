# Changelog

User-visible changes of provider-aws-v2. Format:
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), provider versions
follow [Semantic Versioning](https://semver.org/). Kinds have their own API
version; `v1alpha1` kinds may change incompatibly in a minor release, with
migration notes here
([versioning](docs/development.md#releasing)). Per-kind details are in
`internal/controller/<service>/<kind>/README.md`.

## [Unreleased]

### Added

- opensearch: `Domain`, including its access policy (`accessPolicies`),
  tags, engine upgrades and the master user password from a Secret. Example:
  `examples/opensearch/domain.yaml`.
- cloudwatchlogs: `LogGroup` (retention, KMS key, deletion protection, tags)
  and the account-level `ResourcePolicy`. Examples:
  `examples/cloudwatchlogs/`.

### Changed

- ec2: `SecurityGroup`, `SecurityGroupIngressRule` and
  `SecurityGroupEgressRule` apply only the tags in the spec. They no
  longer add the `crossplane-kind`, `crossplane-name`,
  `crossplane-namespace` and `crossplane-providerconfig` tags, and remove
  them from existing resources on the next reconcile. Add them to
  `spec.forProvider.tags` to keep them
  ([ADR 0011](docs/adr/0011-only-spec-tags.md)).

### Fixed

- docdb: tags starting with `aws:`, which AWS sets and which can't be
  changed, are ignored. Before, the provider tried to remove them and the
  resource never became up to date.
- rds, docdb: creating a resource no longer replaces `spec.forProvider`
  with the values AWS returns. Set fields keep the user's value and are no
  longer cleared when the create response leaves them empty.
  AWS values are still in `status.atProvider` and, where the kind
  late-initializes, filled into unset fields on the next observe.

## [0.1.0] - 2026-10-27

First release (proof of concept). Details per kind are in its README.

### Added

- Native Crossplane v2 provider for AWS: AWS SDK for Go v2,
  namespaced managed resources only, API groups `*.aws2.crossplane.io`.
- Managed resources:
  - rds: `DBInstance`
  - docdb: `DBCluster`, `DBInstance`, `DBSubnetGroup`,
    `DBClusterParameterGroup`
  - ec2: `SecurityGroup`, `SecurityGroupIngressRule`,
    `SecurityGroupEgressRule`
- Drift detection and update of all updatable fields.
- `ProviderConfig` (namespaced) and `ClusterProviderConfig` with `Secret`,
  `IRSA`, `PodIdentity` and `WebIdentity` credentials and role assumption.
- Code generator and importer to add kinds from AWS API models or from
  crossplane-contrib/provider-aws.

[0.1.0]: https://github.com/swisscom/crossplane-provider-aws-v2/releases/tag/v0.1.0

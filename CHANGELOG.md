# Changelog

User-visible changes of provider-aws-v2. Format:
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), provider versions
follow [Semantic Versioning](https://semver.org/). Kinds have their own API
version; `v1alpha1` kinds may change incompatibly in a minor release, with
migration notes here
([versioning](docs/development.md#releasing)). Per-kind details are in
`internal/controller/<service>/<kind>/README.md`.

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

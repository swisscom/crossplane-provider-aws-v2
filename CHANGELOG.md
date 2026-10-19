# Changelog

User-visible changes of provider-aws-v2. Format:
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). There are no
releases yet; everything is under "Unreleased". Per-kind details are in
`internal/controller/<service>/<kind>/README.md`.

## [Unreleased]

### Added

- ec2: `SecurityGroup`, `SecurityGroupIngressRule` and
  `SecurityGroupEgressRule`, generated from scratch (one resource per rule,
  AWS IDs as external names). New groups have no default egress;
  `adoptExistingByName` (opt-in) adopts an existing group by name
  and reports it with an event.
- codegen: `generate --list-kinds` prints the kinds of a service and their
  AWS model names.

- docdb: `DBSubnetGroup` and `DBClusterParameterGroup`, imported from
  provider-aws; all their updatable fields are compared. `DBCluster` can
  reference them (`dbSubnetGroupNameRef`, `dbClusterParameterGroupNameRef`
  and selectors).
- docdb: `DBCluster` and `DBInstance`, imported from provider-aws, including
  restore from snapshot / point in time and `serverlessV2ScalingConfiguration`.
- rds: `DBInstance`, imported from provider-aws, including read replicas,
  restores (S3, snapshot, point in time) and fields new in AWS SDK v2.
- Drift detection of all updatable fields for generated kinds
  (`DiffUpdatableParameters`).
- `float64` fields are allowed in CRDs.
- `ProviderConfig` (namespaced) and `ClusterProviderConfig` with `Secret`,
  `IRSA`, `PodIdentity` and `WebIdentity` credentials, `assumeRole` and
  `endpoint.url`.
- Code generator (`codegen/cmd/generate`) and importer (`codegen/cmd/import`).

### Fixed

- codegen: `ignore.resource_names` written by the importer uses AWS model
  names (e.g. `Vpc`), so kinds renamed by the generator (`VPC`) are
  ignored.
- AWS errors keep their code through wrapping (`errors.As` finds the
  `smithy.APIError`), so hooks can check it.
- docdb/DBCluster: changes of any updatable field are applied (previously a
  hand-picked subset); restore no longer calls `CreateDBCluster`; a changed
  master password is sent to AWS.
- docdb/DBInstance: changes of any updatable field are applied.
- rds/DBInstance: `monitoringInterval: 0` no longer causes a
  `ModifyDBInstance` every poll when enhanced monitoring is off.

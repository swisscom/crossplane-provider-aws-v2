# Changelog

User-visible changes of provider-aws-v2. Format:
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). There are no
releases yet; everything is under "Unreleased". Per-kind details are in
`internal/controller/<service>/<kind>/README.md`.

## [Unreleased]

### Added

- provider config: `ProviderConfig` (namespaced) and `ClusterProviderConfig`
  with `Secret`, `IRSA`, `PodIdentity` and `WebIdentity` credentials,
  `assumeRole` and `endpoint.url`.
- codegen: code generator (`codegen/cmd/generate`) and importer
  (`codegen/cmd/import`).
- codegen: drift detection of all updatable fields for generated kinds
  (`DiffUpdatableParameters`); `float64` fields are allowed in CRDs.
- ec2: `SecurityGroup`, `SecurityGroupIngressRule` and
  `SecurityGroupEgressRule`, generated from scratch (one resource per rule,
  AWS IDs as external names). New groups have no default egress;
  `adoptExistingByName` (opt-in) adopts an existing group by name
  and reports it with an event; `adoptExisting` (opt-in) adopts an existing
  rule with the same content. Rules can change in place, except their
  group and source type (AWS).
- codegen: `generate --list-kinds` prints the kinds of a service and their
  AWS model names.
- codegen: `codegen.yaml` has a per-kind `kinds:` section (`identifier`,
  `identifierInput`, `customObserve`, `customDelete`, `orphanOnDelete`,
  `customIsUpToDate`, `immutableFields`, `allowSensitiveFields`) and is
  parsed strictly.
- importer: `make check-import-todo` (`hack/check-import-todo.sh`) fails
  while a `*_importtodo.go` file or a `TODO(import)` marker is left outside
  `codegen/`, and when a Go file can't be read.
- importer: `generatorConfig` in `apis/<svc>/import.yaml` (repo-local
  generator settings merged over the source `generator-config.yaml`;
  `generatorConfigSHA256` records the file last written), `lastImport` (the
  source revision of the latest run) and `backfilled` on revisions recorded
  by hand. rds and docdb `import.yaml` now hold their former hand edits of
  `generator-config.yaml`, which is derived (no change in generated code).
  rds `DBInstance` records `swisscom/provider-aws@ad8ac93` (backfilled).
- compare: shared helpers for kinds: case-insensitive matching of AWS
  identifiers and lowercased values (e.g. maintenance windows), and an
  overlay of AWS pending modifications before drift detection. ADR 0010.
- rds: `DBInstance`, imported from provider-aws, including read replicas,
  restores (S3, snapshot, point in time) and fields new in AWS SDK v2.
- rds/DBInstance: `tdeCredentialPasswordSecretRef` (a Secret in the
  DBInstance's namespace) for the Oracle TDE credential password. It is sent
  on create and with every modify; restores don't send it. Example
  `examples/rds/dbinstance-managed-password.yaml` for
  `manageMasterUserPassword`.
- docdb: `DBCluster` and `DBInstance`, imported from provider-aws, including
  restore from snapshot / point in time and `serverlessV2ScalingConfiguration`.
- docdb: `DBSubnetGroup` and `DBClusterParameterGroup`, imported from
  provider-aws; all their updatable fields are compared. `DBCluster` can
  reference them (`dbSubnetGroupNameRef`, `dbClusterParameterGroupNameRef`
  and selectors).
- CI: make targets `vulncheck`, `vulncheck.binary`, `services.all`,
  `workflows.lint` and `toolchain.check` (part of `modules.check`).

### Changed

- provider config: **BREAKING:** `endpoint.url` must be an `http`/`https`
  URL with a host and without user info, query or fragment; the CRD and the
  reconciler both check it. Migration: fix such URLs before upgrading the
  CRDs.
- codegen: **BREAKING:** `spec.forProvider.region` is immutable on every
  kind (CEL `self == oldSelf`); changing it used to create a new, empty
  resource in the new region and orphan the old one. Migration: set
  `deletionPolicy: Orphan`, delete the MR and create a new one in the target
  region.
- codegen: **BREAKING:** Create no longer copies the AWS response into
  `spec.forProvider`; it goes to `status.atProvider` only. Unset spec fields
  are filled by late initialization on the next observe; values you set are
  never rewritten. Migration: none; GitOps tools no longer see
  provider-made spec changes. ADR 0011.
- codegen: generated controllers read, update and delete the AWS resource
  by its external name, never by empty or stale status fields. When several
  AWS resources match it, observe fails instead of taking the first.
- codegen: **BREAKING:** `status.atProvider.pendingModifiedValues.masterUserPassword`
  is removed from rds and docdb `DBInstance`; passwords never belong in
  status. Migration: none.
- importer: **BREAKING:** `codegen/cmd/import` exits `3` when the import
  needs a human (anything quarantined, Setup code it could not convert, a
  hand-written `ResolveReferences` that hides marker references) and `2` on
  invalid flags; the report starts with the number of blocking findings.
  Migration: treat a non-zero exit as "port the report's blocking findings"
  (`go run` shows every non-zero code as `exit status N`, rc 1). ADR 0012.
- importer: **BREAKING:** quarantined functions and methods stay as stubs
  that panic with `TODO(import)` until ported, instead of leaving the build.
  Migration: when porting, replace the stub's body with the code from
  `<file>_importtodo.go`.
- importer: **BREAKING:** `apis/<svc>/import.yaml` is parsed strictly
  (unknown keys, invalid kind names, API versions or field paths are
  errors). Migration: fix the reported key.
- importer: **BREAKING:** a hand-edited `generator-config.yaml` is never
  overwritten. Migration: move the edit to `generatorConfig` in
  `import.yaml`, delete `generator-config.yaml` and import again.
- importer: dead `ignore.field_paths` entries of the source config are
  dropped with a notice (an error in `import.yaml`); extra arguments after
  the flags are an error; re-imports don't port new upstream files into a
  controller ported before (the report lists them); `custom_types.go` keeps
  its declaration order and the report has a stable order.
- compare: a map field is up to date only when every key in
  `spec.forProvider` exists on the AWS resource, also when its value is
  unset. No current kind has map fields.
- rds/DBInstance: **BREAKING:** the plaintext `tdeCredentialPassword` is
  removed from the spec. Migration: put the password in a Secret in the same
  namespace, set `tdeCredentialPasswordSecretRef: {name, key}` and remove
  `tdeCredentialPassword` from the manifest before upgrading the CRD (the
  API server prunes the old field, and the password would be lost).
- rds/DBInstance: **BREAKING:** an instance with engine `docdb` or `neptune`
  is never managed: Observe fails with "refusing to manage it", and deleting
  such an MR releases it without an AWS call. Migration: delete the MR (the
  instance is not touched) or point it at an RDS instance.
- rds/DBInstance: the connection secret's `password` is written only after
  AWS has it; after a restore the key is absent until the password was
  applied, and with `manageMasterUserPassword: true` it is empty (the
  password is in Secrets Manager). Clients must tolerate a missing key
  while the instance is provisioned. ADR 0013.
- rds/DBInstance: `manageMasterUserPassword: true` with
  `masterUserPasswordSecretRef` or `autogeneratePassword` is rejected at
  create; turning it off needs one of them. A
  `masterUserPasswordSecretRef` to a missing Secret or an empty value fails
  create with a clear error.
- rds/DBInstance: the password cache Secret `dbinstance.<uid>` is owned by
  the DBInstance and garbage collected with it, also with
  `deletionPolicy: Orphan`. Existing ones get the owner on their next write.
- docdb: **BREAKING:** a DBCluster, DBInstance or DBClusterParameterGroup
  whose external name matches a non-DocumentDB resource (Aurora, RDS,
  Neptune) is no longer adopted, modified or deleted: Observe fails with
  `... has engine "...", not "docdb"; refusing to manage it` (parameter
  groups: family must start with `docdb`), and deleting the MR removes the
  finalizer without an AWS call. DBCluster sends the `engine=docdb` filter,
  so a name used by another cluster ends in `DBClusterAlreadyExistsFault`.
  Migration: delete such MRs or change their external name.
- docdb/DBCluster: **BREAKING:** a changed master password is always sent
  with `ApplyImmediately=true`. Migration: none; to rotate in a maintenance
  window, change the referenced Secret at that time.
- docdb/DBCluster: **BREAKING:** password rotation is detected without
  `writeConnectionSecretToRef`, and a password generated for a restored
  cluster is applied once it is available. The SHA-256 digest of the applied
  password (never the password) is kept in the Secret
  `docdb-dbcluster.<uid>.password-state`, owned by the DBCluster. An MR with
  a password reference but neither that Secret nor a connection secret with
  the same password gets its referenced password applied once after the
  upgrade. ADR 0014.
- docdb/DBCluster: **BREAKING:** `username` and `port` of the connection
  secret come from the observed cluster (restored clusters publish them
  too); `port` is absent while AWS reports none and shows the port AWS
  listens on while a change is pending. Migration: consumers must tolerate a
  missing `port` and not expect the spec's port before AWS applied it.
- docdb: **BREAKING:** the provider removes only tags it set: keys recorded
  in the annotation `docdb.aws2.crossplane.io/managed-tag-keys` (needs the
  `LateInitialize` or `*` management policy) and dropped from the spec. A
  changed value is updated in place. Migration: tags set before the upgrade
  and later dropped stay in AWS; remove them by hand or add their keys to
  the annotation (comma-separated). ADR 0015.
- rds/DBInstance, ec2 (`SecurityGroup`, `SecurityGroupIngressRule`,
  `SecurityGroupEgressRule`): **BREAKING:** like docdb, only tags the
  provider recorded (`rds.aws2.crossplane.io/managed-tag-keys`,
  `ec2.aws2.crossplane.io/managed-tag-keys`) are removed. Migration: tags
  set outside the provider are no longer removed; tags added before upgrade
  are not recorded: the first reconcile records only the current spec keys
  (ec2: plus `crossplane-*`), so keys dropped from the spec before the
  upgrade stay in AWS until removed by hand or added to the annotation.
  ADR 0015.
- docdb: the DBSubnetGroup README lists its name space shared with RDS and
  Neptune (no engine check possible); unused tag helpers are removed.
- ec2: **BREAKING:** `SecurityGroup`, `SecurityGroupIngressRule` and
  `SecurityGroupEgressRule` reject a change of `spec.forProvider.region`.
  Migration: set `deletionPolicy: Orphan`, delete the MR, then create a new
  MR without an external name in the new region (`sg-`/`sgr-` IDs are
  region-scoped).
- ec2: Observe keeps only the group or rule whose ID equals the external
  name, and fails if AWS returns more than one. Create writes
  `status.atProvider` only.
- CI: the code generator runs for every service (`make services.all`)
  before `check-diff`, so stale or hand-edited `zz_*` files fail CI; CI also
  runs `make check-import-todo`.
- CI: publishing goes to `ghcr.io/swisscom`, only on push, after lint,
  tests, check-diff and govulncheck passed, and only when the repository
  variable `XPKG_PUBLISH` is `true`.
- CI: the `/points` and `/backport` comment commands (`commands.yml`) and
  the unused `promote.yml` are removed; backports still work with a
  `backport <branch>` label set before merge. ADR 0016.
- CI: the code generator tests fail instead of skipping when the AWS API
  model can't be fetched (`CI` or `CODEGEN_REQUIRE_MODELS` set); offline
  local runs still skip.

### Fixed

- rds/DBInstance: AWS errors of read replica create, tag and cluster
  describe calls no longer carry the request ID, so the condition message
  doesn't change on every retry.
- provider config: AWS errors keep their code through wrapping (`errors.As`
  finds the `smithy.APIError`), so hooks can check it.
- codegen: `make services` fails with a message naming the config key to
  set instead of generating unsafe defaults: no field holds the external
  name; a secret-like field would become a plain API field; an
  `ignore.field_paths` entry matches nothing (this found a typo and dead
  entries in the rds and docdb configs); no "not found" error code; no read
  or delete operation without an opt-in; several lists in a ReadMany output;
  an updatable GetAttributes kind without a drift check.
- codegen: a nil AWS create or describe response no longer panics the
  controller; an empty describe response is an error.
- codegen: a failed run leaves no half-written or unformatted files, and
  files of removed kinds are deleted. Enum values are quoted safely,
  generated controllers pass staticcheck, and hand-written status fields
  keep their value until the observe hooks run.
- codegen: `ignore.resource_names` written by the importer uses AWS model
  names (e.g. `Vpc`), so kinds renamed by the generator (`VPC`) are
  ignored.
- importer: hand-written code is never quarantined on re-import, and errors
  outside the files the import wrote fail it. Errors it cannot quarantine
  (package name clash, non-converging type check) fail the import instead of
  exiting 0 with a broken tree.
- importer: a quarantined method takes its embedding type along, so an
  embedded method can't silently run in its place. `if … else` blocks and
  controller-builder calls (`Watches`, `Owns`, predicates) in a source
  `Setup` are kept as `TODO(import)` notes instead of being dropped.
- importer: `--service`, `--kinds` and `--api-version` are validated; helper
  import paths can't escape the source and target trees; a merged custom
  struct never gets a duplicate json name; the `dirty` check covers every
  directory read, and a `--source` that is not a git checkout root is
  rejected; `…WithContext` is renamed only on AWS SDK operations, and
  imports whose package name differs from the path are kept.
- importer: a hand-written `ResolveReferences` that misses a marker
  reference is a blocking finding. The import-kind skill points to the real
  commits (`c501153`, `b74da45`).
- compare: drift detection no longer depends on list element order, an
  empty object in `spec.forProvider` (e.g.
  `serverlessV2ScalingConfiguration: {}`) is not a permanent difference
  when AWS returns nothing, and mismatched value types are reported as a
  difference instead of crashing the reconciler.
- rds/DBInstance: restore with `autogeneratePassword` or
  `masterUserPasswordSecretRef` applies the password after the restore and
  keeps it in the connection secret; the cache is never overwritten with an
  empty value. MRs hit by the old behaviour with only `autogeneratePassword`
  have an empty cached password: set `masterUserPasswordSecretRef`.
- rds/DBInstance: a rotated password is published only after
  `ModifyDBInstance` succeeded, and is recorded as "not applied yet" first,
  so a lost response re-sends it instead of reporting a stale password as up
  to date.
- rds/DBInstance: restores no longer report a failed `CreateDBInstance`;
  `manageMasterUserPassword: true` can create an instance and be toggled
  without publishing a stale password.
- rds/DBInstance: deleting no longer sends a full `ModifyDBInstance` from
  stale state; only `deletionProtection: false` is sent, when the spec asks
  for it, and its error is reported.
- rds/DBInstance: no Modify loop now that Create keeps the user's values:
  `engine`, `dbInstanceClass`, `storageType`, `licenseModel` and
  `caCertificateIdentifier` compare case-insensitively, and a KMS key ID
  matches the key ARN AWS returns. An alias is not resolved, so it matches
  any observed key ARN (an alias ARN any key in the same partition, region
  and account) and a changed alias is not detected.
- rds/DBInstance: `monitoringInterval: 0` no longer causes a
  `ModifyDBInstance` every poll when enhanced monitoring is off.
- rds/DBInstance: a mixed-case external name finds the instance AWS stores
  in lowercase; a changed tag value is no longer removed until the next
  reconcile; a tag key listed twice is sent once, with its last value.
- rds/DBInstance: no panic when AWS omits the maintenance or backup window,
  parameter group, option group or cluster; `vpcSecurityGroupIDs` is no
  longer reordered; `enableCloudwatchLogsExports` is compared as a set; the
  drift diff is no longer printed to stderr.
- rds/DBInstance: a failed read of the password cache Secret is an error,
  instead of making a read replica skip its password check.
- docdb/DBCluster: changes of any updatable field are applied (previously a
  hand-picked subset); restore no longer calls `CreateDBCluster`.
- docdb/DBCluster: no `ModifyDBCluster` while the cluster is `creating`,
  `backing-up`, `resetting-master-credentials`, `rebooting`, `renaming`,
  `starting`, `stopping`, `migrating`, `maintenance`, `modifying` or
  `upgrading`, and no second `DeleteDBCluster` while it is `deleting`.
- docdb/DBCluster: the engine-upgrade `ModifyDBCluster` error is wrapped;
  the connection secret only gets a password AWS accepted; while
  `manageMasterUserPassword` is true a referenced password is not compared,
  sent or published.
- docdb/DBInstance: changes of any updatable field are applied, and no
  `ModifyDBInstance` every poll while AWS holds a `dbInstanceClass` or
  `caCertificateIdentifier` change as pending.
- docdb: identifiers, `preferredMaintenanceWindow` and the DBCluster
  `dbClusterParameterGroupName` compare case-insensitively, so Create
  keeping the user's casing no longer causes a Modify loop.
- docdb/DBClusterParameterGroup: only changed parameters are sent, at most
  20 per call; a missing `applyMethod` defaults to `pending-reboot`;
  parameters without a value are not late-initialized; tag-only changes
  don't call `ModifyDBClusterParameterGroup`.
- ec2: a create response without a group or rule ID is an error instead of
  a panic.

### Security

- provider config: STS calls never use `endpoint.url`. Credential-file
  parse errors name only the Secret key, never its lines.
- CI: `make` and CI build with the Go of the `go` line (1.27.3), which
  must match in both modules (`make toolchain.check`).
  `golang.org/x/net` v0.60.0 fixes five reachable HTTP/2 vulnerabilities
  (GO-2026-6603, -6610, -6611, -6612, -6617).
- CI: govulncheck runs on both modules on every PR and push and daily, and
  scans the built binary; trufflehog scans for leaked secrets on PRs and
  pushes plus a daily full-history scan; CodeQL also runs on PRs and weekly
  and covers `codegen/`.
- CI: every workflow and job has least-privilege `permissions:`, every
  action is pinned to a commit SHA (kept current by Renovate), actions on
  removed Node runtimes are replaced, and workflows are linted (actionlint,
  zizmor). ADR 0016.

### Documentation

- README "Trust model": a namespaced `ProviderConfig` with `IRSA`,
  `PodIdentity` or `WebIdentity` acts as the provider pod's identity; only
  Secrets are confined to its namespace. ADR 0002 gets a clarification.

# 0010. Releases are tags on release branches; no published package yet

- Status: Accepted
- Date: 2026-10-27

## Context

Until 0.1.0 all changes were committed directly to `main` and there were no
releases. Users need fixed versions they can pin, and fixes for a version
they run without taking new, possibly breaking, features.

The CI comes from the Crossplane provider template: it builds and publishes
packages to a registry when credentials are configured, and promotes them to
channels. The provider isn't published under crossplane-contrib, and there
are no credentials for a public registry yet. Swisscom builds the package
from the source in its own CI and pushes it to an internal registry.

Options considered: trunk only (tags on `main`, no way to patch an old
version without its newer changes); development on release branches
(merges between branches on every change); the Crossplane model of a
trunk plus release branches for patches.

## Decision

- Follow the Crossplane model: development happens on `main` through pull
  requests; each minor version gets a `release-X.Y` branch cut from `main`
  that only receives backported fixes.
- Releases are SemVer git tags `vX.Y.Z` on a release branch, with a GitHub
  release whose notes are the `CHANGELOG.md` section.
- The provider version and the API version of a kind are separate. Kinds
  follow the Kubernetes API versioning rules: `v1alpha1` may change
  incompatibly in a minor release (with migration notes); from `v1beta1` on,
  incompatible changes need a new API version. Patch releases are always
  compatible.
- GitHub CI validates (lint, generated-code diff, tests, build) but publishes
  nothing; whoever runs the provider builds the package from a tag. Where
  and how to publish a public package is a later decision.
- Release channels and the `promote` workflow are dropped.

## Consequences

- The release steps are in
  [development.md](../development.md#releasing).
- Fixes go to `main` first and are backported with the
  `backport release-X.Y` label or the `/backport` command.
- Builds must see tags and full history, since the version comes from
  `git describe --tags`; a shallow clone builds `v0.0.0-…`.
- Publishing later only needs registry credentials in CI and a new ADR;
  `make publish` already targets `XPKG_REG_ORGS`.

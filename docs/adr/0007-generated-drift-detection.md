# 0007. Generated drift detection of updatable fields

- Status: Accepted
- Date: 2026-10-09

## Context

A managed resource is only kept in sync if `isUpToDate` reports every field
that differs. Imported `isUpToDate` hooks compared hand-picked fields, so
changes to other updatable fields were silently ignored, and every new AWS
field needed hand-written code. The generated default was "always up to
date".

## Decision

Generate `DiffUpdatableParameters(desired, observed)` per kind: it compares
every spec field that the update operation accepts and the read operation
returns, using `internal/utils/compare.IsSubset` (unset = unmanaged,
order-insensitive slices, partial structs). It is the default
`isUpToDate`. Hand-written hooks build on it and only add what it can't
see (renamed/reshaped fields, passwords, tags, special semantics), and
remove fields with `compare.Without` when the API rejects them. Fields can
be excluded in `generator-config.yaml` (`compare.ignore`, `is_ignored`).

## Consequences

- New updatable AWS fields are detected without code changes.
- Fields that can only be set at creation are not compared; changing them is
  silently ignored until CEL immutability rules are added.
- rds/DBInstance still uses its own hand-written comparison (to be migrated).

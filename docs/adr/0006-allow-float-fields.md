# 0006. Allow float64 fields in CRDs

- Status: Accepted
- Date: 2026-10-09

## Context

controller-gen rejects `float64` fields unless `allowDangerousTypes` is set,
because floats can lose precision in non-Go clients. Some AWS APIs use
`double` (e.g. DocumentDB/RDS serverless capacity units). Alternatives:
ignore those fields (feature unusable) or map them to strings (awkward,
custom conversion code per field).

## Decision

Generate CRDs with `crd:allowDangerousTypes=true` (`apis/generate.go`) and
keep the AWS types. Upjet-based AWS providers do the same.

## Consequences

- Float fields work without per-field code.
- Values are JSON numbers; very large values could be rounded by other
  clients. AWS uses floats only for small values.

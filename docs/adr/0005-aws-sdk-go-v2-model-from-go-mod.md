# 0005. AWS SDK for Go v2 only; model version follows go.mod

- Status: Accepted
- Date: 2026-10-05

## Context

AWS SDK for Go v1 reached end of support. Generated code must match the
SDK it is compiled against: a model newer or older than the SDK module
produces code that references missing members.

## Decision

Use only aws-sdk-go-v2 at run time. The generator fetches the API model from
the `service/<svc>/<version>` tag of aws-sdk-go-v2 matching the module
version in the root `go.mod`.

## Consequences

- Updating a service = bump its module (Renovate), `make services`,
  `make generate`, review the diff of types and CRDs.
- Ported SDK v1 code needs conversion (types, errors, options); see
  [porting-notes.md](../porting-notes.md).

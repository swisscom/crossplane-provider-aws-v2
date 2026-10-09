# 0008. Per-kind README and a root changelog for history

- Status: Accepted
- Date: 2026-10-09

## Context

Each kind is a mix of generated, imported and hand-written code. Developers
and coding agents need to know, before changing a kind, where it came from,
how it deviates from provider-aws, what it can't do, and what changed. Git
history has the details but no summary; a changelog per controller would
duplicate git and drift. The provider has no releases yet.

## Decision

- Each kind has `internal/controller/<svc>/<kind>/README.md`: origin
  (imported revision or generated), behavior and hooks, deviations from
  provider-aws, non-updatable fields, known limitations, and a short history
  table of behavior changes (date, commit, change).
- The machine-readable origin is `revisions` in `apis/<svc>/import.yaml`,
  written by the importer.
- One root `CHANGELOG.md` ([Keep a Changelog](https://keepachangelog.com))
  with user-visible changes, under "Unreleased" until releases start.
- Commit subjects are scoped (`docdb/dbcluster: …`, `codegen: …`), so
  `git log -- <dir>` gives the full history.

## Consequences

- Every change to a kind's behavior updates its README history and the
  changelog in the same commit.
- Kind READMEs replace the per-kind section of `porting-notes.md`.

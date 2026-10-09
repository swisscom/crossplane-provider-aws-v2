# 0004. Import kinds and their hand-written code from provider-aws

- Status: Accepted
- Date: 2026-10-05

## Context

provider-aws has years of production-tested, hand-written behavior per kind
(password handling, restores, engine-specific drift rules). Rewriting it
from scratch would lose edge cases; porting by hand is slow and inconsistent.

## Decision

Build an importer (`codegen/cmd/import`) that, per kind, reuses the source
generator config, copies custom types and controller code, rewrites them to
runtime v2 / SDK v2 / namespaced types, and quarantines what doesn't compile
behind a build tag. A human finishes the port with the `import-kind` skill.
Only kinds that are needed are imported. `apis/<svc>/import.yaml` records
the imported kinds and the source revision of each.

## Consequences

- Porting is repeatable; mechanical fixes are taught to the importer once.
- Imported code inherits provider-aws's weaknesses (e.g. hand-picked drift
  checks); they are reviewed during the port and listed in the kind README.
- The generated code uses the current AWS model, which may have more fields
  than provider-aws knew.

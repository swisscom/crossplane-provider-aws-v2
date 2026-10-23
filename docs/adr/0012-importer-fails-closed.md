# 0012. The importer fails closed

- Status: Accepted
- Date: 2026-10-16

## Context

[ADR 0004](0004-import-kinds-from-provider-aws.md) keeps the repository
building by moving code that does not compile behind the `importtodo` build
tag. The build stayed green while behavior changed:

- an overriding method was moved, so the embedded generated method ran
  instead (e.g. a restore `Create` became a plain create);
- a broken hook took `configure<Kind>` and `init` with it, so the kind ran
  on generated defaults only;
- `convertSetup` dropped `else` branches and controller-builder calls
  (`Watches`, `Owns`, predicates) with only a generic report line;
- errors it could not quarantine were swallowed and the importer exited 0;
- a re-import could quarantine hand-written, reviewed code;
- a re-import overwrote hand edits of `generator-config.yaml`.

The report went to stdout only, and no gate checked for leftovers.

Options considered: an `--allow-todo` flag; louder reports only; a separate
`generator-config.local.yaml` read by the generator; merging only
importer-managed keys into `generator-config.yaml`.

## Decision

- Every lossy action is a **blocking finding** plus a `TODO(import)` marker
  in the tree. The importer exits 3 while blocking findings exist, 1 on
  errors and 2 on invalid input (nothing written).
- `make check-import-todo` fails while any `*_importtodo*.go` file or
  `TODO(import)` marker exists outside `codegen/`.
- Quarantined functions and methods stay as **stubs that panic**. Only
  declarations whose signature does not compile are moved; a moved method
  takes its receiver type along if that type embeds another.
- Only files written by the current run are quarantined. Anything else that
  does not compile fails the import.
- `generator-config.yaml` is derived from the source config plus
  `generatorConfig` in `import.yaml`, guarded by a sha256 digest. Upstream
  ignore paths that match nothing in the AWS model are dropped.
- `import.yaml` is parsed strictly; flags and helper paths are validated.
- No flag turns any of this off. A human acknowledges a finding by porting
  the code and removing the marker, which shows up in review.

This amends ADR 0004: the repository still always compiles.

## Consequences

- More explicit human work per import, but unported code can no longer ship
  or run silently.
- A stubbed `init` panics the package's tests and the provider until it is
  ported. That is intended.
- Repo-local generator settings live in one reviewed place, `import.yaml`.
  An existing `generator-config.yaml` without a digest must be migrated
  once: move hand edits to `generatorConfig`, delete the file, import again.
- Scripts must treat a non-zero exit as "port the report's blocking
  findings"; `go run` shows every non-zero code as exit status 1.

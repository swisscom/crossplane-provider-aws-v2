# Build and packaging

## What it is

The repository is based on
[crossplane/provider-template](https://github.com/crossplane/provider-template)
and uses Crossplane's shared make machinery
([crossplane/build](https://github.com/crossplane/build), git submodule in
`build/`).

## How it is used

| Command | Does |
|---------|------|
| `make submodules` | Fetch `build/` (once) |
| `make services SERVICES="…"` | Run the code generator ([code-generator.md](code-generator.md)) |
| `make generate` | controller-gen, angryjet, CRDs |
| `make reviewable` | generate + lint + test |
| `make build` | Binary, image, Crossplane package (xpkg) |
| `make dev` / `make dev-clean` | kind cluster with CRDs, provider running locally |
| `make lint` | golangci-lint of the root and `codegen/` modules (`.golangci.yml`) |
| `make test` | Unit tests of the root and `codegen/` modules |
| `make codegen.test` / `make codegen.lint` | Only the `codegen/` module |
| `make services.all` | Regenerate every service with an `apis/<svc>/generator-config.yaml` |
| `make vulncheck` | govulncheck of the root and `codegen/` modules |
| `make vulncheck.binary` | govulncheck of the built provider binary (after `make build`) |
| `make workflows.lint` | actionlint and zizmor of `.github/workflows` |
| `make toolchain.check` | Both `go.mod` files have the same `go` and `toolchain` lines (part of `modules.check`) |

`codegen/` is a separate Go module, so `./...` in the root skips it; the
`codegen.*` targets are hooked into `test.run`/`lint.run`, and CI lints it in
its own step. In `.golangci.yml`, gocyclo, gosec and noctx are off for
`codegen/`: a developer CLI that works on the paths and runs the `go`
commands it is given, with switch-heavy AST walkers.

`make` pins `GOTOOLCHAIN` to the `go` line of `go.mod`, and CI's `setup-go`
reads the same line, so a Go bump is a change of the `go` line in both
`go.mod` files and nothing else. There is no `toolchain` line.

## CI

Each CI check is a make target, so a local run is the CI run. See
[ADR 0016](../adr/0016-ci-supply-chain-baseline.md) for the rules.

| Workflow / job | Runs | When |
|----------------|------|------|
| `ci.yml` lint | `make modules.check`, golangci-lint (root, `codegen/`) | PR, push |
| `ci.yml` workflow-lint | `make workflows.lint` | PR, push |
| `ci.yml` check-diff | `make check-import-todo`, `make services.all`, `make check-diff` | PR, push |
| `ci.yml` unit-tests | `make test`, coverage to Codecov | PR, push |
| `ci.yml` govulncheck | `make vulncheck` | PR, push |
| `ci.yml` build-artifacts | `make build.all`, `make vulncheck.binary` | PR, push |
| `ci.yml` publish-artifacts | `make publish` to `ghcr.io/swisscom` | push, after all checks, only if the repo variable `XPKG_PUBLISH` is `true` |
| `security.yml` | trufflehog secret scan; govulncheck | PR, push, daily |
| `codeql.yml` | CodeQL of both Go modules | PR, push, weekly |
| `tag.yml` | `git tag` of a semver version | manual |
| `backport.yml` | Backport PR for a `backport <branch>` label | PR merged |

Every workflow starts with `permissions: {}` and each job asks for what it
needs. Actions are pinned to a commit SHA with a `# vX.Y.Z` comment;
Renovate updates them after 14 days (security fixes at once), and zizmor
fails CI on an unpinned action.

## Layout

| Path | Content |
|------|---------|
| `cmd/provider` | `main.go`: flags, manager, feature flags, `SetupGated` of all controllers |
| `package/crossplane.yaml` | Package metadata; `capabilities: [safe-start]` |
| `package/crds` | Generated CRDs |
| `cluster/images`, `cluster/local` | Image build, local integration test |
| `hack/` | License header for generated code |

## Provider flags

`--poll` (1m) is the drift-check interval per resource, `--sync` (1h) the
full resync, `--max-reconcile-rate` (10/s) the global rate. Management
policies are on by default; change logs are opt-in
(`--enable-changelogs`).

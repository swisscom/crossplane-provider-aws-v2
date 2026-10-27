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

`codegen/` is a separate Go module, so `./...` in the root skips it; the
`codegen.*` targets are hooked into `test.run`/`lint.run`, and CI lints it in
its own step. In `.golangci.yml`, gocyclo, gosec and noctx are off for
`codegen/`: a developer CLI that works on the paths and runs the `go`
commands it is given, with switch-heavy AST walkers.

`make` pins `GOTOOLCHAIN` to the Go version of `go.mod`.

## CI and versions

GitHub Actions (`.github/workflows/ci.yml`) runs lint, `make check-diff`
(generated code up to date), unit tests and `make build.all` on pull
requests, `main` and `release-*`, and uploads `_output` as a workflow
artifact. It publishes nothing: the `make publish` step only runs with
registry credentials, which aren't configured
([ADR 0010](../adr/0010-release-branches-and-tags.md)). The **Tag** workflow
creates release tags; **Backport** opens backport PRs to `release-X.Y`.

`VERSION` comes from `git describe --tags` (`v0.1.0` on a tag,
`v0.1.0-3.gabc1234` three commits after it, `v0.0.0-…` without tags) and is linked into
the binary (`internal/version`) and the package file name. Release steps:
[development.md](../development.md#releasing).

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

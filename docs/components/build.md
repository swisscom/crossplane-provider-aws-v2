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
| `make codegen.test` | Tests of the `codegen/` module |

`make` pins `GOTOOLCHAIN` to the Go version of `go.mod`.

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

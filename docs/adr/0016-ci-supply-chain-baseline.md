# 0016. CI and supply-chain baseline

- Status: Accepted
- Date: 2026-10-23

## Context

The provider was built with go1.26.0 and `golang.org/x/net` v0.58.0, both
with reachable vulnerabilities, and nothing in CI would have noticed. CI set
up Go 1.25.11 from a hand-kept `GO_VERSION` while `make` used `go.mod`, so the
two drifted. The workflows had no `permissions:` (the token default can be
write), used actions by mutable tag, several on removed Node runtimes, and a
comment-triggered workflow had write scopes on a public repository. Publish
ran in parallel to the tests and logged in to a registry the Makefile never
pushes to. The code generator and the importer quarantine were not checked
in CI, so a hand-edited `zz_` file passed.

Options considered: keep `GO_VERSION` and sync by hand (that is the drift we
had); `golang/govulncheck-action` (one more action, and it runs its own
setup-go); gitleaks-action (needs a licence for organisations); jumping every
action to its newest major (more behaviour change than the findings need).

## Decision

- **One Go source.** The `go` line of both `go.mod` files is the Go
  version (no `toolchain` line). `make` (`GOTOOLCHAIN`), `setup-go`
  (`go-version-file`) and CodeQL read it; `make toolchain.check` (part of
  `modules.check`) fails when the two modules differ.
- **Every CI check is a make target** (`vulncheck`, `vulncheck.binary`,
  `services.all`, `workflows.lint`, `toolchain.check`,
  `check-import-todo`), so a local run is the CI run.
- **Least privilege.** Every workflow starts with `permissions: {}`; each job
  lists the scopes it needs. Steps that do not push use
  `persist-credentials: false`.
- **Pinned actions.** Every `uses:` is a full commit SHA with a `# vX.Y.Z`
  comment. We pick the newest release, at least 14 days old, of the lowest
  major that runs on node24 (or is composite). Renovate
  (`helpers:pinGitHubActionDigests`, `minimumReleaseAge: 14 days`, security
  fixes exempt) keeps them current; zizmor's `hash-pin` policy fails CI on an
  unpinned action.
- **Vulnerability and secret scanning.** govulncheck (pinned version, live
  database) on both modules and on the shipped binary for every PR and push,
  and daily. trufflehog (action and image pinned) scans PR and push ranges
  and, daily, the whole history. CodeQL runs on PRs, pushes and weekly, and
  builds both modules.
- **Publish last.** `publish-artifacts` runs only on push, only after lint,
  check-diff, unit tests, govulncheck and the build pass, only when the
  repository variable `XPKG_PUBLISH` is `true`, and pushes to
  `ghcr.io/swisscom` with `GITHUB_TOKEN` (`packages: write`). It restores no
  cache.
- `commands.yml` (`/points`, `/backport`) and `promote.yml` are removed; the
  label-driven `backport.yml` stays.

## Consequences

- A Go bump is one line in each `go.mod`; CI follows without a workflow edit.
- A new Go or module advisory turns CI red without a code change; the fix is
  a Go or module bump.
- Renovate must be installed on the repository, or the pins rot. New actions
  must be added pinned, or `make workflows.lint` fails.
- check-diff needs network on a cold model cache (raw.githubusercontent.com).
- Follow-ups that need an owner or an admin:
  - `.github/CODEOWNERS` for `/.github/`, `Makefile`, `go.mod`/`go.sum` (both
    modules), `renovate.json` and `build/`, once the owner's handle is known.
  - Set the default workflow token to read-only and disable "Allow GitHub
    Actions to create and approve pull requests".
  - Set `vars.XPKG_PUBLISH=true` and give the repository write access to the
    ghcr package to enable publishing.
  - When publishing is on: keyless cosign signing of the pushed xpkg digest,
    an SPDX SBOM (syft) attested with cosign, and
    `actions/attest-build-provenance`, in `publish-artifacts` with
    `id-token: write` and `attestations: write`.
  - A kind e2e job ("package installs and becomes Healthy") once
    `cluster/local/integration_tests.sh` is repaired.

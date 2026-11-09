# 0011. Provider-specific credentials sources; namespace isolation by RBAC

- Status: Accepted
- Date: 2026-11-02

## Context

`ProviderConfig` (namespaced) and `ClusterProviderConfig` need a
`credentials.source`. This was implemented at the scaffold (`9355732`) and
released in 0.1.0, but not recorded.

**provider-aws v1** uses crossplane-runtime's generic enum
`None;Secret;InjectedIdentity;Environment;Filesystem`
(`pkg/utils/connect/aws/config.go`):

- `InjectedIdentity` picks its mode implicitly:
  - `assumeRole` is set: assume that role.
  - otherwise, `assumeRoleWithWebIdentity.roleARN` is set: web identity.
  - otherwise: the SDK default chain.

  With both set, web identity is silently ignored.
- `Environment` and `Filesystem` read credentials from the provider pod's
  environment or files.

**crossplane-contrib/provider-upjet-aws** (namespaced API, checked at
`3b1d4ef`) uses `None;Secret;IRSA;WebIdentity;PodIdentity;Upbound`:

- `IRSA` and `PodIdentity` both call the SDK default chain.
- Both config kinds share one spec, with no restriction per kind.
- Its only namespace guard: secret refs of a namespaced `ProviderConfig`
  are forced to the MR's namespace.

With namespaced MRs ([0002](0002-namespaced-resources-only.md)), a namespaced
`ProviderConfig` is the natural object to give a tenant. Some sources let
it use credentials that are not the tenant's own:

- `IRSA` and `PodIdentity` use the provider pod's IAM role. The default
  chain also reads `AWS_*` environment variables and `~/.aws` files of the
  pod.
- `WebIdentity` reads the token from a path chosen in the spec
  (`tokenFile`), on the provider pod. It can be pointed at the pod's own
  service-account token.
- Any MR can reference any `ClusterProviderConfig`. Kubernetes RBAC can't
  restrict a reference.

Options considered for namespaced `ProviderConfig`s:

- (A) any source, with isolation by RBAC, like upjet;
- (B) `Secret` only, with pod identity reachable only through a
  `ClusterProviderConfig`.

## Decision

`credentials.source` is a provider-specific enum,
`Secret;IRSA;PodIdentity;WebIdentity`, with the same names as upjet-aws:

- The source names the AWS mechanism. CEL rules require its options:
  - `Secret` requires `secretRef`;
  - `WebIdentity` requires `assumeRoleWithWebIdentity`.
- `IRSA` and `PodIdentity` stay two values on one code path. They state
  what the platform set up, match upjet-aws, and can diverge later.
- `WebIdentity` is explicit instead of v1's implicit mode. `assumeRole`
  applies on top of any source, so web identity followed by assume role
  works.
- `Environment`, `Filesystem` and `None` are not offered. They expose the
  pod's environment or files, and `None` is useless for AWS.
- `InjectedIdentity` is not accepted as an alias. Moving from v1 rewrites
  every config anyway (new API group, namespaced or cluster kind), and an
  alias would bring back the implicit mode.

Namespace isolation follows (A) for now. A namespaced `ProviderConfig` may
use every source; only `Secret` is limited to the config's own namespace,
which `GetConfig` enforces. Which credentials a tenant can use is decided by
RBAC on `providerconfigs` and by which `ClusterProviderConfig`s exist.

## Consequences

- **Tenant setup:** tenants must not be allowed to create or edit
  `ProviderConfig`s unless they may use the provider pod's identity. The
  platform team creates them per namespace. Every `ClusterProviderConfig`
  must be acceptable for every namespace, or an admission policy
  (ValidatingAdmissionPolicy, Kyverno) has to restrict
  `spec.providerConfigRef` per namespace.
- **Docs:** `docs/components/provider-config.md` says this; the provider
  doesn't enforce it.
- **Migration from v1:** configs with `InjectedIdentity`, `Environment`,
  `Filesystem` or `None` are rejected by the CRD. The mapping:

  | provider-aws v1 | provider-aws-v2 |
  |---|---|
  | `InjectedIdentity` | `IRSA` or `PodIdentity` |
  | `InjectedIdentity` + `assumeRoleWithWebIdentity` | `WebIdentity` + `assumeRoleWithWebIdentity` |
  | `InjectedIdentity` / `Secret` + `assumeRole` | same source + `assumeRole` |
  | `Secret` | `Secret` (namespaced config: in its own namespace) |
  | `Environment`, `Filesystem`, `None` | not supported; use a `Secret` |

  The importer ports MR kinds, not configs, so it needs no change.
- **Changing to (B) later** is incompatible for configs that use pod
  identity. `ProviderConfig` is `v1alpha1`, so it is allowed in a minor
  release with migration notes ([0010](0010-release-branches-and-tags.md)).
  It needs a new ADR that supersedes the isolation part of this one. The
  same applies to a new source (e.g. `Upbound`), which must state which
  config kinds may use it.

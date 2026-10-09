# 0002. Namespaced managed resources only

- Status: Accepted
- Date: 2026-10-02

## Context

Crossplane v2 supports namespaced MRs, which allow multi-tenancy with plain
Kubernetes RBAC. Supporting both scopes doubles CRDs and secret-handling
paths. Crossplane v2 removed `publishConnectionDetailsTo` and external
secret stores.

## Decision

All managed resources are namespaced. Secret references are local
(`LocalSecretKeySelector`) and resolved in the MR's namespace; provider-owned
helper secrets live there too. References are namespaced
(`NamespacedReference`/`NamespacedSelector`). Credentials come from a
namespaced `ProviderConfig` (secrets from its own namespace only) or a
cluster-scoped `ClusterProviderConfig`.

## Consequences

- Tenants are isolated by namespace; deleting a namespace cleans up its
  secrets.
- Ported code must never read secrets from other namespaces or from the
  provider namespace ([porting-notes.md](../porting-notes.md)).
- No cluster-scoped MRs and no external secret stores may be added.

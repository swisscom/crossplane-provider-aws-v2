# 0001. New native provider for Crossplane v2

- Status: Accepted
- Date: 2026-10-02

## Context

Crossplane v2 introduces namespaced managed resources and crossplane-runtime
v2 (typed clients, no external secret stores). The existing native provider,
crossplane-contrib/provider-aws, is built on crossplane-runtime v1, AWS SDK
for Go v1 (end of support) and cluster-scoped resources, and generates from
an old ACK version. The Upbound/upjet providers wrap Terraform, which adds a
second state engine and resource model.

Options: upgrade provider-aws in place; use the upjet provider; start a new
native provider.

## Decision

Start a new native provider, scaffolded from crossplane/provider-template:
crossplane-runtime v2, AWS SDK v2, namespaced MRs. Use the API group
`*.aws2.crossplane.io`, distinct from provider-aws's `*.aws.crossplane.io`,
so both can run in one cluster during migration.

## Consequences

- No legacy constraints: every kind is v2-native from the start.
- Kinds have to be brought over one by one
  ([0003](0003-ack-code-generator-as-a-library.md),
  [0004](0004-import-kinds-from-provider-aws.md)).
- Users migrate by recreating/importing resources under the new group; there
  is no in-place conversion from provider-aws objects.

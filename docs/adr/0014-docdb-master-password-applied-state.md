# 0014. Record the applied DocumentDB master password as a digest

- Status: Accepted
- Date: 2026-10-16

## Context

`docdb/DBCluster` reads the master password from `masterUserPasswordSecretRef`.
To know whether AWS already has it, the controller compared it with the
`password` key of the connection secret. Without `writeConnectionSecretToRef`
a rotation was never detected, and a cluster restored with
`autogeneratePassword` kept the backup's password while the Secret showed a
new one. DocumentDB's `DescribeDBClusters` has no `PendingModifiedValues`, so
AWS can't tell us either. Without `ApplyImmediately` AWS also waits for the
maintenance window to change the password, while the connection secret
already showed the new one.

Options considered: require `writeConnectionSecretToRef` (breaking, and it
hides the gap); a hash in an annotation (anyone who can read the MR but not
Secrets could brute-force a weak password offline); a plaintext cache Secret
as rds/DBInstance keeps (ADR 0013), which would be a third plaintext copy.

## Decision

After AWS accepted a password (CreateDBCluster, or the first ModifyDBCluster
of an update), the provider writes its hex SHA-256 digest to
`data.sha256` of the Secret `docdb-dbcluster.<metadata.uid>.password-state` in
the MR's namespace. The name has a fixed length and contains the UID, so it
fits the 253-character limit and cannot collide with a user's Secret. The
Secret has a controller owner reference to the DBCluster with
`blockOwnerDeletion: false` (no RBAC on finalizers needed) and is garbage
collected with it. An existing Secret of that name controlled by something
else is an error and is never taken over.

A referenced password is "applied" when its digest matches (constant-time
compare). Without a state Secret, a connection secret that already has the
password counts as applied once, so running MRs don't change their password
on upgrade; otherwise the referenced password is applied once. A sent
password always goes with `ApplyImmediately=true`: for DocumentDB the flag
only affects the password and the cluster identifier. A restore records
nothing, so the first update after it applies the referenced password.

rds/DBInstance keeps a plaintext cache (ADR 0013) because an
`autogeneratePassword`-only MR has no other home for the password. DocumentDB
always has one, the referenced Secret, so only "was it applied" must be
stored, and a digest is enough.

## Consequences

- Rotation works with and without a connection secret; nothing about the
  password is in the MR, its status, events or errors.
- One extra Secret per DBCluster with a password reference. The provider
  needs create/patch on Secrets in the MR namespace, which it already has for
  connection secrets.
- An MR with a password reference, no state Secret and no matching
  connection secret gets its referenced password applied once after the
  upgrade.
- A password change for the maintenance window can't be expressed any more.
- Follow-up: converge rds and docdb on one approach (needs an rds migration).

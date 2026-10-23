# 0013. rds/DBInstance master password lifecycle

- Status: Accepted
- Date: 2026-10-16

## Context

rds/DBInstance keeps a provider-owned cache Secret `dbinstance.<uid>` in the
namespace of the MR, with the master password and a "restore pending" flag.
Review found that the password lifecycle around it was unsafe:

- A snapshot or point-in-time restore with `autogeneratePassword` never
  applied the generated password and then cached an empty one, so the
  connection Secret ended with no password and nobody knew the real one.
- A rotated `masterUserPasswordSecretRef` value was published during Observe,
  before `ModifyDBInstance` had applied it; a failed or deferred modify left
  clients with a password the database rejects.
- A restore ran inside `preCreate`, after which the generated `Create` still
  called `CreateDBInstance` and failed; the password was cached after the
  restore, so a failed cache write lost it.
- `manageMasterUserPassword: true` could not create an instance, and toggling
  it published stale passwords.
- The cache Secret had no owner and was left behind with
  `deletionPolicy: Orphan` or out-of-band deletes.

docdb (ADR 0014) keeps only a digest of the applied password, because its
plaintext always lives in the user's ref Secret. rds can't do that: an
`autogeneratePassword`-only MR has no other home for the generated password,
and the cache Secret name is already used by existing MRs (renaming would
orphan their caches).

## Decision

The cache Secret stays the record of "the master password AWS has". Every
publish and every modify derives from it:

1. **Publish only what AWS has.** Observe publishes the cached password;
   nothing while a restore has not applied it; an empty value while AWS
   manages the password in Secrets Manager. Create publishes the password it
   just sent. Every modify that sends a password first caches it marked
   "not applied yet" (the restore flag), and Update clears the mark and
   publishes the password only after `ModifyDBInstance` succeeded. A lost
   response or a failed cache write therefore leads to a re-send of the same
   password, never to the old one being published as current. Observe never
   writes a Secret.
2. **Desired password = ref value, else the cached one.** A restore marks the
   cache "restored" only when there is a password to apply; once the instance
   is available, it is applied with `ModifyDBInstance`. A stored password is
   never replaced by an empty one. `ApplyImmediately` is not forced: per the
   RDS User Guide a new master password "is applied asynchronously, as soon
   as possible. This setting ignores the apply immediately setting."
3. **Restore is Create.** The overridden `Create` caches the password first,
   then calls the restore operation, and never `CreateDBInstance`.
4. **The cache Secret is owned by the MR** (controller ownerReference with
   `blockOwnerDeletion: false`, so the provider needs no finalizer
   permission); garbage collection removes it with the MR.

## Consequences

- The connection Secret has no `password` key after a snapshot or
  point-in-time restore until the password was applied; clients must tolerate
  a missing key while the instance is provisioned.
- The plaintext cache stays. It sits next to the connection Secret, which
  holds the same value, so it adds no new reader. Converging on docdb's
  approach (password only in the user's ref Secret, digest in the cache) is a
  follow-up that needs a migration.
- Existing caches get the ownerReference on their next write; there is no
  migration job. MRs hit by the old restore bug that use only
  `autogeneratePassword` have an empty cache and need a
  `masterUserPasswordSecretRef`.
- While a password is not applied yet, Observe publishes no `password`
  key; the connection Secret keeps its last value (merge patch). After a
  lost modify response or a failed write of the applied mark, the connection
  Secret can briefly keep the replaced password until the re-send succeeds
  (usually one error backoff); it heals by itself.
- Contributors: a new code path that sends a master password must cache it
  before the AWS call (marked "not applied yet" unless the call creates the
  instance with it), clear the mark only after AWS accepted it, and publish
  only through the cached value.

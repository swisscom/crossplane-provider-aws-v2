# 0011. Fail-closed generator defaults

- Status: Accepted
- Date: 2026-10-16

## Context

The generated controller is what a new kind gets before anyone writes
hooks. A review of the generator found that several of its defaults did
the wrong thing quietly. They were hidden only because all five imported
kinds override them in `setup.go`:

- a ReadMany observe listed resources unfiltered and adopted the first
  one, which could belong to someone else, and read its input from
  `status`, which is empty before the first observe;
- Create copied the AWS response into `spec.forProvider`, rewriting the
  user's desired state (casing, aliases, defaults);
- a kind without a read operation never "existed" (Create every poll),
  one without a delete operation reported success and orphaned the AWS
  resource, and a missing 404 code turned every error into "not found";
- `spec.forProvider.region` could be changed, which creates a new, empty
  resource in the other region;
- secret-looking fields (`tdeCredentialPassword`) became plain API fields;
- `ignore.field_paths` entries that matched nothing were silently skipped
  by ACK, so a typo removed nothing.

Options: warn and keep generating; one coarse `unsafe: true` switch; or
explicit per-capability opt-ins.

## Decision

The generator fails closed. A planning pass between loading the model and
rendering checks every kind and collects every problem; if there is any,
`make services` writes nothing and each message names the config line to
write. Safe behaviour is the default, and every exception is a named,
per-kind opt-in in `apis/<svc>/codegen.yaml` (`identifier`,
`customObserve`, `customDelete`, `orphanOnDelete`, `customIsUpToDate`,
`immutableFields`, `allowSensitiveFields` with a reason). That file is
parsed strictly.

The generated code takes the identity from the external name, keeps only
exact matches and fails on several, writes the create response to status
only, treats a nil describe response as an error, and marks `region` (plus
configured fields) immutable with CEL. Generated files are formatted in
memory and written atomically; stale files of removed kinds are deleted.

Warnings were rejected because nobody reads `make services` output, and
every finding was already silent. A single `unsafe` switch was rejected
because it hides which risk was accepted.

## Consequences

- New kinds can't be generated with an unsafe default; a new service may
  need a few explicit lines, which the error messages spell out.
- Breaking for users: `region` is immutable, Create no longer writes spec,
  and `tdeCredentialPassword` (rds) and
  `pendingModifiedValues.masterUserPassword` (rds, docdb) are gone
  (see CHANGELOG).
- Kinds whose API normalises identifiers or values need hand-written
  `filterList`/`isUpToDate` hooks; Create no longer masks such differences.
- The external-name annotation is not protected: CRD CEL can't read
  annotations. Protecting it needs a webhook.
- Follow-ups: pin API model checksums; decide whether an update operation
  with no comparable fields should fail generation.

# 0015. Remove only the tags the provider set (DocumentDB, RDS, EC2)

- Status: Accepted
- Date: 2026-10-16; extended to RDS and EC2 on 2026-10-23

## Context

The DocumentDB kinds made the AWS tags equal to `spec.tags`: every key not in
the spec was removed, also when `tags` was unset. That stripped tags set by
tag policies, AWS Backup selections, cost-allocation tooling or the console,
and failed on every reconcile for `aws:` keys, which only AWS may remove.
A changed value was removed and added again, so tag-based IAM or Backup
selection briefly saw the key missing.

Options considered: unset `tags` means unmanaged, set means authoritative
(still strips foreign tags as soon as one tag is set); a marker tag in AWS
(counts against the 50-tag limit and is visible to users); a status field
(the generated Observe rewrites `status.atProvider`); a new spec field
listing keys to remove (API growth for a rare need).

## Decision

The provider adds or updates every tag of `spec.tags` (`AddTagsToResource`
overwrites values, so nothing is removed first) and removes only keys it
recorded in the annotation `docdb.aws2.crossplane.io/managed-tag-keys`
(sorted, comma-separated) that are no longer in the spec. `aws:` keys are
never recorded or removed. Unset `tags` adds nothing but still removes
recorded keys.

Each Observe sets the record to the spec keys plus recorded keys that are
still in AWS, before Update adds anything, so a tag the provider adds is
always recorded first. A key leaves the record only once it is gone from
AWS. When the record changes, Observe reports the resource as
late-initialized, and the runtime persists the annotation with the
late-init update before it calls Update (crossplane-runtime v2.4.2,
`reconciler.go`). That update replaces the in-memory status, so Update uses
the ARN cached during Observe. Under a management policy without
`LateInitialize` or `*` the record would not persist, so nothing is
recorded and nothing is removed.

## Consequences

- Foreign tags survive; the failure mode is "a tag is not removed", never
  "too much is removed".
- An MR that existed before this change has no record, so tags it set
  earlier and that are no longer in its spec stay until removed by hand.
- With a management policy without `LateInitialize`, tags are added and
  updated but never removed.
- Every Observe lists the tags, also when another field already differs.

## Extension to RDS and EC2 (2026-10-23)

`rds/DBInstance` and the EC2 kinds (`SecurityGroup`,
`SecurityGroupIngressRule`, `SecurityGroupEgressRule`) removed every tag not
in the spec, like DocumentDB before this ADR, so a tag-selected AWS Backup
plan could silently stop covering them. They now follow the same decision:

- The record logic lives once in `internal/utils/tags` (`Record`,
  `Removable`); each service keeps its own diff, because the SDK tag types
  differ. DocumentDB behaves as before.
- Each API group has its own annotation:
  `rds.aws2.crossplane.io/managed-tag-keys` and
  `ec2.aws2.crossplane.io/managed-tag-keys`. A shared key would have
  orphaned the records DocumentDB MRs already carry, and a key in the
  kind's own group is clear about who writes it.
- EC2 records Crossplane's `crossplane-*` tags too, since it sets them.
  `tagsIgnore` does not stop RDS from recording spec keys; it only hides
  matched AWS tags from the diff, so they are never removed.
- RDS reads the tags from `DescribeDBInstances` and EC2 from the describe
  call, so there is no extra list call. EC2 keeps the observed tags for
  Update instead of reading `status`, which the late-init update replaces.
- Upgrade: an existing MR has no record. Its first Observe records the
  current spec keys (EC2: plus `crossplane-*`) and reports a late-init, so
  the record is stored before Update runs. Tags set outside the provider
  are never removed; keys dropped from the spec before the upgrade are not
  recorded and stay until removed by hand.

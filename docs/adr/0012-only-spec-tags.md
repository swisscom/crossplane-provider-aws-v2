# 0012. Only spec tags are applied; AWS tags are ignored

- Status: Accepted
- Date: 2026-11-04

## Context

crossplane-runtime offers `resource.GetExternalTags`, which returns
`crossplane-kind`, `crossplane-name`, `crossplane-namespace` and
`crossplane-providerconfig` for a managed resource. provider-aws and Upjet
providers add these tags to every AWS resource they create, and several of
our kinds did the same.

These tags are not part of the spec, so users can't see or turn them off,
and they count towards AWS tag limits. Also, tag policies, cost reports or
other tools that own the tags of an account may not expect them. The
Kubernetes object they point to is already linked to the AWS resource by
its external name.

AWS sets tags whose keys start with `aws:` (e.g. CloudFormation, Service
Catalog). They can't be changed or removed; trying to remove them fails or
makes the resource never be up to date. Not all kinds ignored them.

## Decision

- A kind applies exactly the tags in `spec.forProvider.tags`, on create and
  on update. Tags not in the spec are removed, including tags that earlier
  versions of the provider added.
- Tags whose keys start with `aws:` (in any case, as AWS reserves all of
  them) are never compared, added or removed. The single rule is
  `awsclient.IsAWSTag` in `internal/clients/aws`.

## Consequences

- AWS resources created by earlier versions lose their `crossplane-*` tags
  on the next reconcile (changelog note).
- Users who want such tags set them in the spec, e.g. from a composition.
- New kinds must not call `resource.GetExternalTags`, and their tag diff
  must skip `awsclient.IsAWSTag` keys.

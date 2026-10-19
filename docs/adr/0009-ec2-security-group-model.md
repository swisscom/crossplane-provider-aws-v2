# 0009. Security groups and their rules are separate kinds

- Status: Accepted
- Date: 2026-10-19

## Context

provider-aws manages a security group with all its rules inline
(`spec.forProvider.ingress`/`egress`); its separate `SecurityGroupRule`
kind uses the older Authorize/Revoke API without rule IDs. Since 2021 the
AWS API identifies each rule by an ID (`sgr-…`;
`DescribeSecurityGroupRules`, `ModifySecurityGroupRules`), so a rule can be
managed on its own.

Inline rules make several owners of one group (compositions, teams)
overwrite each other, and a change to one rule is a diff of the whole list.

provider-aws can also find a group by name, which is convenient where one
team owns the account and VPC.

## Decision

- Follow the AWS API model: `SecurityGroup` (rules only in the status),
  `SecurityGroupIngressRule`, `SecurityGroupEgressRule`, external names are
  AWS IDs. Field names follow the generator (`securityGroupID`, `cidrIPv4`,
  `referencedSecurityGroupID`, …); tags are a list of key/value like all our
  kinds.
- A new group's default allow-all egress rules (added by AWS) are revoked:
  egress is what egress rule resources allow.
- Adopting an existing group by name is opt-in per resource
  (`adoptExistingByName`), filters by VPC if set, and refuses ambiguous
  matches and the `default` group.
- Rules are validated at runtime (before create/update), not with CEL
  rules in the CRD: CEL can't compare against AWS, rejects values that
  references fill in only after admission, and needs newer Kubernetes
  versions. This applies to all kinds (see `AGENTS.md`).
- References (`securityGroupIDRef`, `referencedSecurityGroupIDRef`) are
  custom fields with `+crossplane:generate:reference` markers; generated
  fields can't take markers.

## Consequences

- One resource per rule: more objects, but independent owners and in-place
  updates (`ModifySecurityGroupRules`) that keep rule IDs.
- Existing groups and rules are taken over by setting their AWS IDs as
  external names.
- Migration from provider-aws inline rules needs one rule resource per
  rule.
- With adoption, two resources can manage the same group; the risks are
  documented in the kind README.

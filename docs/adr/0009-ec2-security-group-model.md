# 0009. Security groups and their rules are separate kinds

- Status: Accepted
- Date: 2026-10-19

## Context

provider-aws manages a security group with all its rules inline
(`spec.forProvider.ingress`/`egress`); its separate `SecurityGroupRule`
kind uses the older Authorize/Revoke API without rule IDs. The upjet
provider (Terraform) has a `SecurityGroup` without rules plus
`SecurityGroupIngressRule`/`SecurityGroupEgressRule`, one rule each,
identified by the AWS rule ID (`sgr-…`), as in the AWS API since 2021
(`DescribeSecurityGroupRules`, `ModifySecurityGroupRules`).

Inline rules make several owners of one group (compositions, teams)
overwrite each other, and a change to one rule is a diff of the whole list.
Our existing compositions use the upjet kinds; we want to migrate them to
this provider by changing the API group and field names, keeping the AWS
resources (same sg-/sgr- IDs as external names).

provider-aws can also find a group by name, which is convenient where one
team owns the account and VPC.

## Decision

- Follow the upjet/AWS model: `SecurityGroup` (rules only in the status),
  `SecurityGroupIngressRule`, `SecurityGroupEgressRule`, external names are
  AWS IDs. Field names follow the generator (`securityGroupID`, `cidrIPv4`,
  `referencedSecurityGroupID`, …); tags are a list of key/value like all our
  kinds.
- Like Terraform, a new group's default allow-all egress rules are revoked:
  egress is what egress rule resources allow.
- Adopting an existing group by name is opt-in per resource
  (`adoptExistingByName`), filters by VPC if set, and refuses ambiguous
  matches and the `default` group.
- Rules are validated at runtime (before create/update) instead of with CEL
  rules in the CRD: CEL can't compare against AWS, conflicts with
  references resolved after admission, and needs newer Kubernetes versions.
- References (`securityGroupIDRef`, `referencedSecurityGroupIDRef`) are
  custom fields with `+crossplane:generate:reference` markers; generated
  fields can't take markers.

## Consequences

- One resource per rule: more objects, but independent owners and in-place
  updates (`ModifySecurityGroupRules`) that keep rule IDs.
- Migration from upjet: orphan the upjet resources, then create ours with
  the same external names
  ([internal/controller/ec2/securitygroup/README.md](../../internal/controller/ec2/securitygroup/README.md)).
- Migration from provider-aws inline rules needs one rule resource per
  rule.
- With adoption, two resources can manage the same group; the risks are
  documented in the kind README.

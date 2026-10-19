# ec2/SecurityGroupEgressRule

Manages one egress rule of an EC2 security group
(`SecurityGroupEgressRule.ec2.aws2.crossplane.io`), like
`aws_vpc_security_group_egress_rule` in Terraform
([ADR 0009](../../../../docs/adr/0009-ec2-security-group-model.md)).

## Origin

Generated from scratch on 2026-10-19 (`apis/ec2/generator-config.yaml`;
operations mapped by hand: `AuthorizeSecurityGroupEgress`, `DescribeSecurityGroupRules`,
`ModifySecurityGroupRules`, `RevokeSecurityGroupEgress`).

## Behavior

- **Spec**: `securityGroupID` (or `securityGroupIDRef`/`Selector` to a
  `SecurityGroup`), `ipProtocol` (`tcp`, `udp`, `icmp`, `icmpv6`, a
  number, or `-1` for all), `fromPort`/`toPort` (ICMP type/code), exactly
  one destination (`cidrIPv4`, `cidrIPv6`, `prefixListID`,
  `referencedSecurityGroupID` or its Ref/Selector), `description`, `tags`.
- **Validation** (before create and update, no CEL): `securityGroupID` and
  `ipProtocol` set, exactly one destination.
- **External name**: the rule ID (`sgr-…`), set after creation.
- **Create**: `AuthorizeSecurityGroupEgress` with one `IpPermissions` entry and tags.
- **Drift**: every spec field. Protocols are compared as AWS reports them
  (lower case, `6`/`17`/`1`/`58` → `tcp`/`udp`/`icmp`/`icmpv6`); unset
  ports equal `-1`. A different `securityGroupID`, or an external name
  pointing to an ingress rule, is an error.
- **Update**: `ModifySecurityGroupRules` (all fields but the group, in
  place, keeping the rule ID), then `CreateTags`/`DeleteTags`.
- **Delete**: `RevokeSecurityGroupEgress` by rule ID; done if the rule or
  the group is already gone.
- **Status**: `securityGroupRuleID`, `securityGroupRuleARN`,
  `groupOwnerID`, `tags`.
- **Conditions**: Available once observed.

Shared logic is in [`../utils`](../utils) (`Rule`).

| Hook | Why |
|------|-----|
| `preObserve` | describe by rule ID |
| `isUpToDate` | the read output has another shape than the spec |
| `preCreate` | validation; `IpPermissions` instead of the legacy top-level fields, tags |
| `postCreate` | external name from the created rule |
| `preUpdate`, `postUpdate` | `SecurityGroupRules` of the modify call, tags |
| `preDelete`, `postDelete` | revoke by rule ID; group already deleted |
| `postObserve` | status fields, condition |

## Limitations

- `securityGroupID` can't be changed; delete and recreate.
- Creating a rule identical to an existing one (e.g. created by another
  tool) fails with `InvalidPermission.Duplicate`; set the external name to
  the existing rule ID instead.

## Examples

[`examples/ec2/securitygroup-rules.yaml`](../../../../examples/ec2/securitygroup-rules.yaml).

## History

| Date | Commit | Change |
|------|--------|--------|
| 2026-10-19 | — | Generated from scratch |

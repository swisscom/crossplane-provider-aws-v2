# ec2/SecurityGroupEgressRule

Manages one egress rule of an EC2 security group
(`SecurityGroupEgressRule.ec2.aws2.crossplane.io`), identified by its AWS
security group rule ID (`sgr-…`) as in the AWS API
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
- **Adopt existing** (`adoptExisting: true`, opt-in): rules have no name, but
  AWS allows only one rule per group with the same direction, protocol,
  ports and destination. Without an external name, the rules of the group are
  listed (`DescribeSecurityGroupRules`, filter `group-id`) and the ID of
  the one with the same content (compared as for drift, see below) is
  written to the external name, reported with a Normal event
  `AdoptedExistingSecurityGroupRule`. The description and tags don't count
  and are updated afterwards. No match creates the rule; an invalid spec is
  not looked up (Create reports why).
- **Create**: `AuthorizeSecurityGroupEgress` with one `IpPermissions` entry and tags.
- **Drift**: every spec field. Protocols are compared as AWS reports them
  (lower case, `6`/`17`/`1`/`58` → `tcp`/`udp`/`icmp`/`icmpv6`); unset
  ports equal `-1`. A different `securityGroupID`, another source type
  (see Limitations), or an external name pointing to an ingress rule, is
  an error.
- **Update**: `ModifySecurityGroupRules` (in place, keeping the rule ID),
  then `CreateTags`/`DeleteTags`.
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
| `wrapExternal` (`adoptingExternal.Observe`) | `adoptExisting`, adoption event |

## Risks of `adoptExisting`

Adoption takes over a rule nobody declared to Crossplane:

- Two resources with the same group and content (in any namespace or
  cluster) manage the same rule; deleting one deletes the rule of the other.
- A rule created by hand or another tool is taken over silently, apart from
  the event.
- When moving rules from another Crossplane provider, make that provider
  orphan them first (no `Delete` in its `managementPolicies`), otherwise it
  deletes them while they are adopted.

Use it to take over rules whose group you control; to import a known rule,
set the external name instead.

## Limitations

- `securityGroupID` and the source type (`cidrIPv4`, `cidrIPv6`,
  `prefixListID` or `referencedSecurityGroupID`) can't be changed: AWS
  rejects it. Changing the value of the same source type (another CIDR,
  prefix list or group) works in place. To change the type, delete and
  recreate the rule.
- Creating a rule identical to an existing one (e.g. created by another
  tool) fails with `InvalidPermission.Duplicate`; set the external name to
  the existing rule ID instead, or set `adoptExisting`.

## Examples

[`examples/ec2/securitygroup-rules.yaml`](../../../../examples/ec2/securitygroup-rules.yaml).

## History

| Date | Commit | Change |
|------|--------|--------|
| 2026-10-19 | — | Generated from scratch |
| 2026-10-20 | | A source type change is an error instead of a modify AWS rejects; prefix lists validated against AWS |
| 2026-10-20 | | `adoptExisting` |

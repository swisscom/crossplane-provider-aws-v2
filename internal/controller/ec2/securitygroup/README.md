# ec2/SecurityGroup

Manages an EC2 (VPC) security group without its rules
(`SecurityGroup.ec2.aws2.crossplane.io`). Rules are separate kinds:
[`SecurityGroupIngressRule`](../securitygroupingressrule/README.md) and
[`SecurityGroupEgressRule`](../securitygroupegressrule/README.md), as in AWS,
Terraform and the upjet provider ([ADR 0009](../../../../docs/adr/0009-ec2-security-group-model.md)).

## Origin

Generated from scratch on 2026-10-19 (`apis/ec2/generator-config.yaml`). Not
imported: provider-aws manages rules inline in the group, the upjet model
was chosen instead.

## Behavior

- **External name**: the group ID (`sg-…`), set after creation. Set the
  `crossplane.io/external-name` annotation to manage an existing group.
- **Create**: `CreateSecurityGroup` with `groupName` (default
  `metadata.name`), `description`, `vpcID` (default VPC if unset) and tags
  (spec tags plus Crossplane's `crossplane-*` tags). Then the rules allowing
  all egress that AWS adds to every new group (IPv4 `0.0.0.0/0`, and
  `::/0` in VPCs with IPv6) are revoked, retried while the new group isn't
  visible yet. If revoking fails, the group is deleted
  again and the error reported, so the next reconcile starts over.
- **Adopt by name** (`adoptExistingByName: true`, opt-in): without an
  external name, the group is looked up by `groupName` (and `vpcID` if set)
  before creating one. One match is adopted as is (its rules and default
  egress are not changed); no match creates the group; several matches and
  the group `default` are errors.
- **Late initialization**: `groupName`, `vpcID`.
- **Drift**: tags (keys starting with `aws:` are ignored). A different
  `groupName`, `description` or `vpcID` is an error: AWS can't change them.
- **Update**: `CreateTags` / `DeleteTags`.
- **Delete**: `DeleteSecurityGroup` by ID. While the group is in use
  (network interfaces, rules of other groups referencing it), AWS returns
  `DependencyViolation`; the error is reported and the delete retried.
- **Status**: `groupID`, `securityGroupARN`, `ownerID`, `tags`, and the
  current rules (`ipPermissions`, `ipPermissionsEgress`), read only.
- **Conditions**: Available once observed.

| Hook | Why |
|------|-----|
| `preObserve`, `preDelete` | describe/delete by the external name (group ID) |
| `lateInitialize` | `groupName`, `vpcID` |
| `isUpToDate` | immutable fields, tags |
| `update` | tags; there is no update operation |
| `preCreate` | default name, tags |
| `postCreate` | external name, revoke the default egress rules |
| `postDelete` | clearer error for `DependencyViolation` |
| `wrapExternal` (`adoptingExternal.Observe`) | `adoptExistingByName` |

## Risks of `adoptExistingByName`

Group names are unique per VPC only, and adoption takes over a group nobody
declared to Crossplane:

- Two resources with the same name and VPC (in any namespace or cluster)
  manage the same group; with the default deletion policy, deleting one
  deletes the group of the other.
- Without `vpcID`, groups of all VPCs of the account and region match; with
  more than one match the resource fails instead of guessing.
- An adopted group keeps its rules, including rules not managed by
  Crossplane, and its tags are changed to the spec's.

Use it where one team owns the account and VPC (e.g. one VPC per account).
Otherwise set the external name.

## Migrating from upjet (`ec2.aws.upbound.io`)

The AWS resources are kept; only the Kubernetes objects are replaced.

1. Make the upjet `SecurityGroup` and rule resources orphan their AWS
   resources: `managementPolicies: [Observe, Create, Update, LateInitialize]`
   (namespaced resources have no `deletionPolicy`). Apply, wait until
   they are synced, then delete the upjet objects.
2. Create the resources of this provider with the
   `crossplane.io/external-name` annotation copied from the upjet objects
   (`sg-…` for groups, `sgr-…` for rules). Check that
   `status.atProvider.groupID`/`securityGroupRuleID` equal the copied IDs:
   an ID that doesn't exist makes the provider create a new resource.

Verified on 2026-10-19 in a dev account: IDs unchanged, no create calls,
one update each (Crossplane's `crossplane-*` tags).

| upjet | here |
|-------|------|
| `name` | `groupName` |
| `vpcId` | `vpcID` |
| `tags` (map) | `tags` (list of `key`/`value`) |
| `status.atProvider.id`, `.arn` | `status.atProvider.groupID`, `.securityGroupARN` |
| rule `securityGroupId` (`Ref`, `Selector`) | `securityGroupID` (`Ref`, `Selector`) |
| rule `cidrIpv4`, `cidrIpv6`, `prefixListId` | `cidrIPv4`, `cidrIPv6`, `prefixListID` |
| rule `referencedSecurityGroupId` | `referencedSecurityGroupID` |
| rule `status.atProvider.id`, `.arn` | `status.atProvider.securityGroupRuleID`, `.securityGroupRuleARN` |

Crossplane's `crossplane-*` tags are added to existing groups and rules on
the first reconcile; other tags not in the spec are removed.

## Limitations

- `groupName`, `description` and `vpcID` can't be changed (AWS); delete
  and recreate.
- Rules are not managed by this kind; `ipPermissions*` in the status show
  all rules, including those of other tools.

## Examples

[`examples/ec2/securitygroup.yaml`](../../../../examples/ec2/securitygroup.yaml).

## History

| Date | Commit | Change |
|------|--------|--------|
| 2026-10-19 | — | Generated from scratch |

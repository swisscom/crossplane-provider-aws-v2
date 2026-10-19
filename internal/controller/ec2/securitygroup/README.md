# ec2/SecurityGroup

Manages an EC2 (VPC) security group without its rules
(`SecurityGroup.ec2.aws2.crossplane.io`). Rules are separate kinds:
[`SecurityGroupIngressRule`](../securitygroupingressrule/README.md) and
[`SecurityGroupEgressRule`](../securitygroupegressrule/README.md), as in the AWS
API ([ADR 0009](../../../../docs/adr/0009-ec2-security-group-model.md)).

## Origin

Generated from scratch on 2026-10-19 (`apis/ec2/generator-config.yaml`). Not
imported: provider-aws manages rules inline in the group; here each rule is
a separate resource with its AWS rule ID.

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
  egress are not changed) and reported with a Normal event
  `AdoptedExistingSecurityGroup`; no match creates the group; several
  matches and the group `default` are errors.
- **Late initialization**: `groupName`, `vpcID`.
- **Drift**: tags (keys starting with `aws:` are ignored). A different
  `groupName`, `description` or `vpcID` is an error: AWS can't change them.
- **Update**: `CreateTags` / `DeleteTags`. An existing group (external
  name or adopted) gets Crossplane's `crossplane-*` tags on the first
  reconcile; other tags not in the spec are removed.
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
| `wrapExternal` (`adoptingExternal.Observe`) | `adoptExistingByName`, adoption event |

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

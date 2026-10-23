# Architecture decision records

An ADR records one design decision: the situation, what was decided and
what follows from it. They explain *why* the code looks the way it does, so
that nobody undoes a decision by accident and every change of mind is
deliberate.

| # | Decision | Status |
|---|----------|--------|
| [0001](0001-new-native-provider-for-crossplane-v2.md) | New native provider for Crossplane v2, API group `aws2.crossplane.io` | Accepted |
| [0002](0002-namespaced-resources-only.md) | Namespaced managed resources only | Accepted |
| [0003](0003-ack-code-generator-as-a-library.md) | ACK code-generator as a library, own templates | Accepted |
| [0004](0004-import-kinds-from-provider-aws.md) | Import kinds and their hand-written code from provider-aws | Accepted |
| [0005](0005-aws-sdk-go-v2-model-from-go-mod.md) | AWS SDK for Go v2 only; model version follows `go.mod` | Accepted |
| [0006](0006-allow-float-fields.md) | Allow `float64` fields in CRDs | Accepted |
| [0007](0007-generated-drift-detection.md) | Generated drift detection of updatable fields | Accepted |
| [0008](0008-kind-readme-and-changelog.md) | Per-kind README and a root changelog for history | Accepted |
| [0009](0009-ec2-security-group-model.md) | Security groups and their rules are separate kinds | Accepted |
| [0010](0010-compare-semantics-and-pending-overlay.md) | Compare semantics and the pending-change overlay | Accepted |
| [0011](0011-fail-closed-generator-defaults.md) | Fail-closed generator defaults | Accepted |
| [0012](0012-importer-fails-closed.md) | The importer fails closed | Accepted |
| [0013](0013-rds-dbinstance-password-lifecycle.md) | rds/DBInstance master password lifecycle | Accepted |
| [0014](0014-docdb-master-password-applied-state.md) | Record the applied DocumentDB master password as a digest | Accepted |
| [0015](0015-docdb-tag-ownership.md) | Remove only the tags the provider set (DocumentDB, RDS, EC2) | Accepted |
| [0016](0016-ci-supply-chain-baseline.md) | CI and supply-chain baseline | Accepted |

## Writing an ADR

Write one when a decision is hard to reverse, affects many kinds, or was
discussed with alternatives. Copy [template.md](template.md), take the next
number, keep it to one page. Don't edit an accepted ADR's decision; write a
new one that supersedes it and set the old one's status to
`Superseded by NNNN`.

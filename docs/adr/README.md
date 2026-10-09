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

## Writing an ADR

Write one when a decision is hard to reverse, affects many kinds, or was
discussed with alternatives. Copy [template.md](template.md), take the next
number, keep it to one page. Don't edit an accepted ADR's decision; write a
new one that supersedes it and set the old one's status to
`Superseded by NNNN`.

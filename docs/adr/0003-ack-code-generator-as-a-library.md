# 0003. ACK code-generator as a library, own templates

- Status: Accepted
- Date: 2026-10-05

## Context

Hand-writing types and controllers for AWS services doesn't scale. The ACK
code-generator already infers resources, operations and field mappings from
AWS API models, and provider-aws's `generator-config.yaml` files use its
format. ACK's main templates target the ACK runtime. ACK also ships a
Crossplane target (`ack-generate crossplane`, `templates/crossplane`), which
provider-aws uses (ACK v0.26.1), but its templates generate
crossplane-runtime v1, AWS SDK v1 and cluster-scoped resources, even in
current ACK versions.

## Decision

Use the upstream ACK code-generator as a Go library (model, config, field
mapping snippets) in a separate module `codegen/`, and render our own
templates for crossplane-runtime v2 (they started from ACK's Crossplane
templates and keep their file layout). Extension points for hand-written code
are hooks and two package variables (`configure`, `wrapExternal`) in the
generated controller; generated files are `zz_*` and never edited.

## Consequences

- Upstream ACK fixes and model support come with a version bump of
  `codegen/go.mod`; no fork to maintain.
- Template changes regenerate all kinds; they must keep hooks backward
  compatible or update all hand-written code in the same change.
- ACK specifics leak into the API types (`*int64`, `[]*string`); hand-written
  code converts.
- Our own features (e.g. [0007](0007-generated-drift-detection.md)) live in
  `codegen/internal/generator` and the templates.

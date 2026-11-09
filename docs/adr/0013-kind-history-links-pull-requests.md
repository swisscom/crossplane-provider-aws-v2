# 0013. Kind history rows link pull requests, not commits

- Status: Accepted
- Date: 2026-11-06

## Context

[ADR 0008](0008-kind-readme-and-changelog.md) gives each kind README a
history table of behavior changes with the columns date, commit, change.
In practice the commit column didn't work:

- A commit can't contain its own hash, so the row needed a follow-up commit
  or an amend, which changes the hash again. Most rows stayed empty.
- Hashes change on rebase and squash merge. Six of the recorded hashes no
  longer existed on `main` after history was rewritten.

A pull request number is known before the merge, stays the same through
rebases and squash merges, and leads to the review discussion.

## Decision

The history table has the columns date, PR, change. "PR" links the pull
request that made the change. The row is added with the change and the PR
cell is filled in once the PR is open, in the same PR. Rows from before
pull requests were used show the commit on `main`. This replaces the commit
column of ADR 0008; the rest of ADR 0008 stands.

## Consequences

- No follow-up commits or amends to record a hash.
- Until the PR is open, the PR cell of a new row is empty.
- The table template is in
  [development.md](../development.md#kind-readme).

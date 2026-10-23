# 0010. Compare semantics and the pending-change overlay

- Status: Accepted
- Date: 2026-10-16

## Context

`compare.IsSubset` (ADR 0007) sits behind every drift check. A wrong "differs"
runs Modify on every poll; a wrong "up to date" hides real drift. Review found
that list elements were matched first-fit, so the result depended on order;
that `&T{}` against nil counted as a difference; that a type mismatch made the
reconciler panic; and that AWS lowercases identifiers and maintenance windows
and reports not-yet-applied changes in `PendingModifiedValues`, both of which
caused permanent drift.

## Decision

- **Matching.** Desired list elements are paired one-to-one with observed
  ones: first fit, and when an element fits no free partner it is re-routed
  along an alternating path. If that fails too, no full pairing exists and the
  lists differ. Lists of basic values (or pointers to them) are counted
  instead, in linear time; a nil desired element takes any leftover value.
  Above 1024 non-basic elements there is no re-routing: the first element
  that fits no free partner ends the match, so the old order dependence
  remains there. AWS lists are far smaller.
- **Work budget.** One `IsSubset` call may spend about a million steps
  (`workBudget`, 2^20): every value compared, every element of a counted list,
  every candidate scanned while re-routing, and the memo table (n*n/8 for n
  elements). When it runs out the result is "differs", never "up to date", so
  hidden drift is impossible; the cost is an extra Modify. Re-routing is
  unbounded in principle (cubic in the list length, times the cost of
  comparing two elements), and lists are tenant input, so the budget is the
  real limit and the 1024 cap only bounds memory. Measured on a 4-core
  machine: the worst inputs (1024 elements that need a re-routing each, with
  300 strings per element) give up after at most about 0.2 s (0.3 s under
  `-race`); without the budget the same input took 83 s. Realistic lists use a
  small part of it: 50 tags in reverse order about 5 500 steps, 50 tags with a
  wildcard in front about 15 000, 400 reversed key/value structs about 340 000.
  How long a reversed list can be and still be matched falls with the size of
  its elements: about 700 two-field key/value structs, about 200 structs with
  20 strings each, about 100 with 50 strings, about 50 with 300. Longer or more
  awkward lists are reported as different. Key-based matching (by `Key` or
  `Name`) would be exact and linear; it should land before any kind with a
  long struct list is imported.
- **Nil and empty.** An unset desired value is not managed. A pointer to an
  empty struct matches nil (nil is the zero struct); a pointer to `""`, `false`
  or `0` does not, because an explicit value is managed. `[]` matches nil. Every
  desired map key must exist in observed, even with a nil value.
- **No panics.** Values of different types differ. Funcs are never equal.
  Arrays compare by position. Unexported fields are ignored.
- **Case.** `IdentifierEqual` and `EqualIgnoreCase` fold ASCII only. Unicode
  folding would let look-alikes such as the Kelvin sign match `k`, which in a
  provider could point a name at another resource. Maintenance windows need no
  parser: AWS validates the format and only changes the case.
- **Pending changes.** `Pending` and `PendingSlice` copy a pending value over
  the observed one, one typed call per field, before `DiffUpdatableParameters`.
  A generic overlay by field name was rejected: shared names do not always mean
  the same thing (masked `MasterUserPassword`, the `DBInstanceIdentifier`
  rename), and an automatic overlay would silently hide drift after a model
  update. Kinds list their fields explicitly and must not overlay the password,
  the identifier or the log exports.

## Consequences

- Drift detection no longer depends on list order, and empty objects in the
  spec no longer drift.
- A desired value equal to the pending one is up to date; reverting to the
  current value differs and Modify replaces the pending change.
- A map field with a nil desired value now needs its key in observed. No kind
  has a map field today.
- NaN is not equal to itself. It cannot come from YAML or JSON.
- Struct lists above 1024 elements keep first-fit matching, and a call that
  spends its budget reports "differs".
- Kinds call the helpers from their own `isUpToDate`; new pending fields
  are not overlaid until a kind adds a line for them, which costs an extra
  Modify but never hides drift.
- Extends ADR 0007.

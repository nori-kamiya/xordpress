# 0005. No plugin-hook interface in Phase 1 — two funnel functions instead

- Status: accepted
- Date: 2026-08-05

## Context

`docs/architecture.md`'s roadmap places the Wasm plugin runtime (ADR 0002) in Phase 2, but Phase
1's module boundaries must not make that retrofit painful. A blind design panel (three
independent architects, one per lens: simplest-possible, scale/perf-first, migration-risk-first)
was run for `docs/plans/phase1-mvp/design.md`. Two of the three candidates pre-emptively
introduced a plugin/event abstraction in Phase 1 with zero real implementations behind it:

- One defined a `PostHook` interface with an always-empty slice, "for Phase 2."
- The other defined a `core.Hooks` interface (`Emit`/`Filter`) with only a no-op adapter,
  explicitly admitting the shape "might not" match Extism's eventual host-function ABI.

A judge (independent critic, blind to which candidate authored which idea) scored both of these
as violating this project's stated guardrail against speculative/premature abstraction — building
an interface for a feature with zero current consumers, on a guess about an ABI that doesn't
exist yet.

## Decision

We will not introduce any hook, event-bus, or plugin-observer interface in Phase 1. Instead, the
Phase 2 attachment points are ordinary function boundaries that already have to exist for
Phase 1's own purposes: `content.{Create,Update,Delete,List,Get,ListPublished,GetPublished}` for
the post lifecycle and reads, and `theme.Render(ctx, tmplName string, data any) (string, error)`
for rendering. Phase 2 wraps these call sites with Extism dispatch when it's actually being
built, against the real `extism-go` host-function ABI, not a guessed one.

Note (2026-08-05, post-adversarial-review): an earlier draft of this ADR and of
`docs/plans/phase1-mvp/design.md` listed a standalone `content.Publish` funnel function. It was
removed during adversarial review because it had no corresponding API endpoint in Phase 1 (status
transitions happen through `Create`/`Update`) — an unreachable function is exactly the kind of
"implementation with zero consumers" this ADR argues against for hooks, and the same standard
was applied to it. The publish-timestamp logic now lives inside `Create`/`Update`, not a separate
function; see `docs/plans/phase1-mvp/design.md` §3 and §9.

## Alternatives considered

- **Pre-defined `Hooks` interface with a no-op Phase 1 implementation** (as one panel candidate
  proposed): gives Phase 2 a slightly smaller diff to land. Rejected: the interface shape is a
  guess made before Extism's actual host-function integration is designed, and per that
  candidate's own admission, might not match — meaning Phase 1 would ship an abstraction that
  gets thrown away or reshaped anyway, with no Phase 1 benefit to offset the cost.
- **Empty-slice observer list on the domain type** (the other panel candidate): same objection —
  a collection with a permanently-empty element type is dead code wearing the shape of a future
  feature.

## Consequences

- Phase 1 code stays minimal: no interface, no registration mechanism, no dispatch loop to
  maintain or test against zero real plugins.
- Phase 2 implementation must design the actual hook/dispatch shape from scratch, informed by
  Extism's real host-function and permission-manifest APIs (ADR 0002) — this is accepted as
  Phase 2 scope, not deferred risk, since guessing now was shown (via the panel) to likely produce
  the wrong shape anyway.
- The two funnel functions named above are now a soft contract: Phase 2 planning should confirm
  no Phase 1 work has bypassed them (e.g. a handler calling `storage` directly instead of going
  through `content`) before wrapping them with plugin dispatch.

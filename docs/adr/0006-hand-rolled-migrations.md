# 0006. Hand-rolled SQL migrations instead of goose/golang-migrate

- Status: accepted
- Date: 2026-08-05

## Context

Phase 1 needs a way to create and evolve the SQLite schema (`docs/plans/phase1-mvp/design.md`
§3) on top of `modernc.org/sqlite`, the pure-Go, CGO-free driver selected in ADR 0001
specifically to preserve cross-compile single-binary distribution.

Popular Go migration tools were considered:

- `pressly/goose`: mature, but adds a dependency and typically a companion CLI binary for a
  Phase 1 need that is genuinely simple — one forward-only initial schema, no rollback story
  required yet.
- `golang-migrate/migrate`: its SQLite driver implementations have historically been built
  against `mattn/go-sqlite3`, which is CGO-based. Adopting it without carefully vetting which
  underlying driver it binds to risks silently reintroducing the exact CGO dependency ADR 0001
  eliminated.

All three candidates in the Phase 1 design panel (simplest-possible, scale/perf-first,
migration-risk-first lenses) independently reached the same conclusion on this point.

## Decision

We will hand-roll migrations: numbered `NNNN_description.sql` files embedded via
`//go:embed migrations/*.sql`, applied in order inside a single transaction at startup, tracked
in a `schema_migrations(version, applied_at)` table. No external migration library dependency.

## Alternatives considered

- **`pressly/goose`**: solid tool, but its dependency + CLI footprint isn't justified by Phase
  1's actual need (one initial schema, forward-only). Kept as the explicit escape hatch if the
  project later needs down-migrations, branching schema changes, or multi-environment migration
  tooling.
- **`golang-migrate/migrate`**: same rejection as goose on footprint grounds, plus the added risk
  of accidentally pulling in a CGO-based SQLite binding through one of its driver packages —
  a risk not worth taking for ~60 lines of hand-written equivalent.

## Consequences

- Gains: zero new dependencies, no ambiguity about which underlying SQLite driver is in play,
  full control over the exact `PRAGMA`/transaction semantics used during migration (matters
  since `modernc.org/sqlite`'s DSN pragma syntax was already verified against ADR 0001's driver
  choice — see `docs/plans/phase1-mvp/design.md` §3).
- Loses: no down-migrations, no dry-run/plan mode, no migration-status CLI — acceptable for a
  single-process, forward-only Phase 1 schema history.
- If the project later needs rollback support, multi-instance coordination, or complex branching
  migrations, revisit via a superseding ADR — goose is the named default choice at that point.

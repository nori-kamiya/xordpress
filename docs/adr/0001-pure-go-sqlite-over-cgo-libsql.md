# 0001. Use modernc.org/sqlite (pure Go) instead of the official libSQL driver

- Status: accepted
- Date: 2026-08-05

## Context

XordPress's core value proposition is a single compiled binary with zero infrastructure
dependencies, distributed via cross-compiled Go builds (`GOOS`/`GOARCH` matrix, no per-platform
toolchain). `docs/architecture.md` originally specified "libSQL (Pure Go / Cgo-free)" as the
embedded database.

In practice, the official libSQL Go driver (`tursodatabase/go-libsql`) wraps libSQL's Rust core
via CGO and prebuilt platform-specific shared libraries. This is not pure Go: it requires a C
toolchain (or vendored `.so`/`.dylib`/`.dll` per target) at build time, which directly conflicts
with straightforward `go:embed` + cross-compile single-binary distribution — the project's
central technical bet.

## Decision

We will use `modernc.org/sqlite`, a pure-Go, CGO-free transpilation of SQLite, as the embedded
database driver. All schema, migrations, and query code will target standard SQLite semantics
(not libSQL-specific extensions).

## Alternatives considered

- **`tursodatabase/go-libsql` (official libSQL driver)**: gives native libSQL features (Wasm
  UDFs, embedded-replica sync to Turso) but is CGO-based, breaking cross-compilation and bloating
  the binary with platform-specific native libraries. Rejected: conflicts with the single-binary
  distribution goal, which is non-negotiable for this project's value proposition.
- **`mattn/go-sqlite3`**: mature and widely used, but also CGO-based. Same rejection reason as
  above.
- **Roll a custom SQLite-compatible storage engine**: far too costly for MVP; SQLite's on-disk
  format and query semantics are exactly what's needed, no reason to reinvent them.

## Consequences

- Gains: true cross-compile single-binary builds with no C toolchain dependency; simpler CI/CD
  and release matrix.
- Loses: libSQL-specific features assumed in the original design doc — Wasm UDFs and native
  Turso embedded-replica sync are unavailable. Any "auto sync to Turso/S3" feature (see
  `docs/architecture.md` §2, §3) must be redesigned around a separate mechanism (e.g.
  Litestream-style WAL shipping to S3, or an application-level export/import job) — tracked as a
  follow-up decision, not solved by this ADR.
- `docs/architecture.md` should be updated to say "embedded SQLite (modernc.org/sqlite, pure
  Go)" instead of "libSQL" to avoid future confusion; the product name "XordPress" and marketing
  language may still reference libSQL/SQLite interchangeably, but engineering docs should be
  precise.
- If a future need for genuine libSQL features becomes critical (e.g. built-in replica sync is a
  hard product requirement), that would be a new ADR superseding this one, likely re-accepting
  the CGO/build-complexity cost.

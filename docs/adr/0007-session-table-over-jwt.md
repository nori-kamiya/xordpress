# 0007. Server-side session table with hashed tokens, over JWT

- Status: accepted
- Date: 2026-08-05

## Context

XordPress Phase 1 needs authentication for exactly one admin user (self-hosted, single-tenant —
`docs/architecture.md` §5, `docs/plans/phase1-mvp/design.md` §4). Two shapes were on the table:
a self-contained bearer token (JWT) requiring no server-side state, or a traditional opaque
session token backed by a database table.

## Decision

We will use a server-side `sessions` table (`token_hash`, `user_id`, `expires_at`) with the
session cookie carrying an opaque random token; only the SHA-256 hash of that token is stored,
so a database dump/backup leak does not directly hand over a valid session. Password storage
uses bcrypt.

## Alternatives considered

- **JWT bearer tokens**: no server-side storage needed, slightly less code for the happy path.
  Rejected: revocation (logout, password change, "kill all sessions") requires either a
  short-lived-token + refresh-token dance or a server-side denylist anyway — at which point JWT
  has all the complexity of a session table plus a token format the project is now locked into.
  For a single-admin self-hosted product, the operational simplicity of "delete the row" beats
  JWT's stateless-scaling benefit, which doesn't apply here (there's exactly one process, one
  admin).
- **Storing the raw token in the `sessions` table**: simpler equality check on lookup. Rejected:
  a leaked database file (backup, disk image, misconfigured access) would then directly grant
  session access; hashing costs one `SHA-256` call per request and closes that off.

## Consequences

- Gains: trivial, immediate revocation (delete the row); no token-format migration risk since
  session tokens are opaque and never parsed/verified client-side; database leaks don't equal
  session compromise.
- Loses: every authenticated request requires a database lookup (acceptable at Phase 1's scale —
  single admin, low request volume); horizontal scaling to multiple XordPress processes sharing
  one session store would need the `sessions` table to live in a shared/replicated location —
  not a Phase 1 concern (single-binary, single-process by design), but noted for whenever
  multi-instance deployment is considered.
- If a future requirement demands stateless verification (e.g. a separate edge/CDN layer
  validating sessions without hitting the origin DB), that would warrant revisiting this ADR.

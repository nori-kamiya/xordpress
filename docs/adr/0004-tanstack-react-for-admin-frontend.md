# 0004. Use TanStack (React) for the admin management UI

- Status: accepted
- Date: 2026-08-05

## Context

The management UI (`docs/architecture.md` §3) is a SPA built separately with Vite and embedded
into the Go binary via `go:embed`, so its runtime bundle size matters for final binary size but
its framework choice doesn't affect server-side cross-compilation (ADR 0001, 0002 concerns don't
apply here). The original design doc left this open as "TanStack (or Svelte)".

The deciding factors are ecosystem maturity for the specific UI this admin panel needs
(data-heavy tables/forms, routing, client-side data fetching/caching for the D&D block editor and
content management screens) and fit with the developer's existing strict TypeScript conventions
(`strict`, `noUncheckedIndexedAccess`, `no-explicit-any` as error, per project profile).

## Decision

We will build the admin management UI in React using the TanStack ecosystem (TanStack Router,
TanStack Query, and TanStack Table where applicable), bundled with Vite and embedded via
`go:embed`.

## Alternatives considered

- **Svelte**: smaller runtime and smaller compiled bundle, which would help keep the final Go
  binary lean. Rejected as the primary choice: React/TanStack has deeper ecosystem support for
  the complex, data-heavy admin interactions this project needs (rich tables, forms, drag-and-
  drop block editor state management), and better alignment with the strict-TypeScript tooling
  already standard in this developer's other projects. Bundle size is a real cost but not
  severe enough to outweigh ecosystem fit for an admin panel (as opposed to public-facing,
  performance-critical pages, which are server-rendered by the theme engine, not this SPA).
- **Plain React without TanStack** (custom routing/data-fetching): more control, but reinvents
  patterns (caching, invalidation, type-safe routing) that TanStack already provides well-tested
  and type-safe.

## Consequences

- Gains: mature ecosystem for admin-panel-shaped UI problems; consistent TypeScript strictness
  posture with the rest of the developer's toolchain.
- Loses: larger embedded JS bundle than a Svelte equivalent would produce — acceptable since the
  admin UI is not on the hot path for public site rendering (which is handled server-side by the
  Liquid/AST theme engines, ADR 0003), but should be monitored if binary size becomes a stated
  product constraint (it currently is not — "20MB–30MB" in `docs/architecture.md` §6 is an
  estimate, not a hard budget).
- This decision only affects the admin SPA; it does not constrain the public-facing rendering
  path, which remains server-side Go regardless of admin frontend choice.

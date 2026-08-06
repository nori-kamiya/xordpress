# 0003. Use an existing Go Liquid library for the code-mode theme engine

- Status: accepted
- Date: 2026-08-05

## Context

Mode B of the dual theme engine (`docs/architecture.md` §4) renders designer-authored
`.liquid` templates safely — no arbitrary code execution, unlike PHP templates in WordPress
themes. This is a well-trodden problem (Shopify's Liquid, Jekyll, Ghost) with mature open-source
Go implementations already available (e.g. `osteele/liquid`).

Building a custom Liquid-compatible parser/renderer would give tighter future control — e.g.
native integration with Wasm UDFs from the DB layer, or XordPress-specific custom tags — but is
non-trivial engineering (tokenizer, tag/filter registry, safe evaluation semantics) with no
immediate product benefit over adopting an existing, tested implementation.

## Decision

We will use an existing Go Liquid library (`osteele/liquid`) for Mode B template rendering,
rather than implementing a custom Liquid engine. XordPress-specific objects/filters (e.g.
`post`, `site`, custom tags) will be registered against that library's extension points.

## Alternatives considered

- **Custom Liquid parser/renderer**: full control, deepest long-term integration potential
  (e.g. Wasm UDF-backed filters). Rejected for now: the engineering cost is large and
  front-loaded, and the MVP goal is to prove the dual-theme-engine product concept, not to own
  the templating language implementation. Revisit if `osteele/liquid`'s extension points prove
  too limiting.
- **Different templating language entirely (e.g. Go `html/template`, Handlebars-style)**:
  rejected — Liquid was chosen in the original design specifically for its non-Turing-complete,
  designer-safe semantics and market familiarity (Shopify ecosystem); switching languages loses
  that positioning.

## Consequences

- Gains: fast path to a working Mode B renderer; inherits a template dialect designers already
  know from Shopify/Jekyll experience.
- Loses: XordPress is dependent on the chosen library's maintenance state and extension API
  surface. Deep integrations envisioned in `docs/architecture.md` (e.g. Wasm UDF-backed Liquid
  filters) may require forking or contributing upstream if the library's plugin points don't
  cover them.
- Before adoption, do a light maintenance/health check on `osteele/liquid` (last commit, open
  issues, Shopify Liquid spec coverage) as part of the Phase 1 spike; if it's found lacking,
  this ADR should be revisited rather than silently worked around.

## Addendum (2026-08-05, post-adversarial-review)

The maintenance check above was performed: `osteele/liquid` is actively released (v1.8.1,
2026-02-27) and its source was read directly to confirm behavior used by the Phase 1 design.

However, `docs/plans/phase1-mvp/design.md` §6 does **not** register XordPress objects/filters
against the library's include/layout resolver as this ADR's Decision paragraph originally
assumed. Instead, Phase 1 hand-rolls a two-pass render (body template → string →
`content_for_layout` binding into a layout template via plain `Bindings`), deliberately
sidestepping the library's own layout/include machinery rather than betting on it — this was a
design choice made specifically to avoid the extension-point uncertainty this ADR flagged, not a
discovery that the library can't do it. The library's `Bindings`-based API turned out to support
the two-pass pattern natively (confirmed via its own documentation examples), so no custom tag
registration was needed for Phase 1.

This does not change the Decision (still `osteele/liquid`) but narrows what "registered against
that library's extension points" means in practice for Phase 1: only the top-level
`ParseAndRenderString`/`Bindings` API is used, not custom tags or the include resolver. Revisit
this narrowing if Phase 2 (e.g. plugin-contributed template filters) needs deeper extension-point
integration.

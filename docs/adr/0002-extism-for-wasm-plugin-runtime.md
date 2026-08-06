# 0002. Use Extism for the Wasm plugin runtime

- Status: accepted
- Date: 2026-08-05

## Context

XordPress's plugin system must let third-party code extend the CMS without being able to crash
the host process, corrupt data outside its granted scope, or make unauthorized network calls
(`docs/architecture.md` §5.2, §7 — "Sandboxed Extensibility"). Plugins need to be authorable in
multiple languages (Rust, Go, TypeScript/AssemblyScript, C/C++) and distributed as portable
`.wasm` binaries.

Building this directly on `bytecodealliance/wasmtime-go` is possible but means implementing, from
scratch: the host-function ABI for permission-gated calls (DB table access, outbound HTTP), a
plugin manifest/capability format, and per-language SDK plumbing for each supported plugin
language. That is substantial MVP-blocking work.

This ADR originally noted an open risk against ADR 0001 (pure-Go, CGO-free builds): Extism's Go
host SDK (`extism/go-sdk`) historically used Wasmtime via CGO. As of the current release,
`extism/go-sdk` is built on [`tetratelabs/wazero`](https://github.com/tetratelabs/wazero), a pure-Go
Wasm runtime with no CGO dependency — the SDK moved off Wasmtime/CGO precisely to avoid the
cross-compilation cost that motivated ADR 0001. This was verified against the SDK's own
documentation before accepting this ADR, so no conflict with ADR 0001 remains.

## Decision

We will build the plugin runtime on Extism's Go SDK (`extism/go-sdk`, wazero-backed, pure Go —
consistent with ADR 0001). Plugin capabilities (database table access, outbound HTTP allowlists,
etc.) will be expressed as Extism host functions gated by a per-plugin permission manifest stored
in XordPress's own database, granted by the site admin through the management UI.

## Alternatives considered

- **Raw `wasmtime-go`**: full control over the ABI and sandboxing, no dependency on Extism's
  design choices. Rejected for MVP: reimplementing PDKs, host-function plumbing, and a
  manifest/capability system from scratch is significant scope with no product differentiation
  — Extism already solves this well.
- **Raw `wazero`** (pure-Go Wasm runtime, no CGO, no Extism layer): would give the same CGO-free
  property this ADR ends up with via Extism, but without Extism's PDK/host-function/manifest
  conventions — same rejection reason as raw `wasmtime-go` above, just without the CGO downside.
  Not chosen because Extism's `go-sdk` already uses wazero underneath (see Context), so this
  alternative and the accepted decision converge on the same runtime engine; Extism is preferred
  for its higher-level ergonomics on top of that engine.
- **Native Go plugins (`plugin` package) or subprocess-based plugins**: rejected outright — no
  memory-safe sandboxing, defeats the "cannot crash the host" requirement that is core to the
  product's pitch versus WordPress's plugin ecosystem.

## Consequences

- Gains: multi-language plugin SDKs (PDKs) come largely for free; host-function and manifest
  patterns are already established by Extism's ecosystem, reducing MVP scope.
- Loses: XordPress's plugin capability model is constrained by what Extism's host-function
  mechanism can express; any capability XordPress needs that Extism doesn't cleanly support
  requires either a workaround or a contribution upstream.
- Commits the project to Extism's release cadence and API stability as a core dependency;
  changing Wasm runtimes later would touch every plugin-facing interface. Should Extism's Go SDK
  ever reintroduce a CGO-based engine option, that would require a superseding ADR re-evaluating
  this decision against ADR 0001.

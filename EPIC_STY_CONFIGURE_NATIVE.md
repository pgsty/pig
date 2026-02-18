# Epic: Native `sty conf/configure` in Go

## Goal
Use native Go primitives to replace shell-based `configure` orchestration while keeping migration-safe routing:

- `pig sty configure` => native path (default)
- `pig sty conf` => legacy path (default)
- `pig sty conf --native` => native path via old entrypoint

## Design Principles

1. Reuse existing infrastructure first.
- Region/network probing reuses `get.NetworkCondition()` state (`get.Region`, `get.Source`, `get.InternetAccess`).
- Runtime platform facts reuse `internal/config` (`config.GOOS`, `config.OSArch`, `config.OSType`, etc.).
- Output/error model reuses existing structured result and status-code system (`internal/output`).

2. Keep complexity minimal.
- Template mutation stays text-based for parity with legacy script behavior.
- Native path isolates side effects behind injectable hooks for deterministic tests.
- Compatibility-first route split, without forcing one-shot behavior breakage on existing `conf` users.

3. Improve testability and safety where it matters.
- Region/locale detection, stdin/stderr interaction, command lookup/execution, and local IP probing are injectable.
- Random password generation is cryptographically unbiased (no modulo bias).
- Darwin path is explicitly supported for config generation/admin-node use.

## Stories

### S1. Split `configure` into first-class command
Status: `DONE`
- Add explicit `pig sty configure`.
- Keep `pig sty conf` as independent command (not alias fallback).
- Preserve transition guard: `pig sty conf --native`.
- Add command-routing and flag-parity tests.

### S2. Native configure core (`cli/sty/configure.go`)
Status: `DONE`
- Typed options/result DTO (`ConfigureOptions`, `ConfigureData`).
- Safe mode/path normalization (`conf/<mode>.yml` under pigsty home).
- Native mutation pipeline (IP/region/proxy/PG version/locale/password generation).
- YAML validity check before final write.

### S3. Structured output and status codes
Status: `DONE`
- Add STY configure-specific status codes:
  - `CodeStyConfigureInvalidArgs`
  - `CodeStyConfigureTemplateNotFound`
  - `CodeStyConfigureFailed`
  - `CodeStyConfigureWriteFailed`
- Native configure returns structured payload for text/json/yaml formats.

### S4. Compatibility and hardening tests
Status: `IN PROGRESS`
- Existing command-route tests for legacy/native split.
- Native configure unit and integration-style tests for:
  - invalid mode/version/template paths
  - interactive/non-interactive IP resolution
  - proxy env synthesis
  - region detection hook behavior
  - Darwin/Linux preflight branches
  - password generation format and replacement
- Current `cli/sty` package coverage improved from ~53.8% to ~65.5%.
- Remaining work: golden parity tests against representative upstream templates.

### S5. Routing policy and migration
Status: `DONE`
- `configure` is native-first.
- `conf` remains legacy-first for backward compatibility.
- `conf --native` offers low-risk opt-in migration path.

## Scope Notes

- Native implementation is behavior-compatible for core configure flow, but not required to be byte-for-byte shell output compatible.
- Some legacy text-mutation behaviors are intentionally retained for minimal complexity and migration safety.
- Future iteration should add explicit golden-template allowlist for known intentional deltas.

# Pig Complexity Governance Plan

Audit snapshot: 2026-07-22.

## Objective

Reduce the total cost of understanding, changing, and validating Pig without
changing established CLI behavior. Line count is useful evidence, but it is not
the sole target: a change is worthwhile when it removes duplicated decisions,
repairs a dependency boundary, centralizes a security invariant, or makes one
behavior have one authoritative implementation.

This plan supersedes the command-file splitting advice in the 2026-02 slimming
notes. The current `AGENTS.md` requires a flat `cmd` package with one top-level
command per top-level file. New work must slim command files by moving business
logic into `cli/*` or `internal/*`, not by creating per-command directories.

## Verified Baseline (2026-07-22, not CI-enforced)

- 345 tracked files, including 309 Go files.
- 57,409 lines of production Go and 52,168 lines of Go tests.
- Since `v1.6.0`, production Go grew by about 8,245 lines and tests by about
  4,533 lines, concentrated in `inventory`, `cli/inventory`, `cli/sty`, and
  `internal/cmdb`.
- Mean cyclomatic complexity is 5.33; 39 functions are at least 20 and 6 are
  at least 30.
- A production-only duplicate scan at token threshold 60 found 55 duplicate
  occurrences covering 922 duplicated-line occurrences.
- Baseline `go vet ./...` and `go test ./... -count=1` passed.
- Lower-coverage risk areas are `cli/build` (31.0%), `cli/pgbackrest` (46.7%),
  and `cli/postgres` (48.6%).

The ignored `tmp/`, `dist/`, IDE state, and local binaries are not counted as
source complexity.

## Decision Rule

Merge two implementations only when all of the following are true:

1. They implement the same semantic decision, not merely similar syntax.
2. Their compatibility surface is the same: output bytes, error codes,
   confirmation boundary, privilege identity, file ownership, and ordering.
3. They are expected to change together.
4. The shared seam is narrower and easier to name than the duplicated code.
5. Tests can demonstrate preserved behavior.

Short local duplication is preferable to a misleading cross-package abstraction.

## Adversarial Review Record

The plan was reviewed in two planning rounds with Claude Opus 4.8 at max
effort before implementation. A third round reviewed the resulting diff and
returned `ACCEPT` with no P0 or P1 finding. Its one low-cost P2 suggestion was
applied by locking the exact log-path rejection messages in tests.

Consensus corrections to the initial audit:

- `runLegacyStructured*` is one wrapper family, not itself a duplicated state
  machine. The genuine dual execution paths are PITR, clone, and fork.
- Confirmation gates must not be derived from ANCS annotations: their current
  `confirm` values are not the same contract.
- Shell quoting, atomic writes, and log sorting contain different byte-level,
  ownership, or ordering semantics and must not be globally unified.
- Generic ANCS marshal helpers would retain all nil checks and error strings,
  so they add indirection without meaningful simplification.
- Test compression is opportunistic work accompanying a production merge, not
  an independent line-deletion project.

## Tranche 1: Narrow, Behavior-Preserving Merges

These changes are independently revertible and are the implementation scope of
this pass.

### 1. Repair config/license layering

- Remove `internal/config -> cli/license`.
- Bootstrap the license after config initialization in the `cmd` composition
  root.
- Preserve `license.HomeDir`, Viper lookup, and registration order.

### 2. Canonicalize SQL literal escaping

- Keep `internal/utils.EscapeSQLString` authoritative.
- Preserve exported PostgreSQL compatibility wrappers as one-line forwarders.
- Keep identifier quoting separate from string-literal escaping.

### 3. Centralize lexical log-file containment

- Share only basename and traversal rejection in
  `internal/utils.ResolveLogFilePath`.
- Keep PostgreSQL CSV selection, pgBackRest `.log` validation, sorting,
  privilege fallback, and streaming behavior in their original packages.

### 4. Share extension operation text fragments

- Share the rm/update package-list renderer.
- Share the add/rm/update failed-item and duration tail.
- Preserve DTOs, JSON/YAML field names, and exact text output.

### 5. Parameterize Linux PGRX path construction

- Share EL/DEB explicit and auto-detected argument construction.
- Keep their path patterns and log labels as data.
- Leave macOS discovery unchanged.

### 6. Share PostgreSQL lifecycle affected resources

- Use one affected-resource builder parameterized by the explicit `stop` or
  `restart` impact.
- Preserve resource order, detail strings, and the independent `terminate`
  connection impact.

### 7. Gated cleanup completed with the tranche

- Remove the private, zero-reference `patroniOperationError`.
- Reuse one pgBackRest side-restore stanza command builder from PITR after
  confirming byte equivalence and that every call is display/next-action
  guidance rather than execution.
- Run `go mod tidy` as a separate dependency-only change.

The near-identical CMDB `RawConfig` and `DynamicInventory` readers stay
separate. Their queries, failure codes, and domain meanings differ, while a
parameterized helper would save little and make the call sites less explicit.

### Tranche 1 result

- Production Go: net 75 lines removed after adding the shared log-path helper.
- Go tests: net 53 lines added, primarily adversarial path and exact-output
  contracts.
- Total Go: net 22 lines removed.
- The plan document itself is excluded from those figures.
- Focused affected-package tests and two full `go test ./... -count=1` runs
  passed.
- `go vet ./...`, non-style staticcheck, `go mod tidy -diff`, and
  `git diff --check` passed.
- Default staticcheck still reports 29 pre-existing `ST1005` findings in
  untouched Grafana files; they are not part of this tranche.

## Tranche 2: Contract Baselines Before Structural Moves

Do not begin these changes until the listed compatibility fixtures exist.

### Fork-list boundary

- Add golden text for `NAME PORT STATE PID AGE SOURCE COPY DATA`.
- Add JSON/YAML fixtures proving the current raw `[]ForkInfo` shape.
- Add a DBSU runtime-probe seam.
- Then move probing, row construction, and rendering from `cmd/pg.go` to
  `cli/postgres`, keeping only Cobra wiring in `cmd`.

### ANCS `OpSpec`

- Snapshot the full command tree, annotation maps, help, JSON, and YAML.
- Replace nine positional `ancsAnn` parameters with a typed value while
  preserving every annotation value.
- Remove the redundant explicit command name only through the existing
  render-time `CommandPath()` fallback.
- Keep confirmation gates independent.

### High-complexity orchestration

Split explicit phases, not rule engines, in:

- `inventory.(*pigstyValidator).visitGroup`
- `inventory.validateAnsible`
- `sty.ConfigureNative`
- `sty.mutateTemplateDocument`
- `sty.GrafanaDump`
- `inventorycli.Edit`

Each split must retain a single public entry point and name phases by domain
action: resolve, load, mutate, validate, concurrency-check, stage, and commit.

## Tranche 3: One Core Execution Path per Operation

This is the highest-value and highest-risk phase.

1. Freeze text, JSON/YAML, exit-code, stderr, confirmation, plan, recovery
   guidance, and next-action fixtures for PITR.
2. Extract one result-producing PITR core. Keep confirmation and rendering at
   the boundary.
3. Repeat for database clone and instance fork.
4. Only after those migrations, reassess legacy structured wrappers command by
   command. Do not assume raw text execution and structured failure mapping are
   equivalent.

Do not generalize or extend `captureLegacyOutput`. The end state should make it
unnecessary, but removing it is a migration outcome rather than a first-step
abstraction.

## Explicit No-Go Boundaries

- No per-command subdirectories under `cmd`.
- No global shell-quoting helper assembled from incompatible current variants.
- No merger of SQL identifier quoting and SQL literal escaping.
- No generic atomic-write wrapper across CMDB, Grafana, and DBSU paths until
  mode, ownership, symlink, TOCTOU, and durability policies are identical.
- No merger of PostgreSQL and pgBackRest log listing, sort, sudo, or glob logic.
- No merger of extension result DTOs or compatibility error-code aliases.
- No attempt to replace both `goccy/go-yaml` and `yaml.v3` with one library.
- No generic command executor spanning unrelated privilege and streaming
  contracts.
- No standalone test-line purge.

## Verification Gates

Every patch group must pass its focused package tests. Every tranche must pass:

```bash
gofmt
go test ./... -count=1
go vet ./...
go mod tidy -diff
git diff --check
```

For changes to output or command construction, add exact-string or golden
fixtures before changing the implementation. For path, privilege, file, and
shell boundaries, include adversarial cases rather than relying only on happy
paths.

## Governance After This Pass

- No new `runLegacyStructured*` call sites without an explicit compatibility
  reason.
- No new command-layer business logic; `cmd` owns Cobra definitions, flags,
  argument validation, option mapping, and shared command-boundary handling.
- No new duplicate security primitive for SQL escaping or log-file containment.
- New functions above cyclomatic complexity 30 require a documented reason or
  an explicit phase split.
- A change adding a shared abstraction must state which duplicated decision it
  removes and why the implementations must evolve together.
- PRs with substantial net growth should separate required behavior, tests,
  compatibility code, and accidental infrastructure duplication.

## Definition of Done

A tranche is complete only when source diff, focused tests, full tests, static
checks, and worktree scope are all independently verified. A passing unit test
does not claim PostgreSQL, Patroni, pgBackRest, Grafana, or CMDB runtime
integration; those remain separate validation layers when a tranche changes
their operational behavior.

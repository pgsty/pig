# AGENTS

This repository contains PIG, a PostgreSQL package manager and operations CLI. Its companion
documentation repository is [`pgsty/pig.pgsty.com`](https://github.com/pgsty/pig.pgsty.com),
normally checked out at `../pig.pgsty.com`.

Keep this file in English and `CLAUDE.md` as a relative symlink to `AGENTS.md`.

## Working Tree and Git Contents

- Start with `git status --short --branch` and inspect the relevant diff in each repository touched.
  Preserve existing work; do not reset, stash, overwrite, reformat, or include unrelated changes.
- Make the smallest compatible change. Avoid speculative cleanup and abstractions for hypothetical
  consumers. Keep source, documentation, release metadata, and deployment separately reviewable.
- Track source, configuration, and necessary lightweight documentation and metadata. Keep build
  output, executables, packages, container/VM images, downloaded archives, database dumps, and large
  datasets out of Git by default, including large text datasets.
- Before generating or downloading such files, choose a location outside the repository or add an
  appropriate ignore rule and verify the actual path with `git check-ignore`. This also prevents
  agent snapshots from capturing large artifacts.
- Before staging or committing, inspect new files' sizes and types and the staged file list. Stage
  only task files; never force-add ignored artifacts. Prefer provenance, checksums, and reproducible
  build or download instructions. Ask the user before adding uncertain binary/data/generated files
  or making an exception to the artifact policy.
- These storage rules do not authorize deleting existing files, stashes, or recovery snapshots, or
  rewriting history. Ignore rules do not remove existing Git objects; history cleanup needs its own
  explicit scope.

## Sources of Truth

| Question | Authority |
|:---|:---|
| What does this checkout do? | Current source, tests, and embedded `pig help` output |
| How should users operate PIG? | [Documentation](https://pig.pgsty.com/docs/) and root-level command references |
| Why was it designed this way? | [Design Records](https://pig.pgsty.com/design/) |
| When did behavior ship? | [Release notes](https://pig.pgsty.com/release/) and the corresponding tag/release artifacts |

Source may be ahead of a release. A design decision does not prove implementation, and a release
does not prove deployment. Report each applicable stage separately: design acceptance, local code
tests, local documentation validation, commits, push/CI, tag/release, and deployment verification.
For generated evidence, record the generating command and input revision.

Agent entry points: [documentation index](https://pig.pgsty.com/docs/index.md),
[design index](https://pig.pgsty.com/design/index.md), and [site index](https://pig.pgsty.com/llms.txt).
Reference sources live under `../pig.pgsty.com/content/`; Design Records live under
`content/blog/design/` in that repository. Public reference URLs are root-level, such as `/pg/`;
`/docs/` is the overview.

## Code Layout

- `cmd/`: Cobra names, aliases, annotations, flags, validation, help, registration, and option mapping.
- `cli/*`: command-family behavior; `internal/*`: shared foundations and adapters.
- `inventory/`: lossless Pigsty Inventory parsing and editing.
- Embedded assets, including `cli/*/assets`, are release inputs and count as product changes.

Keep `cmd/` flat: one top-level command per Go file, including large command trees, unless the owner
explicitly approves another layout. Use `cmd/pg.go` for `pg`/`postgres`, `pb.go` for
`pb`/`pgbackrest`, `pt.go` for `pt`/`patroni`, and `pe.go` for `pe`/`pg_exporter`.
Other families follow the same pattern; matching and focused test files may accompany them.

`cmd/root.go` owns root setup, global flags, and top-level registration. `cmd/utils.go` owns shared
command helpers. Reuse its annotation, confirmation, structured-output, plan, log, validation, and
legacy-adapter helpers. Put operational logic in implementation packages, including logic that
would otherwise be hidden inside Cobra closures.

Keep `README.md` as a concise entry point. Product manuals and design history belong in the
companion repository; do not create a local `docs/` tree or duplicate Markdown contracts.

## Command and Safety Contracts

- PIG-owned structured results use one clean machine-readable value on stdout. Diagnostics and
  unstructured subprocess chatter belong on stderr. Supported log snapshots are an exception:
  `-o json` emits JSONL; follow/tail output remains text-only.
- Native Patronictl forwarding preserves its command arguments, prompts, output, and exit codes.
  It rejects PIG structured-output modes; local `pig pt` helpers have their own output contracts.
- Destructive PIG-owned workflows require confirmation and fail closed when it is absent. Expose a
  non-mutating plan where meaningful; the plan and result must describe the action actually run.
- Keep primitives and orchestrators distinct: `pig pb restore` and `pig pitr` must not become aliases
  or claim the same operational scope or success conditions.
- Validate arguments before operational side effects. Reject ambiguous aliases, unsafe paths, and
  unexpected positionals; forward native arguments only across a documented boundary.
- Keep credentials, raw DSNs, Inventory values, and secret-bearing subprocess bodies out of plans,
  diagnostics, and structured results. Explicit raw-data commands such as `inventory show` retain
  their documented text-only behavior; do not treat their output as safe to log.

## Companion Documentation

Code and executable tests belong here. Current bilingual reference, design rationale, and release
history belong in the companion repository. Read its `AGENTS.md` before editing it.

Before a non-trivial product change, read the affected reference, search existing Design Records,
and check source and tests to distinguish current behavior from proposals or historical behavior:

```bash
rg -n -i 'keyword|command|subsystem' ../pig.pgsty.com/content/
```

Create or update a Design Record for material changes to:

- command grammar, aliases, flags, output schemas, or exit behavior;
- destructive-operation safety, confirmation, planning, privilege, rollback, or security/trust;
- architecture, package ownership, persistent state, wire formats, catalogs, locks, or migrations;
- compatibility, supported platforms, or a substantial rejected alternative worth preserving.

Routine catalog refreshes, typos, test refactors, mechanical cleanup, and small fixes without a new
contract normally need no Design Record. Update an existing record when the same decision evolves.
Keep raw exploration in the task, issue, or review; do not commit scratch prompts, transcripts,
review logs, or temporary plans. A useful, reviewed direction may be recorded as a Proposal without
implying a release commitment.

For material contract changes, update the Design Record and affected bilingual reference together
with the implementation. Keep release-note edits for the release gate. Link code comments to public
Design URLs only when the rationale helps explain the code. If the companion checkout is unavailable,
report the exact follow-up and target page instead of adding replacement documentation here.

### Design Records

Use `content/blog/design/<slug>.md` and `<slug>.zh.md` in the companion repository:

- Deliver English and Chinese together as public-ready content with `draft: false`.
- Use the evidence-backed decision formation date for `date` and the latest factual/status review
  for `lastmod`. Preserve `date` when status changes.
- State decision date, status, current reference, and scope in the visible header.
- Use the same explicit anchors in both languages, in this order:
  `decision`, `context`, `alternatives`, `contract`, `impact`, `verification`, `status`.
- Use one to four tags from the site's closed subsystem vocabulary.
- Link current behavior to the maintained reference. Cite fixed source commits, tags, or release
  artifacts for implementation/release claims; use issues for discussion context.
- Exclude local absolute paths, secrets, raw prompts, review transcripts, unfinished markers, and
  claims beyond the available evidence.

| Status | Required meaning |
|:---|:---|
| Proposal | Reviewed enough to publish; not accepted or implemented |
| Accepted | Owner-approved direction; implementation may be absent |
| Implemented | Verified in source with proportionate tests; not necessarily released |
| Released | Verified in a corresponding tag/release artifact |
| Superseded | Retained as history and linked to the successor |

Update status only with evidence. Run `bin/check_design.py` from the companion repository as part
of its checks. It validates metadata, bilingual pairing, anchors, and selected content rules;
factual accuracy, evidence quality, and status still require review.

## Validation

Run `git diff --check` for every change. For Go code or embedded-asset changes, run focused tests
while iterating, then:

```bash
go test ./...
go vet ./...
```

CI checks are defined in `.github/workflows/ci.yml`: dependency integrity/tidiness, shuffled tests,
command race regressions, workflow syntax, static analysis, complexity, dead code, vulnerabilities,
and a GoReleaser snapshot. Run relevant stronger checks when the change affects those areas.
For instruction-only changes, verify the referenced paths, commands, and contracts; Go tests and
site builds are not required unless executable behavior or site content also changes.

Test product behavior directly from Cobra and implementation state. Go unit tests must not read
companion Markdown or fetch the website. When companion documentation changes, run:

```bash
make docs-check
```

This delegates to the companion's `make check`: dependency verification, a warning-strict Hugo
build, Markdown checks, Design Record checks, and internal-link checks. Use `make docs-serve` for
its production-mode local preview. Both targets accept `DOCS_DIR=/path/to/pig.pgsty.com` when the
checkout is elsewhere. For layout changes, also inspect English and Chinese routes at desktop and
mobile widths; build success alone does not verify appearance.

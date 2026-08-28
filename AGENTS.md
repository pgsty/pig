# AGENTS

This repository contains the PIG product source. Its companion documentation repository is
[`pgsty/pig.pgsty.com`](https://github.com/pgsty/pig.pgsty.com), normally checked out next to this
repository as `../pig.pgsty.com`.

`CLAUDE.md` must remain a relative symbolic link to this file. Do not replace it with a copied
second instruction document: one repository has one agent contract.

## Sources of Truth

PIG deliberately separates four kinds of truth:

| Question | Authority |
|:---|:---|
| What does the current program do? | Current source, tests, and embedded `pig help` output |
| How should users operate it now? | <https://pig.pgsty.com/docs/> and the root command reference pages |
| Why was it designed this way? | <https://pig.pgsty.com/design/> |
| When did behavior actually ship? | <https://pig.pgsty.com/release/> plus the corresponding tag/release artifacts |

Do not use one surface as proof of another. A Design Record is not implementation proof; merged
code is not release proof; a release note is not production-deployment proof.

Agent-oriented entry points are:

- current documentation index: <https://pig.pgsty.com/docs/index.md>
- design index: <https://pig.pgsty.com/design/index.md>
- full site discovery: <https://pig.pgsty.com/llms.txt>
- local design sources, when the sibling checkout exists:
  `../pig.pgsty.com/content/blog/design/`

## Companion Documentation

“Companion documentation” means that a material product decision and the implementation it governs
live in separate repositories but evolve as one reviewed change. The source repository owns code
and executable tests. The documentation repository owns current bilingual reference, durable design
rationale, and release history.

### When design work begins

Before proposing a non-trivial change:

1. Read the current command/reference page for the affected subsystem.
2. Search existing Design Records before inventing a new contract:

   ```bash
   rg -n -i 'keyword|command|subsystem' ../pig.pgsty.com/content/blog/design ../pig.pgsty.com/content
   ```

3. Check the source and tests to distinguish current behavior from a historical article.
4. Decide whether the work needs a new record or an update to an existing one.

Create or materially update a Design Record when the change affects one or more of:

- public command grammar, aliases, flags, output schema, or exit behavior;
- destructive-operation safety, confirmation, planning, privilege, or rollback;
- architecture or package ownership boundaries;
- persistent files, state layout, wire formats, catalogs, locks, or migration rules;
- security, trust, credentials, redaction, or supply-chain behavior;
- compatibility policy, supported platforms, or an intentional breaking change;
- a substantial rejected alternative that a future maintainer could otherwise propose again.

Routine catalog refreshes, typo fixes, test-only refactors, mechanical cleanup, and small bug fixes
with no new contract normally do not need a new Design Record. Update current reference or release
content only when those surfaces actually changed.

### Where design thinking goes

- Raw exploration belongs in the active task, issue, or review conversation until it is coherent
  and safe to publish. Do not commit scratch prompts, model transcripts, adversarial-review logs, or
  temporary planning files into this repository.
- The first durable version belongs directly in the companion repository under
  `content/blog/design/<slug>.md` and `<slug>.zh.md`.
- A not-yet-implemented direction may be published as a **Proposal** when it is already useful,
  calibrated, and explicit that it is not a release commitment.
- Update an existing record when the same decision evolves. Do not create a second article merely
  to avoid correcting status or linking a superseding decision.

Never create a local `docs/` tree as a fallback. If the companion checkout is unavailable, report
the exact documentation follow-up and target page; keep the source change free of duplicate
Markdown contracts.

### Design Record contract

Design Records are public, bilingual, and date-addressable. Follow the contract enforced by
`../pig.pgsty.com/bin/check_design.py`:

- English `<slug>.md` and Chinese `<slug>.zh.md` are delivered together.
- Durable Design Records are public-ready and use `draft: false`; do not use the site content tree
  as a scratchpad for unpublished notes.
- `date` is the evidence-backed formation date of the decision, not the migration/publication date.
- `lastmod` records the latest factual or status review.
- The visible header states decision date, status, current reference, and scope.
- Both languages use the same explicit anchors and order:
  `decision`, `context`, `alternatives`, `contract`, `impact`, `verification`, `status`.
- Tags use the site's closed subsystem vocabulary and contain no more than four entries.
- Current behavior links to the maintained reference page; implementation evidence links to a fixed
  source commit, tag, issue, or release as appropriate.
- Public prose contains no local absolute paths, secrets, raw prompts, review transcripts, TODOs, or
  claims that exceed the observed implementation/release/deployment gate.

Use visible status language consistently:

- **Proposal** — reviewed enough to publish, but not accepted or implemented.
- **Accepted** — owner-approved direction; implementation may still be absent.
- **Implemented** — present in source with proportionate tests; not automatically released.
- **Released** — present in a verified tag/release artifact.
- **Superseded** — retained as history and linked to the successor; never silently rewritten as if
  it had always held the final conclusion.

Keep the original `date` when status changes and update `lastmod`. State transitions must follow
evidence; never change a page to Implemented or Released merely because work is planned or locally
green.

### During and after implementation

For a material contract change:

1. Update the Design Record's contract and status only as far as current evidence permits.
2. Update the current bilingual reference pages for actual user-visible behavior.
3. Keep release-note edits for the release gate; do not announce untagged work as shipped.
4. Link code comments to the public Design URL only when the rationale materially helps a reader.
5. Test executable behavior in this repository. Do not make Go unit tests read Markdown from the
   companion repository or fetch the public website.
6. Validate site source, rendering, bilingual parity, and links in the companion repository.

When the sibling checkout exists, use:

```bash
make docs-check
```

This runs the companion site's strict Hugo build and its Markdown, Design Record, and internal-link
checks. Use `make docs-serve` for a pinned-theme local preview.

Treat these as separate completion gates and report each honestly:

1. design accepted;
2. code implemented and locally tested;
3. documentation locally validated;
4. source and documentation committed;
5. pushed and CI green;
6. tagged/released;
7. deployed and verified on the public site or target environment.

## Repository Layout

- `cmd/`: Cobra command grammar and option mapping.
- `cli/*`: product behavior for command families.
- `internal/*`: shared foundations and internal adapters.
- `inventory/`: lossless Pigsty Inventory parsing/editing model.
- `cli/*/assets` and other embedded assets: release inputs; treat changes as product changes.
- `README.md`: concise project entry point, not the full manual.
- `AGENTS.md`: the single local agent contract.
- `CLAUDE.md`: relative symlink to `AGENTS.md`.

The repository must not contain a product documentation tree. Legal files, the concise README,
agent instructions, and test-fixture READMEs are not substitutes for public product documentation.

## Cobra Command Layout

- Keep `cmd` flat: one top-level command belongs in one top-level Go file under `cmd/`.
- Do not create per-command subdirectories under `cmd` for normal CLI commands. The intended shape
  is:
  - `cmd/pg.go` for `pig pg` / `pig postgres`
  - `cmd/pb.go` for `pig pb` / `pig pgbackrest`
  - `cmd/pt.go` for `pig pt` / `pig patroni`
  - `cmd/pe.go` for `pig pe` / `pig pg_exporter`
  - `cmd/sty.go`, `cmd/do.go`, `cmd/repo.go`, `cmd/inventory.go`, etc. for their matching top-level
    commands
- A command may have matching top-level test files, for example `cmd/pg_test.go` and focused guard
  tests.
- Complex commands such as `pg`, `pb`, and `pt` still stay in one command file unless the owner
  explicitly approves a different layout.
- `cmd/root.go` owns root setup, global flags, and top-level registration.
- `cmd/utils.go` owns shared command-layer helpers used by multiple command files.
- Keep `cmd` files as clean Cobra entry points: names, aliases, annotations, flags, argument
  validation, help, registration, and option mapping.
- Put concrete behavior in the relevant `cli/*`, `internal/*`, `inventory`, or another dedicated
  implementation package.
- Avoid duplicated command glue. Reuse the established annotation, structured-output, plan,
  log-output, validation, and legacy-adapter helpers.

## Command and Safety Contracts

- A PIG-owned command may expose stable structured results. Structured stdout must remain one clean
  machine-readable value; diagnostics and wrapped-tool chatter belong on stderr.
- Do not claim global structured output can transform an opaque interactive or streaming tool.
  Transparent passthroughs such as Patronictl preserve native argv, prompts, output, and exit codes.
- PIG-owned destructive workflows fail closed when confirmation is absent and expose a plan when a
  meaningful non-mutating plan exists.
- Keep primitive and orchestrated workflows distinct. For example, pgBackRest restore and the PIG
  PITR orchestrator must not be collapsed through aliases or shared success claims.
- Validate arguments before side effects. Reject ambiguous aliases, unsafe paths, and unexpected
  positionals explicitly.
- Treat secret-bearing text surfaces as such. Do not place credentials, DSNs, Inventory values, or
  subprocess response bodies into plans, diagnostics, or structured results.
- Current command behavior is tested directly from Cobra/implementation state. Documentation checks
  belong to the companion site, not product unit tests.

## Working Tree and Change Boundaries

- Start with `git status --short --branch` and inspect the relevant diff. This repository may carry
  active, unrelated work.
- Preserve user changes. Do not reset, overwrite, reformat, or include unrelated files in the task.
- Keep source, documentation, release metadata, generated artifacts, and deployment as reviewable
  boundaries even when one task touches several of them.
- Prefer the smallest compatible edit. Do not perform speculative cleanup or add abstractions for
  hypothetical consumers.
- Generated output is evidence only when the generating command and input revision are known.
- Never describe local build success as commit, push, CI, release, deployment, or production proof.

## Validation

Run focused tests while iterating, then validate in proportion to the changed contract.

Minimum source handoff:

```bash
git diff --check
go test ./...
go vet ./...
```

CI additionally verifies dependency tidiness, randomized tests, command race regressions,
GitHub Actions syntax, static analysis, complexity, dead code, vulnerabilities, and a GoReleaser
snapshot. Run the relevant stronger checks locally for changes that affect those surfaces.

Documentation handoff, when changed and the sibling checkout is present:

```bash
make docs-check
```

For layout-sensitive documentation changes, inspect real English and Chinese routes at desktop and
mobile widths. A clean diff or successful Hugo build alone is not visual proof.

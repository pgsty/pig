---
title: "pig sty"
description: "Normative design contract for Pigsty installation and IaC management"
weight: 150
icon: fas fa-server
module: [PIG]
categories: [Reference]
---

# `pig sty` Design Contract

Status: **normative, phased implementation**

Contract version: **1.0**

Last updated: **2026-07-17**

This document is the design contract for the complete `pig sty` command family.
It separates implemented behavior from planned behavior and is the authority for
subsequent implementation. A planned command is not a promise that the command
already exists; its phase gate must pass before it is advertised as stable.

The companion [implementation goal prompt](sty-implementation-goal.md) turns
this contract into a phase-gated execution objective.

Normative words such as **MUST**, **MUST NOT**, **SHOULD**, and **MAY** have their
usual RFC 2119 meaning.


## 1. Product boundary and priority

`pig sty` manages the local Pigsty installation, its active Inventory, and a
small number of directly related controller-side services. It is not a generic
replacement for Ansible, Grafana, PostgreSQL, Terraform, or Vagrant.

Implementation priority is fixed as follows:

1. **P0 — Inventory first.** Complete the Inventory query/edit contract, then
   complete Validate, then Check.
2. **P1 — Dashboard.** Replace the local Grafana helper with a safe native HTTP
   client after P0 is closed.
3. **P2 — CMDB, experimental.** Design and prove schema, revision, rollback,
   and source-switch behavior before exposing stable mutation commands.
4. **P3 — Vagrant and Terraform.** Thin adapters only, after explicit
   reprioritization or completion of the earlier gates.

The following are outside this roadmap:

- application lifecycle management;
- application Docker/Compose management, because applications may be remote;
- an unspecialized `pig sty docker` command until its target semantics receive
  a separate design;
- migrating node-side Ansible helpers or application entrypoints merely to
  reduce shell line count.


## 2. Implementation status

| Surface | Status at contract v1.0 |
|:---|:---|
| Lossless `inventory.Document`, ordered semantic model, selectors, editor, mutations, diagnostics | Implemented baseline |
| `sty conf/configure` through Inventory API, with `--raw` fallback | Implemented |
| `sty edit [selector]` | Implemented baseline |
| `sty validate [file]` | Implemented baseline; rule parity incomplete |
| `sty inventory list/show/edit/validate` namespace | Planned P0.1/P0.2 |
| `sty check` | Planned P0.3 |
| `sty dashboard` | Planned P1 |
| `sty cmdb` | Planned P2, experimental until all gates pass |
| `sty vagrant`, `sty terraform` | Deferred P3 |
| `sty doctor` | Name reserved; implementation deferred |


## 3. Command taxonomy and compatibility

### 3.1 Pig 1.x transition contract

Existing 1.x meanings remain compatible while explicit namespaces are added:

```text
pig sty
  init | i
  boot | b | bootstrap
  conf | c | configure
  deploy | d | de | install | ins

  list | l | ls                       # existing release listing in 1.x
  release
    list | ls                         # canonical release listing
    get                               # canonical release download
  get | g | download                  # delegate to `release get`

  inventory | inv
    list [selector]                   # ordered, redacted topology/metadata
    show [selector]                   # verbatim YAML; text mode only
    edit [selector]                   # canonical full/fragment editor
    validate [file]                   # canonical Inventory validator

  edit | e [selector]                 # delegate to `inventory edit`
  validate | val | v [file]           # delegate to `inventory validate`
  check | ck

  dashboard | ds
    list | check | diff | init | dump | clean

  cmdb
    init | load | dump | enable | disable | check

  doctor | dc                         # reserved, not implemented in P0-P2
  vagrant | vg                        # deferred P3
  terraform | tf                      # deferred P3
```

When `sty release list` lands, the 1.x `sty list/l/ls` release path MUST emit one
deprecation notice on stderr in text mode. It MUST preserve stdout and MUST NOT
emit that notice into JSON/YAML structured output. `release get` and the
top-level `get/g/download` delegate MUST accept the same flags and produce the
same result.

Canonical and convenience spellings MUST share one business implementation.
They MUST NOT drift into duplicate code paths.

### 3.2 Pig 2.0 command promise

Pig 2.0 will make the high-frequency Inventory meaning canonical:

```text
pig sty list | l | ls [selector]      # delegate to `inventory list`
pig sty release list | ls             # only release-list spelling
pig sty release get                   # canonical release download
pig sty get | g | download            # compatibility delegate
```

No 1.x release may silently repurpose `sty list`. The 2.0 change requires the
full 1.x deprecation window described above.

### 3.3 Reserved names

`inventory diff`, `doctor`, and broad Dashboard deletion are reserved but not
implemented by this contract. Reserving a name does not authorize speculative
code.


## 4. Layering and code ownership

Repository layout MUST follow `AGENTS.md`:

- `cmd/sty.go` contains only Cobra commands, aliases, annotations, flags,
  argument validation, and option mapping. The command package stays flat.
- `cli/sty` owns use cases and stable command DTOs.
- the top-level `inventory` package owns static Inventory parsing, the lossless
  document, semantic values, selectors, mutations, and validation primitives.
- focused internal adapters MAY own Grafana HTTP, Ansible subprocess, CMDB SQL,
  or external executable integration.

Do not build a universal environment-driver framework. Share only contracts
that have at least two concrete consumers. `Finding` and output/plan helpers are
valid shared concepts; Check probes remain private to Check until another real
consumer exists.


## 5. Inventory model and public behavior

### 5.1 Two-layer model

Static YAML uses two deliberately separate layers:

- **`Document`** is the only lossless write source. It retains original bytes,
  node spans, comments, scalar spelling, padding, indentation, flow/block style,
  line endings, and document boundaries.
- **`Inventory` / `Snapshot`** is an ordered semantic view of declared groups,
  hosts, children, vars, and values. Mapping entries use ordered slices rather
  than unordered Go maps.

No supported write path marshals the entire semantic model or YAML tree back to
the file. Existing scalar updates replace only their source lexeme; insertion,
deletion, rename, uncomment, and fragment replacement use minimal source
patches.

Unknown tags, anchors, aliases, and vault values MUST round-trip unchanged.
Direct edits that cannot be made losslessly MUST fail rather than normalize the
document.

### 5.2 Declaration scope, not effective vars

P0 models source declarations. It MUST NOT pretend to implement:

- Ansible multi-group precedence or `ansible_group_priority`;
- role defaults or external `group_vars/host_vars` directories;
- vault decryption;
- dynamic or directory Inventory;
- a final effective-vars resolver.

Rules MAY read declarations across scopes for precedence-free structural checks
such as uniqueness, duplicate membership, union-based references, or port
collisions. Rules MUST NOT compute override outcomes or report an effective
value as absent when inheritance may provide it. A future effective-vars
resolver is a separate design and must demonstrate parity with Ansible.

### 5.3 Inventory API direction

The shared package supplies typed paths and ordered access. Use cases SHOULD be
expressed through narrow helpers rather than raw text replacement, including:

- global vars: `all.vars`;
- group vars: `all.children.<group>.vars`;
- host vars: `all.children.<group>.hosts.<host>`;
- nested mapping/sequence values;
- set/upsert/delete/rename with optional leading comments;
- exact placeholder replacement across keys, active scalar lexemes, and comment
  tokens when explicitly requested by a template workflow;
- atomic mutation sessions with redacted change sets.

Comments are presentation data owned by `Document`, not semantic values. An API
that accepts a comment MUST define whether it is a leading standalone comment
or an inline comment and MUST preserve surrounding user comments.

### 5.4 `inventory list` and `inventory show`

`inventory list [selector]` is safe for automation. It emits ordered paths,
object kinds, group/host topology, variable keys, and value kinds; it does not
emit arbitrary variable values. JSON/YAML output uses a versioned DTO.

`inventory show [selector]` is a `cat`-equivalent view:

- no selector prints the original complete document;
- a selector prints the exact/dedented YAML fragment defined by fragment-edit
  semantics;
- output is verbatim and may contain passwords or other secrets;
- the command is text-only and MUST reject JSON/YAML structured mode with a
  parameter-class error;
- the body MUST NOT be copied to logs, diagnostics, or a structured Result.

A future `show --redact` MAY be additive, but redacted output MUST NOT be called
lossless.


## 6. Selector contract

The existing selector grammar is stable:

- empty selector or `/` selects the whole document;
- `vars` means `/all/vars`;
- a leading name such as `pg-meta` means `/all/children/pg-meta`;
- under a group, `vars` and `hosts` are reserved container shorthands;
- other group segments omit `children`, so `parent.child.vars` resolves nested
  groups;
- `[0]` addresses a sequence index;
- `["quoted.key"]` addresses a key containing dots, brackets, IP punctuation,
  quotes, or other ambiguous characters;
- `/all/children/...` is the canonical JSON-Pointer-style spelling, including
  `~0` and `~1` unescaping.

Examples:

```text
vars.infra_portal
pg-meta
pg-meta.vars.pg_databases
pg-meta.vars.pg_databases[0]
pg-meta.hosts["10.10.10.10"]
/all/children/pg-meta/hosts/10.10.10.10
```

Ambiguous shorthand MUST fail; callers use the canonical pointer when a literal
key collides with shorthand resolution. Stable selector failures are:

- `SELECTOR_SYNTAX`;
- `SELECTOR_NOT_FOUND`;
- `SELECTOR_TYPE`.

IP-key, dotted-key, quoted-key, nested-group, index, and pointer escape fixtures
are normative compatibility tests.


## 7. Source resolution

`-H/--home` resolves Pigsty home first. `-i/--inventory` always names a concrete
static YAML source for one invocation; it may live outside Pigsty home and never
persists a CMDB source switch.

When `-i` is absent, the target resolution order is:

1. `PIGSTY_INVENTORY`;
2. `inventory` in `~/.pig/config.yml`;
3. the exact Inventory directive deliberately parsed from
   `<resolved-pigsty-home>/ansible.cfg`;
4. `<resolved-pigsty-home>/pigsty.yml`.

Until CMDB support lands, the existing static resolution remains valid. Once
CMDB exists, the `ansible.cfg` Inventory directive is the authoritative
persistent Ansible source. Pig reads it deliberately; it never infers CMDB mode
from database reachability or incidental files. Unsupported dynamic/directory
sources fail with an explicit diagnostic instead of silently falling back.

Lossless commands (`show`, static fragment `edit`) require a static `Document`.
Semantic commands (`list`, `validate`, `check`) consume a `Snapshot` and may
later use a CMDB adapter.


## 8. Mutation, revision, and file safety

For a static file, revision means the exact original bytes, represented by a
content hash or an equivalent byte comparison captured at read time.

Every edit MUST:

1. parse and validate the original source;
2. stage non-overlapping source changes transactionally;
3. reparse and validate the candidate;
4. re-read the destination immediately before commit and reject a revision
   mismatch;
5. write a sibling temporary file, flush it, rename atomically, and sync the
   parent directory when supported.

On conflict or validation failure, the destination remains untouched and no
partial transaction is exposed.

Additional requirements:

- symlink Inventory destinations are refused;
- editor directories are mode `0700`; fragments and candidates are `0600`;
- a new committed Inventory defaults to `0600`; an existing Inventory keeps any
  stricter owner-only mode, while Pig removes group/other access and never
  loosens permissions;
- a mode change produces one human notice;
- replacement ownership follows the writing process and this is documented;
- a failed interactive edit retains the private recovery fragment, prints its
  path, and leaves deletion to the user; it is never uploaded or logged;
- change sets contain identifiers and operation kinds, never old/new secret
  values.

Scope and consent are distinct. Flags such as `--prune` or `--reset` select
scope but never imply confirmation. Destructive operations require an
interactive prompt or explicit `--yes`; a non-TTY invocation without `--yes`
fails with the confirmation-required status class.


## 9. Output, status, and redaction

### 9.1 Structured output

New Sty DTOs reuse the repository-wide Result envelope with `api: 1`. Every new
structured data payload MUST also carry a stable `kind`, for example:

```text
pig.sty.release.list/v1
pig.sty.inventory.list/v1
pig.sty.inventory.validate/v1
pig.sty.check/v1
pig.sty.dashboard.diff/v1
```

The envelope API versions envelope shape. Payload `kind` prevents a consumer
from interpreting 2.0 Inventory-list data as the old release-list payload.
Breaking DTO changes require a new payload kind/version and, where the envelope
itself changes, a new envelope API version.

### 9.2 Status and shell exit contract

Sty uses the existing MMCCNN status-code scheme and global shell mapping:

| Result category | Shell exit |
|:---|---:|
| success | 0 |
| operation/internal | 1 |
| parameter/usage | 2 |
| permission | 3 |
| dependency | 4 |
| network | 5 |
| resource | 6 |
| confirmation required | 7 |
| configuration/validation | 8 |
| state/conflict | 9 |

Composite Check output contains one deduplicated finding set. Command setup or
argument errors fail immediately. For completed probes, the selected failure is
deterministic: highest finding severity, then fixed stage order
`validate → local → ansible → network → ssh → privilege`, then stable diagnostic
code order. Its MMCCNN code determines the shell exit.

`--strict` changes the command failure threshold. It MUST NOT rewrite a stored
warning into an error diagnostic.

### 9.3 Redaction matrix

| Operation | Human stdout/editor | Structured output | Logs/errors |
|:---|:---|:---|:---|
| `inventory list` | topology/keys/kinds only | same safe metadata | no arbitrary values |
| `inventory show` | verbatim, secret-bearing | refused | body never copied |
| `inventory edit` | selected private fragment | refused | identifiers only |
| `validate`, `check` | redacted diagnostics | redacted diagnostics | redacted diagnostics |
| `configure` | identifiers and output path | identifiers/change kinds | no generated passwords |
| Dashboard | UID/title/status, no credentials | same | sanitized HTTP errors |
| CMDB | counts, identifiers, revision, plan | same | no variable payloads |

Redaction uses a central, tested secret classifier for known password, secret,
token, API key, private-key, and credential fields. Safer surfaces avoid emitting
arbitrary values at all. External adapters discard stdout that may contain
Inventory secrets and sanitize stderr before creating diagnostics.


## 10. Validate contract (P0.2)

`validate` is deterministic source-configuration analysis. Its default engine
is native Go. `--ansible` is an optional compatibility adapter; absence of
Ansible is reported explicitly and does not change native rule semantics.

Validation layers are:

1. YAML document and structural safety;
2. Ansible static YAML Inventory structure;
3. Pigsty declaration rules.

Pigsty rules include declared global identity, group/host shape, module entity
identity, sequence type/range/uniqueness, `pg_role`, cluster naming, explicitly
declared users, databases, services, HBA, VIP, and other checks covered by the
pinned legacy validator.

Parity is falsifiable:

- P0.2 pins the reviewed legacy `bin/validate` source beside test fixtures;
- representative inventories run through both validators;
- every intentional mismatch is recorded in a divergence registry with stable
  rule ID, legacy result, native result, and rationale;
- known legacy defects are fixed and registered, not copied silently.

Vault/tagged values remain opaque. A rule needing plaintext emits a stable
`UNVERIFIABLE` informational diagnostic rather than a false error.


## 11. Check contract (P0.3)

`check` answers whether the controller and selected targets appear ready for a
Pigsty operation. It is not another validator and never mutates or auto-fixes.

Default `check` is deterministic and local:

- resolve and validate the active semantic Inventory;
- verify Pigsty home/project layout and required local files;
- inspect required local executable availability and versions;
- perform no network, SSH, privilege, or package mutation.

Additional profiles are explicit:

- `--profile ansible`: run bounded `ansible-inventory --list`, discard stdout,
  and sanitize stderr;
- `--profile network`: bounded TCP reachability checks;
- `--profile ssh`: allowlisted `ssh` command ending in remote `true`;
- `--sudo`: separately opt in to allowlisted `sudo -n true` over SSH.

Each probe has an individual timeout and a documented command allowlist. Remote
probes are observational, not side-effect-free: authentication logs, alerting,
and lockout counters may change. Findings are deduplicated by stable code, path,
and target.

`doctor` remains deferred. If implemented later, it collects post-deployment
diagnostics and a redacted support bundle; it does not become a second Check
engine and receives no repair authority from this contract.


## 12. Dashboard contract (P1)

Dashboard management uses a native Grafana HTTP adapter. It MUST NOT shell out
to `files/grafana/grafana.py`.

Initial verbs are:

- `list`: list repository-managed and live status;
- `check`: credentials/API compatibility and manifest integrity;
- `diff`: normalized repository-versus-live differences;
- `init`: idempotent upsert of active managed dashboards/data sources;
- `dump`: export dashboards through atomic staging;
- `clean`: remove repository-managed resources only.

### 12.1 Ownership

A versioned repository manifest is authoritative:

- a live dashboard is currently managed iff its UID is in the active UID set;
- folder is placement, never ownership;
- a copied dashboard with a new UID is unmanaged and untouched;
- a moved dashboard with the same UID remains managed;
- a folder is deleted only after it contains no unmanaged resources.

The manifest also carries removed-UID tombstones. UIDs MUST NOT be reused.
`init --prune` may delete only live UIDs in that tombstone lineage. Without
tombstones, a resource removed from the current manifest would become
indistinguishable from user content and could never be pruned safely.

Initial P1 does not support broad deletion of unmanaged Grafana resources.

### 12.2 Apply, dump, and diff

`init` applies active manifest resources without a global clean. `--prune`
shows a deletion plan and requires confirmation/`--yes`.

Grafana does not return datasource secure fields. Therefore:

- `dump` is not a datasource backup/restore mechanism;
- datasource credentials are never dumped;
- `init` reinjects secrets from Inventory, environment, or explicit protected
  input;
- empty/redacted dump fields MUST NOT overwrite live credentials.

Grafana mutates volatile fields such as IDs, versions, iteration values, and
defaults. `diff` uses a versioned normalization fixture; naive raw JSON diff is
not supported.

Provisioned/read-only resources receive explicit per-object results. A batch
MUST NOT report silent success after partial failure. Tests use both an HTTP fake
and a disposable real Grafana container.


## 13. CMDB contract (P2, experimental)

CMDB does not start with code. Its entry artifact is a separate RFC covering:

- versioned, idempotent schema migrations;
- semantic Snapshot and revision representation;
- source provenance and optimistic concurrency;
- load/dump diff semantics;
- rollback and backups;
- active-source split-brain detection;
- containerized destructive/conflict tests.

The native implementation MUST NEVER execute these legacy destructive helpers:

- `files/cmdb.sql`;
- `bin/inventory_load`;
- `bin/inventory_cmdb`;
- `bin/inventory_conf`.

Target verbs have these minimum semantics:

- `init`: apply idempotent migrations; drop/reset requires `--reset` and
  confirmation;
- `load`: validate, plan, lock, revision-check, snapshot, and apply one database
  transaction; deletion additionally requires `--prune` and confirmation;
- `dump`: emit canonical generated YAML with a machine-readable marker;
- `dump --merge`: perform a provenance-sensitive three-way merge using the
  recorded base, current static Document, and CMDB change; base mismatch is a
  conflict, never a silent two-way overwrite;
- `enable/disable`: back up and atomically patch only one unambiguous
  `inventory =` directive in `[defaults]` of `ansible.cfg`;
- `check`: verify schema version, connectivity, active-source shape, revision,
  and semantic parity.

Missing, duplicated, templated, or ambiguous `ansible.cfg` directives fail
closed with manual instructions. This is a narrow patch contract, not a general
lossless INI library.

If an invocation-specific static source shadows a persistent CMDB source (or
vice versa), semantic commands emit a one-line stderr split-brain notice. The P2
RFC defines exact revision storage and three-way merge algorithms before code.

Plain CMDB dump has canonical generated style only. It MUST NOT claim to
reconstruct comments, alignment, or scalar lexemes that were never stored.


## 14. Vagrant and Terraform contract (P3)

P3 commands are thin adapters around the native executables and state engines.
They may expose status/plan/up/down/destroy/output/ssh-config operations, but:

- Pig MUST NOT reimplement provider or state semantics;
- native exit status and useful stderr remain observable after sanitization;
- destroy requires a plan and confirmation;
- SSH configuration writes require an explicit destination, backup, and
  confirmation;
- hard-coded demo passwords are not migrated into Go defaults.

No P3 code lands merely because a Make target exists.


## 15. Script and Makefile migration

Migration uses compatibility wrappers rather than an immediate deletion:

| Existing surface | Target command | Earliest phase |
|:---|:---|:---|
| `configure` / configure-related Make target | `pig sty conf` | already available |
| `make validate`, `bin/validate` | `pig sty validate` | P0.2 parity gate |
| `dashboard-init` | `pig sty dashboard init` | P1 |
| `dashboard-dump` | `pig sty dashboard dump` | P1 |
| `dashboard-clean` | `pig sty dashboard clean` | P1 |
| `make cmdb` and Inventory CMDB helpers | explicit `sty cmdb` sequence | P2 only |
| Vagrant/Terraform Make targets | thin P3 commands | P3 only |

An old wrapper remains for at least one released deprecation window and until
its parity fixtures have no unresolved regression. Its removal is a separate,
reviewed change.


## 16. Phase gates and acceptance

### G0 — Design contract

This document freezes taxonomy, aliases, selectors, output/exit behavior,
redaction, source resolution, mutation safety, priorities, and non-goals.

### P0.1 — Inventory completion

Deliver list/show services and DTOs, namespace/delegates, comment-aware editing,
revision and mode notices, and source-path behavior.

Acceptance:

- parse/no-op is byte-for-byte identical over the portable fixture set and the
  adjacent Pigsty `conf/**/*.yml` corpus;
- scalar/source-patch tests prove untouched bytes remain identical;
- selector tests cover dotted/IP keys, indexes, nested groups, and pointers;
- conflict tests write nothing and retain a private recovery fragment;
- symlink refusal, 0600 mode, tighten-notice, and structured-show refusal pass;
- canonical/delegate commands call the same use case.

P0.2 MUST NOT begin before this gate closes.

### P0.2 — Validate completion

Deliver declaration-scope rule parity, pinned legacy fixtures, the divergence
registry, vault opacity, and optional Ansible compatibility.

Acceptance:

- every reference-corpus mismatch is fixed or registered;
- rule IDs and diagnostic paths/ranges are stable;
- warnings and `--strict` threshold behavior are tested;
- vault/tag fixtures produce `UNVERIFIABLE`, not false failures;
- current Pigsty config corpus parses and validates to the documented baseline.

### P0.3 — Check

Deliver deterministic local Check and explicit opt-in profiles.

Acceptance:

- default tests prove no network I/O;
- allowlists and per-probe timeouts are enforced;
- findings deduplicate deterministically;
- mixed-result status selection follows section 9.2;
- disposable targets cover network/SSH behavior without persistent mutation.

### P1 — Dashboard

Acceptance includes manifest/tombstone validation, copied-dashboard immunity,
folder-empty rules, normalized diff fixtures, dump secret absence, per-object
partial-failure reporting, an HTTP fake, and a disposable Grafana compatibility
run.

### P2 — CMDB

The RFC is mandatory before implementation. PostgreSQL-container tests must
prove migration idempotence, transaction rollback, destructive confirmation,
revision conflict, three-way base mismatch, and fail-closed source switching.
CMDB remains experimental until every gate passes.

### P3 — Vagrant/Terraform

Only native-CLI passthrough, plan rendering, exit preservation, and destructive
confirmation are accepted. P3 requires explicit reprioritization.

Every phase runs proportional package tests plus `go test ./...`, `go vet ./...`,
and `git diff --check`. Inventory and concurrent adapters additionally run the
relevant race tests.


## 17. Adversarial review record

The contract was reviewed in two rounds in one Claude Code session:

- Claude Code: `2.1.207`
- model: `claude-fable-5` (`fable` alias), effort `max`
- session: `39e24edc-08d9-46ce-a599-54aec12f8dab`
- Llama 3 70B was not exposed by the installed Claude Code, so the available
  Fable model was used explicitly.

Round 1 rejected the draft contract while accepting its architecture. Its
required corrections covered secret-bearing `show`, stable selector/output
contracts, static revision semantics, validation divergence tracking, Check
probe boundaries, Dashboard ownership/dump limitations, and CMDB provenance.

Round 2 retested those findings against repository evidence. It withdrew the
proposal to compute effective vars, accepted declaration-scope validation,
accepted the explicit 1.x-to-2.0 `list` migration, added Dashboard removed-UID
tombstones and structured payload kinds, and returned the verdict
**CONVERGED**.


## 18. Current stable workflow

The currently implemented installation path remains:

```bash
pig sty init                     # install Pigsty to ~/pigsty
pig sty boot                     # install controller prerequisites
pig sty conf                     # native Inventory-aware configure
pig sty conf --raw               # legacy configure fallback
pig sty edit [selector]          # edit full Inventory or one fragment
pig sty validate [file]          # native baseline validation
pig sty deploy                   # run deploy.yml/install.yml
pig sty list                     # list Pigsty releases in 1.x
pig sty get [version]            # download a Pigsty release
```

`sty configure` remains an alias of `sty conf`. Generic templates may apply
explicit PostgreSQL version/region/proxy/locale/password options through the
Inventory mutation API; version-pinned templates retain their pinned kernel
semantics. `--raw` remains the rollback path during native stabilization.

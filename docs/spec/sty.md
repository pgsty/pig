---
title: "pig sty"
description: "Normative design contract for Pigsty installation and IaC management"
weight: 150
icon: fas fa-server
module: [PIG]
categories: [Reference]
---

# `pig sty` Design Contract

Status: **normative; Inventory/CMDB migration and native Grafana management implemented**

Contract version: **1.4**

Last updated: **2026-07-18**

This document is the design contract for the `pig sty` installation and
controller command family. It also retains the shared lossless Inventory model,
selector, validation, status, and redaction contracts used by Sty operations.

The top-level [`pig inventory` contract](inventory.md) is the authority for the
public Inventory command tree, the thin adapter to Pigsty's existing CMDB, and
migration from older Sty paths. A planned command is not a promise that the
command already exists; its migration gate must pass before it is advertised
as stable.

The former [implementation goal prompt](sty-implementation-goal.md) records the
historical P0–P2 execution objective and is archival; its greenfield CMDB phase
is explicitly superseded. New root-command work MUST follow the delivery plan
and gates in
[`inventory.md`](inventory.md#11-delivery-plan-and-explicit-non-goals).

Normative words such as **MUST**, **MUST NOT**, **SHOULD**, and **MAY** have their
usual RFC 2119 meaning.


## 1. Product boundary and priority

`pig sty` manages the local Pigsty installation and a small number of directly
related controller-side services. Configure, Check, and Grafana management
consume the active Inventory, but generic Inventory operations belong to the root
`pig inventory|inv` domain. Sty is not a generic replacement for Ansible,
Grafana, PostgreSQL, Terraform, or Vagrant.

Implementation priority is fixed as follows:

1. **P0 — Inventory first.** The lossless Inventory, root command, Validate,
   and Check gates passed.
2. **P1 — Grafana dashboards.** The native HTTP implementation remains under
   `sty`.
3. **P2 — CMDB adapter.** The former revision-store direction was withdrawn;
   the implemented surface is a thin adapter to existing `cmdb.sql` and the
   Inventory scripts.
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

| Surface | Status at contract v1.4 |
|:---|:---|
| Lossless `inventory.Document`, ordered semantic model, selectors, editor, mutations, diagnostics | Implemented baseline |
| `sty conf/configure` through Inventory API, with `--raw` fallback | Implemented |
| `sty edit [selector]` | Implemented |
| `sty validate [file]` | Implemented |
| root `inventory|inv` | Implemented through the v1.3 P0-P2 contract |
| `sty check` | Implemented |
| `sty grafana|gf info/list/boot/load/init/dump/clean/lang/style` | Implemented |
| root `inventory cmdb` thin adapter | Implemented with Check, plans, confirmation, dump, and source guards |
| existing Pigsty `cmdb.sql` and Inventory scripts | Current CMDB authority |
| former `pigsty_cmdb` revision-store surface | Removed |
| `sty vagrant`, `sty terraform` | Deferred P3 |
| `sty doctor` | Name reserved; implementation deferred |


## 3. Command taxonomy and compatibility

### 3.1 Sty stable and compatibility surface

Sty retains installation/controller commands and the two released compatibility
delegates while Inventory becomes a root domain:

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

  edit | e [selector]                 # compatibility Inventory delegate
  validate | val | v [file]           # compatibility Inventory delegate
  check | ck

  grafana | gf
    info | list | boot
    init [path] | load [path] | dump [path] | clean [path]
    lang <language-code>
    style <dark|light|auto>

  doctor | dc                         # reserved, not implemented in P0-P2
  vagrant | vg                        # deferred P3
  terraform | tf                      # deferred P3
```

The `sty list/l/ls` release path emits at most one deprecation notice on stderr
in text mode. It preserves stdout and never emits that notice into JSON/YAML
structured output. `release get` and the top-level `get/g/download` delegate
accept the same flags and produce the same result.

Canonical and convenience spellings MUST share one business implementation.
They MUST NOT drift into duplicate code paths.

### 3.2 Root Inventory migration promise

The canonical Inventory spelling is now outside Sty:

```text
pig inventory | inv status
pig inventory | inv list | ls [selector] [--depth N]
pig inventory | inv show [selector]
pig inventory | inv edit | e [selector] [--from FILE|-]
pig inventory | inv validate | v [file] [--strict] [--ansible] [--timeout DURATION]
pig inventory | inv diff <other-file>
pig inventory | inv cmdb
  check [file] [-d DATABASE]
  init [-d DATABASE] [--plan] [--yes|-y]
  load [file] [-d DATABASE] [--plan] [--yes|-y] [--strict]
  dump [file] [-d DATABASE] [--force|-f]
  enable [-d DATABASE] [--plan]
  disable [--plan]
```

Pig v1.6.0 released only `sty edit/validate`. They become deprecated delegates
after their canonical root paths ship. The unreleased `sty inventory` and
`sty cmdb` namespaces are removed before the next release rather than becoming
permanent aliases. The exact mapping, payload-kind freeze, deprecation behavior,
and phased removal are defined by
[`inventory.md`](inventory.md#9-compatibility-and-release-contract).

`sty list/l/ls` MUST NEVER be repurposed to mean Inventory list, including in
Pig 2.0. It remains a release-list delegate until it is hidden and eventually
removed in a reviewed major-version change. `pig inv ls` provides the short
Inventory spelling without changing an existing command's meaning.

### 3.3 Reserved names

`doctor` is reserved but not implemented by this contract. Instance-wide
Grafana deletion is available only through the explicit
`pig sty grafana clean --all` mode.


## 4. Layering and code ownership

Repository layout MUST follow `AGENTS.md`:

- `cmd/sty.go` contains only Cobra commands, aliases, annotations, flags,
  argument validation, and option mapping for Sty and its compatibility
  delegates. The command package stays flat.
- `cmd/inventory.go` owns the canonical root Inventory command glue; root
  registration remains in `cmd/root.go`.
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
- **`Inventory` / `Value`** is an ordered semantic view of declared groups,
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

### 5.4 `pig inventory list` and `pig inventory show`

`pig inventory list [selector]` is safe for automation. It emits ordered paths,
object kinds, group/host topology, variable keys, and value kinds; it does not
emit arbitrary variable values. JSON/YAML output uses a versioned DTO.

For a static source, `pig inventory show [selector]` is a `cat`-equivalent view:

- no selector prints the original complete document;
- a selector prints the exact/dedented YAML fragment defined by fragment-edit
  semantics;
- output is verbatim and may contain passwords or other secrets;
- the command is text-only and MUST reject JSON/YAML structured mode with a
  parameter-class error;
- the body MUST NOT be copied to logs, diagnostics, or a structured Result.

A managed CMDB source has no original presentation bytes, so root `list` and
`show` do not pretend to support it. Canonical generated export is the explicit
`pig inventory cmdb dump` operation. A future `show --redact` MAY be additive,
but redacted output MUST NOT be called lossless.


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

The `ansible.cfg` Inventory directive is the authoritative persistent Ansible
source. Pig reads it deliberately; it never infers CMDB mode from database
reachability or incidental files. Unsupported dynamic/directory sources fail
with an explicit diagnostic instead of silently falling back. The value-free
source descriptor, exact relative-path bases, and local-only `inventory status`
contract are frozen in `inventory.md`.

Lossless and declaration-semantic operations currently require one static
`Document`. The known CMDB adapter is limited to the explicit `inventory cmdb`
commands. Database-generated YAML never claims lossless source presentation.
No generic Snapshot/source backend is part of the current contract.


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

Pig IaC DTOs reuse the repository-wide Result envelope with `api: 1`. Every new
structured data payload MUST also carry a stable `kind`, for example:

```text
pig.sty.release.list/v1
pig.sty.inventory.validate/v1
pig.sty.check/v1
pig.sty.grafana.info/v1
pig.sty.grafana.list/v1
pig.sty.grafana.boot/v1
pig.sty.grafana.load/v1
pig.sty.grafana.init/v1
pig.sty.grafana.dump/v1
pig.sty.grafana.clean/v1
pig.sty.grafana.lang/v1
pig.sty.grafana.style/v1
```

The envelope API versions envelope shape. The released Sty validation delegate
retains its v1 payload during the compatibility window; new root Inventory
commands use `pig.inventory.*` kinds defined by `inventory.md`. Breaking DTO
changes require a new payload kind/version and, where the envelope itself
changes, a new envelope API version.

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
| interactive `inventory edit` | selected private fragment | refused | identifiers only |
| `inventory edit --from` | no fragment on stdout | identifiers/change kinds | no source bytes |
| `validate`, `check` | redacted diagnostics | redacted diagnostics | redacted diagnostics |
| `configure` | identifiers and output path | identifiers/change kinds | no generated passwords |
| Grafana | UID/folder/action/status, no credentials | same | sanitized HTTP errors |
| CMDB | operation, row/object counts, output path | same safe metadata | no variable, SQL, or DSN payloads |

Redaction uses a central, tested secret classifier for known password, secret,
token, API key, private-key, and credential fields. Safer surfaces avoid emitting
arbitrary values at all. External adapters discard stdout that may contain
Inventory secrets and sanitize stderr before creating diagnostics.


## 10. Validate contract (P0.2)

`pig inventory validate|v` is the canonical public path after its migration
gate. `pig sty validate|val|v` remains a delegate to the same use case during
its compatibility window.

`validate` is deterministic source-configuration analysis. Its default engine
is native Go. `--ansible` is an optional compatibility adapter; absence of
Ansible is reported explicitly and does not change native rule semantics.

Validation layers are:

1. YAML document and structural safety;
2. Ansible static YAML Inventory structure;
3. Pigsty declaration and identity rules;
4. complete-Inventory advisory rules.

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


## 12. Grafana dashboard contract (P1)

Dashboard management uses a small native Grafana HTTP adapter. It MUST NOT shell
out to `files/grafana/grafana.py` and MUST NOT manage data sources. The public
surface is deliberately limited to:

```text
pig sty grafana info
pig sty grafana list
pig sty grafana boot
pig sty grafana load  [PATH]
pig sty grafana init  [PATH]
pig sty grafana dump  [PATH]
pig sty grafana dump  -d|--dir DIR
pig sty grafana clean [PATH] [--yes] [--dry-run]
pig sty grafana clean --all [--yes] [--dry-run]
pig sty grafana lang  <LANGUAGE_CODE>
pig sty grafana style <dark|light|auto>
```

`gf` is the command alias for `grafana`. `list` is a read-only remote inventory:
it has no positional path, lists every dashboard visible in the active
organization, and orders results by folder UID, title, then dashboard UID. Its
structured payload includes endpoint, organization, count, UID, title, folder
identity/title, and Grafana URL.

`info` performs the public health request before any authenticated request. It
reports endpoint, database health, Grafana version/commit, authentication
status, active organization, dashboard/folder counts, and organization/user
preferences when those authenticated reads are available. No configured
credential is a successful health-only result with authentication status
`not_configured`. A configured credential rejected by Grafana preserves the
health data but returns the permission-class failure code with status
`unauthorized` or `forbidden`. An unhealthy database or unreachable endpoint is
never reported as success.

When an endpoint is selected explicitly by flag or environment, `info` checks
its public health before loading Inventory credentials. A missing or malformed
Inventory remains a configuration failure, but the structured result preserves
the observed endpoint health and reports authentication status `error`.

The experimental `pig sty dashboard|ds` command, manifest, tombstones, and its
incompatible `check/diff` behaviors are removed rather than retained as
aliases.

### 12.1 Path and scope

Without `PATH`, Pig resolves `<PigstyHome>/files/grafana`. Pigsty home follows
the existing global `-H`, `PIGSTY_HOME`/configuration, and valid `~/pigsty`
resolution. Failure to resolve home is an error; Grafana commands MUST NOT fall
back to the current working directory.

For a complete root dump, `-d/--dir DIR` replaces that default output root and
may use any safe directory name; it does not need to be named `grafana`. The
directory may already exist or may be created when its parent exists. `--dir`
and positional `PATH` are mutually exclusive.

An explicit path uses the resolved root when it is contained by that root.
Otherwise the nearest ancestor named exactly `grafana` becomes the root. The
only valid relative shapes are:

```text
.                              complete corpus
<folder>                       one direct Grafana folder
<uid>.json                     one General dashboard
<folder>/<uid>.json            one dashboard in a folder
```

Deeper paths, unsafe UIDs, filename/JSON UID mismatches, duplicate UIDs,
oversized JSON, and symlink scopes fail before network mutation. Directory
scans are one level deep, select non-hidden `*.json`, and execute in bytewise
name order. A directory name is the stable `folderUid`; its uppercase form is
only the display title.

### 12.2 Load, boot, and init composition

`load` validates the complete selected local scope before contacting Grafana.
It upserts folders and dashboards with overwrite enabled and never prunes live
resources. Root JSON is placed in General; each direct subdirectory maps to the
equal `folderUid`. It has no authority to rename organizations, star dashboards,
or change preferences, including when the selected root contains `pigsty.json`.

`boot` has no local path. Before mutation it verifies that dashboard UID
`pigsty` exists in the active organization. It idempotently renames the current
organization to `Pigsty`, stars that dashboard for the authenticated identity,
and PATCHes only `homeDashboardUID: pigsty` into the organization and current
user preferences. It MUST NOT set or reset language, theme, timezone, week
start, or any unrelated preference.

`init` is the root-only composition `load` then `boot`. An explicit `PATH` must
resolve to a complete grafana root; folder and single-file callers use `load`.
Init performs all local validation before its first network mutation, executes
the complete load phase, and runs boot only when load has zero failed or skipped
objects. It never renames or changes preferences before dashboard loading. Its
structured result retains kind `pig.sty.grafana.init/v1`, identifies load and
boot phases, and reports a boot failure after a successful load as partial
initialization rather than silent success.

### 12.3 Language and style preferences

`lang <LANGUAGE_CODE>` PATCHes only the `language` field for both the active
organization and current user. Codes are matched case-insensitively against the
versioned language set supported by Pigsty's target Grafana version and are
canonicalized before the first request. Convenience aliases include `en` to
`en-US`, `zh`/`zh-CN` to `zh-Hans`, and `zh-TW` to `zh-Hant`. Unsupported codes
fail locally; Pig MUST NOT store arbitrary strings that Grafana would accept but
cannot render.

`style` accepts exactly `dark`, `light`, or `auto`. The first two map directly
to Grafana themes and `auto` maps to Grafana's `system` value. It PATCHes only
the `theme` field for both the active organization and current user. The empty
Grafana theme means inheritance/default and MUST NOT be used to represent the
explicit CLI value `auto`.

### 12.4 Clean ownership and confirmation

Normal clean ownership is the UID set declared by the selected local JSON:

- JSON scope deletes one UID;
- folder scope deletes UIDs in that directory;
- root scope deletes UIDs in the complete local corpus;
- folders are deleted only after a refreshed remote search proves them empty.

Remote dashboards with other UIDs remain untouched even when they share a
selected folder. `--all` is the sole instance-wide reset, is mutually exclusive
with `PATH`, and deletes all dashboards before folders.

Every clean first returns a deterministic endpoint/organization/object plan.
`--dry-run` succeeds without mutation. Text mode without `--yes` prints the
plan and prompts with default No; structured or non-interactive callers must
provide `--yes`. Interactive execution pins the plan digest and fails if live
state changes before application. A confirmation-required result MUST NOT
synthesize a replay command: callers add `--yes` to their original invocation
so its path, endpoint, and credential context cannot be dropped or rewritten.

### 12.5 Dump direction and local-write safety

`dump` treats Grafana as the export source: file scope exports one UID, folder
scope exports every dashboard with that `folderUid`, and root scope exports the
complete organization. Empty `folderUid` maps to root and non-empty
`folderUid` maps directly to a first-level directory; folder title is never
used as path identity.

Dump never deletes local files. Local-only JSON remains in place and is
reported. Unsafe remote UIDs, target collisions, folder movement during the
operation, and path escapes fail before writes. Exported bytes are canonical:
object keys are sorted, containers up to 400 bytes and nesting depth 10 are
inlined without padding, and larger containers use deterministic two-space
layout. Only a byte-identical canonical file is `unchanged`; a semantically
equal file with non-canonical formatting is rewritten once. Repeating a dump
against unchanged remote state therefore produces byte-identical files and no
second-write churn. Changed files use a same-directory temporary file plus
rename. Atomicity is per file, not a multi-file transaction.

Export normalization sets `id` to null and `version` to 1, records the Pigsty
author/license fields, and clears template-variable current selections. Dump
does not expose or modify datasource credentials because datasources are out of
scope.

### 12.6 Connection and API compatibility

Endpoint resolution is `--endpoint`, `GRAFANA_ENDPOINT`,
`all.vars.grafana_endpoint`, then the controller-facing Pigsty default
`http://i.pigsty/ui`. It MUST NOT infer a controller endpoint from
`grafana_port` or loopback. Username resolution is `--username`,
`GRAFANA_USERNAME`, `all.vars.grafana_admin_username`, then `admin`. Password
resolution is explicit `--password`, owner-only `--password-file`,
`GRAFANA_PASSWORD`, then `all.vars.grafana_admin_password`; no default password
is embedded. `--password` is the emergency escape hatch and is mutually
exclusive with `--password-file`. Help MUST warn that it can be exposed through
argv and shell history. A complete explicit endpoint, username, and password or
password file bypasses Inventory parsing so a missing or malformed Inventory
cannot block recovery. Logs, errors, next actions, structured output, and test
diagnostics never contain credentials or response bodies.

Preference updates use PATCH and `homeDashboardUID`; they MUST NOT use PUT or
the deprecated numeric `homeDashboardId`, because omitted PUT fields reset
unrelated preferences. The health request is sent without authentication even
when credentials were configured. All other API requests use the resolved
credential.

The adapter stays in `internal/grafana`, uses the bounded standard-library HTTP
client, refuses redirects, and encapsulates all legacy `/api` paths. No Grafana
SDK is added. The narrow business interface permits a future `/apis` adapter
without changing command semantics.


## 13. CMDB contract (P2)

Pigsty CMDB is an existing product surface, not a new database design. Its fact
sources are the adjacent Pigsty checkout's:

- `files/cmdb.sql`;
- `bin/inventory_load`;
- `bin/inventory_cmdb`;
- `bin/inventory_conf`.

The canonical public namespace is `pig inventory cmdb`. The unreleased
`pig sty cmdb` namespace is removed before release. The corrected verbs are:

- `check`: verify schema, projections, managed source state, and optional
  value-free drift;
- `init`: apply the exact version-matched `cmdb.sql` baseline to the target
  database;
- `load`: replace the existing Inventory declaration tables using the mapping
  and OS-default behavior of `inventory_load`;
- `dump`: query `pigsty.raw_config` and generate canonical YAML;
- `enable`: activate an `inventory.sh` querying `pigsty.inventory`;
- `disable`: set the Ansible Inventory back to `pigsty.yml`.

The baseline owns the existing `pigsty` and `pglog` schemas. CMDB storage uses
`global_var`, `group`, `group_var`, `host`, and `host_var`; it stores JSONB
declarations and cannot preserve YAML comments or source style.

The withdrawn `pigsty_cmdb` revision-store implementation, migrations,
Snapshot history, CAS, rollback, three-way merge, backup bundle, and source
state machine are not part of the corrected contract. The supersession record
is [`sty-cmdb-rfc.md`](sty-cmdb-rfc.md); exact command behavior is defined by
[`inventory.md`](inventory.md#7-cmdb-safety-and-commands).


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

Released migration surfaces use compatibility wrappers rather than immediate
deletion. The unreleased experimental `sty dashboard` surface is the explicit
exception and is replaced directly by `sty grafana`:

| Existing surface | Target command | Earliest phase |
|:---|:---|:---|
| `configure` / configure-related Make target | `pig sty conf` | already available |
| `make validate`, `bin/validate` | `pig inventory validate` (`pig sty validate` delegate) | root migration |
| `dashboard-init` | future `pig sty grafana init` | deferred Pigsty call-site migration |
| `dashboard-dump` | future `pig sty grafana dump` | deferred Pigsty call-site migration |
| `dashboard-clean` | future `pig sty grafana clean --all --yes` | deferred Pigsty call-site migration |
| `files/cmdb.sql` | `pig inventory cmdb init` | corrected CMDB adapter |
| `bin/inventory_load` | `pig inventory cmdb load` | corrected CMDB adapter |
| `bin/inventory_cmdb` | `pig inventory cmdb enable` | corrected CMDB adapter |
| `bin/inventory_conf` | `pig inventory cmdb disable` | corrected CMDB adapter |
| query `pigsty.raw_config` | `pig inventory cmdb dump` | corrected CMDB adapter |
| Vagrant/Terraform Make targets | thin P3 commands | P3 only |

An old wrapper remains for at least one released deprecation window and until
its parity fixtures have no unresolved regression. Its removal is a separate,
reviewed change.


## 16. Phase gates and acceptance

### G0 — Design contract

This document freezes taxonomy, aliases, selectors, output/exit behavior,
redaction, source resolution, mutation safety, priorities, and non-goals.

The original lossless P0, Validate, and Check acceptance criteria remain
regression requirements. The former revision-store result is superseded and is
not a passed product gate. Root Inventory P0–P2 and the CMDB safety closure are
implemented under the executable gates in
[`inventory.md`](inventory.md#12-acceptance-gates).

### P0.1 — Inventory completion

Deliver list/show services and DTOs, value-free source status, only the released
delegates, comment-aware editing, revision and mode notices, and source-path
behavior.

Acceptance:

- parse/no-op is byte-for-byte identical over the portable fixture set and the
  adjacent Pigsty `conf/**/*.yml` corpus;
- scalar/source-patch tests prove untouched bytes remain identical;
- selector tests cover dotted/IP keys, indexes, nested groups, and pointers;
- conflict tests write nothing and retain a private recovery fragment;
- symlink refusal, 0600 mode, tighten-notice, and structured-show refusal pass;
- read-only source discovery writes nothing and executes no source;
- canonical/delegate commands call the same use case.

### P0.2 — Validate completion

Deliver declaration-scope rule parity, pinned legacy fixtures, the divergence
registry, vault opacity, and optional Ansible compatibility.

Acceptance:

- every reference-corpus mismatch is fixed or registered;
- rule IDs and diagnostic paths/ranges are stable;
- warnings and `--strict` threshold behavior are tested;
- canonical hyphenated group names do not create strict-mode noise;
- Kafka and Mongo/Ferret identity fixtures cover missing, invalid, and duplicate
  declarations;
- vault/tag fixtures produce `UNVERIFIABLE`, not false failures;
- current Pigsty config corpus parses and validates to the documented manifest
  and diagnostic baseline.

### P0.3 — Check

Deliver deterministic local Check and explicit opt-in profiles.

Acceptance:

- default tests prove no network I/O;
- allowlists and per-probe timeouts are enforced;
- findings deduplicate deterministically;
- mixed-result status selection follows section 9.2;
- disposable targets cover network/SSH behavior without persistent mutation.

### P1 — Grafana dashboards

Acceptance includes exact scope classification, symlink/path-escape rejection,
filename/UID validation, pure-load authority, load-before-boot ordering,
boot suppression after any load failure, unauthenticated health, missing versus
rejected credentials, exact preference PATCH bodies, language alias/allowlist
validation, `auto` to `system`, emergency-password Inventory bypass/redaction,
local-ownership clean, explicit all-instance clean, folder-empty protection,
pinned interactive plans, per-object partial-failure reporting, and an HTTP
fake. Live validation may exercise info, load, boot, init, preferences, and clean
independently from dump so a test run cannot overwrite the repository corpus.

### P2 — CMDB

P2 uses the corrected, deliberately small scope. PostgreSQL tests must prove
exact `cmdb.sql` application, table parity with `inventory_load`, transaction
rollback, non-mutating value-free plans, confirmation gates, canonical and
overwrite-safe `raw_config` dump, dynamic Inventory query parity, CMDB check,
and guarded enable/disable behavior. No revision/history/merge system is an
acceptance requirement.

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

The owner decision of 2026-07-18 to promote Inventory to the root command
supersedes that review's `sty list` repurposing conclusion. A second owner
correction on the same date also rejects its greenfield CMDB premise: the
existing `cmdb.sql` and Inventory scripts are authoritative. The lossless
model, selector, and validation conclusions remain; the Dashboard and
revision-store CMDB conclusion does not. `sty list` keeps release semantics
until removal.

The owner decision of 2026-07-18 replaces that experimental Dashboard surface
with the path-scoped `pig sty grafana` contract in section 12. A separate
adversarial review first rejected ambiguous path, dump, and clean semantics,
then converged after local-declaration clean, explicit `clean --all`, stable
`folderUid` path identity, and remote-as-source dump rules were fixed. The exact
requested historical Claude 3.5 Sonnet snapshot was unavailable; the installed
Claude Code `sonnet` alias reported `claude-sonnet-5` for the replacement review.

The v1.4 `info`/`boot`/`load`/preferences extension received a further
adversarial review. It converged on anonymous health before authentication,
root-only `init = load + boot`, boot suppression after any incomplete load,
current-organization rename through `/api/org`, UID-based home preference
PATCHes, fixed language aliases, and `auto` to `system`. Claude Opus was not
exposed in the active agent runtime, so the strongest available
`gpt-5.6-sol` adversarial reviewer was used and that substitution is recorded
rather than represented as a Claude result.


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
pig sty gf info                  # health and authentication status
pig sty gf load [path]           # pure dashboard/folder loading
pig sty gf init [root]           # complete load followed by boot
```

`sty configure` remains an alias of `sty conf`. Generic templates may apply
explicit PostgreSQL version/region/proxy/locale/password options through the
Inventory mutation API; version-pinned templates retain their pinned kernel
semantics. `--raw` remains the rollback path during native stabilization.

The canonical Inventory workflow and compatibility delegates are defined by
[`inventory.md`](inventory.md). New automation should use `pig inventory|inv`;
the existing Sty Inventory paths remain available as compatibility surfaces.

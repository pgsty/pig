---
title: "pig inventory"
description: "Normative CLI contract for Pigsty Inventory and its existing CMDB adapter"
weight: 149
icon: fas fa-list
module: [PIG]
categories: [Reference]
---

# `pig inventory` Design Contract

Status: **normative command contract; phased migration**

Contract version: **1.1**

Last updated: **2026-07-18**

This document defines the public `pig inventory` command tree, aliases,
Inventory behavior, the thin adapter to Pigsty's existing CMDB, and migration
from the older `pig sty` paths.

The [`pig sty` contract](sty.md) remains authoritative for installation,
Configure, Check, Dashboard, the lossless static-document model, selector
grammar, validation, common status codes, and redaction. The existing Pigsty
schema and scripts listed in section 4 are the authority for CMDB behavior.

Normative words such as **MUST**, **MUST NOT**, **SHOULD**, and **MAY** have their
usual RFC 2119 meaning.


## 1. Command boundary and name

Inventory is a first-class Pig domain:

- canonical command: `pig inventory`;
- daily alias: `pig inv`;
- `i` is not an alias because root `pig install|i` already owns it;
- it appears in root `GroupID: "pigsty"`;
- Chinese documentation SHOULD introduce it as “配置清单（Inventory）”;
- bare `pig inventory` prints help and performs no implicit action.

Inventory owns declaration-oriented file and CMDB operations. It does not
absorb unrelated controller operations:

- `pig sty conf/configure` stays under Sty and uses Inventory APIs;
- `pig sty check` stays under Sty because it also checks controller, Ansible,
  network, SSH, and privilege readiness;
- Dashboard and Pigsty installation/release lifecycle stay under Sty;
- application, Docker/Compose, Vagrant, and Terraform lifecycle stay outside
  this command.

V1 models declarations, not Ansible effective variables. It does not simulate
group priority, multi-group precedence, vault decryption, role defaults,
`group_vars/host_vars`, directory Inventory, or arbitrary dynamic plugins.


## 2. Canonical command tree

```text
pig inventory | inv
  list | ls [selector]
  show [selector]
  edit | e [selector]
  validate | v [file] [--strict] [--ansible]

  cmdb
    init [-d DATABASE] [--yes]
    load [file] [-d DATABASE]
    dump [file] [-d DATABASE]
    enable [-d DATABASE]
    disable
```

Only the aliases shown above are canonical. No CMDB revision/history/rollback
or generic source-state command is part of this contract.

The CMDB data contract has only three primitives: `init`, `load`, and `dump`.
`enable` and `disable` are retained solely as thin CLI equivalents of
`inventory_cmdb` and `inventory_conf`; they switch Ansible's configured
Inventory source and do not extend the database model.

The implementation state at contract v1.1 is explicit:

| Surface | State |
|:---|:---|
| Lossless Document, ordered semantic model, selectors, editor, validation | implemented |
| `sty inventory list/show/edit/validate` and `sty edit/validate` | implemented compatibility source |
| existing Pigsty `cmdb.sql` and Inventory scripts | implemented in adjacent Pigsty checkout |
| root `inventory|inv` command | migration target; not yet implemented |
| current experimental `pigsty_cmdb` revision-store code in Pig | outside this corrected contract |

A specification entry is not evidence that its migration phase has shipped.
Current help and command tests determine availability.


## 3. Static Inventory commands

### 3.1 Source and selector

Root flags retain their current meanings:

- `-H/--home <dir>` selects Pigsty home;
- `-i/--inventory <file>` selects one static YAML file for this invocation;
- `-o/--output <text|yaml|json|json-pretty>` selects Result rendering.

Without `-i`, static resolution remains:

1. `PIGSTY_INVENTORY`;
2. `inventory` in `~/.pig/config.yml`;
3. the exact `inventory =` value in `<home>/ansible.cfg`;
4. `<home>/pigsty.yml`.

The first configured tier is authoritative. If it names an executable,
directory, or another unsupported dynamic source, a static command fails with
an explicit diagnostic; it does not silently fall through to a lower tier.

The selector grammar is frozen by [`sty.md`](sty.md#6-selector-contract).
Selectors address declared paths and never imply inherited/effective values.

### 3.2 `list|ls [selector]`

List emits source-ordered topology and safe metadata: path, object role, and
value kind. It never emits arbitrary scalar values, raw YAML, comments, vault
bodies, or generated passwords. Existing structured kind
`pig.sty.inventory.list/v1` remains stable during command relocation.

### 3.3 `show [selector]`

Show is a secret-bearing text surface:

- no selector returns exact original bytes;
- a selector returns the existing exact/dedented fragment;
- JSON/YAML Result modes are rejected;
- body bytes never enter a Result, diagnostic, or log.

### 3.4 `edit|e [selector]`

Edit retains the existing lossless private-fragment transaction: extract,
dedent, invoke `$VISUAL`/`$EDITOR`, re-indent, parse, validate, source-patch,
revision-check, and atomic commit. It never rewrites a static file by marshaling
the whole semantic model or YAML AST.

### 3.5 `validate|v [file]`

Validate runs the existing YAML, Ansible static Inventory, and Pigsty
declaration rules. A positional file selects that file. `--strict` changes the
failure threshold, not diagnostic severity. `--ansible` remains an optional,
bounded compatibility parse whose possible secret-bearing stdout is discarded.
Structured kind remains `pig.sty.inventory.validate/v1` during relocation.


## 4. CMDB fact source and scope

Pigsty CMDB already exists. Pig MUST adapt it; it MUST NOT invent another
database model.

The authoritative assets are:

| Asset | Authority |
|:---|:---|
| `/Users/vonng/pgsty/pigsty/files/cmdb.sql` | exact database baseline and default-variable catalog |
| `/Users/vonng/pgsty/pigsty/bin/inventory_load` | static Inventory to table mapping and OS-default loading behavior |
| `/Users/vonng/pgsty/pigsty/bin/inventory_cmdb` | dynamic Inventory script and activation behavior |
| `/Users/vonng/pgsty/pigsty/bin/inventory_conf` | return-to-static behavior |

The baseline creates the existing `pigsty` schema and its tables, functions,
views, types, and parameter data. It also creates the existing `pglog` objects
present in that file. The command MUST execute the version-matched baseline as
a whole; it MUST NOT translate it into a second schema or a private migration
ledger.

The Inventory data model used by the loader is exactly:

- `pigsty.global_var` for `all.vars`;
- `pigsty.group` for each first-level `all.children.<group>`;
- `pigsty.group_var` for group vars;
- `pigsty.host` for group membership;
- `pigsty.host_var` for host vars.

The existing database projections are also authoritative:

- `pigsty.raw_config` reconstructs declared static configuration as JSONB;
- `pigsty.inventory` emits Ansible dynamic Inventory JSON, including `_meta`;
- `pigsty.global_config`, `group_config`, and `host_config` are derived views,
  not a new storage format.

Consequently, Pigsty CMDB is intentionally narrower than the lossless YAML
Document:

- it stores JSON-compatible declaration values, not comments, whitespace,
  scalar lexemes, anchors, tags, or document style;
- it supports first-level groups as implemented by `inventory_load`; nested
  `children` are not representable and every group has a `hosts` mapping
  (which may be empty);
- host keys must be accepted by the existing `INET` columns;
- dump is canonical generated YAML, never a lossless round trip;
- database JSONB ordering is not source ordering.

No `pigsty_cmdb` schema, immutable Snapshot store, revision token, CAS history,
three-way merge, rollback bundle, runtime record, or source-switch state
machine belongs to this design.


## 5. CMDB connection contract

All CMDB commands use the existing connection convention:

1. explicit `-d/--database` argument;
2. `METADB_URL`;
3. `service=meta`.

`--data` MAY remain a compatibility alias for the Python loader's existing
flag. Connection strings and passwords are never copied into structured output
or logs. Pig does not create the target PostgreSQL database itself; `init`
applies the baseline inside the database selected by the connection.

The baseline and loader must come from the same resolved Pigsty home/version.
Mixing an arbitrary installed schema file with another Pigsty version is not a
supported workflow.


## 6. CMDB commands

### 6.1 `cmdb init [-d DATABASE] [--yes]`

Init applies `<resolved-home>/files/cmdb.sql` to the target database.

The SQL file currently starts by dropping and recreating `pigsty` and later
does the same for `pglog`. Therefore init is replacement, not an idempotent
migration. The command MUST:

1. resolve and verify the exact baseline file before connecting;
2. report the target by safe connection source, never by password-bearing DSN;
3. require an interactive confirmation or `--yes` when the target already has
   either schema;
4. execute the baseline as-is and preserve PostgreSQL's useful error;
5. verify required relations and views such as `pigsty.global_var`,
   `pigsty.group`, `pigsty.host`, `pigsty.raw_config`, and
   `pigsty.inventory` afterward.

Pig MUST NOT reinterpret the SQL into Go migrations or silently run a
different embedded schema.

### 6.2 `cmdb load [file] [-d DATABASE]`

Load is a native equivalent of `bin/inventory_load`, using the shared Inventory
parser instead of another generic YAML conversion. The default file is the
resolved static Inventory; an explicit positional file takes precedence.

Load validates representability before the first database mutation, then in
one database transaction:

1. truncates `pigsty.group`, `pigsty.host`, `pigsty.group_var`,
   `pigsty.host_var`, and `pigsty.global_var` as the existing script does;
2. inserts `all.vars` into `global_var` as JSONB;
3. inserts first-level groups, group vars, hosts, and host vars;
4. commits all replacement rows together or rolls back all of them.

After the Inventory transaction, it mirrors the existing best-effort
OS-default behavior:

- select the current OS/architecture vars file with the same fallback rules;
- update the mapped `default_var` entries;
- replace `package_map` when present;
- warn and continue when OS-specific defaults are unavailable, matching the
  current script's non-fatal behavior.

Load replaces the current declaration rows. It does not append a revision,
compute a merge, or maintain history.

### 6.3 `cmdb dump [file] [-d DATABASE]`

Dump queries the single row from `pigsty.raw_config`, decodes its JSONB into the
shared semantic Inventory value model, and emits canonical compact YAML.

- With a file argument, write through a sibling temporary file and atomically
  replace the requested destination; new files default to `0600`.
- Without a file argument, emit YAML directly in text mode.
- Structured modes require a file destination and return only safe metadata
  such as path and declaration counts; Inventory bytes remain outside Result.
- The output MUST parse to the same CMDB-representable declarations, but it
  MUST NOT claim to preserve comments, padding, quotes, YAML tags, or key order.

Dump uses `raw_config`, not the effective/default-enriched
`pigsty.global_config`, because its purpose is to reconstruct declarations.

### 6.4 `cmdb enable [-d DATABASE]`

Enable is the native equivalent of `bin/inventory_cmdb`:

1. verify that `SELECT text FROM pigsty.inventory` returns valid Ansible
   Inventory JSON;
2. write `<home>/inventory.sh` that executes that exact query against the
   selected database;
3. set mode `0755`;
4. update the supported Inventory directive in `<home>/ansible.cfg` to
   `inventory.sh`.

The Go implementation MAY use a narrow, revision-checked edit instead of the
script's broad `sed`, but the resulting public behavior and query remain the
same. It does not need a source-switch database record or semantic-parity
protocol.

Because `inventory.sh` is executable and world-readable, enable MUST reject a
password-bearing URI/conninfo literal. Normal use is the existing
`service=meta` plus protected libpq credential facilities.

### 6.5 `cmdb disable`

Disable is the native equivalent of `bin/inventory_conf`: update the supported
Inventory directive in `<home>/ansible.cfg` to `pigsty.yml`. It does not modify
CMDB tables and does not delete `inventory.sh`.


## 7. Output and status contract

Structured commands reuse Result `api: 1`. Corrected CMDB payload kinds are:

```text
pig.inventory.cmdb.init/v1
pig.inventory.cmdb.load/v1
pig.inventory.cmdb.dump/v1
pig.inventory.cmdb.enable/v1
pig.inventory.cmdb.disable/v1
```

CMDB Result data contains only operation, safe connection source, row/object
counts, output path, and redacted finding codes. It never contains Inventory
values, dump bodies, SQL bodies, DSNs, or passwords.

The existing experimental `pig.sty.cmdb.*` revision-store DTOs are not frozen
by this corrected contract. Compatibility Sty commands, if retained, MUST
delegate to these corrected operations rather than preserve the abandoned
database model.

Common MMCCNN status and shell-exit mapping remains defined by `sty.md`.


## 8. Compatibility contract

| Existing spelling or script | Canonical command |
|:---|:---|
| `bin/inventory_load` | `pig inv cmdb load` |
| `bin/inventory_cmdb` | `pig inv cmdb enable` |
| `bin/inventory_conf` | `pig inv cmdb disable` |
| applying `files/cmdb.sql` as database baseline | `pig inv cmdb init` |
| querying `pigsty.raw_config` and converting to YAML | `pig inv cmdb dump` |
| `make cmdb` (`inventory_load` then `inventory_cmdb`) | `pig inv cmdb load` then `pig inv cmdb enable` |
| `pig sty inventory ...`, `pig sty edit/validate` | matching `pig inv ...` command |
| corrected `pig sty cmdb ...` delegates | matching `pig inv cmdb ...` command |

Canonical and compatibility paths share one business implementation. Shell
scripts remain available for at least one release after parity is proven.

`pig sty list|l|ls` remains a Pigsty release-list delegate and MUST NEVER be
repurposed as Inventory list. `pig inv ls` is the short Inventory path.


## 9. Code ownership

Repository layout follows `AGENTS.md`:

- flat `cmd/inventory.go` owns canonical Cobra glue;
- `cmd/root.go` only registers the top-level command;
- `cmd/sty.go` keeps unrelated Sty commands and temporary delegates;
- top-level `inventory` remains the reusable lossless/semantic core;
- one small CMDB adapter owns PostgreSQL I/O and the two local file operations.

The CMDB adapter SHOULD reuse the existing PostgreSQL dependency and Inventory
value conversion. It MUST NOT duplicate the YAML parser, shell out to the three
legacy scripts as its final implementation, or introduce a generic backend
framework.

The public API in this contract is the CLI/Result contract. No semver-stable
external Go API is promised.


## 10. Migration plan

### M0 — Correct the contract

- make this document authoritative;
- mark the former native revision-store RFC superseded;
- remove revision/history/rollback/source-state promises from Sty specs;
- record the four existing Pigsty assets as the fact source.

### M1 — Root static Inventory surface

- add flat `cmd/inventory.go` and root `inventory|inv` registration;
- expose list/show/edit/validate and specified aliases;
- reuse current use cases and DTOs;
- keep Sty delegates until root parity is proven.

### M2 — Thin existing-CMDB adapter

- implement `init` from the exact version-matched `files/cmdb.sql`;
- implement `load` with the exact table mapping and OS-default behavior of
  `inventory_load`;
- implement `dump` from `pigsty.raw_config` through canonical YAML;
- implement enable/disable with the exact `pigsty.inventory` query and final
  `ansible.cfg` meanings of the scripts;
- compare Go output/database rows against the existing scripts in a disposable
  PostgreSQL environment.

### M3 — Remove the abandoned implementation

- stop exposing the experimental `pigsty_cmdb` revision-store commands;
- remove its migrations, history/CAS/merge/runtime-record code and tests only
  after the thin adapter passes parity;
- retain no compatibility database writes to the abandoned schema;
- update Make/Bash wrappers and docs to canonical commands after parity.

Each phase is independently reviewable. The existing scripts remain the
rollback path until M2 is accepted.


## 11. Acceptance

The migration must prove:

- root aliases do not collide with `install|i`;
- static Inventory lossless/corpus/edit/validation tests remain green;
- `init` applies the exact selected Pigsty `cmdb.sql` and verifies its existing
  objects;
- `load` produces the same five declaration tables as `inventory_load` for
  representative configs and rolls back on failure;
- OS-specific default/package-map behavior matches the script;
- `dump` reparses to the same CMDB-representable declarations;
- dynamic Inventory JSON matches `SELECT text FROM pigsty.inventory`;
- enable/disable end with the same `ansible.cfg` meanings as the scripts;
- no `pigsty_cmdb` schema is created by corrected commands;
- no Inventory value, SQL body, or DSN enters structured output or logs;
- focused tests, PostgreSQL integration tests, `go test ./...`, `go vet ./...`,
  and `git diff --check` pass.

---
title: "pig inventory"
description: "Normative CLI contract for Pigsty Inventory and its existing CMDB adapter"
weight: 149
icon: fas fa-list
module: [PIG]
categories: [Reference]
---

# `pig inventory` Design Contract

Status: **normative contract; P0–P2 and root Check implemented**

Contract version: **1.6**

Last updated: **2026-07-19**

This document defines the public `pig inventory` command tree, source
resolution, static Inventory behavior, the deliberately thin adapter to
Pigsty's existing CMDB, compatibility, delivery phases, and acceptance gates.

The [`pig sty` contract](sty.md) remains authoritative only for installation,
Configure, release lifecycle, Grafana, and other Sty controller services. This
document is the sole authority for the root Inventory command, selector grammar,
source resolution, lossless static-file behavior, validation, Check, Inventory
Result kinds, status codes, redaction, and CMDB behavior. The existing Pigsty
schema and scripts listed in section 6 are the factual authority for the CMDB
data model.

Normative words such as **MUST**, **MUST NOT**, **SHOULD**, and **MAY** have their
usual RFC 2119 meaning.


## 1. Product boundary

Inventory is a first-class Pig domain:

- canonical command: `pig inventory`;
- daily alias: `pig inv`;
- `i` is not an alias because root `pig install|i` already owns it;
- it appears in root `GroupID: "pigsty"`;
- Chinese documentation SHOULD introduce it as “配置清单（Inventory）”;
- bare `pig inventory` prints help and performs no implicit action.

Inventory owns declaration-oriented static-file operations and a bounded
adapter to Pigsty's existing PostgreSQL CMDB. It does not absorb unrelated
controller operations:

- `pig sty conf/configure` stays under Sty and consumes Inventory APIs;
- `pig inventory check|ck` owns Inventory, controller, Ansible, network, SSH,
  and privilege readiness checks; remote profiles remain explicit opt-ins;
- Grafana and Pigsty installation/release lifecycle stay under Sty;
- application, Docker/Compose, Vagrant, and Terraform lifecycle stay outside
  this command.

The initial supported declaration source is one static Pigsty YAML file. The
semantic model describes declarations, not Ansible effective variables. It does
not simulate group priority, multi-group precedence, vault decryption, role
defaults, vars plugins, Jinja execution, `group_vars/host_vars`, directory
Inventory, multiple source ordering, or arbitrary dynamic plugins.

Known CMDB operations MAY consume Pigsty's verified native CMDB adapter. That
exception does not create a generic source/backend framework and does not make
lossless presentation claims about database-generated YAML.


## 2. Canonical command tree

```text
pig inventory | inv
  status
  list | ls [selector] [--depth N]
  show [selector]
  edit | e [selector] [--from FILE|-]
  validate | v [file] [--strict] [--ansible] [--timeout DURATION]
  check | ck [--profile PROFILE] [--sudo] [--port PORT] [--user USER]
  diff <other-file>

  cmdb
    check [file] [-d DATABASE]
    init [-d DATABASE] [--plan] [--yes|-y]
    load [file] [-d DATABASE] [--plan] [--yes|-y] [--strict]
    dump [file] [-d DATABASE] [--force|-f]
    enable [-d DATABASE] [--plan]
    disable [--plan]
```

Only the aliases shown above are canonical. The command has no implicit default
action. `status`, `check`, and `cmdb check` are distinct: status is local source
discovery without execution or network I/O; Check validates local readiness and
runs remote observations only when requested; CMDB check connects to the
selected database and verifies its schema and projections.

The implementation state at contract v1.5 is explicit:

| Surface | State |
|:---|:---|
| Lossless Document, ordered semantic model, selectors, editor, and native validation | implemented baseline |
| root `inventory|inv` list/show/edit/validate/check and thin CMDB primitives | implemented |
| `status`, source descriptor, compatibility cleanup, validation corrections | P0 implemented |
| CMDB check, plans, confirmation, overwrite and source guards | P1 implemented |
| list depth, bounded timeout flag, non-interactive edit, value-free diff | P2 implemented |
| read-only directory Inventory or source-aware CMDB query commands | deferred and evidence-gated |
| former `pigsty_cmdb` revision store | removed; MUST NOT be revived |

Implementation status is supported by the acceptance gates in section 12, not
merely by the presence of command names.


## 3. Source contract

### 3.1 Flags, precedence, and path bases

Root flags retain these meanings:

- `-H/--home <dir>` selects Pigsty home;
- `-i/--inventory <file>` selects one static YAML file for this invocation;
- `-o/--output <text|yaml|json|json-pretty>` selects Result rendering.

Inventory resolution is ordered and returns a value-free source descriptor, not
only a path:

1. explicit `-i/--inventory`, resolved relative to the process working
   directory;
2. `PIGSTY_INVENTORY`, resolved relative to the process working directory;
3. `inventory` in `~/.pig/config.yml`; an absolute value is used directly and a
   relative value is resolved against the resolved Pigsty home;
4. the exact `inventory =` value in `<home>/ansible.cfg`, resolved relative to
   that Pigsty home;
5. `<home>/pigsty.yml`.

A relative config-file value without a resolved Pigsty home is invalid. The
first configured tier is authoritative even when its target is missing or
unsupported. Pig MUST report that concrete source and MUST NOT silently fall
through to a lower tier.

Read-only Inventory commands MUST NOT create `~/.pig`, touch
`~/.pig/config.yml`, rewrite `ansible.cfg`, execute a dynamic source, or connect
to a database as a side effect of source discovery.

### 3.2 Source descriptor and `status`

`pig inventory status` performs local discovery only. It reports safe metadata:

- resolved Pigsty home;
- selection tier: `flag`, `environment`, `config`, `ansible_config`, or
  `default`;
- resolved absolute path when the source is path-based;
- source type: `static_yaml`, `directory`, `executable_dynamic`,
  `managed_cmdb`, `missing`, or `unsupported`;
- existence, readability, executability, and managed-state findings;
- which static commands are supported for that source;
- required next actions when the active source is unsupported.

Status MUST NOT execute the source or emit Inventory values, arbitrary command
literals, script bodies, DSNs, or credentials. Managed CMDB detection is based
on the supported `ansible.cfg` directive and exact managed-file shape, not on
database reachability or the incidental existence of `inventory.sh`.

Source discovery and CMDB source switching MUST share one supported-directive
parser. The supported file contains exactly one active `[defaults]` section and
exactly one active `inventory =` directive. Duplicate, ambiguous, or malformed
directives classify the source as `unsupported`; status MUST NOT select the
first textual match when enable/disable would refuse the same file.

### 3.3 Static and future sources

`list`, `show`, `edit`, `validate`, `check`, and `diff` initially require one
static YAML file. If the active source is a directory, arbitrary executable, or
unsupported plugin, they fail with a typed diagnostic and point to
`inventory status`. An explicit `-i` remains the per-invocation escape hatch
and never persists a source switch.

P3 MAY add read-only `status`, `list`, and `validate` support for Inventory
directories after source provenance, Ansible precedence, and differential
fixtures are specified. Lossless `show` and `edit` remain physical-file
operations and MUST NOT pretend that an effective multi-file projection has one
lossless source document. Arbitrary dynamic plugins remain out of scope.


## 4. Static Inventory commands

Selectors address declared paths and never imply inherited or effective values.
The stable grammar is:

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

Examples include `vars.infra_portal`, `pg-meta.vars.pg_databases[0]`,
`pg-meta.hosts["10.10.10.10"]`, and
`/all/children/pg-meta/hosts/10.10.10.10`. Ambiguous shorthand MUST fail and
callers MUST use the canonical pointer when a literal key collides with
shorthand resolution. Stable selector failures are `SELECTOR_SYNTAX`,
`SELECTOR_NOT_FOUND`, and `SELECTOR_TYPE`. IP-key, dotted-key, quoted-key,
nested-group, index, and pointer-escape fixtures are normative tests.

### 4.1 `list|ls [selector] [--depth N]`

List emits source-ordered, value-free metadata: canonical path, object role,
and value kind. It never emits arbitrary scalar values, raw YAML, comments,
vault bodies, aliases, generated passwords, or source fragments.

`--depth N` accepts a non-negative integer. The selected node is depth zero;
its direct children are depth one. Omitting the flag preserves the current
recursive behavior. Depth affects traversal only and does not change selectors
or the Result schema.

### 4.2 `show [selector]`

Show is an explicitly secret-bearing text surface:

- no selector returns exact original bytes;
- a selector returns the exact existing, dedented fragment;
- JSON/YAML Result modes are rejected;
- body bytes never enter a Result, diagnostic, next action, or log.

Pig MUST NOT add a best-effort `--redact` mode until a separately reviewed,
complete secret-classification contract and corpus prove that it does not create
false safety.

### 4.3 `edit|e [selector] [--from FILE|-]`

Without `--from`, Edit retains the current private-fragment transaction:
extract, dedent, invoke `$VISUAL`/`$EDITOR`, re-indent, parse, validate,
source-patch, byte-revision check, and atomic commit. Interactive editing
requires text output.

`--from FILE` replaces the selected existing fragment with bytes from a regular
file. `--from -` reads the replacement from stdin; an explicitly empty
`--from` value is invalid. This mode does not invoke an
editor and MAY return safe structured metadata. It MUST NOT accept replacement
values through argv, create missing intermediate containers, follow a source
symlink, or bypass the normal revision and atomic-write checks. To add or remove
mapping entries, callers select and replace the existing parent mapping.

Edit validates both the original and candidate with the same deterministic
rule set. A candidate MUST parse, MUST NOT introduce a new error identified by
stable code plus canonical path, and MUST leave no error inside the selected
subtree. Unchanged pre-existing errors outside a selected fragment MAY remain so
that targeted repair is possible. Selecting the whole document therefore
requires the candidate to contain no errors. Warnings remain diagnostics but do
not block Edit.

No Edit path rewrites a static file by marshaling the semantic model or YAML
AST. A no-op is byte-identical. Successful writes preserve stricter existing
permissions and tighten group/world-readable modes to owner-only. Concurrent
byte changes fail without writing and retain the private recovery fragment.

### 4.4 `validate|v [file]`

Validate runs, in order:

1. YAML document and structural-safety rules;
2. Ansible static YAML Inventory rules;
3. Pigsty declaration and identity rules;
4. complete-Inventory advisory rules.

A positional file overrides the resolved source for that invocation. Errors
fail. Warnings retain warning severity and fail only with `--strict`. Pigsty's
canonical letters/digits/underscore/hyphen group names MUST NOT receive the
generic Ansible transformation warning merely because they contain a hyphen;
genuinely unsafe names remain diagnostic.

Pigsty identity validation covers the critical statically knowable invariants
used by the adjacent roles. In addition to the existing PostgreSQL, infra, etcd,
MinIO, and Redis rules, it includes at least:

- Kafka: required cluster and sequence declarations, non-negative integral
  sequence, cluster-local sequence uniqueness, and declared role membership in
  `combined|broker|controller` when a role is present;
- Mongo/Ferret: required `mongo_cluster`, non-negative integral and
  cluster-local unique `mongo_seq`, and a non-empty declared `mongo_pgurl`.

Module detection is parameter-based, not group-name based. A module rule is
triggered when its cluster or sequence identity key is declared in `all.vars`,
a group vars mapping, or a host vars mapping. Required identity may be satisfied
at any of those declaring scopes, so co-located module declarations in a group
such as `pg-mongo` remain valid and no dedicated `kafka` or `mongo` group name is
required. For these named identity keys only, the validator associates a host
with its host declaration, otherwise its declaring group declaration, otherwise
the global declaration. An opaque or conflicting association is
`UNVERIFIABLE`, not guessed. Sequence uniqueness is keyed by the declared
`kafka_cluster` or `mongo_cluster` value, never by the YAML group name. This
narrow identity association is not a general effective-vars resolver.

Validation does not copy runtime-only play constraints, lifecycle tags, port
availability, role defaults, or every module parameter into the Inventory
validator.

`--ansible` runs the installed `ansible-inventory` as a bounded compatibility
oracle. Its stdout is discarded and stderr is sanitized. `--timeout` accepts a
positive duration and is valid only with `--ansible`; the default is 10 seconds.
Native rules remain deterministic and do not change with the installed Ansible
version.

### 4.5 `check|ck`

Check answers whether the active Inventory, controller, and explicitly selected
targets appear ready for a Pigsty operation. It is not another validator and
never mutates or auto-fixes.

Default Check is deterministic and local: it validates the active Inventory,
verifies Pigsty project layout and required local files, and inspects required
local executables. It performs no network, SSH, privilege, or package mutation.

Additional observations require explicit flags:

- `--profile ansible` runs bounded `ansible-inventory --list`, discards stdout,
  and sanitizes stderr;
- `--profile network` performs bounded TCP reachability checks;
- `--profile ssh` permits only an allowlisted SSH command ending in remote
  `true`;
- `--sudo` additionally permits only `sudo -n true` and requires the SSH
  profile;
- `--port` and `--user` affect only the explicit network/SSH observations and
  do not infer effective Inventory variables.

Every probe has an individual timeout and a fixed command allowlist. Remote
profiles are observational rather than side-effect-free because authentication
logs, alerts, and lockout counters may change. Findings are value-free,
deterministic, and deduplicated by stable code, path, and target.

### 4.6 `diff <other-file>`

Diff compares the resolved static source, including an explicit `-i`, with one
other static file. It is a semantic, value-free review surface:

- operations are `add`, `remove`, and `replace`;
- each operation contains a canonical declaration path and object/value kind;
- scalar values, source fragments, comments, and secret material are omitted;
- mappings compare by key and ignore declaration reordering;
- sequences are order-sensitive and MAY initially be reported as one atomic
  replacement;
- identical declarations succeed with an empty change set.

Formatting- and value-bearing comparison remains the responsibility of tools
such as `git diff`. The same internal value-free diff primitive SHOULD produce
CMDB load plans; this does not create Snapshot, merge, or history semantics.


## 5. Static write and file-safety contract

All static writes resolve an absolute target and reject directory, device,
socket, FIFO, executable source, and source symlink targets. They use a sibling
temporary file, preserve or tighten permissions as specified, fsync where the
existing utility requires it, and atomically rename only after validation and
revision checks pass.

Diagnostics and ChangeSets contain stable identifiers, canonical paths, source
ranges, counts, and operation kinds only. They MUST NOT retain old/new values or
edited source bytes.


## 6. Existing CMDB fact source and connection

Pigsty CMDB already exists. Pig MUST adapt it and MUST NOT invent another
database model. The authoritative assets are:

| Asset | Authority |
|:---|:---|
| `/Users/vonng/pgsty/pigsty/files/cmdb.sql` | exact database baseline and default-variable catalog |
| `/Users/vonng/pgsty/pigsty/bin/inventory_load` | static Inventory to table mapping and OS-default loading behavior |
| `/Users/vonng/pgsty/pigsty/bin/inventory_cmdb` | dynamic Inventory script and activation behavior |
| `/Users/vonng/pgsty/pigsty/bin/inventory_conf` | return-to-static behavior |

The version-matched baseline creates the existing `pigsty` and `pglog` objects
as a whole. Pig MUST NOT translate it into Go migrations or a private ledger.

The loader's declaration model is exactly:

- `pigsty.global_var` for `all.vars`;
- `pigsty.group` for each first-level `all.children.<group>`;
- `pigsty.group_var` for group vars;
- `pigsty.host` for group membership;
- `pigsty.host_var` for host vars.

A declaration set is CMDB-representable exactly when `all` exists and declares
no direct `hosts`; every first-level group is a mapping with a mapping-valued
`hosts` and no nested `children`; every host key parses as a PostgreSQL INET
value; every host value is a mapping; and every vars mapping has string keys and
JSON-convertible, untagged, unaliased values. Here string keys means the direct
Inventory variable names; nested scalar mapping keys follow the existing shared
JSON conversion and must be losslessly string-coercible. Load fails closed on
any unrepresentable input. This is a deliberate, recorded safety divergence
from `bin/inventory_load`, which can silently drop `all.hosts` and nested
children.

`pigsty.raw_config` reconstructs declared JSONB and `pigsty.inventory` emits
Ansible dynamic Inventory JSON. The CMDB is narrower than the lossless Document:
it does not preserve comments, whitespace, scalar lexemes, tags, anchors, source
ordering, or nested children; host keys must fit the existing `INET` columns;
dump is generated canonical YAML, not a lossless round trip.

All database commands resolve connections in this order:

1. explicit `-d/--database`;
2. `METADB_URL`;
3. `service=meta`.

`--data` and `--name/profile` are not part of the Pig CLI contract. Connection
strings and passwords never enter structured output, plans, next actions, or
logs. Documentation SHOULD prefer environment or libpq service configuration
over password-bearing argv. Pig does not create the target PostgreSQL database.

Pig standardizes the default connection at `service=meta`. This deliberately
differs from `inventory_load` when `METADB_URL` is unset: the Python loader uses
empty conninfo and therefore generic libpq environment defaults, while the two
source-switch scripts and Pigsty's documented workflow use `service=meta`.

No `pigsty_cmdb` schema, immutable Snapshot store, revision token, CAS history,
three-way merge, rollback bundle, runtime record, or source-switch state machine
belongs to this design.


## 7. CMDB safety and commands

### 7.1 Common plan and confirmation behavior

`init`, `load`, `enable`, and `disable` use Pig's standard Plan Result. A plan
contains, as applicable, the operation, safe connection source, local paths,
object/row counts, the value-free pinned database-target fingerprint, content
digests, affected schemas/files, risks, confirmation level, and safe
next-action guidance.
Value-free path operations join Load plans and Check findings through the
shared static diff primitive; changed versus no-op is decided by canonical
declaration-equivalence digest. A plan never contains
Inventory values, SQL bodies, dump bodies, DSNs, or passwords.

`--plan` performs every safe preflight needed to make the plan useful but makes
no database or file mutation. `--plan` and `--yes` are mutually exclusive.
Text-mode TTY execution MAY prompt. Structured and non-interactive modes never
prompt and require `--yes` whenever confirmation is required. A verified
no-op succeeds without confirmation. An interactive confirmation pins the
displayed source revision, database-target fingerprint, and applicable target
digest/schema state; a change while the prompt is open aborts instead of
silently rebuilding and executing a different plan.

Next actions MUST NOT silently substitute another database target. When the
target comes from the environment or the default service, a safe replay command
may be emitted. When an explicit `-d` value cannot be reproduced without
copying connection material, the action omits `command` and requires rerunning
the original command with the same explicit target; the target fingerprint
still identifies the reviewed database without exposing it.

### 7.2 `cmdb check [file] [-d DATABASE]`

Check is read-only and connects to the selected database. It verifies:

- connection and the required existing schema objects;
- that `pigsty.raw_config` returns valid declaration JSON;
- that `pigsty.inventory` returns valid dynamic Inventory JSON with `_meta`;
- local `ansible.cfg` and exact managed `inventory.sh` consistency when the
  active source claims managed CMDB mode;
- password-bearing managed-script literals are absent.

With a file argument, Check additionally runs full static validation and
CMDB-representability validation, then reports canonical-equivalence, safe
digests, and value-free declaration paths against `raw_config` through the
shared diff primitive. Check never emits compared values.

### 7.3 `cmdb init [-d DATABASE] [--plan] [--yes|-y]`

Init applies `<resolved-home>/files/cmdb.sql` as-is. Before mutation it verifies
the exact regular baseline file, computes its digest, connects, inspects the
target, and builds a plan. When either `pigsty` or `pglog` already exists, Init
requires interactive confirmation or `--yes` because the baseline replaces
those schemas. A clean target does not require confirmation.

After execution, Init verifies the required relations and projections. It MUST
NOT silently substitute an embedded schema, reinterpret statements as Go
migrations, or continue after a baseline failure.

### 7.4 `cmdb load [file] [-d DATABASE] [--plan] [--yes|-y] [--strict]`

Load is the native equivalent of `bin/inventory_load`. The default file is the
resolved static Inventory; an explicit positional file takes precedence. Before
the first database mutation it:

1. parses the file through the shared lossless Inventory model;
2. runs the complete native validation stack; errors block and `--strict`
   additionally blocks warnings;
3. verifies CMDB representability;
4. reads current `raw_config` and builds a digest-based change plan;
5. records the source revision/digest and rechecks it before execution.

An unchanged declaration set is a no-op. Any changed declaration set requires
interactive confirmation or `--yes`, because Load replaces all five declaration
tables. The replacement itself remains one database transaction: truncate,
insert globals/groups/group vars/hosts/host vars, then commit or roll back all
declaration rows together.

When no positional file is given and the resolved active source is not one
static YAML file, Load fails with a typed source diagnostic and a next action
naming an explicit file argument. It never silently falls back to
`<home>/pigsty.yml`, including after CMDB mode has been enabled.

After that transaction, Load mirrors the existing separate, best-effort
OS-default/package-map behavior. Missing or failed OS defaults are explicit
warnings and never masquerade as a completely clean result. Load appends no
revision and maintains no history.

### 7.5 `cmdb dump [file] [-d DATABASE] [--force|-f]`

Dump reads the single `pigsty.raw_config` row and emits canonical compact YAML.
Without a file, it writes secret-bearing YAML directly in text mode. Structured
output requires a destination and returns safe metadata only.

For a destination:

- reject a symlink or non-regular existing target;
- create a missing file atomically with mode `0600`;
- return unchanged when existing bytes are identical;
- refuse differing existing bytes unless `--force` is explicit;
- on force, replace through a validated sibling temporary file;
- reparse output and prove declaration equivalence before writing.

Dump uses `raw_config`, not effective/default-enriched views, and never claims
to preserve YAML presentation.

Dump follows idempotence plus refusal plus explicit `--force`; `--force`
without a destination is invalid. Dump has no plan mode even though a
destination writes a local file.

### 7.6 `cmdb enable [-d DATABASE] [--plan]`

Enable is a guarded native equivalent of `bin/inventory_cmdb`:

1. verify required CMDB objects and valid dynamic Inventory JSON;
2. render the exact supported query adapter without a password-bearing literal;
3. inspect `<home>/inventory.sh` and `<home>/ansible.cfg`;
4. recognize as managed either Pig's exact rendered form or the legacy
   `bin/inventory_cmdb` two-line form consisting of `#!/bin/bash` followed by
   `psql <one connection token> -AXtwc 'SELECT text FROM pigsty.inventory;'`;
5. write mode `0755` and switch only when `inventory.sh` is absent or matches a
   managed shape and the current directive literal is `pigsty.yml` or
   `inventory.sh`.

Enable is idempotent. It refuses to overwrite a different `inventory.sh` and
refuses to replace a third-party Inventory directive. A refusal identifies the
blocking file or directive and gives a replayable manual rename/removal next
action without printing script bytes. There is deliberately no force flag that
silently destroys those user-owned sources. Recognizing a password-bearing
legacy managed shape authorizes replacing it with the newly selected safe
password-free rendering; Pig never reuses or prints the legacy connection
literal. `--plan` reports the two local file changes without writing them.

### 7.7 `cmdb disable [--plan]`

Disable is the guarded native equivalent of `bin/inventory_conf`. It changes
the supported `inventory.sh` directive back to `pigsty.yml`, does not connect to
or modify the database, and does not delete `inventory.sh`. It is idempotent and
refuses to replace a third-party Inventory directive. `--plan` reports the one
local configuration change without writing it.


## 8. Output and public API contract

Structured commands reuse the common Result envelope but use root-domain kinds:

```text
pig.inventory.status/v1
pig.inventory.list/v1
pig.inventory.edit/v1
pig.inventory.validate/v1
pig.inventory.check/v1
pig.inventory.diff/v1
pig.inventory.cmdb.check/v1
pig.inventory.cmdb.init/v1
pig.inventory.cmdb.load/v1
pig.inventory.cmdb.dump/v1
pig.inventory.cmdb.enable/v1
pig.inventory.cmdb.disable/v1
```

Go orchestration uses `CodeInventory*` names. Their numeric MM range remains
the existing Pigsty module value `20` for wire compatibility; moving the
command to the root MUST NOT renumber already observed status codes.

Public diagnostics, list items, edit ChangeSets, diff items, and plans MUST
expose a canonical pointer string such as
`/all/children/pg-meta/hosts/10.10.10.10/pg_seq`. They MAY additionally expose
structured segments whose kind is the string `key` or `index`; a numeric enum
MUST NOT be the sole public path representation. Core Go `Path` representation
is internal and is not itself the CLI wire contract.

Safe structured data contains only source descriptors, identifiers, paths,
ranges, kinds, counts, digests, findings, and operation metadata. It never
contains arbitrary Inventory values, comments, source fragments, YAML bodies,
SQL bodies, executable bodies, DSNs, or passwords.

`show` and stdout `cmdb dump` are raw, secret-bearing text streams outside the
Result envelope. Interactive `edit` refuses structured output; `edit --from`
MAY emit its safe Result. External adapters discard possible secret-bearing
stdout and sanitize stderr before diagnostics.


## 9. Compatibility and release contract

Release evidence, not an unreleased worktree command, creates compatibility
debt. The local v1.6.0 tag at commit
`2c824971d85b0c6b9d9030832cac360948899c33` registers `pig sty edit` and
`pig sty validate` and registers neither the root Inventory command,
`pig sty inventory`, nor `pig sty cmdb`.

Therefore:

- `pig sty edit|e` and `pig sty validate|val|v` are retired now that their root
  Inventory paths have shipped. The v1.6.0 fixture records the historical
  edit/validate release, while current-tree tests MUST assert every old Sty
  spelling is absent;
- `pig sty check|ck` is also absent; readiness is available only as
  `pig inventory check|ck`;
- retired `pig.sty.*` Inventory payload kinds are no longer emitted;
- unreleased `pig sty inventory ...` and `pig sty cmdb ...` are removed before
  the next release instead of manufacturing permanent aliases;
- root commands use the corrected `pig.inventory.*` payload kinds;
- `pig sty list|l|ls` remains the Pigsty release-list delegate and MUST NEVER be
  repurposed as Inventory list; `pig inv ls` is the short Inventory path.

The pinned compatibility fixture MUST continue to describe the exact v1.6.0
tag command tree. A separate current-tree expectation records deliberate
post-migration removals; historical evidence is never rewritten to make a
removed spelling appear unreleased.

Legacy Pigsty operational mappings remain explicit:

| Existing surface | Canonical Pig command |
|:---|:---|
| `bin/inventory_load` | `pig inv cmdb load` |
| `bin/inventory_cmdb` | `pig inv cmdb enable` |
| `bin/inventory_conf` | `pig inv cmdb disable` |
| applying `files/cmdb.sql` | `pig inv cmdb init` |
| querying `pigsty.raw_config` | `pig inv cmdb dump` |
| `make cmdb` | `pig inv cmdb load` then `pig inv cmdb enable` |

The adjacent scripts remain operational parity references until a separate
Pigsty-repository change removes them after proven CLI parity.


## 10. Code ownership

Repository layout follows `AGENTS.md`:

- flat `cmd/inventory.go` owns canonical Cobra glue;
- `cmd/root.go` owns only root setup, global flags, and registration;
- `cmd/sty.go` retains only unrelated Sty commands and contains no Inventory
  edit, validate, or Check command construction;
- the top-level `inventory` package owns static parsing, lossless Document,
  semantic values, selectors, mutations, validation, and value-free diff;
- `cli/inventory` owns Inventory, Validate, Check, and CMDB orchestration plus
  their safe command DTOs;
- `cli/sty` MAY consume the top-level lossless `inventory` package for
  Configure or Grafana, but MUST NOT own root Inventory command use cases;
- `internal/config` owns value-free source discovery;
- one small `internal/cmdb` adapter owns PostgreSQL I/O and the two guarded local
  source-file operations.

Command files define names, aliases, annotations, flags, argument validation,
and option mapping only. Business logic MUST NOT grow in `cmd`. Canonical and
compatibility paths share constructors/helpers and one use case. The CMDB
adapter MUST NOT duplicate the YAML parser, shell out to legacy scripts as its
final implementation, or introduce a generic backend framework.

The public API in this contract is the CLI/Result contract. No semver-stable
external Go API is promised.


## 11. Delivery plan and explicit non-goals

### P0 — Contract and correctness before release

- make this document authoritative and remove stale Snapshot/source-state
  promises from Sty;
- add source descriptor and `inventory status` without read-only config writes;
- document and implement the deliberate correction that a relative persistent
  `~/.pig/config.yml` Inventory resolves against Pigsty home rather than cwd;
- remove unshipped Sty duplicate namespaces and retire
  `sty edit|e`, `sty validate|val|v`, and `sty check|ck` after their root
  Inventory paths ship;
- remove the unreleased `--data` alias and freeze its absence in help tests;
- correct root Result kinds and canonical path DTOs;
- correct canonical hyphenated group validation;
- add minimal Kafka and Mongo/Ferret identity validation;
- replace count-only adjacent-corpus drift with an actionable relative-path
  manifest and diagnostic baseline;
- implement original/candidate diagnostic-delta behavior for fragment repair.

### P1 — CMDB safety closure

- add `cmdb check`;
- add standard plans and conditional confirmations;
- add full validation and digest-based change detection to Load and reuse the
  P2 diff primitive for path-level drift joins;
- add Dump idempotence and overwrite protection;
- add exact managed-file and `ansible.cfg` guards;
- prove exact baseline/script/table/query parity in disposable PostgreSQL.

### P2 — Bounded usability

- add `list --depth`;
- expose the bounded Ansible timeout;
- add `edit --from FILE|-`;
- add static value-free `diff` and reuse it for CMDB plans.

### P3 — Evidence-gated only

- known-CMDB source-aware read-only list/validate;
- read-only directory Inventory after provenance and Ansible differential tests;
- typed set/unset only if real automation demonstrates fragment replacement is
  insufficient;
- redacted show only after a complete secret-classification contract.

This contract explicitly excludes revision/history/rollback/reset, three-way
merge, generic source plugins, arbitrary dynamic execution, effective-vars
rendering, format/whole-document marshal, implicit `load+enable` sync, direct
CMDB CRUD, Inventory template generation, and unused legacy profile/name flags.
No excluded feature lands merely because old experimental code or documentation
mentions it.


## 12. Acceptance gates

The contract is complete only when the relevant phase proves:

### 12.1 Static and source gates

- help/golden tests freeze the exact tree, aliases, flags, and absence of alias
  collisions with `install|i` and `sty list`;
- every source tier and relative base has deterministic tests;
- missing, directory, executable, managed CMDB, and third-party source states
  produce value-free typed status/findings;
- read-only source discovery performs no file write, process execution, or
  network/database access;
- parse/no-op and untouched source regions remain byte-identical;
- selectors cover dotted/IP/quoted keys, indexes, nested groups, and pointer
  escapes;
- edit conflict, incremental repair, private recovery, symlink refusal, mode
  tightening, and `--from -` tests pass;
- structured paths are canonical strings and no numeric kind is the only path
  representation.

### 12.2 Validation and corpus gates

- representative pinned fixtures freeze each native rule and intentional legacy
  divergence;
- current adjacent Pigsty `conf/**/*.yml` plus active `pigsty.yml` have no native
  errors and pass installed `ansible-inventory` when that oracle is available;
- the optional corpus manifest reports added and removed relative paths rather
  than only a count mismatch;
- canonical hyphenated Pigsty groups pass strict validation;
- invalid Kafka and Mongo/Ferret identities have focused negative fixtures;
- vault/tag values remain opaque and produce `UNVERIFIABLE`, not false values or
  secret-bearing diagnostics.

### 12.3 CMDB gates

- Init applies the exact selected `cmdb.sql` and verifies existing objects;
- Load produces the same five declaration tables as `inventory_load` for every
  CMDB-representable input; unrepresentable inputs fail closed with recorded
  divergence codes, and any failed replacement rolls back completely;
- OS-default/package-map behavior matches the script and partial warnings are
  explicit;
- every plan is non-mutating, value-free, deterministic, and has a replayable
  safe next action or an explicit same-target instruction when `-d` cannot be
  reproduced safely;
- no-op Load needs no confirmation; changed structured/non-interactive Load
  fails closed without `--yes`;
- Dump reparses to equivalent declarations, is byte-idempotent, and refuses an
  unforced differing destination;
- Check validates schema, raw config, dynamic JSON, optional drift, and managed
  local source state;
- Enable/Disable are idempotent and never overwrite a third-party script or
  Inventory directive;
- managed-source fixtures cover Pig's exact form, the legacy single-token form,
  safe replacement of a password-bearing legacy token, and actionable refusal
  of a legacy space-containing conninfo outside the bounded grammar;
- no corrected command creates or writes a `pigsty_cmdb` schema.

### 12.4 Security, compatibility, and repository gates

- no Inventory value, YAML/SQL/script body, DSN, password, or adapter stdout
  enters safe Results, plans, diagnostics, next actions, or logs;
- `sty edit|e`, `sty validate|val|v`, and `sty check|ck` are absent after
  migration to the root command;
- a pinned v1.6.0 command-tree fixture preserves historical evidence and a
  separate current-tree assertion gates every compatibility removal;
- legacy script parity and any later wrapper removal are reviewed separately in
  the Pigsty repository;
- focused tests, PostgreSQL integration tests, proportional race tests,
  `go test ./...`, `go vet ./...`, and `git diff --check` pass;
- Pig, Pigsty, and Pigsty documentation describe the same source and CMDB
  limitations without claiming audit history or time travel that the schema
  does not implement.


## 13. Adversarial review record

Contract v1.3 received two read-only review rounds in one Claude Code session:

- Claude Code: `2.1.207`;
- model: `claude-fable-5` (`fable` alias), effort `max`;
- session: `31360fd5-e829-43c1-a721-37fb4685dadd`;
- scope: this contract, Inventory/CMDB clauses in `sty.md`, current Pig
  implementation, the local v1.6.0 tag, and the adjacent Pigsty CMDB, Kafka,
  Ferret, Ansible, and configuration facts.

Round 1 returned **REJECTED** with four contract blockers:

1. P1 plans depended on the P2 path-diff primitive;
2. exact-only managed-script recognition would reject every legacy
   `bin/inventory_cmdb` deployment;
3. the reviewer could not verify the v1.6.0 surface without Bash and therefore
   challenged the namespace-removal premise;
4. CMDB representability and the safer fail-closed divergence from the Python
   loader were not explicit.

The revision moved P1 to digest-based change detection, bounded the managed
shape set while retaining actionable refusal, defined representability and
script divergences, unified directive parsing and validation layers, and pinned
compatibility removal to the exact v1.6.0 command-tree fixture. Local tag
evidence at commit `2c824971d85b0c6b9d9030832cac360948899c33` resolved the
third finding: only Sty edit/validate were registered and no root Inventory,
`sty inventory`, or `sty cmdb` command was present.

Round 2 re-read the revised files, accepted the tag evidence, found no remaining
MUST_FIX item, and returned **CONVERGED**. The consensus boundary is sections
1–12 of this document, including all standing exclusions; the review does not
authorize revision/history, a generic source framework, effective-vars work, or
any other deferred feature.

Post-review maintainer amendments: contract v1.4 retired
`pig sty validate|val|v`; contract v1.5 also retired `pig sty edit|e` and moved
Check to the root-only `pig inventory check|ck` path. These are explicit
compatibility decisions, not claims that the Sty edit/validate commands were
never released; the pinned v1.6.0 fixture remains unchanged as historical
evidence.

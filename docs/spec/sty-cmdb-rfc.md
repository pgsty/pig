---
title: "pig sty CMDB RFC"
description: "Proposed native CMDB design for the experimental P2 gate"
weight: 151
icon: fas fa-database
module: [PIG]
categories: [RFC]
---

# `pig sty cmdb` Native Control Plane RFC

Status: **PROPOSED — owner approval required before implementation**

RFC version: **1**

Last updated: **2026-07-17**

Gate: **P2 RFC only**. This document authorizes no CMDB mutation code by
itself. The owner must explicitly approve the decisions and destructive test
plan in this RFC before implementation starts.

The sole normative parent contract is [`sty.md`](sty.md). If this RFC conflicts
with that contract, `sty.md` wins and this RFC must be amended before code is
written.


## 1. Decision summary

The proposed P2 implementation has these fixed boundaries:

1. Native CMDB data lives in a new `pigsty_cmdb` PostgreSQL schema. It never
   creates, drops, truncates, or rewrites the legacy `pigsty` or `pglog`
   schemas.
2. The database stores immutable, declaration-oriented semantic Snapshots,
   not a lossy reconstruction of YAML source text and not Ansible effective
   variables.
3. One singleton head points to an append-only revision chain. Every write uses
   a transaction, a stable advisory lock, and compare-and-swap against the
   exact planned head token.
4. Static `load` records both the exact source-byte digest and an immutable
   semantic merge base. `dump --merge` is a real three-way merge over that
   base; absence or mismatch of the base is a conflict.
5. Plain `dump` emits canonical generated YAML with a machine-readable marker.
   It never claims to restore comments, alignment, scalar lexemes, anchors, or
   other source presentation that was not stored.
6. Ansible source switching edits only one unambiguous literal `inventory =`
   value inside one `[defaults]` section. It uses an exact-byte backup,
   revision check, sibling temporary file, flush, atomic rename, and parent
   directory sync.
7. An internal, hidden Ansible inventory adapter emits declaration JSON from
   the current Snapshot. It is not a new public command promise and it never
   computes effective variables.
8. CMDB v1 accepts only values that can be represented without semantic loss
   by both canonical YAML and Ansible inventory JSON. Unsupported tags,
   vault/tagged values, aliases/anchors, non-string mapping keys, non-finite
   numbers, or conflicting duplicate-host declarations fail closed before any
   database write.
9. Reset, rollback, restore, and any load that removes declarations are planned
   operations with explicit confirmation. A scope flag such as `--reset` or
   `--prune` never implies consent.
10. P2 remains experimental until every PostgreSQL-container and file-switch
    acceptance test in section 15 passes.


## 2. Evidence and legacy boundary

The design was checked against the adjacent Pigsty tree at revision
`cd5d3f62ba30a1e63ad8ae29ea87ef3e48b4d759`. The relevant files were clean at
review time.

| Evidence | Observed behavior | Native decision |
|:---|:---|:---|
| `files/cmdb.sql` | begins with `DROP SCHEMA IF EXISTS pigsty CASCADE` and also owns unrelated legacy views/types | never execute or translate it; use separate `pigsty_cmdb` migrations |
| `bin/inventory_load` | truncates group/host/variable tables before reloading; stores only a narrowed top-level/INET model | append an exact semantic Snapshot in one transaction; preserve nested groups and string host names |
| `bin/inventory_cmdb` | writes an executable query script, then applies a broad `sed` replacement to `ansible.cfg` | prebuild a bounded native adapter and patch one verified value span only |
| `bin/inventory_conf` | broadly rewrites every matching `inventory` line to `pigsty.yml` | restore a recorded, validated prior literal source through the same narrow patch contract |
| `Makefile` `cmdb` target | chains the destructive loader and source switch | do not wrap, call, or silently retarget it during experimental P2 |

The native implementation MUST NEVER execute:

- `files/cmdb.sql`;
- `bin/inventory_load`;
- `bin/inventory_cmdb`;
- `bin/inventory_conf`.

It also does not automatically import a legacy database. The supported initial
intake is a validated static Inventory or a native backup bundle. Existing
legacy schemas and dashboards may coexist, but they are not a native source of
truth.


## 3. Non-goals

P2 does not:

- manage applications or application Docker/Compose state;
- add an unspecialized `sty docker` command;
- implement Vagrant or Terraform;
- become a general INI editor, YAML round-trip serializer, database UI, or
  replacement for Ansible precedence;
- read `group_vars`, `host_vars`, role defaults, dynamic plugins, or vault
  plaintext to synthesize effective variables;
- expose arbitrary SQL, accept arbitrary executable Inventory sources, or
  populate legacy operational/job/default-variable tables;
- promise comment, whitespace, ordering, style, or scalar-lexeme recovery from
  a plain CMDB dump;
- perform an in-place `dump --merge` in v1.


## 4. Vocabulary and source identity

### 4.1 Static Document

A static Document is the existing lossless `inventory.Document`: exact bytes,
source ranges, semantic declarations, and a SHA-256 revision of the original
bytes. It remains the only authority for source-preserving edits.

### 4.2 Semantic Snapshot

A Snapshot is an immutable, source-presentation-free representation of one
complete declaration Inventory. It preserves:

- the `all` group;
- nested `children` relationships;
- group declarations and group variables;
- string host names and host declaration variables;
- mapping keys and scalar types;
- sequence order.

It does not contain comments, byte ranges, alignment, quote style, flow/block
style, or raw scalar lexemes. It never contains computed effective variables.

### 4.3 Source descriptor

Resolution returns a descriptor rather than an untyped path:

```text
kind:       static | cmdb | unsupported
origin:     cli | environment | user-config | ansible-config | home-default
locator:    canonical local path or verified native adapter identity
persistent: true only for the ansible.cfg directive
```

`-i/--inventory` is always a concrete static source for one invocation and
never persists a switch. A verified native adapter is the only executable that
classifies as `cmdb`; directories, comma-separated source lists, inventory
plugins, and arbitrary executables classify as `unsupported` and fail with a
manual diagnostic.


## 5. Canonical Snapshot v1

### 5.1 Typed encoding

The stored payload kind is `pig.inventory.snapshot/v1`. It uses typed JSON with
arrays for mappings so the application, not PostgreSQL's JSON renderer, defines
canonical bytes. Conceptually:

```json
{"kind":"pig.inventory.snapshot/v1","root":{"t":"map","v":[{"k":"all","v":{"t":"map","v":[]}}]}}
```

Value nodes are:

```text
map:    {"t":"map", "v":[{"k":<string>,"v":<value>}, ...]}
seq:    {"t":"seq", "v":[<value>, ...]}
string: {"t":"str", "v":<UTF-8 string>}
bool:   {"t":"bool","v":true|false}
int:    {"t":"int", "v":<canonical base-10 string>}
float:  {"t":"float","v":<canonical finite decimal string>}
null:   {"t":"null"}
```

Mapping entries are sorted by UTF-8 key bytes. Sequence order is preserved.
Canonical JSON uses no insignificant whitespace, a fixed object-field order,
and standard JSON string escaping. The semantic digest is:

```text
SHA-256(canonical Snapshot v1 JSON bytes)
```

This means presentation-only mapping reordering does not create semantic
drift. Sequence reordering does.

### 5.2 Representability gate

Before `load` can plan a write, the static Document must pass native Inventory
validation and the CMDB v1 representability gate. V1 rejects:

- a custom/tagged or vault value;
- an anchor, alias, or YAML merge key;
- a non-string mapping key;
- a duplicate mapping key;
- an unknown scalar kind or non-finite float;
- nesting or byte size beyond the existing bounded Inventory limits;
- duplicate declarations of one host name whose host-variable maps are not
  semantically identical.

The stable diagnostic is `CMDB_SNAPSHOT_UNREPRESENTABLE` with a source path and
reason only. It never includes the rejected value. The database remains
untouched.

This is deliberately narrower than static Inventory support. A user with vault
or tagged values keeps the static workflow; P2 must not silently flatten those
values into strings.

### 5.3 Ansible declaration projection

The internal adapter projects a Snapshot to ordinary Ansible inventory JSON:

- group `hosts`, `children`, and `vars` remain declarations;
- `all.vars` remains under the `all` group;
- host variables are emitted once through `_meta.hostvars`;
- repeated identical host declarations collapse to one hostvars entry;
- no group/global value is merged into hostvars by Pig.

The projection and reverse test fixture must have the same semantic digest as
the Snapshot. Pig never invokes Ansible to calculate an effective result.


## 6. PostgreSQL schema and migrations

### 6.1 Schema ownership

The native schema name is exactly `pigsty_cmdb`. Migration v1 owns only:

- `pigsty_cmdb.schema_migration`;
- `pigsty_cmdb.snapshot`;
- `pigsty_cmdb.state`;
- immutable-snapshot enforcement functions/triggers inside that schema.

It revokes access from `PUBLIC`. It creates no login role and stores no
credential. Role grants are explicit migration inputs and are never inferred
from the legacy schema.

### 6.2 Migration ledger

Migrations are embedded, ordered application assets with:

```text
version, name, SHA-256(sql bytes), minimum PostgreSQL version, SQL
```

`schema_migration` stores the version, name, checksum, tool version, and apply
timestamp. `init`:

1. opens a bounded connection and transaction;
2. takes the stable CMDB advisory transaction lock;
3. validates an existing bootstrap schema before trusting it;
4. rejects an unknown newer version or checksum mismatch;
5. applies each pending migration exactly once in order;
6. records the checksum in the same transaction;
7. commits only after the final shape check.

Repeated `init` at the current version is a no-op. There are no automatic down
migrations. A schema with objects but no valid native ledger is foreign and is
refused unless the explicitly destructive reset flow is approved.

### 6.3 Snapshot tables

The logical v1 columns are:

```text
snapshot
  revision              bigint primary key, > 0
  parent_revision       bigint null references snapshot
  merge_base_revision   bigint not null references snapshot, deferred
  semantic_sha256       bytea not null, exactly 32 bytes
  payload               jsonb not null
  origin                static_load | rollback | restore | future_native_change
  source_kind           static | revision | backup | native
  source_locator        text null
  source_bytes_sha256   bytea null, exactly 32 bytes
  tool_version          text not null
  created_at            timestamptz not null

state
  singleton             boolean primary key, always true
  head_revision         bigint null references snapshot
  updated_at            timestamptz not null
```

The payload is re-canonicalized and its digest verified on every read. Snapshot
rows are append-only: update/delete triggers reject mutation. Reset drops only
`pigsty_cmdb` after a verified private backup.

A static load creates revision `head + 1`, with `parent_revision = old head`
and `merge_base_revision = its own revision`. A future native change inherits
the parent's merge base. Rollback creates a new head; it never moves the head
pointer backward or edits history.

### 6.4 Exact revision token

The public optimistic-concurrency token is:

```text
cmdb-v1:<decimal revision>:sha256:<64 lowercase hex characters>
```

Both the number and digest must match. Revision number alone is insufficient.
The singleton row is locked and the planned token is compared again inside the
write transaction. A mismatch returns `CMDB_REVISION_CONFLICT`; no row is
inserted and no state is changed.


## 7. Connection and secret handling

The implementation uses one focused native PostgreSQL adapter (proposed driver:
`pgx/v5`). It does not shell out to `psql` for normal operations.

Connection resolution is:

1. `--connection-file`, an owner-only bounded regular file containing one
   PostgreSQL conninfo/URI line;
2. `PIGSTY_CMDB_DSN`;
3. `service=meta`.

There is no password-bearing `--dsn` flag. A connection value never enters a
Result, Plan, log, generated adapter, error detail, process argument, or backup
name. Safe output identifies only `connection-file`, `environment`, or
`service:meta` as the source. The adapter applies connect, statement, lock, and
transaction timeouts and refuses redirects or fallback to another database.

The generated Ansible adapter uses the same protected resolver. If it requires
a connection file, its runtime record contains only that file path and verifies
owner-only permissions before each use.


## 8. Runtime adapter

`cmdb init` prepares a per-home runtime directory outside the Pigsty repository:

```text
~/.pig/cmdb/<first-16-hex-of-SHA256(absolute-Pigsty-home)>/
```

The directory is `0700`. It contains:

- `inventory`, an atomic `0700` wrapper with marker
  `pig-sty-cmdb-adapter/v1`;
- `runtime.json`, non-secret adapter identity and connection-source metadata,
  mode `0600`;
- `switch.json`, the source-switch record when enabled, mode `0600`;
- `backups/`, private source-switch and reset bundles.

The wrapper executes the absolute Pig binary recorded by `init` and only the
hidden `sty cmdb _inventory` adapter. It forwards only Ansible's exact
`--list` or `--host <name>` grammar. The hidden adapter writes raw inventory
JSON to stdout, sanitized errors to stderr, and no progress/log output. It is
not advertised in help or treated as a stable public verb.

`check` detects a moved/replaced Pig binary or altered wrapper. `enable` refuses
until `init` refreshes the adapter. `enable` itself therefore only backs up and
patches `ansible.cfg`.


## 9. Command semantics

All public CMDB commands are annotated experimental until the entire P2 gate
passes.

### 9.1 `cmdb init`

Default `init` applies pending idempotent migrations, validates the current
head, and prepares or refreshes the non-secret runtime adapter. It never loads
an Inventory and never changes `ansible.cfg`.

`init --reset`:

1. validates that the target is exactly the native schema;
2. renders a plan with migration/head/history counts and a future private
   backup path;
3. requires interactive confirmation or `--yes`;
4. streams a checksummed `0600` native backup bundle and fsyncs it;
5. in one transaction, drops only `pigsty_cmdb`, reapplies migrations, and
   optionally restores a validated bundle;
6. leaves the backup in place whether reset succeeds or rolls back.

`--reset` without confirmation performs no DDL. A foreign or malformed schema
requires the same reset flow; it is never silently adopted.

`init --restore <bundle>` is permitted only with `--reset`. It verifies the
bundle kind, format version, checksum, migration compatibility, revision chain,
and every Snapshot digest before the destructive confirmation. Restore is one
transaction.

### 9.2 `cmdb load [static-file]`

Load uses one static Document or one historical revision, never both:

```text
pig sty cmdb load [static-file]
pig sty cmdb load --revision <exact-token>
```

The flow is:

1. resolve, parse, and natively validate the complete source;
2. pass the v1 representability gate;
3. read and verify the current CMDB head and migration version;
4. compute a value-free semantic plan from head to candidate;
5. classify declaration removals;
6. if removals exist, require `--prune` to select them and confirmation/`--yes`
   to consent;
7. historical rollback always requires confirmation; if it removes
   declarations it also requires `--prune`;
8. open a serializable transaction, take the advisory lock, lock the singleton,
   and compare the exact planned head token;
9. insert one immutable Snapshot and update the singleton in that transaction;
10. re-read and verify the committed token.

An unchanged candidate is a no-op and does not allocate a revision. Any failure
after transaction start rolls back the Snapshot insert and head update
together.

Deletion means a mapping/host/group declaration present in head but absent in
the candidate, or a sequence change that removes elements. A replacement that
can discard sequence members is conservatively classified as deletion.

### 9.3 `cmdb dump`

Plain dump reads head, or an explicit historical `--revision`, verifies its
digest, and emits canonical generated YAML. The first line is an exact marker:

```text
# pig-sty-cmdb: kind=pig.inventory.generated/v1 revision=<token>
```

The serializer has a versioned fixture and fixed indentation, scalar spelling,
key ordering, and final newline. It does not emit comments or source styles.

Inventory bodies may contain credentials. Therefore:

- raw stdout is allowed only in text mode and is never wrapped in a Result;
- JSON/YAML output without `--output` is refused;
- `--output <new-file>` writes an owner-only file through private atomic
  staging and returns only safe metadata;
- the body never enters logs, error details, diagnostics, or structured data;
- an existing output is not overwritten in v1.

`dump --merge` follows section 10. It emits the lossless current static source
with non-conflicting CMDB changes patched into it and a merge marker. It never
writes in place; stdout or a distinct new `--output` is required.

### 9.4 `cmdb enable`

Enable requires:

- current migrations and a valid non-empty head;
- a current, verified runtime adapter;
- semantic parity between head and the adapter projection;
- exactly one supported static `inventory =` directive in `[defaults]`;
- a parseable source file at that directive.

It creates an exact-byte backup and `switch.json`, then replaces only the value
span with the absolute verified adapter path. It re-reads `ansible.cfg`
immediately before commit and uses sibling-temp/fsync/rename/parent-sync. An
already enabled, verified state is a no-op.

It does not execute the adapter, Ansible, or a legacy source-switch script as a
mutation step. Read-only compatibility checks occur before the patch.

### 9.5 `cmdb disable`

Disable restores the literal source recorded by `switch.json`. It first parses
and validates that static file and requires semantic parity with the current
head; presentation-only changes are allowed. It backs up current `ansible.cfg`
and patches only the verified adapter value span.

If no valid switch record exists, `--static <literal-file>` is required. This
is the safe migration path from a legacy `inventory.sh`; no legacy helper is
run. The static source must parse, validate, and match head before switching.

An already disabled state matching the record is a no-op. Drift, a missing
fallback, or an ambiguous directive fails without changing any file.

### 9.6 `cmdb check`

Check is read-only and verifies:

- connection and bounded-query behavior;
- migration ledger version/checksums and exact schema shape;
- singleton, head, parent/base lineage, and all referenced digests;
- runtime adapter identity and permissions;
- strict `ansible.cfg` source shape;
- active-source classification;
- native adapter projection parity when CMDB is active;
- static-vs-head semantic parity when static is active;
- source-switch record and backup availability;
- split-brain state and exact revision token.

It emits redacted findings and never repairs state.


## 10. Provenance-sensitive three-way merge

For head `C`, `dump --merge` obtains `B = C.merge_base_revision` and parses the
current recorded static source as `W`:

```text
B: immutable semantic Snapshot loaded from the recorded static source
W: current lossless static Document at the same canonical locator
C: current CMDB head Snapshot
```

Before diffing, all of the following must hold:

1. `B` exists and its stored payload rehashes to its recorded digest;
2. `C` is a descendant of `B` through the parent chain;
3. the head's recorded base token exactly names `B`;
4. `B` records a static source locator and exact source-byte digest;
5. the current source canonical path equals that recorded locator;
6. an optional `--expect-base` token, when supplied, exactly equals `B`.

Failure of any item is `CMDB_BASE_MISMATCH`. No two-way fallback is attempted.
A changed current byte digest is not itself a base mismatch: those changes are
the `B -> W` side of the three-way merge.

The merge computes declaration-path changes `B -> W` and `B -> C`. Mappings are
compared by key; sequences are atomic at their containing path in v1. Two
changes conflict when their paths are equal or ancestor/descendant-related,
unless both yield the same semantic value. Remove-vs-change always conflicts.

Only non-conflicting `B -> C` changes are applied to the current lossless
Document through existing source-patch primitives. A new CMDB subtree may be
rendered canonically as a fragment, but the whole file is never marshaled. The
candidate is reparsed and validated before output.

Conflict output contains paths and operation kinds only, never old/new values.
Any conflict produces no stdout body and no destination file. A successful
merge begins with:

```text
# pig-sty-cmdb: kind=pig.inventory.merge/v1 base=<B-token> head=<C-token>
```

Dump is read-only. A later `load` of the merged file creates a new static merge
base and exact source-byte provenance.


## 11. Strict `ansible.cfg` patch contract

The patcher accepts one bounded regular UTF-8 file and refuses symlinks. It
preserves LF/CRLF and all bytes outside one value span.

Supported shape:

- exactly one `[defaults]` section, compared case-insensitively;
- exactly one active `inventory` assignment in that section;
- one single-line literal relative or absolute file path;
- optional ordinary whitespace, simple matching quotes, and an inline comment.

It rejects:

- a missing or duplicated section/directive;
- an assignment outside `[defaults]` as the only candidate;
- comma-separated/multiple sources;
- a directory, inventory plugin, or arbitrary executable as an enable target or
  active supported source;
- interpolation, Jinja, shell variables, escapes, or line continuation;
- an altered/unverified native adapter;
- a concurrent exact-byte revision mismatch.

The failure gives one manual instruction and leaves the file byte-identical:

```text
edit <home>/ansible.cfg manually so [defaults] contains exactly one literal inventory = <path> directive
```

Each successful switch keeps an exact backup under the private runtime backup
directory. Backups are never uploaded, logged, or deleted automatically.

The one migration exception is `disable --static <literal-file>`: it may replace
one otherwise unsupported legacy executable literal after locating the same
strict single directive. It never opens or executes that legacy target, and the
new static source must pass validation and head-parity checks first.


## 12. Split-brain detection

The `ansible.cfg` directive is the authoritative persistent source. Higher
precedence invocation sources remain valid, but when their kind differs from
the persistent kind every semantic command emits exactly one stderr line:

```text
NOTICE: invocation static Inventory shadows persistent CMDB source from ansible.cfg; using the invocation source
```

or:

```text
NOTICE: invocation CMDB source shadows persistent static Inventory from ansible.cfg; using the invocation source
```

The notice is emitted in text and structured modes and never enters structured
stdout. Repeated internal reads during one command do not repeat it.

Lossless `show` and `edit` require a static descriptor. When CMDB is effective
they fail with a safe next action to `cmdb dump`; they never edit a generated
cache or infer a fallback static file.


## 13. Backup and rollback

Three recovery layers are distinct:

1. **Append-only history.** Every committed head remains an immutable Snapshot.
   `load --revision <token>` plans a rollback and creates a new head revision;
   it never rewrites history.
2. **Source-switch backup.** Every `ansible.cfg` patch has an exact local backup
   and a private switch record. Disable restores the recorded literal source by
   narrow patch, not by replacing unrelated file bytes.
3. **Reset bundle.** Before `init --reset`, all native migrations, state, and
   Snapshot rows stream into a versioned, checksummed `0600` bundle. A failed
   backup aborts reset. `init --reset --restore <bundle>` verifies the entire
   bundle before confirmation and restores it in one transaction.

Backup payloads can contain Inventory secrets. Paths may be reported, but
contents and scalar values never appear in Result data or logs. The tool does
not claim these bundles replace normal PostgreSQL physical/logical backups.


## 14. Result, plan, and error contract

Proposed safe payload kinds are:

```text
pig.sty.cmdb.init/v1
pig.sty.cmdb.load/v1
pig.sty.cmdb.dump/v1
pig.sty.cmdb.enable/v1
pig.sty.cmdb.disable/v1
pig.sty.cmdb.check/v1
```

Result and Plan data may contain only:

- schema and migration versions;
- exact revision tokens/digests;
- counts;
- declaration paths and operation kinds;
- source kind/origin, never a connection value;
- backup/output paths and file digests;
- redacted finding codes.

They never contain Inventory scalar values, DSNs, SQL text, adapter JSON, dump
bodies, or backup bodies. Database errors are classified and sanitized before
they reach output.

Required stable failure families are parameter, permission, dependency,
network, resource, configuration, state/conflict, operation/partial, and
confirmation. Exact MMCCNN leaf numbers are assigned with implementation and
locked by fixtures; no second envelope or exit-code system is introduced.


## 15. Destructive and conflict test plan

### 15.1 Container guard

Database tests use a disposable, pinned PostgreSQL container. The harness
generates a random database named with prefix `pig_sty_cmdb_test_`, binds only
to loopback, and passes the resulting DSN explicitly. Destructive tests refuse
to run unless both the host is loopback and the database has that prefix. They
never use ambient `service=meta`.

The container is removed in test cleanup. A sentinel schema/table outside
`pigsty_cmdb` proves every reset and migration remains in scope.

### 15.2 Required cases

| Gate | Proof |
|:---|:---|
| Migration idempotence | first `init` applies ordered checksummed migrations; second is byte/state no-op; concurrent init serializes |
| Migration fail closed | checksum drift, foreign bootstrap objects, and newer schema version do not mutate state |
| Reset confirmation | `--reset` without confirmation performs no DDL; confirmed reset creates/verifies backup first; outside sentinel survives |
| Backup restore | reset bundle round-trips every revision/digest; tampered or incompatible bundle is rejected before reset |
| Transaction rollback | an injected failure after Snapshot insert and before head update leaves neither row nor head change |
| Revision conflict | two clients plan one head; first commits; second fails exact CAS with no partial row |
| Prune confirmation | deletion requires both `--prune` and confirmation; plan/no leaves the head unchanged |
| Historical rollback | rollback creates a new child revision and retains the prior head/history |
| Snapshot fidelity | nested groups, string/non-IP hosts, scalars, maps, and sequences round-trip semantically without effective-var synthesis |
| Representability | vault/tag/anchor/alias/non-string-key/conflicting-host fixtures fail before SQL and redact values |
| Three-way merge | disjoint changes merge through source patches; equal/ancestor/remove conflicts emit paths only and no body |
| Base mismatch | missing, corrupt, non-ancestor, wrong-locator, and wrong expected base all fail without two-way fallback |
| Plain dump | canonical fixture and marker are stable; comments/styles are absent by contract; secret-bearing body never enters Result/logs |
| Source switch | valid enable/disable patches only one value span, preserves modes/CRLF/unrelated edits, and keeps exact backups |
| Switch fail closed | missing/duplicate/templated/list/directory/symlink/changed-revision fixtures remain byte-identical with manual instructions |
| Active parity | adapter JSON and Snapshot digest agree; disable rejects a semantically stale static fallback |
| Split brain | each direction emits the exact one-line notice once and keeps structured stdout valid |
| Secret hygiene | DSN, password, vault body, Inventory scalar values, adapter body, and backup body are absent from errors and structured output |

The implementation also runs proportional package tests, relevant race tests,
`go test ./...`, `go test -race ./...`, `go vet ./...`, and
`git diff --check` before P2 can close.


## 16. Post-approval implementation gates

Owner approval unlocks code in this order only:

1. **P2.1 Snapshot foundation:** typed canonical Snapshot, representability,
   semantic diff, canonical dump, and three-way merge fixtures; no DB write.
2. **P2.2 PostgreSQL adapter:** bounded connection, embedded migrations,
   immutable revision store, CAS, reset backup/restore, and container gates.
3. **P2.3 Load/dump/check:** plans, confirmation, rollback, provenance, raw-body
   separation, and split-brain diagnostics.
4. **P2.4 Source switch:** runtime adapter, strict parser, backups, atomic narrow
   patch, enable/disable, and fail-closed fixtures.
5. **P2.5 Cobra exposure:** flat `cmd/sty.go` wiring, experimental annotations,
   structured contracts, complete container acceptance, and functional commits.

No later subgate starts before the previous one passes. `go.sum` or other user
changes are never included incidentally.


## 17. Owner approval checklist

Approval should explicitly accept or amend these choices:

- [ ] separate `pigsty_cmdb` schema and no legacy-schema compatibility writes;
- [ ] singleton append-only Snapshot store and exact revision-token format;
- [ ] CMDB v1 representability limits for tags/vault/anchors/aliases;
- [ ] hidden native Ansible adapter and private per-home runtime directory;
- [ ] connection-file/environment/`service=meta` resolver with no DSN flag;
- [ ] reset bundle plus `init --reset --restore` recovery contract;
- [ ] `load --revision` as rollback without a new public verb;
- [ ] exact three-way merge and base-mismatch rules;
- [ ] strict source-switch grammar, parity requirement, and backup behavior;
- [ ] container guard and destructive/conflict test matrix.

An approval such as **“批准 P2 CMDB RFC v1，按文档进入 P2.1”** is sufficient
to begin implementation. Until then, the implementation gate remains closed.

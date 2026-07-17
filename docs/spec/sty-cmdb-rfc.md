---
title: "pig sty CMDB RFC"
description: "Proposed native CMDB design for the experimental P2 gate"
weight: 151
icon: fas fa-database
module: [PIG]
categories: [RFC]
---

# `pig sty cmdb` Native Control Plane RFC

Status: **APPROVED — P2.1 implementation authorized by owner**

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
5. Plain `dump` writes canonical generated YAML with a machine-readable marker
   to a new private file. Public stdout remains value-free. Dump never claims
   to restore comments, alignment, scalar lexemes, anchors, or other source
   presentation that was not stored.
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
- promise comment, whitespace, style, or scalar-lexeme recovery from a plain
  CMDB dump; declaration order is semantic and is preserved;
- perform an in-place `dump --merge` in v1.

Code ownership follows the parent contract and repository `AGENTS.md`:
canonical Snapshot/diff/fragment primitives belong in `inventory`; CMDB use
cases and stable DTOs belong in `cli/sty`; one focused `internal/cmdb` adapter
may own PostgreSQL and private runtime details; and all Cobra wiring remains in
the flat `cmd/sty.go`. No universal environment-driver framework is introduced.


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
- mapping declaration order, keys, and scalar types;
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

Descriptor construction keeps the exact parent-contract precedence:
`-i` first, then `PIGSTY_INVENTORY`, `~/.pig/config.yml`, `ansible.cfg`, and the
resolved-home `pigsty.yml` fallback. It never probes PostgreSQL to choose a
source and never falls through after selecting an invalid higher-precedence
source.


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

Mapping entries retain declaration order exactly; sequence order is also
preserved. Canonical JSON uses no insignificant whitespace, a fixed
object-field order, and standard JSON string escaping. The semantic digest is:

```text
SHA-256(canonical Snapshot v1 JSON bytes)
```

V1 scalar canonicalization is exact:

- integers must decode losslessly to signed or unsigned 64-bit values and use
  base-10 digits with no leading zero;
- floats must be finite IEEE-754 binary64 values and use Go
  `FormatFloat(value, 'g', -1, 64)` spelling, including distinct `-0` for
  negative zero;
- booleans are `true` or `false`, and null has no value member;
- strings retain their UTF-8 code points without Unicode normalization;
- JSON escaping does not HTML-escape characters, and digest input has no final
  newline.

This follows the parent contract's ordered semantic model: mapping or sequence
reordering changes the Snapshot digest. Comments, whitespace, quote style, and
other source-only presentation do not.

### 5.2 Representability gate

Before `load` can plan a write, the static Document must pass native Inventory
validation and the CMDB v1 representability gate. V1 rejects:

- a custom/tagged or vault value;
- an anchor, alias, or YAML merge key;
- a non-string mapping key;
- a duplicate mapping key;
- an unknown scalar kind or non-finite float;
- nesting deeper than the existing Inventory limit of 128, a static source over
  16 MiB, more than 250,000 value nodes, or canonical Snapshot/generated output
  over 64 MiB;
- duplicate declarations of one host name whose host-variable maps are not
  identical in typed value and declaration order.

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

The emitter writes JSON object members deterministically in Snapshot declaration
order. JSON object order is not an Ansible semantic guarantee, however, so
adapter JSON is never reverse-decoded to define a Snapshot revision. Projection
parity compares declared groups, relationships, host membership, typed values,
and sequence order; the ordered Snapshot remains the revision authority. Public
commands compute this projection in-process and never capture its secret-bearing
protocol body in a Result or log. Integration fixtures execute the hidden
adapter only inside a private harness and discard the body after comparison.
Pig never invokes Ansible to calculate an effective result.

### 5.4 Canonical generated YAML

Plain dump traverses the ordered Snapshot directly; it does not marshal a
`Document` or generic YAML tree. The v1 serializer uses two-space block
indentation, retains mapping declaration order, renders empty maps/sequences as
`{}`/`[]`, double-quotes every string key and value with JSON-compatible
escapes, and emits lowercase booleans and `null`. Integers use the spelling from
section 5.1. Floats use that canonical spelling but append `.0` when it contains
no decimal point or exponent, so a value such as float `1.0` cannot reparse as
an integer. The generated YAML has no YAML directive or document marker and has
exactly one final LF.

Before a dump destination is committed, Pig reparses the generated YAML,
rebuilds a Snapshot, and requires its semantic digest to equal the selected
revision. This round-trip check is separate from the output-file digest and
fails without leaving a destination file.


## 6. PostgreSQL schema and migrations

### 6.1 Schema ownership

The native schema name is exactly `pigsty_cmdb`. Migration v1 owns only:

- `pigsty_cmdb.schema_migration`;
- `pigsty_cmdb.snapshot`;
- `pigsty_cmdb.state`;
- append-only ledger/Snapshot enforcement functions and triggers inside that
  schema.

It revokes access from `PUBLIC`. It creates no login role and stores no
credential. V1 grants no role beyond the schema owner; later grant management
requires a contract extension and is never inferred from the legacy schema.

### 6.2 Migration ledger

Migrations are embedded, ordered application assets with:

```text
version, name, SHA-256(sql bytes), minimum PostgreSQL version, SQL
```

Migration v1 supports PostgreSQL 14 through 18 with one schema and no
major-version conditionals. PostgreSQL 14 is the minimum accepted server; an
older server fails before DDL. The complete destructive/conflict gate runs on
the pinned PostgreSQL 18 image in section 15, and P2.2 also pins an official
`postgres:14.23-alpine` manifest for migration, load/dump, and reset/restore
compatibility cases.

The stable lock is `pg_advisory_xact_lock(bigint)` with the signed two's-
complement value of the first eight big-endian bytes of
`SHA-256("pig.sty.cmdb/v1")`: `5644146564720209289`. A fixture freezes the
derivation and integer. SQL uses only fixed, schema-qualified identifiers.
Values are parameters, never SQL string interpolation.

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
migrations. Any pre-existing schema without a valid native ledger, even an
empty schema, is foreign and is always refused with manual DBA backup/removal
instructions. V1 does not claim it can create a trustworthy native backup of
unknown objects.

### 6.3 Snapshot tables

The logical v1 columns are:

```text
snapshot
  revision              bigint primary key, > 0
  parent_revision       bigint null references snapshot
  merge_base_revision   bigint not null references snapshot, deferred
  semantic_sha256       bytea not null, exactly 32 bytes
  payload               jsonb not null
  origin                static_load | rollback
  source_kind           static | revision
  source_locator        text null
  source_bytes_sha256   bytea null, exactly 32 bytes
  tool_version          text not null
  created_at            timestamptz not null

state
  singleton             boolean primary key, always true
  head_revision         bigint null references snapshot
  updated_at            timestamptz not null
```

The payload is re-canonicalized and its digest verified on every read. Exact
shape invariants are: `state` has one row; an empty head has no Snapshot rows;
otherwise revisions are contiguous from 1 through `head_revision`, the first
parent is null, every later parent is the preceding revision, and the head is
the maximum revision. Every merge base is an ancestor no newer than its row and
resolves to a static-load row with a non-null canonical locator and exact
source-byte digest. Shape checking also rejects missing/orphan rows, invalid
origin/source combinations, and semantic digest mismatch.

Snapshot rows are append-only: update/delete triggers reject mutation. Reset
drops only `pigsty_cmdb` after a verified private backup.

A static load creates revision `head + 1`, with `parent_revision = old head`
and `merge_base_revision = its own revision`; its source locator is the
canonical absolute static path and its byte digest is required. Rollback stores
the exact target revision token as its source locator, has no source-byte
digest, inherits the current head's merge base rather than the target's older
base, and creates a new child revision. It never moves the head pointer backward
or edits history. The revision allocator requires `head_revision` to equal the
maximum stored revision; shape validation fails closed if that invariant is
broken. Restore reinstates an already verified chain exactly and does not
synthesize a restore revision.

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

1. `--connection-file`, a symlink-free regular file owned by the effective user,
   with no group/other permission bits, at most 64 KiB, and containing one
   PostgreSQL conninfo/URI line;
2. `PIGSTY_CMDB_DSN`;
3. `service=meta`.

There is no password-bearing `--dsn` flag. A connection value never enters a
Result, Plan, log, generated adapter, error detail, process argument, or backup
name. Safe output identifies only `connection-file`, `environment`, or
`service:meta` as the source. The adapter applies connect, statement, lock, and
transaction timeouts. It records the connected database name and server
identity after the first connection, verifies them on later pooled connections,
and refuses a redirect or fallback to a different target.

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
- `switch.json`, the latest source-switch intent, retained until superseded and
  mode `0600`;
- `backups/`, private source-switch and reset bundles.

`runtime.json` records the complete canonical home identity as well as the
short directory key. A short-key collision or home mismatch fails closed; it
never reuses another home's adapter directory.

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

1. proves that the target schema is absent or is a fully readable native schema
   with a valid migration ledger and revision chain;
2. renders a plan with migration/head/history counts and, for an existing
   native schema, a future private backup path;
3. requires interactive confirmation or `--yes`;
4. opens a serializable transaction, takes the advisory lock, and rechecks the
   exact planned schema/head state;
5. for an existing native schema, streams its ledger, state, and Snapshot rows
   from that transaction into a checksummed `0600` bundle and fsyncs it;
6. in the same transaction, drops only an existing `pigsty_cmdb`, reapplies
   migrations, and optionally restores a validated bundle;
7. leaves any completed backup in place whether reset succeeds or rolls back.

`--reset` without confirmation performs no DDL. A foreign or malformed schema
is not reset even with `--reset`; it remains untouched with manual DBA
instructions and is never silently adopted.

`init --restore <bundle>` is permitted only with `--reset`. It verifies the
bundle kind, format version, checksum, migration compatibility, revision chain,
and every Snapshot digest before the destructive confirmation, then rechecks
the exact bundle bytes after taking the lock and before DDL. Restore into an
absent schema is supported and has no pre-reset backup; restore into a verified
native schema follows the mandatory backup flow above. Restore is one
transaction.

### 9.2 `cmdb load [static-file]`

Load uses one static Document or one historical revision, never both:

```text
pig sty cmdb load [static-file]
pig sty cmdb load --revision <exact-token>
```

`--revision` is mutually exclusive with a positional file and the global
`-i/--inventory` override; ambiguity is a parameter error before any database
connection.

With no positional file, static load uses the normal source resolver but
requires the result to be static. If CMDB or an unsupported source resolves, it
fails with a safe instruction to name a static file; it never reads its own head
as a new static load. A relative explicit file is canonicalized from the
caller's working directory and records that absolute locator.

The candidate is either the parsed static Document or the payload of the exact
historical revision after its digest and lineage are verified. The common flow
is:

1. obtain the candidate through exactly one of those two paths;
2. natively validate a static source and pass the v1 representability gate, or
   fully verify a historical Snapshot before using it;
3. read and verify the current CMDB head and migration version;
4. compute a value-free semantic plan from head to candidate;
5. classify declaration removals;
6. if removals exist, require `--prune` to select them and confirmation/`--yes`
   to consent;
7. historical rollback always requires confirmation; if it removes
   declarations it also requires `--prune`;
8. open a serializable transaction, take the advisory lock, lock the singleton,
   and compare the exact planned head token (or require SQL `NULL` for the
   planned empty-head state);
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
digest inside one read-only repeatable-read transaction, and emits canonical
generated YAML. The first line is an exact marker:

```text
# pig-sty-cmdb: kind=pig.inventory.generated/v1 revision=<token>
```

The serializer follows section 5.4 and has a versioned fixture. In particular,
key order is stored declaration order. It does not emit comments or source
styles.

Inventory bodies may contain credentials. Therefore:

- `--output <new-file>` is mandatory in every public output mode;
- the new owner-only file is written through private atomic staging;
- stdout returns only safe Result metadata such as revision, counts, output
  path, and digest;
- the body never enters logs, error details, diagnostics, or structured data;
- an existing output is not overwritten in v1.

`dump --merge` follows section 10. It emits the lossless current static source
with non-conflicting CMDB changes patched into it and a merge marker. It never
writes in place; a distinct new `--output` is required. Merge always uses the
current head and recorded static locator, so `--merge` is mutually exclusive
with historical `--revision` and the global `-i/--inventory` override.

### 9.4 `cmdb enable`

Enable requires:

- current migrations and a valid non-empty head;
- a current, verified runtime adapter;
- semantic parity between head and the adapter projection;
- exactly one supported static `inventory =` directive in `[defaults]`;
- a parseable and representable source file at that directive whose ordered
  semantic digest equals the current head.

It creates an exact-byte backup and writes/fsyncs `switch.json` containing the
before/after file revisions, prior literal and canonical static locator,
verified head token, adapter identity, operation kind, and backup path; no
Inventory values are recorded. It then replaces only the value span with the
absolute verified adapter path. It re-reads
`ansible.cfg` immediately before commit and uses
sibling-temp/fsync/rename/parent-sync. An already enabled, verified state is a
no-op.

It does not execute the adapter, Ansible, or a legacy source-switch script as a
mutation step. Read-only compatibility checks occur before the patch.

### 9.5 `cmdb disable`

Disable restores the literal source recorded by `switch.json`, unless an
explicit `--static <literal-file>` selects a new fallback. It first parses,
validates, and passes the representability gate for that static file, then
requires ordered semantic parity with the current head; source-only changes
such as comments or scalar quote style are allowed. It backs up current
`ansible.cfg` and patches only the verified adapter value span.

If no valid switch record exists, `--static <literal-file>` is required. This
is also the safe migration path from a legacy `inventory.sh`; no legacy helper
is run. The same validation, representability, and head-parity checks apply.

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

For head `C`, `dump --merge` holds the shared form of the stable advisory lock
inside one read-only repeatable-read transaction, obtains
`B = C.merge_base_revision`, and parses the current recorded static source as
`W`:

```text
B: immutable semantic Snapshot loaded from the recorded static source
W: current lossless static Document at the same canonical locator
C: current CMDB head Snapshot
```

Before diffing, all of the following must hold:

1. `B` exists and its stored payload rehashes to its recorded digest;
2. `C` is a descendant of `B` through the parent chain;
3. `C.merge_base_revision` exactly equals `B.revision`, and `B`'s verified
   digest derives the exact base token;
4. `B` records a static source locator and exact source-byte digest;
5. the current source canonical path equals that recorded locator;
6. the current source parses, validates, and passes the v1 representability
   gate without exposing values;
7. an optional `--expect-base` token, when supplied, exactly equals `B`.

Failure of provenance item 1–5 or 7 is `CMDB_BASE_MISMATCH`. Item 6 returns its
value-free validation or `CMDB_SNAPSHOT_UNREPRESENTABLE` diagnostic. Neither
case attempts a two-way fallback. A changed current byte digest is not itself a
base mismatch: those changes are the `B -> W` side of the three-way merge.

The merge computes declaration-path changes `B -> W` and `B -> C`. Mapping
values are compared by key. A changed relative order among keys that survive
from `B` is a separate atomic `reorder` at the containing path. An added key
carries its exact gap between surviving base-key anchors, so addition does not
implicitly reorder the whole mapping; two sides adding into the same gap, or
one side removing an anchor used by the other, conflict. Sequences and explicit
mapping reorders are atomic at their containing path in v1. Two changes also
conflict when their paths are equal or ancestor/descendant-related, unless both
yield the same semantic value and order. Remove-vs-change always conflicts.

Only non-conflicting `B -> C` changes are applied to the current lossless
Document through existing source-patch primitives. A new CMDB subtree may be
rendered canonically as a fragment, but the whole file is never marshaled. The
candidate is reparsed and validated before output. An ordered move must use
exact non-overlapping entry spans; if comment attachment or source shape makes
that unsafe, merge fails rather than normalizing the containing document.

Conflict output contains paths and operation kinds only, never old/new values.
Any conflict produces no destination file. A successful merge file begins
with:

```text
# pig-sty-cmdb: kind=pig.inventory.merge/v1 base=<B-token> head=<C-token> source-sha256=<W-bytes-sha256>
```

The output has exactly one leading Pig CMDB marker. An existing recognized
generated/merge marker is replaced by a narrow comment-span patch; unrelated
leading comments remain byte-identical. Pig re-reads the source immediately
before committing the distinct destination and rejects an exact-byte revision
mismatch. The shared database lock remains held through destination commit, so
native writers cannot change `C` mid-dump.

Dump is read-only. A later `load` of the merged file creates a new static merge
base and exact source-byte provenance.


## 11. Strict `ansible.cfg` patch contract

The patcher accepts one bounded regular UTF-8 file and refuses symlinks. It
preserves LF/CRLF, the existing permission bits, and all bytes outside one
value span. The exact backup keeps the original mode. Atomic replacement
ownership follows the writing process, is reported when it differs from the
pre-patch owner, and is never silently presented as preserved ownership.

Supported shape:

- exactly one `[defaults]` section, compared case-insensitively;
- exactly one active `inventory` assignment in that section;
- one single-line literal relative or absolute file path;
- optional ordinary whitespace, simple matching quotes, and an inline comment.

Relative Inventory literals resolve against the directory containing
`ansible.cfg`, never the process working directory. The patch span includes any
existing quotes around the value. Pig chooses an unambiguous quoted or unquoted
literal for the replacement and reparses it to the same canonical path before
commit; a path that cannot be represented by this grammar is refused. The
switch record retains both the exact prior literal and its resolved canonical
path.

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

The backup and a complete before/after intent are written and directory-synced
before the `ansible.cfg` rename. Cross-file atomicity is not claimed. After an
interruption, current config bytes must match exactly either the intent's
`before` revision (pending/not applied) or `after` revision (applied); any third
state is drift and fails closed. Repeating the same explicitly requested
operation may finish a matching pending intent. `check` only reports pending,
applied, or drift and never repairs it. A superseded intent is retained beside
its exact backup, so disable does not destroy the evidence required to recover
an earlier switch.

The one migration exception is `disable --static <literal-file>`: it may replace
one otherwise unsupported literal only when that literal resolves exactly to
`<resolved-Pigsty-home>/inventory.sh`, the path written by the reviewed legacy
helper, after locating the same strict single directive. Classification uses
the resolved literal only; Pig never stats, opens, or executes that legacy
target. Any other executable source requires the manual instruction. The new
static source must pass validation and head-parity checks first.


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
they fail with a safe next action to `cmdb dump --output <new-file>`; they never
edit a generated cache or infer a fallback static file.


## 13. Backup and rollback

Three recovery layers are distinct:

1. **Append-only history.** Every committed head remains an immutable Snapshot.
   `load --revision <token>` plans a rollback and creates a new head revision;
   it never rewrites history.
2. **Source-switch backup.** Every `ansible.cfg` patch has an exact local backup
   and a private switch record. Disable restores the recorded literal source by
   narrow patch, not by replacing unrelated file bytes.
3. **Reset bundle.** Before resetting an existing verified native schema, its
   migration ledger, state, and Snapshot rows stream into a versioned,
   checksummed `0600` bundle. A failed backup aborts reset. An absent-schema
   restore has nothing to back up. `init --reset --restore <bundle>` verifies
   the entire bundle before confirmation and restores it in one transaction.

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

The implementation reuses one tested secret-key classifier for password,
secret, token, API-key, private-key, and credential names. It does not create a
CMDB-only redaction vocabulary. Value-free DTOs remain the primary defense;
classification is a second boundary for diagnostics and external errors.

Required stable failure families are parameter, permission, dependency,
network, resource, configuration, state/conflict, operation/partial, and
confirmation. Exact MMCCNN leaf numbers are assigned with implementation and
locked by fixtures; no second envelope or exit-code system is introduced.


## 15. Destructive and conflict test plan

### 15.1 Container guard

Database tests use a disposable PostgreSQL 18 container pinned to the reviewed
official multi-platform reference
`postgres:18.4-alpine@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15`.
Changing that manifest digest is an explicit review change. The harness also
records the selected platform digest. It generates a random database named with
prefix `pig_sty_cmdb_test_`, binds only
to loopback, and passes the resulting DSN explicitly. Destructive tests refuse
to run unless both the host is loopback and the database has that prefix. They
never use ambient `service=meta`.

The container is removed in test cleanup. Sentinel rows in pre-created
`pigsty`, `pglog`, and an unrelated third schema prove every reset and migration
remains in scope and never treats the legacy names as native state.

### 15.2 Required cases

| Gate | Proof |
|:---|:---|
| Migration idempotence | first `init` applies ordered checksummed migrations; second is byte/state no-op; concurrent init serializes |
| Migration fail closed | checksum drift, foreign bootstrap objects, and newer schema version do not mutate state |
| Reset confirmation | `--reset` without confirmation performs no DDL; confirmed reset of a verified native schema creates/verifies backup first; outside sentinel survives |
| Backup restore | reset bundle round-trips every revision/digest into both fresh and verified-native targets; tampered or incompatible bundles and foreign/malformed targets are untouched |
| Transaction rollback | an injected failure after Snapshot insert and before head update leaves neither row nor head change |
| Revision conflict | two clients plan one head; first commits; second fails exact CAS with no partial row |
| Prune confirmation | deletion requires both `--prune` and confirmation; plan/no leaves the head unchanged |
| Historical rollback | rollback creates a new child revision and retains the prior head/history |
| Snapshot fidelity | nested groups, string/non-IP hosts, typed scalars, ordered maps, and sequences round-trip semantically without effective-var synthesis; reordering changes the digest |
| Canonical YAML | ambiguous strings remain strings; integral floats remain floats; generated output reparses to the exact selected Snapshot digest |
| Representability | vault/tag/anchor/alias/non-string-key/conflicting-host fixtures fail before SQL and redact values |
| Three-way merge | disjoint changes merge through source patches; equal/ancestor/remove conflicts emit paths only and no body |
| Base mismatch | missing, corrupt, non-ancestor, wrong-locator, and wrong expected base all fail without two-way fallback |
| Plain dump | canonical fixture and marker are stable; comments/styles are absent by contract; `--output` is mandatory, mode is `0600`, existing/symlink destinations are refused, and the body never enters Result/logs |
| Source switch | enable rejects static/head drift; valid enable/disable patches only one value span, preserves modes/CRLF/unrelated edits, and keeps exact backups |
| Switch fail closed | missing/duplicate/templated/list/directory/symlink/changed-revision fixtures remain byte-identical with manual instructions |
| Source resolution | each precedence tier is covered; an invalid selected tier never falls through; relative `ansible.cfg` literals resolve from its directory |
| Active parity | projected declarations agree with the verified head while the ordered Snapshot digest remains authoritative; disable rejects a semantically stale static fallback |
| Split brain | each direction emits the exact one-line notice once and keeps structured stdout valid |
| Secret hygiene | DSN, password, vault body, Inventory scalar values, adapter body, dump body, and backup body are absent from errors and structured output; hidden-adapter stdout is confined to its private protocol harness |

The implementation also runs proportional package tests, relevant race tests,
`go test ./...`, `go test -race ./...`, `go vet ./...`, and
`git diff --check` before P2 can close.


## 16. Parent-contract traceability

This table is the review index from the sole normative parent into the proposed
design and its falsifiable proof. It adds detail but does not weaken a parent
requirement.

| `sty.md` P2 obligation | RFC decision | Required proof |
|:---|:---|:---|
| RFC before implementation | status header; sections 1–17 remain proposal-only until section 18 approval | no mutation code before recorded owner approval |
| Flat command and focused ownership | section 3 and section 17 | `cmd/sty.go` glue-only review and package-boundary tests |
| Versioned idempotent migrations | sections 6.1–6.2 and 9.1 | migration idempotence/fail-closed rows in section 15 |
| Semantic Snapshot and exact revision | sections 4–6 | Snapshot fidelity, canonical YAML, rollback, and revision-conflict rows |
| Source provenance and optimistic concurrency | sections 6.3–6.4, 9.2, and 10 | revision conflict, base mismatch, and transaction rollback rows |
| Load/dump diff and merge semantics | sections 5.4 and 9.2–10 | prune, plain dump, three-way merge, and base-mismatch rows |
| Rollback and backups | sections 9.1–9.2 and 13 | reset confirmation, backup restore, and historical rollback rows |
| Narrow fail-closed source switching | sections 4.3, 8, 9.4–9.6, and 11 | source-switch, switch-failure, source-resolution, and active-parity rows |
| Split-brain detection | section 12 | both exact notices and structured-output isolation |
| No legacy destructive helpers | section 2 | forbidden-executable guards and legacy-schema sentinels |
| Counts/identifiers/revision/plan only | sections 7, 9.3, and 14 | plain-dump and secret-hygiene rows |
| Containerized destructive/conflict proof | section 15 | digest-pinned PostgreSQL run plus the complete required-case matrix |


## 17. Post-approval implementation gates

Owner approval unlocks code in this order only:

1. **P2.1 Snapshot foundation — PASSED 2026-07-17:** typed canonical
   Snapshot, representability, semantic diff, canonical dump, and three-way
   merge fixtures; no DB write. Functional commits: `ad23ddf`, `1ad05de`, and
   `73b3ffc`. The final package/full tests, race tests, `go vet ./...`, and
   `git diff --check` passed with the pre-existing `go.sum` change excluded.
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


## 18. Owner approval checklist

Owner approval recorded: **2026-07-17 — “批准 P2 CMDB RFC v1，按文档进入
P2.1”**. This approval unlocks P2.1 only; the ordered gates in section 17
remain binding.

Approval should explicitly accept or amend these choices:

- [x] separate `pigsty_cmdb` schema and no legacy-schema compatibility writes;
- [x] singleton append-only Snapshot store and exact revision-token format;
- [x] declaration-order-preserving Snapshot digest and atomic reorder merge;
- [x] CMDB v1 representability and size limits for tags/vault/anchors/aliases;
- [x] hidden native Ansible adapter and private per-home runtime directory;
- [x] connection-file/environment/`service=meta` resolver with no DSN flag;
- [x] PostgreSQL 14 minimum with pinned 14/18 compatibility coverage;
- [x] reset bundle plus `init --reset --restore` recovery contract;
- [x] `load --revision` as rollback without a new public verb;
- [x] mandatory private `--output` for secret-bearing dump and merge bodies;
- [x] exact three-way merge and base-mismatch rules;
- [x] strict source-switch grammar, parity requirement, and backup behavior;
- [x] container guard and destructive/conflict test matrix.

An approval such as **“批准 P2 CMDB RFC v1，按文档进入 P2.1”** is sufficient
to begin implementation. Until then, the implementation gate remains closed.

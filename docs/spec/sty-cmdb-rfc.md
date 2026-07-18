---
title: "Pigsty CMDB integration record"
description: "Superseded native CMDB RFC and corrected existing-schema boundary"
weight: 151
icon: fas fa-database
module: [PIG]
categories: [RFC]
---

# Pigsty CMDB Integration Record

Status: **SUPERSEDED — do not implement or extend the native revision-store RFC**

Last updated: **2026-07-18**

The earlier RFC in this file incorrectly treated Pigsty CMDB as a greenfield
database backend and proposed a separate `pigsty_cmdb` schema, migrations,
immutable Snapshots, revision/CAS history, rollback, three-way merge, backup
bundles, and runtime source-switch records.

That design is withdrawn by owner correction on 2026-07-18. It solved a problem
Pigsty had already solved and added substantial implementation complexity
without matching the existing product model.

The authoritative public contract is now
[`docs/spec/inventory.md`](inventory.md). This file remains only to make the
retraction explicit and prevent the former implementation record from being
mistaken for an approved direction.


## Existing CMDB truth

Pigsty already provides the complete CMDB baseline and workflows:

| Asset | Existing behavior |
|:---|:---|
| `/Users/vonng/pgsty/pigsty/files/cmdb.sql` | recreates the existing `pigsty` and `pglog` schemas, objects, views, and parameter catalog |
| `/Users/vonng/pgsty/pigsty/bin/inventory_load` | replaces Inventory rows in the existing CMDB tables and loads OS-specific defaults |
| `/Users/vonng/pgsty/pigsty/bin/inventory_cmdb` | writes `inventory.sh` querying `pigsty.inventory` and activates it in `ansible.cfg` |
| `/Users/vonng/pgsty/pigsty/bin/inventory_conf` | switches `ansible.cfg` back to `pigsty.yml` |

The stored declaration tables are `pigsty.global_var`, `pigsty.group`,
`pigsty.group_var`, `pigsty.host`, and `pigsty.host_var`.

The existing projections are:

- `pigsty.raw_config`: declared configuration JSONB for static export;
- `pigsty.inventory`: Ansible dynamic Inventory JSON;
- `pigsty.global_config`, `group_config`, and `host_config`: derived views.


## Correct Pig responsibility

Pig provides a thin native adapter:

```text
pig inventory cmdb init       # apply version-matched files/cmdb.sql
pig inventory cmdb load       # fill existing tables like inventory_load
pig inventory cmdb dump       # raw_config JSONB -> canonical YAML
pig inventory cmdb enable     # activate inventory.sh like inventory_cmdb
pig inventory cmdb disable    # restore pigsty.yml like inventory_conf
```

Only `init`, `load`, and `dump` are CMDB data primitives. `enable` and
`disable` merely expose the two existing source-switch scripts; they add no
storage semantics.

It may improve argument validation, transaction boundaries, atomic local-file
writes, error reporting, and tests. Those improvements do not authorize a new
schema or a different data model.


## Explicitly rejected scope

The corrected implementation MUST NOT add or depend on:

- a `pigsty_cmdb` schema;
- a schema migration ledger separate from `files/cmdb.sql`;
- immutable Snapshot or revision tables;
- revision tokens, compare-and-swap history, history listing, or rollback;
- three-way Inventory merge or provenance records;
- CMDB reset/restore bundles;
- a source-switch state database or private runtime identity protocol;
- compatibility writes between the abandoned schema and `pigsty`.

Static lossless YAML editing remains valuable, but the existing CMDB stores
JSONB declarations and therefore cannot preserve YAML comments, padding,
quotes, tags, or ordering. CMDB dump is canonical generated YAML by contract.


## Existing Pig implementation status

The experimental revision-store implementation was removed on 2026-07-18 after
the thin adapter passed corpus, local PostgreSQL, and pinned-container parity
gates. `internal/cmdb` now contains only the existing-schema adapter and the
narrow `ansible.cfg` source switch. `pig sty cmdb` delegates to the same five
use cases exposed by `pig inventory cmdb`; it does not preserve the withdrawn
flags or private runtime adapter.

The existing Pigsty scripts remain operational entry points and the reference
behavior for future parity checks.

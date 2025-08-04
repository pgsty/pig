---
title: PGSQL Repo
description: The repo for PostgreSQL Extensions & Kernel Forks
icon: Database
---


import { File, Folder, Files } from 'fumadocs-ui/components/files';
import { Badge } from "@/components/ui/badge";
import { KeyRound, FileTerminal } from "lucide-react";

The `pigsty-pgsql` repo contains packages that are ad hoc to specific PostgreSQL Major Versions.
(Often ad hoc to a specific Linux distro major version, too) Including extensions, and some kernel forks.

## Quick Start

```bash tab="pig"
curl https://repo.pigsty.io/pig | bash      # download and install the pig CLI tool
pig repo add all pigsty -u                  # add pigsty-pgsql repo and update cache
```
```bash tab="apt"
# Add Pigsty's GPG public key to your system keychain to verify package signatures
curl -fsSL https://repo.pigsty.io/key | sudo gpg --dearmor -o /etc/apt/keyrings/pigsty.gpg

# Get Debian distribution codename (distro_codename=jammy, focal, bullseye, bookworm), and write the corresponding upstream repository address to the APT List file
distro_codename=$(lsb_release -cs)
sudo tee /etc/apt/sources.list.d/pigsty-io.list > /dev/null <<EOF
deb [signed-by=/etc/apt/keyrings/pigsty.gpg] https://repo.pigsty.io/apt/pgsql/${distro_codename} ${distro_codename} main
EOF

# Refresh APT repository cache
sudo apt update
```
```bash tab="yum"
# Add Pigsty's GPG public key to your system keychain to verify package signatures
curl -fsSL https://repo.pigsty.io/key | sudo tee /etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty >/dev/null

# Add Pigsty Repo definition files to /etc/yum.repos.d/ directory, including two repositories
sudo tee /etc/yum.repos.d/pigsty-pgsql.repo > /dev/null <<-'EOF'
[pigsty-pgsql]
name=Pigsty PGSQL For el$releasever.$basearch
baseurl=https://repo.pigsty.io/yum/pgsql/el$releasever.$basearch
skip_if_unavailable = 1
enabled = 1
priority = 1
gpgcheck = 1
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty
module_hotfixes=1
EOF

# Refresh YUM/DNF repository cache
sudo yum makecache;
```



--------

## Compatibility

|   OS/Arch    | OS/Arch |                                                                                                  x86_64                                                                                                  |                                                                                                 aarch64                                                                                                  |
|:------------:|:-------:|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------:|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------:|
|     EL8      |   el8   |   <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge>    |   <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge>    |
|     EL9      |   el9   |   <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge>    |   <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge>    |
|  Debian 12   |   d12   |   <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge>    |   <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge>    |
| Ubuntu 22.04 |   u22   |   <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge>    |   <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge>    |
| Ubuntu 24.04 |   u24   |   <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge>    |   <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge>    |


--------

## Source

Building specs of this repo is open-sourced on GitHub:

- https://github.com/pgsty/rpm
- https://github.com/pgsty/deb

If the platform is not supported, you can also build the packages from source code by yourself.


------

## Changelog


|      Date      |        Name         |  Old  |                                               New                                                | RPM | DEB |
|:--------------:|:-------------------:|:-----:|:------------------------------------------------------------------------------------------------:|:---:|:---:|
| **2025-05-26** |        pgdd         |   -   |                          [0.6.0](https://github.com/rustprooflabs/pgdd)                          |  ✓  |  ✓  |
|                |       convert       |   -   |                        [0.0.4](https://github.com/rustprooflabs/convert)                         |  ✓  |  ✓  |
|                |      pg_idkit       |   -   |                          [0.3.0](https://github.com/VADOSWARE/pg_idkit)                          |  ✓  |  ✓  |
|                |   pg_tokenizer.rs   |   -   |                     [0.1.0](https://github.com/tensorchord/pg_tokenizer.rs)                      |  ✓  |  ✓  |
|                |      pg_render      |   -   |                           [0.1.2](https://github.com/mkaski/pg_render)                           |  ✓  |  ✓  |
|                |      pgx_ulid       |   -   |                          [0.2.0](https://github.com/pksunkara/pgx_ulid)                          |  ✓  |  ✓  |
|                |       pg_ivm        |   -   |                            [1.11.0](https://github.com/sraoss/pg_ivm)                            |     |  ✓  |
|                |      orioledb       |   -   |                         [1.4.0b11](https://github.com/orioledb/orioledb)                         |  ✓  |  ✓  |
| **2025-05-22** |     openhalodb      |   -   |                            [14.10](https://github.com/pgsty/openHalo)                            |  ✓  |  ✓  |
|                |        spat         |   -   |                        [0.1.0a4](https://github.com/Florents-Tselai/spat)                        |  ✓  |  ✓  |
|                |     pgsentinel      |   -   |              [1.1.0](https://github.com/pgsentinel/pgsentinel/releases/tag/v1.1.0)               |  ✓  |  ✓  |
|                |     timescaledb     |   -   |              [2.20.0](https://github.com/timescale/timescaledb/releases/tag/2.20.0)              |  ✓  |  ✓  |
|                |     sqlite_fdw      |   -   |               [2.5.0](https://github.com/pgspider/sqlite_fdw/releases/tag/v2.5.0)                |  ✓  |  ✓  |
|                |     documentdb      |   -   |      [0.103.0](https://github.com/FerretDB/documentdb/releases/tag/v0.103.0-ferretdb-2.2.0)      |  ✓  |  ✓  |
|                |         tzf         |   -   |                [0.2.2](https://github.com/ringsaturn/pg-tzf/releases/tag/v0.2.2)                 |  ✓  |  ✓  |
|                |    pg_vectorize     |   -   |             [0.22.2](https://github.com/ChuckHend/pg_vectorize/releases/tag/v0.22.2)             |  ✓  |  ✓  |
|                |      wrappers       |   -   |                [0.5.0](https://github.com/supabase/wrappers/releases/tag/v0.5.0)                 |  ✓  |  ✓  |
| **2025-05-07** |      omnigres       |   -   | [20250507](https://github.com/omnigres/omnigres/commit/413feff21f9f7310023d8cfd92b83f2a251b1aa4) |  ✓  |  ✓  |
|                |        citus        |   -   |                [12.0.3](https://github.com/citusdata/citus/releases/tag/v13.0.3)                 |  ✓  |  ✓  |
|                |     timescaledb     |   -   |              [2.19.3](https://github.com/timescale/timescaledb/releases/tag/2.19.3)              |  ✓  |  ✓  |
|                |      supautils      |   -   |                [2.9.1](https://github.com/supabase/supautils/releases/tag/v2.9.1)                |  ✓  |  ✓  |
|                |      pg_envvar      |   -   |                 [1.0.1](https://github.com/theory/pg-envvar/releases/tag/v1.0.1)                 |  ✓  |  ✓  |
|                |    pgcollection     |   -   |                 [1.0.0](https://github.com/aws/pgcollection/releases/tag/v1.0.0)                 |  ✓  |  ✓  |
|                |    aggs_for_vecs    |   -   |              [1.4.0](https://github.com/pjungwir/aggs_for_vecs/releases/tag/1.4.0)               |  ✓  |  ✓  |
|                |     pg_tracing      |   -   |                [0.1.3](https://github.com/DataDog/pg_tracing/releases/tag/v0.1.3)                |  ✓  |  ✓  |
|                |        pgmq         |   -   |                    [1.5.1](https://github.com/pgmq/pgmq/releases/tag/v1.5.1)                     |  ✓  |  ✓  |
|                |       tzf-pg        |   -   |                [0.2.0](https://github.com/ringsaturn/tzf-pg/releases/tag/v0.2.0)                 |  ✓  |  ✓  |
|                |      pg_search      |   -   |              [0.15.18](https://github.com/paradedb/paradedb/releases/tag/v0.15.18)               |  ✓  |  ✓  |
|                |        anon         |   -   |   [2.1.1](https://gitlab.com/dalibo/postgresql_anonymizer/-/tree/latest/debian?ref_type=heads)   |  ✓  |  ✓  |
|                |     pg_parquet      |   -   |              [0.4.0](https://github.com/CrunchyData/pg_parquet/releases/tag/v0.3.2)              |  ✓  |  ✓  |
|                |     pg_cardano      |   -   |                 [1.0.5](https://github.com/Fell-x27/pg_cardano/commits/master/)                  |  ✓  |  ✓  |
|                |    pglite_fusion    |   -   |              [0.0.5](https://github.com/frectonz/pglite-fusion/releases/tag/v0.0.5)              |  ✓  |  ✓  |
|                |     vchord_bm25     |   -   |           [0.2.1](https://github.com/tensorchord/VectorChord-bm25/releases/tag/0.2.1)            |  ✓  |  ✓  |
|                |       vchord        |   -   |              [0.3.0](https://github.com/tensorchord/VectorChord/releases/tag/0.3.0)              |  ✓  |  ✓  |
|                | timescaledb-toolkit |   -   |          [1.21.0](https://github.com/timescale/timescaledb-toolkit/releases/tag/1.21.0)          |  ✓  |  ✓  |
|                |    pgvectorscale    |   -   |              [0.7.1](https://github.com/timescale/pgvectorscale/releases/tag/0.7.1)              |  ✓  |  ✓  |
|                |   pg_session_jwt    |   -   |           [0.3.1](https://github.com/neondatabase/pg_session_jwt/releases/tag/v0.3.1)            |  ✓  |  ✓  |
| **2025-03-20** |     timescaledb     |   -   |                                              2.19.0                                              |  ✓  |  ✓  |
|                |        citus        |   -   |                                              13.0.2                                              |  ✓  |  ✓  |
|                |     documentdb      |   -   |                                              1.102                                               |  ✓  |  ✓  |
|                |    pg_analytics     |   -   |                                              0.3.7                                               |  ✓  |  ✓  |
|                |      pg_search      |   -   |                                              0.15.8                                              |  ✓  |  ✓  |
|                |       pg_ivm        |   -   |                                               1.10                                               |     |  ✓  |
|                |        emaj         |   -   |                                              4.6.0                                               |  ✓  |  ✓  |
|                |    pgsql_tweaks     |   -   |                                              0.11.0                                              |  ✓  |  ✓  |
|                |    pgvectorscale    |   -   |                                              0.6.0                                               |  ✓  |  ✓  |
|                |   pg_session_jwt    |   -   |                                              0.2.0                                               |  ✓  |  ✓  |
|                |      wrappers       |   -   |                                              0.4.5                                               |  ✓  |  ✓  |
|                |     pg_parquet      |   -   |                                              0.3.1                                               |  ✓  |  ✓  |
|                |       vchord        |   -   |                                              0.2.2                                               |  ✓  |  ✓  |
|                |       pg_tle        | 1.2.0 |                                              1.5.0                                               |  ✓  |  ✓  |
|                |      supautils      | 2.5.0 |                                              2.6.0                                               |  ✓  |  ✓  |
|                |      sslutils       |  1.3  |                                               1.4                                                |  ✓  |  ✓  |
|                |     pg_profile      |  4.7  |                                               4.8                                                |  ✓  |  ✓  |
|                |    pg_jsonschema    | 0.3.2 |                                              0.3.3                                               |  ✓  |  ✓  |
|                |   pg_incremental    | 1.1.1 |                                              1.2.0                                               |  ✓  |  ✓  |
|                |  ddl_historization  |  0.7  |                                              0.0.7                                               |  ✓  |  ✓  |
|                |      pg_sqlog       | 3.1.7 |                                               1.6                                                |  ✓  |  ✓  |
|                |      pg_random      |   -   |                                                -                                                 |  ✓  |  ✓  |
|                |   pg_stat_monitor   | 2.1.0 |                                              2.1.1                                               |  ✓  |  ✓  |
| **2025-02-12** |     pg_profile      |  4.7  |                                               4.8                                                |  ✓  |  ✓  |
| **2024-10-16** |       pg_ivm        |   -   |                                               1.9                                                |     |  ✓  |
|                |    pg_timeseries    |   -   |                                              0.1.6                                               |  ✓  |  ✓  |
|                |        pgmq         |   -   |                                              1.4.4                                               |  ✓  |  ✓  |
|                |     pg_protobuf     |   -   |                                              16 17                                               |  ✓  |  ✓  |
|                |      pg_uuidv7      |   -   |                                               1.6                                                |  ✓  |  ✓  |
|                |     pg_readonly     |   -   |                                              latest                                              |  ✓  |  ✓  |
|                |        pgddl        |   -   |                                               0.28                                               |  ✓  |  ✓  |
|                |    pg_safeupdate    |   -   |                                              latest                                              |  ✓  |  ✓  |
|                |   pg_stat_monitor   |   -   |                                               2.1                                                |  ✓  |  ✓  |
|                |     pg_profile      |   -   |                                               4.7                                                |  ✓  |  ✓  |
|                |    system_stats     |   -   |                                               3.2                                                |  ✓  |  ✓  |
|                |     pg_auth_mon     |   -   |                                               3.0                                                |  ✓  |  ✓  |
|                |     login_hook      |   -   |                                               1.6                                                |  ✓  |  ✓  |
|                |      logerrors      |   -   |                                              2.1.3                                               |  ✓  |  ✓  |
|                |     pg-orphaned     |   -   |                                              latest                                              |  ✓  |  ✓  |
|                |      pgnodemx       |   -   |                                               1.7                                                |  ✓  |  ✓  |
|                |      sslutils       |   -   |                                           1.4 (+16,17)                                           |  ✓  |  ✓  |

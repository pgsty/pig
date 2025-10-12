---
title: "CMD: ext"
description: How to manage extensions with pig ext subcommand
icon: SquareTerminal
weight: 620
---

The `pig ext` command is a comprehensive tool for managing PostgreSQL extensions.
It allows users to search, install, remove, update, and manage PostgreSQL extensions and even kernel packages.

------

## Overview

```bash
pig ext - Manage PostgreSQL Extensions

  pig repo add -ru             # add all repo and update cache (brute but effective)
  pig ext add pg17             # install optional postgresql 17 package
  pig ext list duck            # search extension in catalog
  pig ext scan -v 17           # scan installed extension for pg 17
  pig ext add pg_duckdb        # install certain postgresql extension

Usage:
  pig ext [command]

Aliases:
  ext, e, ex, pgext, extension

Examples:

  pig ext list    [query]      # list & search extension
  pig ext info    [ext...]     # get information of a specific extension
  pig ext status  [-v]         # show installed extension and pg status
  pig ext add     [ext...]     # install extension for current pg version
  pig ext rm      [ext...]     # remove extension for current pg version
  pig ext update  [ext...]     # update extension to the latest version
  pig ext import  [ext...]     # download extension to local repo
  pig ext link    [ext...]     # link postgres installation to path
  pig ext upgrade              # upgrade to the latest extension catalog


Available Commands:
  add         install postgres extension
  import      import extension packages to local repo
  info        get extension information
  link        link postgres to active PATH
  list        list & search available extensions
  rm          remove postgres extension
  scan        scan installed extensions for active pg
  status      show installed extension on active pg
  update      update installed extensions for current pg version
  upgrade     upgrade extension catalog to the latest version

Flags:
  -h, --help          help for ext
  -p, --path string   specify a postgres by pg_config path
  -v, --version int   specify a postgres by major version

Global Flags:
      --debug              enable debug mode
  -i, --inventory string   config inventory path
      --log-level string   log level: debug, info, warn, error, fatal, panic (default "info")
      --log-path string    log file path, terminal by default

Use "pig ext [command] --help" for more information about a command.
```

------

## Examples

To install postgres extensions, you’ll have to add the [**repo**](/pig/repo/) first:

```bash
pig repo add pgdg pigsty -u    # gental way to add pgdg and pigsty repo
pig repo set -u                # brute way to remove and add all required repos
```

Then you can search and install PostgreSQL extensions:

```bash
pig ext install pg_duckdb
pig ext install pg_partman
pig ext install pg_cron
pig ext install pg_repack
pig ext install pg_stat_statements
pig ext install pg_stat_kcache
```

Check [**extension list**](/list/) for available extensions and their names.

1. When no PostgreSQL version is specified, the tool will try to detect the active PostgreSQL installation from `pg_config` in your `PATH`
2. PostgreSQL can be specified either by major version number (`-v`) or by pg_config path (`-p`). if `-v` is given, pig will use the well-known default path of PGDG kernel packages for the given version. - On EL distros, it’s `/usr/pgsql-$v/bin/pg_config` for PG$v, - On DEB distros, it’s `/usr/lib/postgresql/$v/bin/pg_config` for PG$v, etc. if `-p` is given, pig will use the `pg_config` path to find the PostgreSQL installation.
3. The extension manager supports different package formats based on the underlying operating system:
- RPM packages for RHEL/CentOS/Rocky Linux/AlmaLinux
- DEB packages for Debian/Ubuntu
4. Some extensions may have dependencies that will be automatically resolved during installation.
5. Use the `-y` flag with caution as it will automatically confirm all prompts.

Pigsty assumes you already have installed the official PGDG kernel packages, if not, you can install them with:

```bash
pig ext install pg17          # install PostgreSQL 17 kernels (all but devel)
```

------

## `ext list`

List (or search) available extensions in the extension catalog.

```bash
list & search available extensions

Usage:
  pig ext list [query] [flags]

Aliases:
  list, l, ls, find

Examples:

  pig ext list                # list all extensions
  pig ext list postgis        # search extensions by name/description
  pig ext ls olap             # list extension of olap category
  pig ext ls gis -v 16        # list gis category for pg 16
```

The default extension catalog is defined in [**`cli/ext/assets/extension.csv`**](https://github.com/pgsty/pig/blob/main/cli/ext/assets/extension.csv)

You can update to the latest extension catalog with: `pig ext upgrade` it will download the latest extension catalog data to `~/.pig/extension.csv`.

------

## `ext info`

Display detailed information about specific extensions.

```bash
pig ext info [ext...]
```

**Examples:**

```bash
pig ext info postgis        # Show detailed information about PostGIS
pig ext info timescaledb    # Show information about TimescaleDB
$ pig ext info postgis        # Show detailed information about PostGIS

╭────────────────────────────────────────────────────────────────────────────╮
│ postgis                                                                    │
├────────────────────────────────────────────────────────────────────────────┤
│ PostGIS geometry and geography spatial types and functions                 │
├────────────────────────────────────────────────────────────────────────────┤
│ Extension : postgis                                                        │
│ Alias     : postgis                                                        │
│ Category  : GIS                                                            │
│ Version   : 3.5.2                                                          │
│ License   : GPL-2.0                                                        │
│ Website   : https://git.osgeo.org/gitea/postgis/postgis                    │
│ Details   : https://pigsty.io/gis/postgis                                  │
├────────────────────────────────────────────────────────────────────────────┤
│ Extension Properties                                                       │
├────────────────────────────────────────────────────────────────────────────┤
│ PostgreSQL Ver │  Available on: 17, 16, 15, 14, 13                         │
│ CREATE  :  Yes │  CREATE EXTENSION postgis;                                │
│ DYLOAD  :  No  │  no need to load shared libraries                         │
│ TRUST   :  No  │  require database superuser to install                    │
│ Reloc   :  No  │  Schemas: []                                              │
│ Depend  :  No  │                                                           │
├────────────────────────────────────────────────────────────────────────────┤
│ Required By                                                                │
├────────────────────────────────────────────────────────────────────────────┤
│ - postgis_topology                                                         │
│ - postgis_raster                                                           │
│ - postgis_sfcgal                                                           │
│ - postgis_tiger_geocoder                                                   │
│ - pgrouting                                                                │
│ - pointcloud_postgis                                                       │
│ - h3_postgis                                                               │
│ - mobilitydb                                                               │
│ - documentdb                                                               │
├────────────────────────────────────────────────────────────────────────────┤
│ RPM Package                                                                │
├────────────────────────────────────────────────────────────────────────────┤
│ Repository     │  PGDG                                                     │
│ Package        │  postgis35_$v*                                            │
│ Version        │  3.5.2                                                    │
│ Availability   │  17, 16, 15, 14, 13                                       │
├────────────────────────────────────────────────────────────────────────────┤
│ DEB Package                                                                │
├────────────────────────────────────────────────────────────────────────────┤
│ Repository     │  PGDG                                                     │
│ Package        │  postgresql-$v-postgis-3 postgresql-$v-postgis-3-scripts  │
│ Version        │  3.5.2                                                    │
│ Availability   │  17, 16, 15, 14, 13                                       │
╰────────────────────────────────────────────────────────────────────────────╯
```

------

## `status`

Display the status of installed extensions for the active PostgreSQL instance.

```bash
pig ext status [-c]
```

**Options:**

- `-c, --contrib`: Include contrib extensions in the results

**Examples:**

```bash
pig ext status              # Show installed extensions
pig ext status -c           # Show installed extensions including contrib ones
pig ext status -v 16        # Show installed extensions for PostgreSQL 16
```

------

## `ext scan`

Scan the active PostgreSQL instance for installed extensions.

```bash
pig ext scan [-v version]
```

It will scan the postgres extension folder to find all the actually installed extensions.

```
$ pig ext status

Installed:
* PostgreSQL 17.4 (Debian 17.4-1.pgdg120+2)  85  Extensions

Active:
PG Version      :  PostgreSQL 17.4 (Debian 17.4-1.pgdg120+2)
Config Path     :  /usr/lib/postgresql/17/bin/pg_config
Binary Path     :  /usr/lib/postgresql/17/bin
Library Path    :  /usr/lib/postgresql/17/lib
Extension Path  :  /usr/share/postgresql/17/extension
Extension Stat  :  18 Installed (PIGSTY 8, PGDG 10) + 67 CONTRIB = 85 Total

Name                          Version  Cate   Flags   License       Repo    Package                                                  Description
----                          -------  ----   ------  -------       ------  ------------                                             ---------------------
timescaledb                   2.18.2   TIME   -dsl--  Timescale     PIGSTY  postgresql-17-timescaledb-tsl                            Enables scalable inserts and complex queries for time-series dat
postgis                       3.5.2    GIS    -ds---  GPL-2.0       PGDG    postgresql-17-postgis-3 postgresql-$v-postgis-3-scripts  PostGIS geometry and geography spatial types and functions
postgis_topology              3.5.2    GIS    -ds---  GPL-2.0       PGDG    postgresql-17-postgis-3 postgresql-$v-postgis-3-scripts  PostGIS topology spatial types and functions
postgis_raster                3.5.2    GIS    -ds---  GPL-2.0       PGDG    postgresql-17-postgis-3 postgresql-$v-postgis-3-scripts  PostGIS raster types and functions
postgis_sfcgal                3.5.2    GIS    -ds--r  GPL-2.0       PGDG    postgresql-17-postgis-3 postgresql-$v-postgis-3-scripts  PostGIS SFCGAL functions
postgis_tiger_geocoder        3.5.2    GIS    -ds-t-  GPL-2.0       PGDG    postgresql-17-postgis-3 postgresql-$v-postgis-3-scripts  PostGIS tiger geocoder and reverse geocoder
address_standardizer          3.5.2    GIS    -ds--r  GPL-2.0       PGDG    postgresql-17-postgis-3 postgresql-$v-postgis-3-scripts  Used to parse an address into constituent elements. Generally us
address_standardizer_data_us  3.5.2    GIS    -ds--r  GPL-2.0       PGDG    postgresql-17-postgis-3 postgresql-$v-postgis-3-scripts  Address Standardizer US dataset example
vector                        0.8.0    RAG    -ds--r  PostgreSQL    PGDG    postgresql-17-pgvector                                   vector data type and ivfflat and hnsw access methods
pg_search                     0.15.2   FTS    -ds-t-  AGPL-3.0      PIGSTY  postgresql-17-pg-search                                  pg_search: Full text search for PostgreSQL using BM25
pgroonga                      4.0.0    FTS    -ds-tr  PostgreSQL    PIGSTY  postgresql-17-pgroonga                                   Use Groonga as index, fast full text search platform for all lan
pgroonga_database             4.0.0    FTS    -ds-tr  PostgreSQL    PIGSTY  postgresql-17-pgroonga                                   PGroonga database management module
citus                         13.0.1   OLAP   -dsl--  AGPL-3.0      PIGSTY  postgresql-17-citus                                      Distributed PostgreSQL as an extension
citus_columnar                11.3-1   OLAP   -ds---  AGPL-3.0      PIGSTY  postgresql-17-citus                                      Citus columnar storage engine
pg_mooncake                   0.1.2    OLAP   ------  MIT           PIGSTY  postgresql-17-pg-mooncake                                Columnstore Table in Postgres
plv8                          3.2.3    LANG   -ds---  PostgreSQL    PIGSTY  postgresql-17-plv8                                       PL/JavaScript (v8) trusted procedural language
pg_repack                     1.5.2    ADMIN  bds---  BSD 3-Clause  PGDG    postgresql-17-repack                                     Reorganize tables in PostgreSQL databases with minimal locks
wal2json                      2.5.3    ETL    --s--x  BSD 3-Clause  PGDG    postgresql-17-wal2json                                   Changing data capture in JSON format

(18 Rows) (Flags: b = HasBin, d = HasDDL, s = HasSolib, l = NeedLoad, t = Trusted, r = Relocatable, x = Unknown)
```

------

## `ext add`

Install one or more PostgreSQL extensions.

```bash
install postgres extension

Usage:
  pig ext add [flags]

Aliases:
  add, a, install, ins

Examples:

Description:
  pig ext install pg_duckdb                  # install one extension
  pig ext install postgis timescaledb        # install multiple extensions
  pig ext add     pgvector pgvectorscale     # other alias: add, ins, i, a
  pig ext ins     pg_search -y               # auto confirm installation
  pig ext install pgsql                      # install the latest version of postgresql kernel
  pig ext a pg17                             # install postgresql 17 kernel packages
  pig ext ins pg16                           # install postgresql 16 kernel packages
  pig ext install pg15-core                  # install postgresql 15 core packages
  pig ext install pg14-main -y               # install pg 14 + essential extensions (vector, repack, wal2json)
  pig ext install pg13-devel --yes           # install pg 13 devel packages (auto-confirm)
  pig ext install pgsql-common               # install common utils such as patroni pgbouncer pgbackrest,...


Flags:
  -h, --help   help for add
  -y, --yes    auto confirm install
```

------

## `ext rm`

Remove one or more PostgreSQL extensions.

```bash
pig ext rm [ext...] [-y]
```

**Options:**

- `-y, --yes`: Auto-confirm removal

**Examples:**

```bash
pig ext rm pg_duckdb                   # Remove a specific extension
pig ext rm postgis timescaledb         # Remove multiple extensions
pig ext rm pgvector -y                 # Remove with auto-confirmation
```

------

## `ext update`

Update installed extensions to their latest versions.

```bash
pig ext update [ext...] [-y]
```

**Options:**

- `-y, --yes`: Auto-confirm updates

**Examples:**

```bash
pig ext update                         # Update all installed extensions
pig ext update postgis                 # Update a specific extension
pig ext update postgis timescaledb     # Update multiple extensions
pig ext update -y                      # Update with auto-confirmation
```

------

## `pig import`

Download extension packages to the local repo for offline installation.

```bash
Usage:
  pig ext import [ext...] [flags]

Aliases:
  import, get

Examples:

  pig ext import postgis                # import postgis extension packages
  pig ext import timescaledb pg_cron    # import multiple extensions
  pig ext import pg16                   # import postgresql 16 packages
  pig ext import pgsql-common           # import common utilities
  pig ext import -d /www/pigsty postgis # import to specific path


Flags:
  -h, --help          help for import
  -d, --repo string   specify repo dir (default "/www/pigsty")
```

**Options:**

- `-d, --repo`: Specify the repository directory (default: /www/pigsty)

**Examples:**

```bash
pig ext import postgis                 # Import PostGIS packages
pig ext import timescaledb pg_cron     # Import multiple extension packages
pig ext import pg16                    # Import PostgreSQL 16 packages
pig ext import pgsql-common            # Import common utility packages
```

------

## `ext link`

Link a PostgreSQL installation to the system PATH.

```bash
link postgres to active PATH

Usage:
  pig ext link <-v pgver|-p pgpath> [flags]

Aliases:
  link, ln

Examples:

  pig ext link 16                      # link pgdg postgresql 16 to /usr/pgsql
  pig ext link /usr/pgsql-16           # link specific pg to /usr/pgsql
  pig ext link /u01/polardb_pg         # link polardb pg to /usr/pgsql
  pig ext link null|none|nil|nop|no    # unlink current postgres install


Flags:
  -h, --help   help for link
```

**Examples:**

```bash
pig ext link 17                        # Link PostgreSQL 17 to /usr/pgsql
pig ext link 16                        # Link PostgreSQL 16 to /usr/pgsql
pig ext link /usr/pgsql-16             # Link from a specific path to /usr/pgsql
pig ext link null                      # Unlink current PostgreSQL installation
```

------

## `upgrade`

Update the extension catalog to the latest version.

```bash
pig ext upgrade
```


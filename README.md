# PIG - Postgres Install Genius

[![Website: pgext.cloud](https://img.shields.io/badge/Website-pgext.cloud-slategray?style=flat&logo=cilium&logoColor=white)](https://pgext.cloud)
[![Doc: pig](https://img.shields.io/badge/Docs-pig-slategray?style=flat)](https://pigsty.io/docs/pig)
[![Version: v1.1.2](https://img.shields.io/badge/version-v1.1.2-slategray?style=flat)](https://github.com/pgsty/pig/releases/tag/v1.1.2)
[![Pigsty: v4.1.0](https://img.shields.io/badge/Pigsty-v4.1.0-slategray?style=flat)](https://pigsty.io/docs/about/release)
[![License: Apache-2.0](https://img.shields.io/github/license/pgsty/pig?logo=opensourceinitiative&logoColor=green&color=slategray)](https://github.com/pgsty/pig/blob/main/LICENSE)
[![Extensions: 451](https://img.shields.io/badge/extensions-451-%233E668F?style=flat&logo=postgresql&logoColor=white&labelColor=3E668F)](https://pgext.cloud/list)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/pgsty/pig)

[**pig**](https://pgext.cloud/pig) is an open-source PostgreSQL (& Extension) Package Manager for [mainstream](https://pgext.cloud/os) (EL/Debian/Ubuntu) Linux.

Install PostgreSQL 13 ~ 18 along with [451 extensions](https://pgext.cloud/list) on (`amd64` / `arm64`) with native OS package manager

All commands support structured output (`-o yaml/json`) with self-describing schema, making it an **Agent-Friendly** PostgreSQL CLI tool.
Also check the [**PGEXT.CLOUD**](https://pgext.cloud) to get details about the available extensions.

[![PostgreSQL Extension Ecosystem](https://pigsty.io/img/pigsty/ecosystem.jpg)](https://medium.com/@fengruohang/postgres-is-eating-the-database-world-157c204dcfc4)


--------

## Get Started

[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-18,17,16,15,14,13-%233E668F?style=flat&logo=postgresql&labelColor=3E668F&logoColor=white)](https://pigsty.io/docs/pgsql)
[![Linux](https://img.shields.io/badge/Linux-amd64/arm64-%23FCC624?style=flat&logo=linux&labelColor=FCC624&logoColor=black)](https://pigsty.io/docs/node)
[![EL Support: 8/9/10](https://img.shields.io/badge/EL-8/9/10-red?style=flat&logo=redhat&logoColor=red)](https://pigsty.io/docs/ref/linux#el)
[![Debian Support: 12/13](https://img.shields.io/badge/Debian-12/13-%23A81D33?style=flat&logo=debian&logoColor=%23A81D33)](https://pigsty.io/docs/ref/linux#debian)
[![Ubuntu Support: 22/24](https://img.shields.io/badge/Ubuntu-22/24-%23E95420?style=flat&logo=ubuntu&logoColor=%23E95420)](https://pigsty.io/docs/ref/linux#ubuntu)

[**Install**](https://pgext.cloud/pig/install) the `pig` package first, (you can also use the `apt` / `yum` or just copy the binary)

```bash
curl -fsSL https://repo.pigsty.io/pig | bash
```

Then it's ready to use, assume you want to install the [`pg_duckdb`](https://pgext.cloud/e/pg_duckdb) extension:

```bash
$ pig repo add pigsty pgdg -u       # add pgdg & pigsty repo, then update repo cache
$ pig ext install pg18              # install PostgreSQL 18 kernels with native PGDG packages
$ pig ext install pg_duckdb -v 18   # install the pg_duckdb extension (for current pg18)
```

That's it, All set! Check the [advanced usage](#advanced-usage) for details and [the full list 451 available extensions](https://pgext.cloud/list).

[![asciicast](https://asciinema.org/a/695902.svg)](https://asciinema.org/a/695902)



--------

## Installation

The `pig` util is a standalone go binary with no dependencies. You can install with the script:

```bash
curl -fsSL https://repo.pigsty.io/pig | bash   # cloudflare default
curl -fsSL https://repo.pigsty.cc/pig | bash   # mainland china mirror
```

Or just download and extract the binary, or use the `apt/dnf` package manager:
 
For Ubuntu 22.04 / 24.04 & Debian 12 / 13 or any compatible platforms:

```bash
sudo tee /etc/apt/sources.list.d/pigsty.list > /dev/null <<EOF
deb [trusted=yes] https://repo.pigsty.io/apt/infra generic main 
EOF
sudo apt update; sudo apt install -y pig
```

For EL 8 / 9 / 10 and compatible platforms:

```bash
sudo tee /etc/yum.repos.d/pigsty.repo > /dev/null <<-'EOF'
[pigsty-infra]
name=Pigsty Infra for $basearch
baseurl=https://repo.pigsty.io/yum/infra/$basearch
enabled = 1
gpgcheck = 0
module_hotfixes=1
EOF
sudo yum makecache; sudo yum install -y pig
```

> For mainland china user: consider replace the `repo.pigsty.io` with `repo.pigsty.cc`

`pig` has self-update feature, you can update pig itself to the latest version with:

```bash
pig update                  # self-update to the latest version
pig update -v 1.1.2         # self-update to the specific version
pig ext reload              # update extension catalog metadata only
```




--------

## Advanced Usage

**Environment Status**

```bash
pig status                    # show os & pg & pig status
pig repo status               # show upstream repo status
pig ext  status               # show pg extensions status 
```

**Extension Management**

```bash
pig ext list    [query]       # list & search extension
pig ext info    [ext...]      # get information of a specific extension
pig ext avail   [ext...]      # show extension availability matrix
pig ext status  [-c]          # show installed extension and pg status
pig ext scan                  # scan installed extensions for active pg
pig ext add     [ext...]      # install extension for current pg version
pig ext rm      [ext...]      # remove extension for current pg version
pig ext update  [ext...]      # update extension to the latest version
pig ext import  [ext...]      # download extension to local repo
pig ext link    [ext...]      # link postgres installation to path
pig ext reload                # reload the latest extension catalog
```

**Repo Management**

```bash
pig repo list                    # available repo list
pig repo info   [repo|module...] # show repo info
pig repo status                  # show current repo status
pig repo add    [repo|module...] # add repo and modules
pig repo set    [repo|module...] # overwrite & update after add
pig repo rm     [repo|module...] # remove repo & modules
pig repo update                  # update repo pkg cache
pig repo create                  # create repo on current system
pig repo boot                    # boot repo from offline package
pig repo cache                   # cache repo as offline package
pig repo reload                  # reload repo catalog to latest version
```

**Build Management**

```bash
pig build repo                   # init build repo (=repo set -ru)
pig build tool  [mini|full|...]  # init build toolset
pig build proxy [id@host:port ]  # init build proxy (optional)
pig build rust                   # init rustc cargo
pig build pgrx  [-v 0.16.1]      # init pgrx
pig build spec                   # init build spec repo
pig build get   [all|std|..]     # get ext code tarball with prefixes
pig build dep   [extname...]     # install extension build deps
pig build ext   [extname...]     # build extension
pig build pkg   [extname...]     # get+dep+ext, build extension in one-pass
```

**Radical Repo Admin**

The default `pig repo add pigsty pgdg` will add the [`PGDG`](https://pgext.cloud/repo/pgdg) repo and [`PIGSTY`](https://pgext.cloud/repo/pgsql) repo to your system.
While the following command will backup & wipe your existing repo and add all require repo to your system.

```bash
pig repo add all --ru        # This will OVERWRITE all existing repo with node,pgdg,pigsty repo
pig repo set                 # = pig repo add all --update --remove
```

There's a brutal version of repo add: `repo set`, which will overwrite you existing repo (`-ru`) by default.

And you can recover you old repos at `/etc/apt/backup` or `/etc/yum.repos.d/backup`.


**Install PostgreSQL**

You can also install PostgreSQL kernel packages with

```bash
pig ext install postgresql -v 18 # install PostgreSQL 18 kernels
pig ext install pg17             # install PostgreSQL 17 kernels (all except test/devel)
pig ext install pg16-mini        # install PostgreSQL 16 kernels with minimal packages
pig ext install pg15 -y          # install PostgreSQL 15 kernels with auto-confirm
pig ext install pg14=14.3        # install PostgreSQL 14 kernels with an specific minor version
pig ext install pg13=13.10       # install PostgreSQL 13 kernels
```

You can link the installed PostgreSQL to the system path with:

```bash
pig ext link 17               # create /usr/pgsql -> /usr/pgsql-17 or /usr/lib/postgresql/17 and put its bin dir in your PATH
. /etc/profile.d/pgsql.sh     # reload the path and take effect immediately
```

You can also use other package aliases, it will translate to corresponding package on your OS distro
and the `$v` will be replaced with the active or given pg version number, such as `17`, `16`, etc...

```yaml
pgsql:        "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl"
pg18:         "postgresql18 postgresql18-server postgresql18-libs postgresql18-contrib postgresql18-plperl postgresql18-plpython3 postgresql18-pltcl"
pg17-client:  "postgresql17"
pg17-server:  "postgresql17-server postgresql17-libs postgresql17-contrib"
pg17-devel:   "postgresql17-devel"
pg17-basic:   "pg_repack_17 wal2json_17 pgvector_17"
pg16-mini:    "postgresql16 postgresql16-server postgresql16-libs postgresql16-contrib"
pg15-full:    "postgresql15 postgresql15-server postgresql15-libs postgresql15-contrib postgresql15-plperl postgresql15-plpython3 postgresql15-pltcl postgresql15-llvmjit postgresql15-test postgresql15-devel"
pg14-main:    "postgresql14 postgresql14-server postgresql14-libs postgresql14-contrib postgresql14-plperl postgresql14-plpython3 postgresql14-pltcl pg_repack_14 wal2json_14 pgvector_14"
pg13-core:    "postgresql13 postgresql13-server postgresql13-libs postgresql13-contrib postgresql13-plperl postgresql13-plpython3 postgresql13-pltcl"
```

<details><summary>More Alias</summary>

Take el for examples:

```yaml
"postgresql":          "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl",
"pgsql-common":        "patroni patroni-etcd pgbouncer pgbackrest pg_exporter pgbackrest_exporter vip-manager",
"patroni":             "patroni patroni-etcd",
"pgbouncer":           "pgbouncer",
"pgbackrest":          "pgbackrest",
"pg_exporter":         "pg_exporter",
"pgbackrest_exporter": "pgbackrest_exporter",
"vip-manager":         "vip-manager",
"pgbadger":            "pgbadger",
"pg_activity":         "pg_activity",
"pg_filedump":         "pg_filedump",
"pgxnclient":          "pgxnclient",
"pgformatter":         "pgformatter",
"pgcopydb":            "pgcopydb",
"pgloader":            "pgloader",
"pg_timetable":        "pg_timetable",
"timescaledb-utils":   "timescaledb-tools timescaledb-event-streamer",
"ivorysql":            "ivorysql5",
"wiltondb":            "wiltondb",
"polardb":             "PolarDB",
"orioledb":            "orioledb_17 oriolepg_17",
"openhalodb":          "openhalodb",
"percona-core":        "percona-postgresql18,percona-postgresql18-server,percona-postgresql18-contrib,percona-postgresql18-plperl,percona-postgresql18-plpython3,percona-postgresql18-pltcl,percona-pg_tde18",
"percona-main":        "percona-postgresql18,percona-postgresql18-server,percona-postgresql18-contrib,percona-postgresql18-plperl,percona-postgresql18-plpython3,percona-postgresql18-pltcl,percona-pg_tde18,percona-postgis35_18,percona-postgis35_18-client,percona-postgis35_18-utils,percona-pgvector_18,percona-wal2json18,percona-pg_repack18,percona-pgaudit18,percona-pgaudit18_set_user,percona-pg_stat_monitor18,percona-pg_gather",
"ferretdb":            "ferretdb2",
"duckdb":              "duckdb",
"etcd":                "etcd",
"haproxy":             "haproxy",
"pig":                 "pig",
"vray":                "vray",
"juicefs":             "juicefs",
"restic":              "restic",
"rclone":              "rclone",
"genai-toolbox":       "genai-toolbox",
"tigerbeetle":         "tigerbeetle",
"clickhouse":          "clickhouse-server clickhouse-client clickhouse-common-static",
"victoria":            "victoria-metrics victoria-metrics-cluster vmutils grafana-victoriametrics-ds victoria-logs vlogscil vlagent grafana-victorialogs-ds",
"vmetrics":            "victoria-metrics victoria-metrics-cluster vmutils grafana-victoriametrics-ds",
"vlogs":               "victoria-logs vlogscil vlagent grafana-victorialogs-ds",
```

</details>



**Install for another PG**

`pig` will use the default postgres installation in your active `PATH`,
but you can install extension for a specific installation with `-v` (when using the PGDG convention),
or passing any `pg_config` path for custom installation.

```bash
pig ext install pg_duckdb -v 17     # install the extension for pg17
pig ext install pg_duckdb -p /usr/lib/postgresql/16/bin/pg_config    # specify a pg16 pg_config  
```

**Install a specific Version**

You can also install PostgreSQL kernel packages with:

```bash
pig ext install pgvector=0.8.0 # install pgvector 0.8.0
pig ext install pg16=16.5      # install PostgreSQL 16 with a specific minor version
```

> Beware the **APT** repo may only have the latest minor version for its software (and require the full version string)


**Search Extension**

You can perform fuzzy search on extension name, description, and category.

```bash
$ pig ext ls olap
INFO[14:04:25] found 14 extensions matching 'olap':
Name            Status     Version  Cate  Flags   License       Repo     PGVer  Package               Description
----            ------     -------  ----  ------  -------       ------   -----  ------------          ---------------------
citus           installed  14.0.0   OLAP  -dsl--  AGPL-3.0      PIGSTY   16-18  citus_18              Distributed PostgreSQL as an extension
citus_columnar  installed  14.0.0   OLAP  -ds---  AGPL-3.0      PIGSTY   16-18  citus_18              Citus columnar storage engine
columnar        not avail  1.1.2    OLAP  -ds---  AGPL-3.0      PIGSTY   13-16  hydra_18              Hydra Columnar extension
pg_analytics    not avail  0.3.7    OLAP  -ds-t-  PostgreSQL    PIGSTY   14-17  pg_analytics_18       Postgres for analytics, powered by DuckDB
pg_duckdb       installed  1.1.1    OLAP  -dsl--  MIT           PIGSTY   14-18  pg_duckdb_18          DuckDB Embedded in Postgres
pg_mooncake     installed  0.2.0    OLAP  -d-l--  MIT           PIGSTY   14-18  pg_mooncake_18        Columnstore Table in Postgres
pg_clickhouse   available  0.1.2    OLAP  -ds---  Apache-2.0    PIGSTY   13-18  pg_clickhouse_18      Interfaces to query ClickHouse databases from PostgreSQL
duckdb_fdw      available  1.1.2    OLAP  -ds--r  MIT           PIGSTY   13-17  duckdb_fdw_18         DuckDB Foreign Data Wrapper
pg_parquet      installed  0.5.1    OLAP  -dslt-  PostgreSQL    PIGSTY   14-18  pg_parquet_18         copy data between Postgres and Parquet
pg_fkpart       available  1.7.0    OLAP  -d----  GPL-2.0       PIGSTY   13-18  pg_fkpart_18          Table partitioning by foreign key utility
pg_partman      available  5.4.0    OLAP  -ds---  PostgreSQL    PGDG     13-18  pg_partman_18         Extension to manage partitioned tables by time or ID
plproxy         available  2.11.0   OLAP  -ds---  BSD 0-Clause  PGDG     13-18  plproxy_18            Database partitioning implemented as procedural language
pg_strom        not avail  6.0      OLAP  -ds--x  PostgreSQL    PGDG     13-17  pg_strom_18           PG-Strom - big-data processing acceleration using GPU and NVME
tablefunc       installed  1.0      OLAP  -ds-tx  PostgreSQL    CONTRIB  13-18  postgresql18-contrib  functions that manipulate whole tables, including crosstab

(14 Rows) (Status: installed, available, not avail | Flags: b = HasBin, d = HasDDL, s = HasLib, l = NeedLoad, t = Trusted, r = Relocatable, x = Unknown)
```

You can use the `-v 16` or `-p /path/to/pg_config` to find extension availability for other PostgreSQL installation.

**Extension Availability Matrix**

You can check the availability matrix for extensions across different OS/Arch/PG combinations:

```bash
$ pig ext avail pg_duckdb            # show availability matrix for pg_duckdb
$ pig ext avail postgis pgvector     # show matrix for multiple extensions
$ pig ext avail                      # show all packages availability on current OS
```


**Print Extension Summary**

You can get extension metadata with `pig ext info` subcommand:

```bash
$ pig ext info pg_duckdb
╭──────────────────────────────────────────────────────────────────────────────────────────────╮
│ pg_duckdb                                                                                    │
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ DuckDB Embedded in Postgres                                                                  │
├──────────────┬───────────────────────────────────────────────────────────────────────────────┤
│ Extension    │ pg_duckdb                                                                     │
│ Package      │ pg_duckdb                                                                     │
│ Leading Ext  │ pg_duckdb                                                                     │
│ Category     │ OLAP                                                                          │
│ License      │ MIT                                                                           │
│ Language     │ C++                                                                           │
│ Website      │ https://github.com/duckdb/pg_duckdb                                           │
│ Details      │ https://pgext.cloud/e/pg_duckdb                                               │
│ Source       │ pg_duckdb-1.1.1.tar.gz                                                        │
├──────────────┴───────────────────────────────────────────────────────────────────────────────┤
│ Properties                                                                                   │
├──────────────┬────────────┬─────────────┬───────────┬────────────┬─────────────┬─────────────┤
│  Attributes  │ Has Binary │ Has Library │ Need Load │ Create DDL │ Relocatable │   Trusted   │
├──────────────┼────────────┼─────────────┼───────────┼────────────┼─────────────┼─────────────┤
│    -dsl--    │     No     │     Yes     │    Yes    │    Yes     │     No      │     No      │
├──────────────┴────────────┴─────────────┴───────────┴────────────┴─────────────┴─────────────┤
│ Relationship                                                                                 │
├──────────────┬───────────────────────────────────────────────────────────────────────────────┤
│ Requires:    │                                                                               │
│ Required By: │ pg_mooncake                                                                   │
│ See Also:    │ pg_mooncake, duckdb_fdw, pg_analytics, pg_parquet, columnar, citus, citus_... │
├──────────────┴───────────────────────────────────────────────────────────────────────────────┤
│ EXT Summary                                                                                  │
├──────────────┬───────────────────────────────────────────────────────────────────────────────┤
│ Repository   │ PIGSTY                                                                        │
│ Version      │ 1.1.1                                                                         │
│ PG Version   │ 18, 17, 16, 15, 14                                                            │
│ Schemas      │                                                                               │
├──────────────┴───────────────────────────────────────────────────────────────────────────────┤
│ RPM Package                                                                                  │
├──────────────┬───────────────────────────────────────────────────────────────────────────────┤
│ Package      │ pg_duckdb_$v                                                                  │
│ Repository   │ PIGSTY                                                                        │
│ Version      │ 1.1.1                                                                         │
│ PG Version   │ 18, 17, 16, 15, 14                                                            │
├──────────────┴───────────────────────────────────────────────────────────────────────────────┤
│ DEB Package                                                                                  │
├──────────────┬───────────────────────────────────────────────────────────────────────────────┤
│ Package      │ postgresql-$v-pg-duckdb                                                       │
│ Repository   │ PIGSTY                                                                        │
│ Version      │ 1.1.1                                                                         │
│ PG Version   │ 18, 17, 16, 15, 14                                                            │
├──────────────┴───────────────────────────────────────────────────────────────────────────────┤
│ Operation                                                                                    │
├──────────────┬───────────────────────────────────────────────────────────────────────────────┤
│ INSTALL      │ pig ext add pg_duckdb                                                         │
│ CONFIG       │ shared_preload_libraries = 'pg_duckdb'                                        │
│ CREATE       │ CREATE EXTENSION pg_duckdb;                                                   │
│ BUILD        │ pig build pkg pg_duckdb;  # build rpm / deb                                   │
├──────────────┴───────────────────────────────────────────────────────────────────────────────┤
│ Comment: conflict with duckdb_fdw                                                            │
╰──────────────────────────────────────────────────────────────────────────────────────────────╯

# Print in JSON format
$ pig ext info pg_duckdb -o json
{"success":true,"code":0,"message":"Extension: pg_duckdb","data":{"name":"pg_duckdb","pkg":"pg_duckdb","lead_ext":"pg_duckdb","category":"OLAP","license":"MIT","language":"C++","version":"1.1.1","url":"https://github.com/duckdb/pg_duckdb","source":"pg_duckdb-1.1.1.tar.gz","description":"DuckDB Embedded in Postgres","zh_desc":"在PostgreSQL中的嵌入式DuckDB扩展","properties":{"has_bin":false,"has_lib":true,"need_load":true,"need_ddl":true,"relocatable":"f","trusted":"f"},"required_by":["pg_mooncake"],"see_also":["pg_mooncake","duckdb_fdw","pg_analytics","pg_parquet","columnar","citus","citus_columnar","orioledb"],"pg_ver":["18","17","16","15","14"],"rpm_package":{"package":"pg_duckdb_$v","repository":"PIGSTY","version":"1.1.1","pg_ver":["18","17","16","15","14"]},"deb_package":{"package":"postgresql-$v-pg-duckdb","repository":"PIGSTY","version":"1.1.1","pg_ver":["18","17","16","15","14"]},"operations":{"install":"pig ext add pg_duckdb","config":"shared_preload_libraries = 'pg_duckdb'","create":"CREATE EXTENSION pg_duckdb;","build":"pig build pkg pg_duckdb;  # build rpm / deb"},"comment":"conflict with duckdb_fdw"}}
```

**Print Extension Availability**

You can get extension package availability with `pig ext avail` subcommand:

```bash
$ pig ext avail citus

citus (citus) - Distributed PostgreSQL as an extension
Latest: 14.0.0 | 70/84 avail, PG16, PG17, PG18
Details: https://pgext.cloud/e/citus  (green = PIGSTY, blue = PGDG)

╭──────────────┬──────────────┬──────────────┬──────────────┬──────────────┬──────────────┬──────────────╮
│ OS \ PG      │      18      │      17      │      16      │      15      │      14      │      13      │
├──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┤
│ el8.x86_64   │    14.0.0    │    14.0.0    │    14.0.0    │    13.2.0    │    13.0.0    │    11.3.0    │
├──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┤
│ el8.aarch64  │    14.0.0    │    14.0.0    │    14.0.0    │    13.2.0    │    13.0.0    │    11.3.0    │
├──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┤
│ el9.x86_64   │    14.0.0    │    14.0.0    │    14.0.0    │    13.2.0    │    13.0.0    │    11.3.0    │
├──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┤
│ el9.aarch64  │    14.0.0    │    14.0.0    │    14.0.0    │    13.2.0    │    13.0.0    │    11.3.0    │
├──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┤
│ el10.x86_64  │    14.0.0    │    14.0.0    │    14.0.0    │    13.2.0    │              │              │
├──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┤
│ el10.aarch64 │    14.0.0    │    14.0.0    │    14.0.0    │    13.2.0    │              │              │
├──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┤
│ d12.x86_64   │    14.0.0    │    14.0.0    │    14.0.0    │    13.2.0    │    13.0.0    │              │
├──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┤
│ d12.aarch64  │    14.0.0    │    14.0.0    │    14.0.0    │    13.2.0    │    13.0.0    │              │
├──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┤
│ d13.x86_64   │    14.0.0    │    14.0.0    │    14.0.0    │    13.2.0    │              │              │
├──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┤
│ d13.aarch64  │    14.0.0    │    14.0.0    │    14.0.0    │    13.2.0    │              │              │
├──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┤
│ u22.x86_64   │    14.0.0    │    14.0.0    │    14.0.0    │    13.2.0    │    13.0.0    │              │
├──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┤
│ u22.aarch64  │    14.0.0    │    14.0.0    │    14.0.0    │    13.2.0    │    13.0.0    │              │
├──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┤
│ u24.x86_64   │    14.0.0    │    14.0.0    │    14.0.0    │    13.2.0    │    13.0.0    │              │
├──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┼──────────────┤
│ u24.aarch64  │    14.0.0    │    14.0.0    │    14.0.0    │    13.2.0    │    13.0.0    │              │
╰──────────────┴──────────────┴──────────────┴──────────────┴──────────────┴──────────────┴──────────────╯
````

**List Repo**

You can list all available repo / module (repo collection) with `pig repo list`:

```bash
$ pig repo list

os_environment: {code: d13, arch: amd64, type: deb, major: 13}
repo_upstream:  # Available Repo: 15
  - { name: pigsty-local   ,description: 'Pigsty Local'       ,module: local    ,releases: [11,12,13,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'http://${admin_ip}/pigsty ./' }
  - { name: pigsty-pgsql   ,description: 'Pigsty PgSQL'       ,module: pgsql    ,releases: [11,12,13,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'https://repo.pigsty.io/apt/pgsql/${distro_codename} ${distro_codename} main' }
  - { name: pigsty-infra   ,description: 'Pigsty Infra'       ,module: infra    ,releases: [11,12,13,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'https://repo.pigsty.io/apt/infra/ generic main' }
  - { name: docker-ce      ,description: 'Docker'             ,module: infra    ,releases: [11,12,13,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.docker.com/linux/${distro_name} ${distro_codename} stable' }
  - { name: base           ,description: 'Debian Basic'       ,module: node     ,releases: [11,12,13         ] ,arch: [x86_64, aarch64]  ,baseurl: 'http://deb.debian.org/debian/ ${distro_codename} main non-free-firmware' }
  - { name: updates        ,description: 'Debian Updates'     ,module: node     ,releases: [11,12,13         ] ,arch: [x86_64, aarch64]  ,baseurl: 'http://deb.debian.org/debian/ ${distro_codename}-updates main non-free-firmware' }
  - { name: security       ,description: 'Debian Security'    ,module: node     ,releases: [11,12,13         ] ,arch: [x86_64, aarch64]  ,baseurl: 'http://security.debian.org/debian-security ${distro_codename}-security main non-free-firmware' }
  - { name: pgdg           ,description: 'PGDG'               ,module: pgsql    ,releases: [11,12,13,22,24   ] ,arch: [x86_64, aarch64]  ,baseurl: 'http://apt.postgresql.org/pub/repos/apt/ ${distro_codename}-pgdg main' }
  - { name: pgdg-beta      ,description: 'PGDG Beta'          ,module: beta     ,releases: [11,12,13,22,24   ] ,arch: [x86_64, aarch64]  ,baseurl: 'http://apt.postgresql.org/pub/repos/apt/ ${distro_codename}-pgdg-testing main 19' }
  - { name: groonga        ,description: 'Groonga Debian'     ,module: groonga  ,releases: [11,12,13         ] ,arch: [x86_64, aarch64]  ,baseurl: 'https://packages.groonga.org/debian/ ${distro_codename} main' }
  - { name: grafana        ,description: 'Grafana'            ,module: grafana  ,releases: [11,12,13,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'https://apt.grafana.com stable main' }
  - { name: kubernetes     ,description: 'Kubernetes'         ,module: kube     ,releases: [11,12,13,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'https://pkgs.k8s.io/core:/stable:/v1.33/deb/ /' }
  - { name: gitlab-ee      ,description: 'Gitlab EE'          ,module: gitlab   ,releases: [11,12,13,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'https://packages.gitlab.com/gitlab/gitlab-ee/${distro_name}/ ${distro_codename} main' }
  - { name: gitlab-ce      ,description: 'Gitlab CE'          ,module: gitlab   ,releases: [11,12,13,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'https://packages.gitlab.com/gitlab/gitlab-ce/${distro_name}/ ${distro_codename} main' }
  - { name: clickhouse     ,description: 'ClickHouse'         ,module: click    ,releases: [11,12,13,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'https://packages.clickhouse.com/deb/ stable main' }
repo_modules:   # Available Modules: 20
  - all       : pigsty-infra, pigsty-pgsql, pgdg, base, updates, extras, epel, centos-sclo, centos-sclo-rh, baseos, appstream, powertools, crb, security, backports
  - pigsty    : pigsty-infra, pigsty-pgsql
  - pgdg      : pgdg
  - node      : base, updates, extras, epel, centos-sclo, centos-sclo-rh, baseos, appstream, powertools, crb, security, backports
  - infra     : pigsty-infra, nginx
  - docker    : docker-ce
  - pgsql     : pigsty-pgsql, pgdg-common, pgdg-el8fix, pgdg-el9fix, pgdg13, pgdg14, pgdg15, pgdg16, pgdg17, pgdg18, pgdg
  - extra     : pgdg-extras, pgdg13-nonfree, pgdg14-nonfree, pgdg15-nonfree, pgdg16-nonfree, pgdg17-nonfree, timescaledb, citus
  - mssql     : wiltondb
  - mysql     : mysql
  - kube      : kubernetes
  - grafana   : grafana
  - beta      : pgdg19-beta, pgdg-beta
  - click     : clickhouse
  - gitlab    : gitlab-ee, gitlab-ce
  - groonga   : groonga
  - haproxy   : haproxyd, haproxyu
  - local     : pigsty-local
  - mongo     : mongo
  - percona   : percona
  - redis     : redis
```

**Pigsty Management**

The **pig** can also be used as a [**CLI**](https://pigsty.io/docs/pig/sty) tool for PostgreSQL & [Pigsty](https://pigsty.io/) — the battery-include free PostgreSQL RDS.
Which brings HA, PITR, Monitoring, IaC, and all the extensions to your PostgreSQL cluster.

```bash
pig sty init     # install pigsty to ~/pigsty 
pig sty boot     # install ansible and other pre-deps 
pig sty conf     # auto-generate pigsty.yml config file
pig sty deploy   # run the deploy.yml playbook
```

You can use the `pig sty` subcommand to bootstrap pigsty on current node.


**Usage in Docker**

The pig cli is intended to be used on standard linux environment, but you can also use it inside container:

```dockerfile
FROM debian:13
USER root
WORKDIR /root/
CMD ["/bin/bash"]

RUN apt update && apt install -y ca-certificates curl && curl https://repo.pigsty.io/pig | bash
RUN pig repo set && pig install -y -v 18 pgsql pg_duckdb timescaledb postgis pgvector
```

**Build Extension**

You can even build rpm / deb directly with `pig build` subcommand:

You can prepare an extension building environment with docker & pig easily:

<details><summary>Building RPM/DEB with pig</summary>

```dockerfile
FROM debian:13
USER root
WORKDIR /root/
CMD ["/bin/bash"]

RUN apt update && apt install -y ca-certificates vim ncdu wget curl rsync unzip && \
    curl https://repo.pigsty.io/pig | bash -s && pig repo add --remove && apt clean
RUN pig repo set && pig build tool && pig build spec && pig build rust && pig build pgrx -v 0.16.1
```

```bash
docker build -t d13:latest .
docker run --name=d13 -d -it d13:latest /bin/bash
docker exec -it d13 /bin/bash
```

</details>

Then building an extension such as timescaledb is as simple as:

```bash
$ pig build pkg timescaledb  # now you can build extension with pig!
```



--------

## Compatibility

`pig` runs on: RHEL 8/9/10, Ubuntu 22.04/24.04, and Debian 12/13 and [compatible OS](https://pigsty.io/docs/ref/linux)

|   Code   | Distribution                      |  `x86_64`  | `aarch64`  |
|:--------:|-----------------------------------|:----------:|:----------:|
| **el10** | RHEL 10 / Rocky10 / Alma10  / ... | PG 18 - 13 | PG 18 - 13 |
| **el9**  | RHEL 9 / Rocky9 / Alma9  / ...    | PG 18 - 13 | PG 18 - 13 |
| **el8**  | RHEL 8 / Rocky8 / Alma8 / ...     | PG 18 - 13 | PG 18 - 13 |
| **u24**  | Ubuntu 24.04 (`noble`)            | PG 18 - 13 | PG 18 - 13 |
| **u22**  | Ubuntu 22.04 (`jammy`)            | PG 18 - 13 | PG 18 - 13 |
| **d13**  | Debian 13 (`trixie`)              | PG 18 - 13 | PG 18 - 13 |
| **d12**  | Debian 12 (`bookworm`)            | PG 18 - 13 | PG 18 - 13 |


--------

## About

[![Author: RuohangFeng](https://img.shields.io/badge/Author-Ruohang_Feng-steelblue?style=flat)](https://blog.vonng.com/en/about)
[![About: @Vonng](https://img.shields.io/badge/%40Vonng-steelblue?style=flat)](https://vonng.com/en/)
[![Mail: rh@vonng.com](https://img.shields.io/badge/rh%40vonng.com-steelblue?style=flat)](mailto:rh@vonng.com)
[![Copyright: 2018-2025 rh@vonng.com](https://img.shields.io/badge/Copyright-2025_(rh%40vonng.com)-red?logo=c&color=steelblue)](https://github.com/Vonng)
[![License: Apache](https://img.shields.io/badge/License-Apache--2.0-steelblue?style=flat&logo=opensourceinitiative&logoColor=green)](https://github.com/pgsty/pig/blob/main/LICENSE)

[![pig-meme](https://github.com/user-attachments/assets/4c2310a0-7551-4233-875f-18d0bd87a03e)](https://medium.com/@fengruohang/the-idea-way-to-deliver-postgresql-extensions-35646464bb71)

# PIG - Postgres Install Genius

[![Webite: pigsty.io](https://img.shields.io/badge/website-ext.pigsty.io-slategray?style=flat&logo=cilium&logoColor=white)](https://ext.pigsty.io)
[![Version: v0.1.1](https://img.shields.io/badge/version-v0.1.1-slategray?style=flat&logo=cilium&logoColor=white)](https://github.com/pgsty/pig/releases/tag/v0.1.1)
[![License: Apache-2.0](https://img.shields.io/github/license/pgsty/pig?logo=opensourceinitiative&logoColor=green&color=slategray)](https://github.com/pgsty/pig/blob/main/LICENSE)
[![Extensions: 350](https://img.shields.io/badge/extensions-350-%233E668F?style=flat&logo=postgresql&logoColor=white&labelColor=3E668F)](https://ext.pigsty.io/#/list)

[**pig**](https://ext.pigsty.io/#/pig) is an open-source PostgreSQL (& Extension) Package Manager for [mainstream](#compatibility) (EL/Debian/Ubuntu) Linux distro.

Install PostgreSQL 13-17 along with [350 extensions](https://ext.pigsty.io/#/list) on (`amd64` / `arm64`) with native package manager

[![pig](https://github.com/user-attachments/assets/e377ed91-37a9-4c27-8854-034c81fa1b29)](https://medium.com/@fengruohang/the-idea-way-to-deliver-postgresql-extensions-35646464bb71)

> Blog: [The idea way to deliver PostgreSQL extensions](https://medium.com/@fengruohang/the-idea-way-to-deliver-postgresql-extensions-35646464bb71)


--------

## Get Started

[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17,16,15,14,13-%233E668F?style=flat&logo=postgresql&labelColor=3E668F&logoColor=white)](https://pigsty.io/docs/pgsql)
[![Linux](https://img.shields.io/badge/Linux-amd64/arm64-%23FCC624?style=flat&logo=linux&labelColor=FCC624&logoColor=black)](https://pigsty.io/docs/node)
[![EL Support: 8/9](https://img.shields.io/badge/EL-8/9-red?style=flat&logo=redhat&logoColor=red)](https://ext.pigsty.io/#/rpm)
[![Debian Support: 12](https://img.shields.io/badge/Debian-12-%23A81D33?style=flat&logo=debian&logoColor=%23A81D33)](https://pigsty.io/docs/reference/compatibility/)
[![Ubuntu Support: 22/24](https://img.shields.io/badge/Ubuntu-22/24-%23E95420?style=flat&logo=ubuntu&logoColor=%23E95420)](https://ext.pigsty.io/#/deb)

[Install](#installation) the `pig` package first, you can also install via [apt/yum](#installation) command.

```bash
curl -fsSL https://repo.pigsty.io/pig | bash     # cloudflare, default 
curl -fsSL https://repo.pigsty.cc/pig | bash     # mainland china mirror
```

Then it's ready to use, assume you want to install the [`pg_duckdb`](https://ext.pigsty.io/#/pg_duckdb) extension:

```bash
$ pig repo add pigsty pgdg -u  # add pgdg & pigsty repo, update cache
$ pig repo set -u              # alternatively, you can overwrite all existing repos, brute but effective

$ pig ext install pg17         # install PostgreSQL 17 kernels with native PGDG packages
$ pig ext install pg_duckdb    # install the pg_duckdb extension (for current pg17)
```

That's it! All set! you can check with the `pig ext status` sub command:

```bash
$ pig ext status               # show installed extension and pg status
                               # to print built-in contrib extension, use -c|--contrib flag
Installed PG Vers :  17 (active)
Active PostgreSQL :  PostgreSQL 17.2
PostgreSQL        :  PostgreSQL 17.2
Binary Path       :  /usr/pgsql-17/bin
Library Path      :  /usr/pgsql-17/lib
Extension Path    :  /usr/pgsql-17/share/extension
Extension Stat    :  1 Installed (PIGSTY 1, PGDG 0) + 67 CONTRIB = 68 Total

Name       Version  Cate  Flags   License  Repo    Package        Description
----       -------  ----  ------  -------  ------  ------------   ---------------------
pg_duckdb  0.2.0    OLAP  -dsl--  MIT      PIGSTY  pg_duckdb_17*  DuckDB Embedded in Postgres

(1 Rows) (Flags: b = HasBin, d = HasDDL, s = HasSolib, l = NeedLoad, t = Trusted, r = Relocatable, x = Unknown)
```

Check the [advanced usage](#advanced-usage) for details and [list 350 available extensions](https://ext.pigsty.io/#/list).

[![asciicast](https://asciinema.org/a/695902.svg)](https://asciinema.org/a/695902)


--------

## Installation

The `pig` util is a standalone go binary with no dependencies. you can just download the binary
or use the following commands to add the repo and install it via package manager (recommended).

For Ubuntu 22.04 / 24.04 & Debian 12 or any compatible platforms:

```bash
sudo tee /etc/apt/sources.list.d/pigsty.list > /dev/null <<EOF
deb [trusted=yes] https://repo.pigsty.io/apt/infra generic main 
EOF
sudo apt update; sudo apt install -y pig
```

For EL 8/9 and compatible platforms:

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



--------

## Advanced Usage

**Environment Status**

```bash
pig status                       # show os & pg status
pig repo status                  # show upstream repo status
pig ext  status                  # show extension status 
```

**Extension Management**

```bash
pig ext list    [query]      # list & search extension      
pig ext info    [ext...]     # get information of a specific extension
pig ext status  [-v]         # show installed extension and pg status
pig ext add     [ext...]     # install extension for current pg version
pig ext rm      [ext...]     # remove extension for current pg version
pig ext update  [ext...]     # update extension to the latest version
pig ext import  [ext...]     # download extension to local repo
pig ext link    [ext...]     # link postgres installation to path
pig ext build   [ext...]     # setup building env for extension
```

**Repo Management**

```bash
pig repo list                    # available repo list             (info)
pig repo info   [repo|module...] # show repo info                  (info)
pig repo status                  # show current repo status        (info)
pig repo add    [repo|module...] # add repo and modules            (root)
pig repo rm     [repo|module...] # remove repo & modules           (root)
pig repo update                  # update repo pkg cache           (root)
pig repo create                  # create repo on current system   (root)
pig repo boot                    # boot repo from offline package  (root)
pig repo cache                   # cache repo as offline package   (root)
```

**Radical Repo Admin**

The default `pig repo add pigsty pgdg` will add the `PGDG` repo and [`PIGSTY`](https://ext.pigsty.io) repo to your system.
While the following command will backup & wipe your existing repo and add all require repo to your system.

```bash
pig repo add all --ru        # This will OVERWRITE all existing repo with node,pgdg,pigsty repo
```

And you can recover you old repos at `/etc/apt/backup` or `/etc/yum.repos.d/backup`.



**Install PostgreSQL**

You can also install PostgreSQL kernel packages with

```bash
pig ext install pg17          # install PostgreSQL 17 kernels (all but devel)
pig ext install pg16-simple   # install PostgreSQL 16 kernels with minimal packages
pig ext install pg15 -y       # install PostgreSQL 15 kernels with auto-confirm
pig ext install pg14=14.3     # install PostgreSQL 14 kernels with an specific minor version
pig ext install pg13=13.10    # install PostgreSQL 13 kernels
```

You can also use other package alias, it will translate to corresponding package on your OS distro
and the `$v` will be replaced with the active or given pg version number, such as `17`, `16`, etc...

```yaml
pg17:        "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl postgresql$v-llvmjit",
pg16-core:   "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl postgresql$v-test postgresql$v-devel postgresql$v-llvmjit",
pg15-simple: "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl",
pg14-client: "postgresql$v",
pg13-server: "postgresql$v-server postgresql$v-libs postgresql$v-contrib",
pg17-devel:  "postgresql$v-devel",
```

<details><summary>More Alias</summary>


```yaml
pgsql:        "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl postgresql$v-llvmjit",
pgsql-core:   "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl postgresql$v-test postgresql$v-devel postgresql$v-llvmjit",
pgsql-simple: "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl",
pgsql-client: "postgresql$v",
pgsql-server: "postgresql$v-server postgresql$v-libs postgresql$v-contrib",
pgsql-devel:  "postgresql$v-devel",
pgsql-basic:  "pg_repack_$v* wal2json_$v* pgvector_$v*",
postgresql:   "postgresql$v*",
pgsql-common: "patroni patroni-etcd pgbouncer pgbackrest pg_exporter pgbadger vip-manager",
patroni:      "patroni patroni-etcd",
pgbouncer:    "pgbouncer",
pgbackrest:   "pgbackrest",
pg_exporter:  "pg_exporter",
vip-manager:  "vip-manager",
pgbadger:     "pgbadger",
pg_activity:  "pg_activity",
pg_filedump:  "pg_filedump",
pgxnclient:   "pgxnclient",
pgformatter:  "pgformatter",
pgcopydb:     "pgcopydb",
pgloader:     "pgloader",
pg_timetable: "pg_timetable",
wiltondb:     "wiltondb",
polardb:      "PolarDB",
ivorysql:     "ivorysql3 ivorysql3-server ivorysql3-contrib ivorysql3-libs ivorysql3-plperl ivorysql3-plpython3 ivorysql3-pltcl ivorysql3-test",
ivorysql-all: "ivorysql3 ivorysql3-server ivorysql3-contrib ivorysql3-libs ivorysql3-plperl ivorysql3-plpython3 ivorysql3-pltcl ivorysql3-test ivorysql3-docs ivorysql3-devel ivorysql3-llvmjit",
```

</details>



**Install for another PG**

`pig` will use the default postgres installation in your active `PATH`,
but you can install extension for a specific installation with `-v` (when using the PGDG convention),
or passing any `pg_config` path for custom installation.

```bash
pig ext install pg_duckdb -v 16     # install the extension for pg16
pig ext install pg_duckdb -p /usr/lib/postgresql/17/bin/pg_config    # specify a pg17 pg_config  
```

**Install a specific Version**

You can also install PostgreSQL kernel packages with

```bash
pig ext install pgvector=0.7.0 # install pgvector 0.7.0
pig ext install pg16=16.5      # install PostgreSQL 16 with a specific minor version
```

> Beware the PGDG **APT** repo may only have the latest minor version for its software


**Search Extension**

You can perform fuzzy search on extension name, description, and category.

```bash
$ pig ext ls duck  # fuzzy search
INFO[11:16:05] found 6 extensions matching 'duck':
Name        State  Version  Cate   Flags   License       Repo     PGVer     Package                   Description
----        -----  -------  ----   ------  -------       ------   --------  ------------              ---------------------
pg_duckdb   avail  0.2.0    OLAP   -dsl--  MIT           PIGSTY   14-17     postgresql-17-pg-duckdb   DuckDB Embedded in Postgres
duckdb_fdw  avail  1.1.2    OLAP   -ds--r  MIT           PIGSTY   13-17     postgresql-17-duckdb-fdw  DuckDB Foreign Data Wrapper
pguecc      avail  1.0      FUNC   -ds-xr  BSD 2-Clause  PIGSTY   13-17     postgresql-17-pg-ecdsa    uECC bindings for Postgres
adminpack   n/a    2.1      ADMIN  -ds--x  PostgreSQL    CONTRIB  13-16     postgresql-17             administrative functions for PostgreSQL
credcheck   avail  2.8.0    SEC    -ds---  MIT           PGDG     13-17     postgresql-17-credcheck   credcheck - postgresql plain text credential checker
dblink      added  1.2      FDW    -ds--x  PostgreSQL    CONTRIB  13-17     postgresql-17             connect to other PostgreSQL databases from within a database

$ pig ext ls pg_duckdb # exact match
INFO[11:16:22] found 1 extensions matching 'pg_duckdb':
Name       State  Version  Cate  Flags   License  Repo    PGVer     Package                  Description
----       -----  -------  ----  ------  -------  ------  --------  ------------             ---------------------
pg_duckdb  avail  0.2.0    OLAP  -dsl--  MIT      PIGSTY  14-17     postgresql-17-pg-duckdb  DuckDB Embedded in Postgres

$ pig ext ls time   # list exact category
INFO[15:11:23] found 9 extensions matching 'time':
Name             State  Version  Cate  Flags   License       Repo    PGVer     Package              Description
----             -----  -------  ----  ------  -------       ------  --------  ------------         ---------------------
timescaledb      avail  2.17.2   TIME  -dsl--  PIGSTY        PIGSTY  14-17     pg_timescaledb_17*   Enables scalable inserts and complex queries for time-series dat
timeseries       n/a    0.1.6    TIME  -d----  PostgreSQL    PIGSTY  13-16     pg_timeseries_17     Convenience API for Tembo time series stack
periods          avail  1.2      TIME  -ds---  PostgreSQL    PGDG    13-17     periods_17*          Provide Standard SQL functionality for PERIODs and SYSTEM VERSIO
temporal_tables  avail  1.2.2    TIME  -ds--r  BSD 2-Clause  PIGSTY  13-17     temporal_tables_17*  temporal tables
emaj             avail  4.5.0    TIME  -ds---  GPL-3.0       PGDG    13-17     e-maj_17*            Enables fine-grained write logging and time travel on subsets of
table_version    avail  1.11.1   TIME  -ds---  BSD 3-Clause  PIGSTY  13-17     table_version_17*    PostgreSQL table versioning extension
pg_cron          avail  1.6      TIME  -dsl--  PostgreSQL    PGDG    13-17     pg_cron_17*          Job scheduler for PostgreSQL
pg_later         avail  0.2.0    TIME  -ds---  PostgreSQL    PIGSTY  13-17     pg_later_17          pg_later: Run queries now and get results later
pg_background    avail  1.3      TIME  -ds--r  GPL-3.0       PGDG    13-17     pg_background_17*    Run SQL queries in the background

(9 Rows) (State: added|avail|n/a,Flags: b = HasBin, d = HasDDL, s = HasSolib, l = NeedLoad, t = Trusted, r = Relocatable, x = Unknown)
```



**Print Extension Summary**

You can get extension metadata with `pig ext info` subcommand:

```bash
$ pig ext info pg_duckdb
╭────────────────────────────────────────────────────────────────────────────╮
│ pg_duckdb                                                                  │
├────────────────────────────────────────────────────────────────────────────┤
│ DuckDB Embedded in Postgres                                                │
├────────────────────────────────────────────────────────────────────────────┤
│ Extension : pg_duckdb                                                      │
│ Alias     : pg_duckdb                                                      │
│ Category  : OLAP                                                           │
│ Version   : 0.2.0                                                          │
│ License   : MIT                                                            │
│ Website   : https://github.com/duckdb/pg_duckdb                            │
│ Details   : https://ext.pigsty.io/#/pg_duckdb                              │
├────────────────────────────────────────────────────────────────────────────┤
│ Extension Properties                                                       │
├────────────────────────────────────────────────────────────────────────────┤
│ PostgreSQL Ver │  Available on: 17, 16, 15, 14                             │
│ CREATE  :  Yes │  CREATE EXTENSION pg_duckdb;                              │
│ DYLOAD  :  Yes │  SET shared_preload_libraries = 'pg_duckdb'               │
│ TRUST   :  No  │  require database superuser to install                    │
│ Reloc   :  No  │  Schemas: []                                              │
│ Depend  :  No  │                                                           │
├────────────────────────────────────────────────────────────────────────────┤
│ RPM Package                                                                │
├────────────────────────────────────────────────────────────────────────────┤
│ Repository     │  PIGSTY                                                   │
│ Package        │  pg_duckdb_$v*                                            │
│ Version        │  0.2.0                                                    │
│ Availability   │  17, 16, 15, 14                                           │
├────────────────────────────────────────────────────────────────────────────┤
│ DEB Package                                                                │
├────────────────────────────────────────────────────────────────────────────┤
│ Repository     │  PIGSTY                                                   │
│ Package        │  postgresql-$v-pg-duckdb                                  │
│ Version        │  0.2.0                                                    │
│ Availability   │  17, 16, 15, 14                                           │
├────────────────────────────────────────────────────────────────────────────┤
│ Known Issues                                                               │
├────────────────────────────────────────────────────────────────────────────┤
│ el8                                                                        │
├────────────────────────────────────────────────────────────────────────────┤
│ Additional Comments                                                        │
├────────────────────────────────────────────────────────────────────────────┤
│ broken on el8 (libstdc++ too low), conflict with duckdb_fdw                │
╰────────────────────────────────────────────────────────────────────────────╯
```

**List Repo**

```bash
os_environment: {code: el9, arch: amd64, type: rpm, major: 9}
repo_upstream:  # Available Repo: 31
  - { name: pigsty-local   ,description: 'Pigsty Local'       ,module: local    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'file:///www/pigsty' }
  - { name: pigsty-infra   ,description: 'Pigsty INFRA'       ,module: infra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://repo.pigsty.io/yum/infra/$basearch' }
  - { name: pigsty-pgsql   ,description: 'Pigsty PGSQL'       ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://repo.pigsty.io/yum/pgsql/el$releasever.$basearch' }
  - { name: nginx          ,description: 'Nginx Repo'         ,module: infra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://nginx.org/packages/rhel/$releasever/$basearch/' }
  - { name: baseos         ,description: 'EL 8+ BaseOS'       ,module: node     ,releases: [8,9]            ,arch: [x86_64, aarch64]  ,baseurl: 'https://dl.rockylinux.org/pub/rocky/$releasever/BaseOS/$basearch/os/' }
  - { name: appstream      ,description: 'EL 8+ AppStream'    ,module: node     ,releases: [8,9]            ,arch: [x86_64, aarch64]  ,baseurl: 'https://dl.rockylinux.org/pub/rocky/$releasever/AppStream/$basearch/os/' }
  - { name: extras         ,description: 'EL 8+ Extras'       ,module: node     ,releases: [8,9]            ,arch: [x86_64, aarch64]  ,baseurl: 'https://dl.rockylinux.org/pub/rocky/$releasever/extras/$basearch/os/' }
  - { name: crb            ,description: 'EL 9 CRB'           ,module: node     ,releases: [9]              ,arch: [x86_64, aarch64]  ,baseurl: 'https://dl.rockylinux.org/pub/rocky/$releasever/CRB/$basearch/os/' }
  - { name: epel           ,description: 'EL 8+ EPEL'         ,module: node     ,releases: [8,9]            ,arch: [x86_64, aarch64]  ,baseurl: 'http://download.fedoraproject.org/pub/epel/$releasever/Everything/$basearch/' }
  - { name: pgdg-common    ,description: 'PostgreSQL Common'  ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/common/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg-el9fix    ,description: 'PostgreSQL EL9FIX'  ,module: pgsql    ,releases: [9]              ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/common/pgdg-rocky9-sysupdates/redhat/rhel-9-x86_64/' }
  - { name: pgdg12         ,description: 'PostgreSQL 12'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/12/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg13         ,description: 'PostgreSQL 13'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/13/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg14         ,description: 'PostgreSQL 14'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/14/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg15         ,description: 'PostgreSQL 15'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/15/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg16         ,description: 'PostgreSQL 16'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/16/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg17         ,description: 'PostgreSQL 17'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/17/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg-extras    ,description: 'PostgreSQL Extra'   ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/common/pgdg-rhel$releasever-extras/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg12-nonfree ,description: 'PostgreSQL 12+'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/12/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg13-nonfree ,description: 'PostgreSQL 13+'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/13/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg14-nonfree ,description: 'PostgreSQL 14+'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/14/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg15-nonfree ,description: 'PostgreSQL 15+'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/15/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg16-nonfree ,description: 'PostgreSQL 16+'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/16/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg17-nonfree ,description: 'PostgreSQL 17+'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64]           ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/17/redhat/rhel-$releasever-$basearch' }
  - { name: timescaledb    ,description: 'TimescaleDB'        ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://packagecloud.io/timescale/timescaledb/el/$releasever/$basearch' }
  - { name: docker-ce      ,description: 'Docker CE'          ,module: docker   ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.docker.com/linux/centos/$releasever/$basearch/stable' }
  - { name: kubernetes     ,description: 'Kubernetes'         ,module: kube     ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://pkgs.k8s.io/core:/stable:/v1.31/rpm/' }
  - { name: wiltondb       ,description: 'WiltonDB'           ,module: mssql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.copr.fedorainfracloud.org/results/wiltondb/wiltondb/epel-$releasever-$basearch/' }
  - { name: ivorysql       ,description: 'IvorySQL'           ,module: ivory    ,releases: [7,8,9]          ,arch: [x86_64]           ,baseurl: 'https://repo.pigsty.io/yum/ivory/el$releasever.$basearch' }
  - { name: mysql          ,description: 'MySQL'              ,module: mysql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://repo.mysql.com/yum/mysql-8.0-community/el/$releasever/$basearch/' }
  - { name: grafana        ,description: 'Grafana'            ,module: grafana  ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://rpm.grafana.com' }
repo_modules:   # Available Modules: 15
  - all       : pigsty-infra, pigsty-pgsql, pgdg-common, pgdg-el8fix, pgdg-el9fix, pgdg17, pgdg16, pgdg15, pgdg14, pgdg13, baseos, appstream, extras, powertools, crb, epel, base, updates, security, backports
  - pigsty    : pigsty-infra, pigsty-pgsql
  - pgdg      : pgdg-common, pgdg-el8fix, pgdg-el9fix, pgdg17, pgdg16, pgdg15, pgdg14, pgdg13
  - node      : baseos, appstream, extras, powertools, crb, epel, base, updates, security, backports
  - infra     : pigsty-infra, nginx
  - pgsql     : pigsty-pgsql, pgdg-common, pgdg-el8fix, pgdg-el9fix, pgdg12, pgdg13, pgdg14, pgdg15, pgdg16, pgdg17, pgdg
  - extra     : pgdg-extras, pgdg12-nonfree, pgdg13-nonfree, pgdg14-nonfree, pgdg15-nonfree, pgdg16-nonfree, pgdg17-nonfree, timescaledb, citus
  - mssql     : wiltondb
  - mysql     : mysql
  - docker    : docker-ce
  - kube      : kubernetes
  - grafana   : grafana
  - pgml      : pgml
  - ivory     : ivorysql
  - local     : pigsty-local
```

**Install Pigsty**

The **pig** can also be used as a cli tool for [Pigsty](https://pigsty.io) - the battery-include free PostgreSQL RDS

```bash
pig sty init     # install embed pigsty to ~/pigsty 
pig sty boot     # install ansible and other pre-deps 
pig sty conf     # auto-generate pigsty.yml config file
pig sty install  # run the install.yml playbook
```

You can use the `pig sty` subcommand to bootstrap pigsty on current node.


**Self-Updating**

To update pig itself to the latest version, you can use the following command:

```bash
pig update
```


--------

## Compatibility

`pig` runs on: RHEL 8/9, Ubuntu 22.04/24.04, and Debian 12, on both `amd64/arm64` arch

|  Code   | Distribution                   |  `x86_64`  | `aarch64`  |
|:-------:|--------------------------------|:----------:|:----------:|
| **el9** | RHEL 9 / Rocky9 / Alma9  / ... | PG 17 - 13 | PG 17 - 13 |
| **el8** | RHEL 8 / Rocky8 / Alma8 / ...  | PG 17 - 13 | PG 17 - 13 |
| **u24** | Ubuntu 24.04 (`noble`)         | PG 17 - 13 | PG 17 - 13 |
| **u22** | Ubuntu 22.04 (`jammy`)         | PG 17 - 13 | PG 17 - 13 |
| **d12** | Debian 12 (`bookworm`)         | PG 17 - 13 | PG 17 - 13 |

Here are some bad cases and limitation for above distros:

- [`citus`](https://ext.pigsty.io/#/citus) is not available on `aarch64` and ubuntu 24.04
- [`pljava`](https://ext.pigsty.io/#/pljava) is missing on `el8`
- [`jdbc_fdw`](https://ext.pigsty.io/#/jdbc_fdw) is missing on `el8.aarch64` and `el9.aarch64`
- [`pllua`](https://ext.pigsty.io/#/pllua) is missing on `el8.aarch64` for pg 13,14,15
- [`topn`](https://ext.pigsty.io/#/topn) is missing on `el8.aarch64` and `el9.aarch64` for pg13, and all `deb.aarch64`
- [`pg_partman`](https://ext.pigsty.io/#/pg_partman) and [`timeseries`](https://ext.pigsty.io/#/timeseries) is missing on `u24` for pg13
- [`wiltondb`](https://ext.pigsty.io/#/wiltondb) is missing on `d12`


--------

## About

[![Author: RuohangFeng](https://img.shields.io/badge/Author-Ruohang_Feng-steelblue?style=flat)](https://vonng.com/)
[![About: @Vonng](https://img.shields.io/badge/%40Vonng-steelblue?style=flat)](https://vonng.com/en/)
[![Mail: rh@vonng.com](https://img.shields.io/badge/rh%40vonng.com-steelblue?style=flat)](mailto:rh@vonng.com)
[![Copyright: 2018-2024 rh@Vonng.com](https://img.shields.io/badge/Copyright-2024_(rh%40vonng.com)-red?logo=c&color=steelblue)](https://github.com/Vonng)
[![License: Apache](https://img.shields.io/badge/License-Apaehc--2.0-steelblue?style=flat&logo=opensourceinitiative&logoColor=green)](https://github.com/pgsty/pig/blob/main/LICENSE)

![pig](https://github.com/user-attachments/assets/17333d0d-a77a-4f6a-8fae-9e3f57fa798e)
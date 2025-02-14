# PIG - Postgres Install Genius

[![Catalog: pigsty.io](https://img.shields.io/badge/catalog-ext.pigsty.io-slategray?style=flat&logo=cilium&logoColor=white)](https://ext.pigsty.io)
[![Version: v0.1.4](https://img.shields.io/badge/version-v0.1.4-slategray?style=flat&logo=cilium&logoColor=white)](https://github.com/pgsty/pig/releases/tag/v0.1.4)
[![Pigsty: v3.2.2](https://img.shields.io/badge/Pigsty-v3.2.2-slategray?style=flat&logo=cilium&logoColor=white)](https://pgsty.com)
[![License: Apache-2.0](https://img.shields.io/github/license/pgsty/pig?logo=opensourceinitiative&logoColor=green&color=slategray)](https://github.com/pgsty/pig/blob/main/LICENSE)
[![Extensions: 400](https://img.shields.io/badge/extensions-400-%233E668F?style=flat&logo=postgresql&logoColor=white&labelColor=3E668F)](https://ext.pigsty.io/#/list)

[**pig**](https://ext.pigsty.io/#/pig) is an open-source PostgreSQL (& Extension) Package Manager for [mainstream](#compatibility) (EL/Debian/Ubuntu) Linux.

Install PostgreSQL 13-17 along with [400 extensions](https://ext.pigsty.io/#/list) on (`amd64` / `arm64`) with native OS package manager

> Blog: [The idea way to deliver PostgreSQL extensions](https://medium.com/@fengruohang/the-idea-way-to-deliver-postgresql-extensions-35646464bb71)

[![PostgreSQL Extension Ecosystem](https://pigsty.io/img/pigsty/ecosystem.jpg)](https://medium.com/@fengruohang/postgres-is-eating-the-database-world-157c204dcfc4)


--------

## Get Started

[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17,16,15,14,13-%233E668F?style=flat&logo=postgresql&labelColor=3E668F&logoColor=white)](https://pigsty.io/docs/pgsql)
[![Linux](https://img.shields.io/badge/Linux-amd64/arm64-%23FCC624?style=flat&logo=linux&labelColor=FCC624&logoColor=black)](https://pigsty.io/docs/node)
[![EL Support: 8/9](https://img.shields.io/badge/EL-8/9-red?style=flat&logo=redhat&logoColor=red)](https://ext.pigsty.io/#/rpm)
[![Debian Support: 12](https://img.shields.io/badge/Debian-12-%23A81D33?style=flat&logo=debian&logoColor=%23A81D33)](https://pigsty.io/docs/reference/compatibility/)
[![Ubuntu Support: 22/24](https://img.shields.io/badge/Ubuntu-22/24-%23E95420?style=flat&logo=ubuntu&logoColor=%23E95420)](https://ext.pigsty.io/#/deb)

[Install](#installation) the `pig` package first, (you can also use the `apt` / `yum` or just copy the binary)

```bash
curl -fsSL https://repo.pigsty.io/pig | bash
```

Then it's ready to use, assume you want to install the [`pg_duckdb`](https://ext.pigsty.io/#/pg_duckdb) extension:

```bash
$ pig repo add pigsty pgdg -u  # add pgdg & pigsty repo, then update repo cache
$ pig ext install pg17         # install PostgreSQL 17 kernels with native PGDG packages
$ pig ext install pg_duckdb    # install the pg_duckdb extension (for current pg17)
```

That's it, All set! Check the [advanced usage](#advanced-usage) for details and [the full list 350 available extensions](https://ext.pigsty.io/#/list).

[![asciicast](https://asciinema.org/a/695902.svg)](https://asciinema.org/a/695902)



--------

## Installation

The `pig` util is a standalone go binary with no dependencies. You can install with the script:

```bash
curl -fsSL https://repo.pigsty.io/pig | bash   # cloudflare default
curl -fsSL https://repo.pigsty.cc/pig | bash   # mainland china mirror
```

Or just download and extract the binary, or use the `apt/dnf` package manager:
 
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

`pig` has self update feature, you can update pig itself to the latest version with:

```bash
pig update
```

Or if you only want to update the extension catalog, you can fetch the latest catalog with:

```bash
pig ext upgrade
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
pig ext status  [-v]          # show installed extension and pg status
pig ext add     [ext...]      # install extension for current pg version
pig ext rm      [ext...]      # remove extension for current pg version
pig ext update  [ext...]      # update extension to the latest version
pig ext import  [ext...]      # download extension to local repo
pig ext link    [ext...]      # link postgres installation to path
pig ext upgrade               # fetch the latest extension catalog
```

**Repo Management**

```bash
pig repo list                    # available repo list
pig repo info   [repo|module...] # show repo info
pig repo status                  # show current repo status
pig repo add    [repo|module...] # add repo and modules
pig repo rm     [repo|module...] # remove repo & modules
pig repo update                  # update repo pkg cache
pig repo create                  # create repo on current system
pig repo boot                    # boot repo from offline package
pig repo cache                   # cache repo as offline package
```


**Radical Repo Admin**

The default `pig repo add pigsty pgdg` will add the `PGDG` repo and [`PIGSTY`](https://ext.pigsty.io) repo to your system.
While the following command will backup & wipe your existing repo and add all require repo to your system.

```bash
pig repo add all --ru        # This will OVERWRITE all existing repo with node,pgdg,pigsty repo
```

There's a brutal version of repo add: `repo set`, which will overwrite you existing repo (`-r`) by default.

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

You can link the installed PostgreSQL to the system path with:

```bash
pig ext link pg17             # create /usr/pgsql soft links, and write it to /etc/profile.d/pgsql.sh
. /etc/profile.d/pgsql.sh     # reload the path and take effect immediately
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

You can also install PostgreSQL kernel packages with:

```bash
pig ext install pgvector=0.7.0 # install pgvector 0.7.0
pig ext install pg16=16.5      # install PostgreSQL 16 with a specific minor version
```

> Beware the **APT** repo may only have the latest minor version for its software (and require the full version string)


**Search Extension**

You can perform fuzzy search on extension name, description, and category.

```bash
u22:~$ pig ext ls olap

INFO[01:16:25] found 12 extensions matching 'olap':
Name            State  Version  Cate  Flags   License       Repo     PGVer  Package                     Description
----            -----  -------  ----  ------  -------       ------   -----  ------------                ---------------------
citus           n/a    12.1-1   OLAP  -dsl--  AGPL-3.0      PIGSTY   14-16  postgresql-17-citus         Distributed PostgreSQL as an extension
citus_columnar  n/a    11.3-1   OLAP  -ds---  AGPL-3.0      PIGSTY   14-16  postgresql-17-citus         Citus columnar storage engine
columnar        n/a    11.1-11  OLAP  -ds---  AGPL-3.0      PIGSTY   13-16  postgresql-17-hydra         Hydra Columnar extension
pg_analytics    avail  0.2.4    OLAP  -ds-t-  PostgreSQL    PIGSTY   14-17  postgresql-17-pg-analytics  Postgres for analytics, powered by DuckDB
pg_duckdb       avail  0.2.0    OLAP  -dsl--  MIT           PIGSTY   14-17  postgresql-17-pg-duckdb     DuckDB Embedded in Postgres
duckdb_fdw      avail  1.1.2    OLAP  -ds--r  MIT           PIGSTY   13-17  postgresql-17-duckdb-fdw    DuckDB Foreign Data Wrapper
pg_parquet      avail  0.2.0    OLAP  -dslt-  PostgreSQL    PIGSTY   14-17  postgresql-17-pg-parquet    copy data between Postgres and Parquet
pg_fkpart       avail  1.7      OLAP  -d----  GPL-2.0       PIGSTY   13-17  postgresql-17-pg-fkpart     Table partitioning by foreign key utility
pg_partman      avail  5.2.4    OLAP  -ds---  PostgreSQL    PGDG     13-17  postgresql-17-partman       Extension to manage partitioned tables by time or ID
plproxy         avail  2.11.0   OLAP  -ds---  BSD 0-Clause  PGDG     13-17  postgresql-17-plproxy       Database partitioning implemented as procedural language
pg_strom        n/a    5.1      OLAP  -ds--x  PostgreSQL             n/a                                PG-Strom - big-data processing acceleration using GPU and NVME
tablefunc       added  1.0      OLAP  -ds-tx  PostgreSQL    CONTRIB  13-17  postgresql-17               functions that manipulate whole tables, including crosstab

(12 Rows) (State: added|avail|n/a,Flags: b = HasBin, d = HasDDL, s = HasSolib, l = NeedLoad, t = Trusted, r = Relocatable, x = Unknown)
```

You can use the `-v 16` or `-p /path/to/pg_config` to find extension availability for other PostgreSQL installation.


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
│ Version        │  0.1.0                                                    │
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

You can list all available repo / module (repo collection) with `pig repo list`:

```bash
u22:~$ pig repo list

os_environment: {code: u22, arch: amd64, type: deb, major: 22}
repo_upstream:  # Available Repo: 17
  - { name: pigsty-local   ,description: 'Pigsty Local'       ,module: local    ,releases: [11,12,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'file:///www/pigsty ./' }
  - { name: pigsty-pgsql   ,description: 'Pigsty PgSQL'       ,module: pgsql    ,releases: [11,12,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'https://repo.pigsty.io/apt/pgsql/${distro_codename} ${distro_codename} main' }
  - { name: pigsty-infra   ,description: 'Pigsty Infra'       ,module: infra    ,releases: [11,12,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'https://repo.pigsty.io/apt/infra/ generic main' }
  - { name: nginx          ,description: 'Nginx'              ,module: infra    ,releases: [11,12,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'http://nginx.org/packages/${distro_name} ${distro_codename} nginx' }
  - { name: base           ,description: 'Ubuntu Basic'       ,module: node     ,releases: [20,22,24]       ,arch: [x86_64]           ,baseurl: 'https://mirrors.edge.kernel.org/ubuntu/ ${distro_codename}           main universe multiverse restricted' }
  - { name: updates        ,description: 'Ubuntu Updates'     ,module: node     ,releases: [20,22,24]       ,arch: [x86_64]           ,baseurl: 'https://mirrors.edge.kernel.org/ubuntu/ ${distro_codename}-backports main restricted universe multiverse' }
  - { name: backports      ,description: 'Ubuntu Backports'   ,module: node     ,releases: [20,22,24]       ,arch: [x86_64]           ,baseurl: 'https://mirrors.edge.kernel.org/ubuntu/ ${distro_codename}-security  main restricted universe multiverse' }
  - { name: security       ,description: 'Ubuntu Security'    ,module: node     ,releases: [20,22,24]       ,arch: [x86_64]           ,baseurl: 'https://mirrors.edge.kernel.org/ubuntu/ ${distro_codename}-updates   main restricted universe multiverse' }
  - { name: pgdg           ,description: 'PGDG'               ,module: pgsql    ,releases: [11,12,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'http://apt.postgresql.org/pub/repos/apt/ ${distro_codename}-pgdg main' }
  - { name: citus          ,description: 'Citus'              ,module: extra    ,releases: [11,12,20,22]    ,arch: [x86_64, aarch64]  ,baseurl: 'https://packagecloud.io/citusdata/community/${distro_name}/ ${distro_codename} main' }
  - { name: timescaledb    ,description: 'Timescaledb'        ,module: extra    ,releases: [11,12,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'https://packagecloud.io/timescale/timescaledb/${distro_name}/ ${distro_codename} main' }
  - { name: grafana        ,description: 'Grafana'            ,module: grafana  ,releases: [11,12,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'https://apt.grafana.com stable main' }
  - { name: pgml           ,description: 'PostgresML'         ,module: pgml     ,releases: [22]             ,arch: [x86_64, aarch64]  ,baseurl: 'https://apt.postgresml.org ${distro_codename} main' }
  - { name: wiltondb       ,description: 'WiltonDB'           ,module: mssql    ,releases: [20,22,24]       ,arch: [x86_64, aarch64]  ,baseurl: 'https://ppa.launchpadcontent.net/wiltondb/wiltondb/ubuntu/ ${distro_codename} main' }
  - { name: mysql          ,description: 'MySQL'              ,module: mysql    ,releases: [11,12,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'https://repo.mysql.com/apt/${distro_name} ${distro_codename} mysql-8.0 mysql-tools' }
  - { name: docker-ce      ,description: 'Docker'             ,module: docker   ,releases: [11,12,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.docker.com/linux/${distro_name} ${distro_codename} stable' }
  - { name: kubernetes     ,description: 'Kubernetes'         ,module: kube     ,releases: [11,12,20,22,24] ,arch: [x86_64, aarch64]  ,baseurl: 'https://pkgs.k8s.io/core:/stable:/v1.31/deb/ /' }
repo_modules:   # Available Modules: 15
  - all       : pigsty-infra, pigsty-pgsql, pgdg, baseos, appstream, extras, powertools, crb, epel, base, updates, security, backports
  - pigsty    : pigsty-infra, pigsty-pgsql
  - pgdg      : pgdg
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

**Pigsty Management**

The **pig** can also be used as a cli tool for [Pigsty](https://www.pigsty.io) - the battery-include free PostgreSQL RDS.
Which brings HA, PITR, Monitoring, IaC, and all the extensions to your PostgreSQL cluster.

```bash
pig sty init     # install embed pigsty to ~/pigsty 
pig sty boot     # install ansible and other pre-deps 
pig sty conf     # auto-generate pigsty.yml config file
pig sty install  # run the install.yml playbook
```

You can use the `pig sty` subcommand to bootstrap pigsty on current node.




--------

## Compatibility

`pig` runs on: RHEL 8/9, Ubuntu 22.04/24.04, and Debian 12 and compatible OS

|  Code   | Distribution                   |  `x86_64`  | `aarch64`  |
|:-------:|--------------------------------|:----------:|:----------:|
| **el9** | RHEL 9 / Rocky9 / Alma9  / ... | PG 17 - 13 | PG 17 - 13 |
| **el8** | RHEL 8 / Rocky8 / Alma8 / ...  | PG 17 - 13 | PG 17 - 13 |
| **u24** | Ubuntu 24.04 (`noble`)         | PG 17 - 13 | PG 17 - 13 |
| **u22** | Ubuntu 22.04 (`jammy`)         | PG 17 - 13 | PG 17 - 13 |
| **d12** | Debian 12 (`bookworm`)         | PG 17 - 13 | PG 17 - 13 |

Here are some bad cases and limitation for above Linux distros:

- [`pg_duckdb`](https://ext.pigsty.io/#/pg_duckdb) `el8:*:*`
- [`pljava`](https://ext.pigsty.io/#/pljava): `el8:*:*`
- [`pllua`](https://ext.pigsty.io/#/pllua): `el8:arm:13,14,15`
- [`h3`](https://ext.pigsty.io/#/h3): `el8.amd.pg17`
- [`jdbc_fdw`](https://ext.pigsty.io/#/jdbc_fdw): `el:arm:*`
- [`pg_partman`](https://ext.pigsty.io/#/pg_partman): `u24:*:13`
- [`wiltondb`](https://ext.pigsty.io/#/babelfishpg_common): `d12:*:*`
- [`citus`](https://ext.pigsty.io/#/citus) and [`hydra`](https://ext.pigsty.io/#/hydra) are mutually exclusive
- [`pg_duckdb`](https://ext.pigsty.io/#/pg_duckdb) and [`pg_mooncake`](https://ext.pigsty.io/#/pg_mooncake) are mutually exclusive
- [`pg_duckdb`](https://ext.pigsty.io/#/pg_duckdb) will invalidate [`duckdb_fdw`](https://ext.pigsty.io/#/duckdb_fdw)
- [`documentdb_core`](https://ext.pigsty.io/#/documentdb_core) is not available on `arm` arch
- [`vchord`](https://ext.pigsty.io/#/vchord) 0.2+ is not available on `d12/u22` (0.1 available)


--------

## About

[![Author: RuohangFeng](https://img.shields.io/badge/Author-Ruohang_Feng-steelblue?style=flat)](https://vonng.com/)
[![About: @Vonng](https://img.shields.io/badge/%40Vonng-steelblue?style=flat)](https://vonng.com/en/)
[![Mail: rh@vonng.com](https://img.shields.io/badge/rh%40vonng.com-steelblue?style=flat)](mailto:rh@vonng.com)
[![Copyright: 2018-2025 rh@Vonng.com](https://img.shields.io/badge/Copyright-2025_(rh%40vonng.com)-red?logo=c&color=steelblue)](https://github.com/Vonng)
[![License: Apache](https://img.shields.io/badge/License-Apache--2.0-steelblue?style=flat&logo=opensourceinitiative&logoColor=green)](https://github.com/pgsty/pig/blob/main/LICENSE)

![pig](https://github.com/user-attachments/assets/17333d0d-a77a-4f6a-8fae-9e3f57fa798e)

[![pig-meme](https://github.com/user-attachments/assets/6d6e8740-0474-4f9e-8960-a141f90986b9)](https://medium.com/@fengruohang/the-idea-way-to-deliver-postgresql-extensions-35646464bb71)
---
title: Get Started
description: Get Started with pig, the PostgreSQL extension manager.
icon: Play
weight: 200
breadcrumbs: false
---


[**Install**](/pig/install/) the `pig` package via the install script (or [other approaches](/pig/install/)):

```bash tab="default"
curl -fsSL https://repo.pigsty.io/pig | bash
```
```bash tab="mirror"
curl -fsSL https://repo.pigsty.cc/pig | bash
```

Then it’s ready to use, assume you want to install the [**`pg_duckdb`**](/e/pg_duckdb/) extension:

```bash
$ pig repo add pigsty pgdg -u  # add pgdg & pigsty repo, then update repo cache
$ pig ext install pg17         # install PostgreSQL 17 kernels with native PGDG packages
$ pig ext install pg_duckdb    # install the pg_duckdb extension (for current pg17)
```

Check details about [**repo**](/pig/repo/) and [**ext**](/pig/ext/) sub command.





------

## Examples

Now let's get started with some real examples.


### Overwrite Repo

The default `pig repo add pigsty pgdg` will add the [`PGDG`](https://www.postgresql.org/download/linux/) repo and [`PIGSTY`](/repo/) repo to your system.
While the following command will backup & wipe your existing repo and add all require repo to your system.

```bash
pig repo add all --ru        # This will OVERWRITE all existing repo with node,pgdg,pigsty repo
```

<Callout title="Where are my existing repo files?" type="warn">

There’s a brutal version of [`repo add`](/pig/repo#repo-add): `repo set`, which will overwrite you existing repo (`-r`) by default.
And you can recover you old repos at `/etc/apt/backup` or `/etc/yum.repos.d/backup`.

</Callout>


--------

### Install PG Kernel

You can also install PostgreSQL kernel packages with:

```bash
pig ext install pg17          # install PostgreSQL 17 kernels (all but devel)
pig ext install pg16-simple   # install PostgreSQL 16 kernels with minimal packages
pig ext install pg15 -y       # install PostgreSQL 15 kernels with auto-confirm
pig ext install pg14=14.3     # install PostgreSQL 14 kernels with an specific minor version
pig ext install pg13=13.10    # install PostgreSQL 13 kernels
```

To activate certain postgres kernel binaries in your `PATH`, use the `pig ext link` command:

```bash
pig ext link pg17             # create /usr/pgsql soft links, and write it to /etc/profile.d/pgsql.sh
. /etc/profile.d/pgsql.sh     # reload the path and take effect immediately
```


--------

### Using Alias

You can also use other package aliases, it will translate to corresponding package on your OS distro
and the `$v` will be replaced with the active or given pg version number, such as `17`, `16`, etc… ([**All Aliases**](https://github.com/pgsty/pig/blob/main/cli/ext/catalog.go#L149)):

```yaml tab="EL"
"pgsql":        "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl postgresql$v-llvmjit",
"pgsql-mini":   "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib",
"pgsql-core":   "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl postgresql$v-llvmjit",
"pgsql-full":   "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl postgresql$v-llvmjit postgresql$v-test postgresql$v-devel",
"pgsql-main":   "postgresql$v postgresql$v-server postgresql$v-libs postgresql$v-contrib postgresql$v-plperl postgresql$v-plpython3 postgresql$v-pltcl postgresql$v-llvmjit pg_repack_$v* wal2json_$v* pgvector_$v*",
"pgsql-client": "postgresql$v",
"pgsql-server": "postgresql$v-server postgresql$v-libs postgresql$v-contrib",
"pgsql-devel":  "postgresql$v-devel",
"pgsql-basic":  "pg_repack_$v* wal2json_$v* pgvector_$v*",
```
```yaml tab="Debian"
"pgsql":        "postgresql-$v postgresql-client-$v postgresql-plpython3-$v postgresql-plperl-$v postgresql-pltcl-$v",
"pgsql-mini":   "postgresql-$v postgresql-client-$v",
"pgsql-core":   "postgresql-$v postgresql-client-$v postgresql-plpython3-$v postgresql-plperl-$v postgresql-pltcl-$v",
"pgsql-full":   "postgresql-$v postgresql-client-$v postgresql-plpython3-$v postgresql-plperl-$v postgresql-pltcl-$v postgresql-server-dev-$v",
"pgsql-main":   "postgresql-$v postgresql-client-$v postgresql-plpython3-$v postgresql-plperl-$v postgresql-pltcl-$v postgresql-$v-repack postgresql-$v-wal2json postgresql-$v-pgvector",
"pgsql-client": "postgresql-client-$v",
"pgsql-server": "postgresql-$v",
"pgsql-devel":  "postgresql-server-dev-$v",
"pgsql-basic":  "postgresql-$v-repack postgresql-$v-wal2json postgresql-$v-pgvector",
```

--------

### Install Extensions





### **Install for another PG**

`pig` will use the default postgres installation in your active `PATH`, but you can install extension for a specific installation with `-v` (when using the PGDG convention), or passing any `pg_config` path for custom installation.

```bash
pig ext install pg_duckdb -v 16     # install the extension for pg16
pig ext install pg_duckdb -p /usr/lib/postgresql/17/bin/pg_config    # specify a pg17 pg_config
```


------

### Install a specific Version

You can also install PostgreSQL kernel packages with:

```bash
pig ext install pgvector=0.7.0 # install pgvector 0.7.0
pig ext install pg16=16.5      # install PostgreSQL 16 with a specific minor version
```

> Beware the **APT** repo may only have the latest minor version for its software (and require the full version string)


------

### Install PG Forks

Pig also supports installs other [Postgres kernel forks](https://pgsty.com/docs/feat/kernel)




------

### Search Extension

You can perform fuzzy search on extension name, description, and category.

```bash
$ pig ext ls olap

INFO[14:48:13] found 13 extensions matching 'olap':
Name            State  Version  Cate  Flags   License       Repo     PGVer  Package               Description
----            -----  -------  ----  ------  -------       ------   -----  ------------          ---------------------
citus           avail  13.0.1   OLAP  -dsl--  AGPL-3.0      PIGSTY   14-17  citus_17*             Distributed PostgreSQL as an extension
citus_columnar  avail  11.3-1   OLAP  -ds---  AGPL-3.0      PIGSTY   14-17  citus_17*             Citus columnar storage engine
columnar        n/a    11.1-11  OLAP  -ds---  AGPL-3.0      PIGSTY   13-16  hydra_17*             Hydra Columnar extension
pg_analytics    avail  0.3.4    OLAP  -ds-t-  PostgreSQL    PIGSTY   14-17  pg_analytics_17       Postgres for analytics, powered by DuckDB
pg_duckdb       avail  0.2.0    OLAP  -dsl--  MIT           PIGSTY   14-17  pg_duckdb_17*         DuckDB Embedded in Postgres
pg_mooncake     avail  0.1.2    OLAP  ------  MIT           PIGSTY   14-17  pg_mooncake_17*       Columnstore Table in Postgres
duckdb_fdw      avail  1.0.0    OLAP  -ds--r  MIT           PIGSTY   13-17  duckdb_fdw_17*        DuckDB Foreign Data Wrapper
pg_parquet      avail  0.2.0    OLAP  -dslt-  PostgreSQL    PIGSTY   14-17  pg_parquet_17         copy data between Postgres and Parquet
pg_fkpart       avail  1.7      OLAP  -d----  GPL-2.0       PIGSTY   13-17  pg_fkpart_17          Table partitioning by foreign key utility
pg_partman      avail  5.2.4    OLAP  -ds---  PostgreSQL    PGDG     13-17  pg_partman_17*        Extension to manage partitioned tables by time or ID
plproxy         avail  2.11.0   OLAP  -ds---  BSD 0-Clause  PIGSTY   13-17  plproxy_17*           Database partitioning implemented as procedural language
pg_strom        avail  5.2.2    OLAP  -ds--x  PostgreSQL    PGDG     13-17  pg_strom_17*          PG-Strom - big-data processing acceleration using GPU and NVME
tablefunc       added  1.0      OLAP  -ds-tx  PostgreSQL    CONTRIB  13-17  postgresql17-contrib  functions that manipulate whole tables, including crosstab

(13 Rows) (State: added|avail|n/a,Flags: b = HasBin, d = HasDDL, s = HasSolib, l = NeedLoad, t = Trusted, r = Relocatable, x = Unknown)
```

You can use the `-v 16` or `-p /path/to/pg_config` to find extension availability for other PostgreSQL installation.

### **Print Extension Summary**

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
│ Version   : 0.3.1                                                          │
│ License   : MIT                                                            │
│ Website   : https://github.com/duckdb/pg_duckdb                            │
│ Details   : /ext/pg_duckdb                                                 │
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
│ Version        │  0.3.1                                                    │
│ Availability   │  17, 16, 15, 14                                           │
├────────────────────────────────────────────────────────────────────────────┤
│ DEB Package                                                                │
├────────────────────────────────────────────────────────────────────────────┤
│ Repository     │  PIGSTY                                                   │
│ Package        │  postgresql-$v-pg-duckdb                                  │
│ Version        │  0.3.1                                                    │
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

### **List Repo**

You can list all available repo / module (repo collection) with `pig repo list`:

```bash
$ pig repo list

os_environment: {code: el8, arch: amd64, type: rpm, major: 8}
repo_upstream:  # Available Repo: 32
  - { name: pigsty-local   ,description: 'Pigsty Local'       ,module: local    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'file:///www/pigsty' }
  - { name: pigsty-infra   ,description: 'Pigsty INFRA'       ,module: infra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://repo.pigsty.io/yum/infra/$basearch' }
  - { name: pigsty-pgsql   ,description: 'Pigsty PGSQL'       ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://repo.pigsty.io/yum/pgsql/el$releasever.$basearch' }
  - { name: nginx          ,description: 'Nginx Repo'         ,module: infra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://nginx.org/packages/rhel/$releasever/$basearch/' }
  - { name: baseos         ,description: 'EL 8+ BaseOS'       ,module: node     ,releases: [8,9]            ,arch: [x86_64, aarch64]  ,baseurl: 'https://dl.rockylinux.org/pub/rocky/$releasever/BaseOS/$basearch/os/' }
  - { name: appstream      ,description: 'EL 8+ AppStream'    ,module: node     ,releases: [8,9]            ,arch: [x86_64, aarch64]  ,baseurl: 'https://dl.rockylinux.org/pub/rocky/$releasever/AppStream/$basearch/os/' }
  - { name: extras         ,description: 'EL 8+ Extras'       ,module: node     ,releases: [8,9]            ,arch: [x86_64, aarch64]  ,baseurl: 'https://dl.rockylinux.org/pub/rocky/$releasever/extras/$basearch/os/' }
  - { name: powertools     ,description: 'EL 8 PowerTools'    ,module: node     ,releases: [8]              ,arch: [x86_64, aarch64]  ,baseurl: 'https://dl.rockylinux.org/pub/rocky/$releasever/PowerTools/$basearch/os/' }
  - { name: epel           ,description: 'EL 8+ EPEL'         ,module: node     ,releases: [8,9]            ,arch: [x86_64, aarch64]  ,baseurl: 'http://download.fedoraproject.org/pub/epel/$releasever/Everything/$basearch/' }
  - { name: pgdg-common    ,description: 'PostgreSQL Common'  ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/common/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg-el8fix    ,description: 'PostgreSQL EL8FIX'  ,module: pgsql    ,releases: [8]              ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/common/pgdg-centos8-sysupdates/redhat/rhel-8-x86_64/' }
  - { name: pgdg13         ,description: 'PostgreSQL 13'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/13/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg14         ,description: 'PostgreSQL 14'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/14/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg15         ,description: 'PostgreSQL 15'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/15/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg16         ,description: 'PostgreSQL 16'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/16/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg17         ,description: 'PostgreSQL 17'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/17/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg-extras    ,description: 'PostgreSQL Extra'   ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/common/pgdg-rhel$releasever-extras/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg13-nonfree ,description: 'PostgreSQL 13+'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64]           ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/13/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg14-nonfree ,description: 'PostgreSQL 14+'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64]           ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/14/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg15-nonfree ,description: 'PostgreSQL 15+'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64]           ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/15/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg16-nonfree ,description: 'PostgreSQL 16+'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64]           ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/16/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg17-nonfree ,description: 'PostgreSQL 17+'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64]           ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/17/redhat/rhel-$releasever-$basearch' }
  - { name: timescaledb    ,description: 'TimescaleDB'        ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://packagecloud.io/timescale/timescaledb/el/$releasever/$basearch' }
  - { name: wiltondb       ,description: 'WiltonDB'           ,module: mssql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.copr.fedorainfracloud.org/results/wiltondb/wiltondb/epel-$releasever-$basearch/' }
  - { name: ivorysql       ,description: 'IvorySQL'           ,module: ivory    ,releases: [7,8,9]          ,arch: [x86_64]           ,baseurl: 'https://repo.pigsty.io/yum/ivory/el$releasever.$basearch' }
  - { name: groonga        ,description: 'Groonga'            ,module: groonga  ,releases: [8,9]            ,arch: [x86_64, aarch64]  ,baseurl: 'https://packages.groonga.org/almalinux/$releasever/$basearch/' }
  - { name: mysql          ,description: 'MySQL'              ,module: mysql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://repo.mysql.com/yum/mysql-8.0-community/el/$releasever/$basearch/' }
  - { name: mongo          ,description: 'MongoDB'            ,module: mongo    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://repo.mongodb.org/yum/redhat/$releasever/mongodb-org/8.0/$basearch/' }
  - { name: redis          ,description: 'Redis'              ,module: redis    ,releases: [8,9]            ,arch: [x86_64, aarch64]  ,baseurl: 'https://rpmfind.net/linux/remi/enterprise/$releasever/redis72/$basearch/' }
  - { name: grafana        ,description: 'Grafana'            ,module: grafana  ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://rpm.grafana.com' }
  - { name: docker-ce      ,description: 'Docker CE'          ,module: docker   ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.docker.com/linux/centos/$releasever/$basearch/stable' }
  - { name: kubernetes     ,description: 'Kubernetes'         ,module: kube     ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://pkgs.k8s.io/core:/stable:/v1.31/rpm/' }
repo_modules:   # Available Modules: 19
  - all       : pigsty-infra, pigsty-pgsql, pgdg-common, pgdg-el8fix, pgdg-el9fix, pgdg17, pgdg16, pgdg15, pgdg14, pgdg13, baseos, appstream, extras, powertools, crb, epel, base, updates, security, backports
  - pigsty    : pigsty-infra, pigsty-pgsql
  - pgdg      : pgdg-common, pgdg-el8fix, pgdg-el9fix, pgdg17, pgdg16, pgdg15, pgdg14, pgdg13
  - node      : baseos, appstream, extras, powertools, crb, epel, base, updates, security, backports
  - infra     : pigsty-infra, nginx
  - pgsql     : pigsty-pgsql, pgdg-common, pgdg-el8fix, pgdg-el9fix, pgdg13, pgdg14, pgdg15, pgdg16, pgdg17, pgdg
  - extra     : pgdg-extras, pgdg13-nonfree, pgdg14-nonfree, pgdg15-nonfree, pgdg16-nonfree, pgdg17-nonfree, timescaledb, citus
  - mssql     : wiltondb
  - mysql     : mysql
  - docker    : docker-ce
  - kube      : kubernetes
  - grafana   : grafana
  - pgml      : pgml
  - groonga   : groonga
  - haproxy   : haproxyd, haproxyu
  - ivory     : ivorysql
  - local     : pigsty-local
  - mongo     : mongo
  - redis     : redis
```


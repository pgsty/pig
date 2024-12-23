# pig - CLI tools for PostgreSQL & Pigsty

> Manage PostgreSQL extensions and clusters with ease

[![Postgres: 17](https://img.shields.io/badge/PostgreSQL-17-%233E668F?style=flat&logo=postgresql&labelColor=3E668F&logoColor=white)](https://pigsty.io/docs/pgsql)
[![Postgres: 16](https://img.shields.io/badge/PostgreSQL-16-%233E668F?style=flat&logo=postgresql&labelColor=3E668F&logoColor=white)](https://pigsty.io/docs/pgsql)
[![Postgres: 15](https://img.shields.io/badge/PostgreSQL-15-%233E668F?style=flat&logo=postgresql&labelColor=3E668F&logoColor=white)](https://pigsty.io/docs/pgsql)
[![Postgres: 14](https://img.shields.io/badge/PostgreSQL-14-%233E668F?style=flat&logo=postgresql&labelColor=3E668F&logoColor=white)](https://pigsty.io/docs/pgsql)
[![Postgres: 13](https://img.shields.io/badge/PostgreSQL-13-%233E668F?style=flat&logo=postgresql&labelColor=3E668F&logoColor=white)](https://pigsty.io/docs/pgsql)


--------

## Why Pig?

The `pig` is a CLI tool for managing PostgreSQL extensions. 

It can install PostgreSQL (13-17) along with [340 PostgreSQL extensions](https://ext.pigsty.io/#/list) on 10 Linux mainstream distributions with **native** OS package manager.  

[![Linux](https://img.shields.io/badge/Linux-AMD64-%23FCC624?style=flat&logo=linux&labelColor=FCC624&logoColor=black)](https://pigsty.io/docs/node)
[![Linux](https://img.shields.io/badge/Linux-ARM64-%23FCC624?style=flat&logo=linux&labelColor=FCC624&logoColor=black)](https://pigsty.io/docs/node)
[![EL Support: 7/8/9](https://img.shields.io/badge/EL-7/8/9-red?style=flat&logo=redhat&logoColor=red)](https://ext.pigsty.io/#/rpm)
[![Debian Support: 11/12](https://img.shields.io/badge/Debian-11/12-%23A81D33?style=flat&logo=debian&logoColor=%23A81D33)](https://pigsty.io/docs/reference/compatibility/)
[![Ubuntu Support: 20/22/24](https://img.shields.io/badge/Ubuntu-20/22/24-%23E95420?style=flat&logo=ubuntu&logoColor=%23E95420)](https://ext.pigsty.io/#/deb)

**Extension Management**

```bash
pig ext list                 # list & search extension      
pig ext info    [ext...]     # get information of a specific extension
pig ext install [ext...]     # install extension for current pg version
pig ext remove  [ext...]     # remove extension for current pg version
pig ext update  [ext...]     # update extension to the latest version
pig ext status               # show installed extension and pg status
```

**Repo Management**

```bash
pig repo add                 # add all necessary repo (pgdg + pigsty + node)
pig repo rm                  # remove yum/atp repo (move existing repo to backup dir)  
pig repo list                # list current system repo dir and active repos  
pig repo update              # update yum/apt repo cache (apt update or dnf makecache)
```



--------

## Get Started

[Install](#installation) the util via Pigsty Repo or just download and copy the binary to your system path.

```bash
$ pig repo add pigsty pgdg -u  # add pgdg & pigsty repo, update cache      
$ pig ext install pg17         # install PostgreSQL 17 kernels with PGDG native packages
$ pig ext install pg_duckdb    # install the pg_duckdb extension (for current pg17)
$ pig ext status               # show installed extension and pg status

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


--------

## Installation

You can just download the binary / rpm / deb and install it manually, or use the following commands to add the repo and install it via package manager (recommended).

### APT Repo

[![Linux AMD](https://img.shields.io/badge/Linux-AMD64-%23FCC624?style=flat&logo=linux&labelColor=FCC624&logoColor=black)](https://pigsty.io/docs/node)
[![Linux ARM](https://img.shields.io/badge/Linux-ARM64-%23FCC624?style=flat&logo=linux&labelColor=FCC624&logoColor=black)](https://pigsty.io/docs/node)
[![Ubuntu Support: 24](https://img.shields.io/badge/Ubuntu-24/noble-%23E95420?style=flat&logo=ubuntu&logoColor=%23E95420)](https://pigsty.io/docs/pgext/list/deb/)
[![Ubuntu Support: 22](https://img.shields.io/badge/Ubuntu-22/jammy-%23E95420?style=flat&logo=ubuntu&logoColor=%23E95420)](https://pigsty.io/docs/pgext/list/deb/)
[![Debian Support: 12](https://img.shields.io/badge/Debian-12/bookworm-%23A81D33?style=flat&logo=debian&logoColor=%23A81D33)](https://pigsty.io/docs/reference/compatibility/)

For Ubuntu 22.04 / 24.04 & Debian 12 or any compatible platforms, use the following commands to add the APT repo:

```bash
sudo tee /etc/apt/sources.list.d/pigsty.list > /dev/null <<EOF
deb [trusted=yes] https://repo.pigsty.io/apt/infra generic main 
EOF
sudo apt update; sudo apt install -y pig
```

### YUM Repo

[![Linux x86_64](https://img.shields.io/badge/Linux-x86_64-%23FCC624?style=flat&logo=linux&labelColor=FCC624&logoColor=black)](https://pigsty.io/docs/node)
[![Linux AARCH64](https://img.shields.io/badge/Linux-Aarch64-%23FCC624?style=flat&logo=linux&labelColor=FCC624&logoColor=black)](https://pigsty.io/docs/node)
[![RHEL Support: 8/9](https://img.shields.io/badge/EL-7/8/9-red?style=flat&logo=redhat&logoColor=red)](https://pigsty.io/docs/pgext/list/rpm/)
[![RHEL](https://img.shields.io/badge/RHEL-slategray?style=flat&logo=redhat&logoColor=red)](https://pigsty.io/docs/pgext/list/rpm/)
[![CentOS](https://img.shields.io/badge/CentOS-slategray?style=flat&logo=centos&logoColor=%23262577)](https://almalinux.org/)
[![RockyLinux](https://img.shields.io/badge/RockyLinux-slategray?style=flat&logo=rockylinux&logoColor=%2310B981)](https://almalinux.org/)
[![AlmaLinux](https://img.shields.io/badge/AlmaLinux-slategray?style=flat&logo=almalinux&logoColor=black)](https://almalinux.org/)
[![OracleLinux](https://img.shields.io/badge/OracleLinux-slategray?style=flat&logo=oracle&logoColor=%23F80000)](https://almalinux.org/)

For EL 8/9 and compatible platforms, use the following commands to add the YUM repo:

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




--------

## Usage

To list all available extensions:

```bash
$ pig ext ls time

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


$ pig ext ls duckdb

INFO[15:11:08] found 5 extensions matching 'duckdb':
Name         State  Version  Cate  Flags   License     Repo    PGVer     Package                   Description
----         -----  -------  ----  ------  -------     ------  --------  ------------              ---------------------
pg_duckdb    added  0.2.0    OLAP  -dsl--  MIT         PIGSTY  14-17     pg_duckdb_17*             DuckDB Embedded in Postgres
duckdb_fdw   avail  1.1.2    OLAP  -ds--r  MIT         PIGSTY  13-17     duckdb_fdw_17*            DuckDB Foreign Data Wrapper
odbc_fdw     n/a    0.5.1    FDW   -ds--r  PostgreSQL  PGDG    13-16     odbc_fdw_17*              Foreign data wrapper for accessing remote databases using ODBC
jdbc_fdw     n/a    1.2      FDW   -ds--r  PostgreSQL  PGDG    13-16     jdbc_fdw_17*              foreign-data wrapper for remote servers available over JDBC
decoderbufs  avail  0.1.0    ETL   --s--x  MIT         PGDG    13-17     postgres-decoderbufs_17*  Logical decoding plugin that delivers WAL stream changes using a


(340 Rows) (State: added|avail|n/a,Flags: b = HasBin, d = HasDDL, s = HasSolib, l = NeedLoad, t = Trusted, r = Relocatable, x = Unknown)


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



--------

## About

[![Webite: https://pigsty.io](https://img.shields.io/badge/Website-https%3A%2F%2Fpigsty.io-slategray?style=flat)](https://pigsty.io)
[![Github: Discussions](https://img.shields.io/badge/GitHub-Discussions-slategray?style=flat&logo=github&logoColor=black)](https://github.com/Vonng/pigsty/discussions)
[![Telegram: gV9zfZraNPM3YjFh](https://img.shields.io/badge/Telegram-gV9zfZraNPM3YjFh-cornflowerblue?style=flat&logo=telegram&logoColor=cornflowerblue)](https://t.me/joinchat/gV9zfZraNPM3YjFh)
[![Discord: j5pG8qfKxU](https://img.shields.io/badge/Discord-j5pG8qfKxU-mediumpurple?style=flat&logo=discord&logoColor=mediumpurple)](https://discord.gg/j5pG8qfKxU)
[![Wechat: pigsty-cc](https://img.shields.io/badge/WeChat-pigsty--cc-green?style=flat&logo=wechat&logoColor=green)](https://pigsty.io/img/pigsty/pigsty-cc.jpg)

[![Author: RuohangFeng](https://img.shields.io/badge/Author-Ruohang_Feng-steelblue?style=flat)](https://vonng.com/)
[![About: @Vonng](https://img.shields.io/badge/%40Vonng-steelblue?style=flat)](https://vonng.com/en/)
[![Mail: rh@vonng.com](https://img.shields.io/badge/rh%40vonng.com-steelblue?style=flat)](mailto:rh@vonng.com)
[![Copyright: 2018-2024 rh@Vonng.com](https://img.shields.io/badge/Copyright-2018--2024_(rh%40vonng.com)-red?logo=c&color=steelblue)](https://github.com/Vonng)
[![License: Apache](https://img.shields.io/badge/License-Apaehc--2.0-steelblue?style=flat&logo=opensourceinitiative&logoColor=green)](https://pigsty.io/docs/about/license/)
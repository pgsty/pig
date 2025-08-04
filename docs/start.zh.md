---
title: 上手
description: 快速上手 pig，PostgreSQL 包管理器
icon: Play
weight: 200
breadcrumbs: false
---


[**安装**](/zh/pig/install/) `pig` 包，可通过安装脚本（或[其他方式](/zh/pig/install/)）完成：

```bash tab="默认"
curl -fsSL https://repo.pigsty.io/pig | bash
```
```bash tab="镜像"
curl -fsSL https://repo.pigsty.cc/pig | bash
```

安装完成即可使用。假设你想安装 [**`pg_duckdb`**](/zh/e/pg_duckdb/) 扩展：

```bash
$ pig repo add pigsty pgdg -u  # 添加 pgdg & pigsty 源，并更新仓库缓存
$ pig ext install pg17         # 安装 PostgreSQL 17 内核及原生 PGDG 包
$ pig ext install pg_duckdb    # 为当前 pg17 安装 pg_duckdb 扩展
```

详细命令请参阅 [**repo**](/zh/pig/repo/) 与 [**ext**](/zh/pig/ext/) 子命令。




------

## 示例

现在让我们通过实际示例快速上手。


### 覆盖软件源

默认的 `pig repo add pigsty pgdg` 会向系统添加 [`PGDG`](https://www.postgresql.org/download/linux/) 与 [`PIGSTY`](/zh/repo/) 源。
而下述命令会备份并清空现有源，添加所有所需源：

```bash
pig repo add all --ru        # 此命令会覆盖所有现有源，添加 node、pgdg、pigsty 源
```

<Callout title="我的现有源文件在哪里？" type="warn">

[`repo add`](/zh/pig/repo#repo-add) 有一个更激进的版本：`repo set`，默认会覆盖（-r）现有源。
你可以在 `/etc/apt/backup` 或 `/etc/yum.repos.d/backup` 找回原有源文件。

</Callout>


--------

### 安装 PG 内核

你也可以通过如下命令安装 PostgreSQL 内核包：

```bash
pig ext install pg17          # 安装 PostgreSQL 17 内核（除 devel 包）
pig ext install pg16-simple   # 安装 PostgreSQL 16 精简内核包
pig ext install pg15 -y       # 自动确认安装 PostgreSQL 15 内核
pig ext install pg14=14.3     # 安装指定小版本的 PostgreSQL 14 内核
pig ext install pg13=13.10    # 安装 PostgreSQL 13 内核
```

如需将特定版本的 PostgreSQL 内核二进制加入 `PATH`，可用 `pig ext link` 命令：

```bash
pig ext link pg17             # 创建 /usr/pgsql 软链接，并写入 /etc/profile.d/pgsql.sh
. /etc/profile.d/pgsql.sh     # 立即生效
```


--------

### 使用别名

你也可以使用包别名，系统会自动转换为对应操作系统的包名，
`$v` 会被替换为当前或指定的 PostgreSQL 版本号，如 `17`、`16` 等…（[**全部别名**](https://github.com/pgsty/pig/blob/main/cli/ext/catalog.go#L149)）：

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

### 安装扩展





### **为其他 PG 安装扩展**

`pig` 默认会使用当前 `PATH` 下的 PostgreSQL 安装，但你也可以通过 `-v`（PGDG 规范）或指定 `pg_config` 路径为特定安装安装扩展。

```bash
pig ext install pg_duckdb -v 16     # 为 pg16 安装扩展
pig ext install pg_duckdb -p /usr/lib/postgresql/17/bin/pg_config    # 指定 pg17 的 pg_config 路径
```


------

### 安装指定版本

你也可以通过如下命令安装指定版本的 PostgreSQL 内核或扩展：

```bash
pig ext install pgvector=0.7.0 # 安装 pgvector 0.7.0 版本
pig ext install pg16=16.5      # 安装 PostgreSQL 16 的指定小版本
```

> 注意：**APT** 源通常只提供最新小版本（需完整版本号）。


------

### 安装 PG 分支内核

Pig 也支持安装其他 [Postgres 内核分支](https://pgsty.com/zh/docs/feat/kernel)




------

### 扩展搜索

你可以对扩展名称、描述、分类进行模糊搜索。

```bash
$ pig ext ls olap

INFO[14:48:13] 找到 13 个与 'olap' 匹配的扩展：
名称            状态   版本     分类   标志    许可证         源       PG版本  包名                    描述
----            -----  -------  ----  ------  -------       ------   -----  ------------          ---------------------
citus           avail  13.0.1   OLAP  -dsl--  AGPL-3.0      PIGSTY   14-17  citus_17*             分布式 PostgreSQL 扩展
citus_columnar  avail  11.3-1   OLAP  -ds---  AGPL-3.0      PIGSTY   14-17  citus_17*             Citus 列存储引擎
columnar        n/a    11.1-11  OLAP  -ds---  AGPL-3.0      PIGSTY   13-16  hydra_17*             Hydra 列存扩展
pg_analytics    avail  0.3.4    OLAP  -ds-t-  PostgreSQL    PIGSTY   14-17  pg_analytics_17       基于 DuckDB 的分析型 Postgres
pg_duckdb       avail  0.2.0    OLAP  -dsl--  MIT           PIGSTY   14-17  pg_duckdb_17*         DuckDB 内嵌于 Postgres
pg_mooncake     avail  0.1.2    OLAP  ------  MIT           PIGSTY   14-17  pg_mooncake_17*       Postgres 列存表
duckdb_fdw      avail  1.0.0    OLAP  -ds--r  MIT           PIGSTY   13-17  duckdb_fdw_17*        DuckDB FDW
pg_parquet      avail  0.2.0    OLAP  -dslt-  PostgreSQL    PIGSTY   14-17  pg_parquet_17         Postgres 与 Parquet 数据互导
pg_fkpart       avail  1.7      OLAP  -d----  GPL-2.0       PIGSTY   13-17  pg_fkpart_17          外键分区工具
pg_partman      avail  5.2.4    OLAP  -ds---  PostgreSQL    PGDG     13-17  pg_partman_17*        按时间或 ID 管理分区表
plproxy         avail  2.11.0   OLAP  -ds---  BSD 0-Clause  PIGSTY   13-17  plproxy_17*           数据库分区过程语言
pg_strom        avail  5.2.2    OLAP  -ds--x  PostgreSQL    PGDG     13-17  pg_strom_17*          GPU/NVME 加速大数据处理
tablefunc       added  1.0      OLAP  -ds-tx  PostgreSQL    CONTRIB  13-17  postgresql17-contrib  表操作函数（如交叉表）

(共 13 行)（状态: added|avail|n/a，标志: b = 有二进制, d = 有 DDL, s = 有共享库, l = 需加载, t = 可信, r = 可迁移, x = 未知）
```

可通过 `-v 16` 或 `-p /path/to/pg_config` 查询其他 PostgreSQL 版本的扩展可用性。

### **打印扩展摘要**

可用 `pig ext info` 子命令获取扩展元数据：

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

### **列出软件源**

可用 `pig repo list` 列出所有可用软件源/模块（仓库集合）：

```bash
$ pig repo list

os_environment: {code: el8, arch: amd64, type: rpm, major: 8}
repo_upstream:  # 可用源: 32
  - { name: pigsty-local   ,description: 'Pigsty 本地'       ,module: local    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'file:///www/pigsty' }
  - { name: pigsty-infra   ,description: 'Pigsty 基础设施'    ,module: infra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://repo.pigsty.io/yum/infra/$basearch' }
  - { name: pigsty-pgsql   ,description: 'Pigsty PGSQL'       ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://repo.pigsty.io/yum/pgsql/el$releasever.$basearch' }
  - { name: nginx          ,description: 'Nginx 源'           ,module: infra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://nginx.org/packages/rhel/$releasever/$basearch/' }
  - { name: baseos         ,description: 'EL 8+ BaseOS'       ,module: node     ,releases: [8,9]            ,arch: [x86_64, aarch64]  ,baseurl: 'https://dl.rockylinux.org/pub/rocky/$releasever/BaseOS/$basearch/os/' }
  - { name: appstream      ,description: 'EL 8+ AppStream'    ,module: node     ,releases: [8,9]            ,arch: [x86_64, aarch64]  ,baseurl: 'https://dl.rockylinux.org/pub/rocky/$releasever/AppStream/$basearch/os/' }
  - { name: extras         ,description: 'EL 8+ Extras'       ,module: node     ,releases: [8,9]            ,arch: [x86_64, aarch64]  ,baseurl: 'https://dl.rockylinux.org/pub/rocky/$releasever/extras/$basearch/os/' }
  - { name: powertools     ,description: 'EL 8 PowerTools'    ,module: node     ,releases: [8]              ,arch: [x86_64, aarch64]  ,baseurl: 'https://dl.rockylinux.org/pub/rocky/$releasever/PowerTools/$basearch/os/' }
  - { name: epel           ,description: 'EL 8+ EPEL'         ,module: node     ,releases: [8,9]            ,arch: [x86_64, aarch64]  ,baseurl: 'http://download.fedoraproject.org/pub/epel/$releasever/Everything/$basearch/' }
  - { name: pgdg-common    ,description: 'PostgreSQL 通用'    ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/common/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg-el8fix    ,description: 'PostgreSQL EL8FIX'  ,module: pgsql    ,releases: [8]              ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/common/pgdg-centos8-sysupdates/redhat/rhel-8-x86_64/' }
  - { name: pgdg13         ,description: 'PostgreSQL 13'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/13/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg14         ,description: 'PostgreSQL 14'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/14/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg15         ,description: 'PostgreSQL 15'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/15/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg16         ,description: 'PostgreSQL 16'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/16/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg17         ,description: 'PostgreSQL 17'      ,module: pgsql    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/17/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg-extras    ,description: 'PostgreSQL 扩展'    ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64, aarch64]  ,baseurl: 'https://download.postgresql.org/pub/repos/yum/common/pgdg-rhel$releasever-extras/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg13-nonfree ,description: 'PostgreSQL 13+ 非开源'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64]           ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/13/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg14-nonfree ,description: 'PostgreSQL 14+ 非开源'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64]           ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/14/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg15-nonfree ,description: 'PostgreSQL 15+ 非开源'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64]           ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/15/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg16-nonfree ,description: 'PostgreSQL 16+ 非开源'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64]           ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/16/redhat/rhel-$releasever-$basearch' }
  - { name: pgdg17-nonfree ,description: 'PostgreSQL 17+ 非开源'     ,module: extra    ,releases: [7,8,9]          ,arch: [x86_64]           ,baseurl: 'https://download.postgresql.org/pub/repos/yum/non-free/17/redhat/rhel-$releasever-$basearch' }
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
repo_modules:   # 可用模块: 19
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


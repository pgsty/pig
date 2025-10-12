---
title: "CMD: ext"
description: 如何使用 pig ext 子命令管理扩展
icon: SquareTerminal
weight: 620
---

`pig ext` 命令是一个用于管理 PostgreSQL 扩展的全能工具。
它允许用户搜索、安装、移除、更新和管理 PostgreSQL 扩展，甚至支持内核包的管理。

------

## 总览

```bash
pig ext - 管理 PostgreSQL 扩展

  pig repo add -ru             # 添加所有仓库并更新缓存（简单粗暴但有效）
  pig ext add pg17             # 安装可选的 postgresql 17 内核包
  pig ext list duck            # 在目录中搜索扩展
  pig ext scan -v 17           # 扫描 PG 17 已安装扩展
  pig ext add pg_duckdb        # 安装指定的 postgresql 扩展

用法：
  pig ext [命令]

别名：
  ext, e, ex, pgext, extension

示例：

  pig ext list    [query]      # 列出 & 搜索扩展
  pig ext info    [ext...]     # 获取指定扩展信息
  pig ext status  [-v]         # 显示已安装扩展及 PG 状态
  pig ext add     [ext...]     # 为当前 PG 版本安装扩展
  pig ext rm      [ext...]     # 为当前 PG 版本移除扩展
  pig ext update  [ext...]     # 更新扩展到最新版
  pig ext import  [ext...]     # 下载扩展到本地仓库
  pig ext link    [ext...]     # 将 Postgres 安装链接到 PATH
  pig ext upgrade              # 升级到最新扩展目录

可用命令：
  add         安装 postgres 扩展
  import      导入扩展包到本地仓库
  info        获取扩展信息
  link        链接 postgres 到活动 PATH
  list        列出 & 搜索可用扩展
  rm          移除 postgres 扩展
  scan        扫描当前 PG 已安装扩展
  status      显示当前 PG 已安装扩展
  update      更新当前 PG 版本已安装扩展
  upgrade     升级扩展目录到最新版

参数：
  -h, --help          获取 ext 帮助
  -p, --path string   指定 pg_config 路径定位 postgres
  -v, --version int   指定 postgres 主版本号

全局参数：
      --debug              启用调试模式
  -i, --inventory string   配置清单路径
      --log-level string   日志级别: debug, info, warn, error, fatal, panic（默认 "info"）
      --log-path string    日志文件路径，默认输出到终端

使用 "pig ext [命令] --help" 获取某命令的详细帮助。
```

------

## 示例

在安装 PostgreSQL 扩展前，你需要先添加 [**repo**](/zh/pig/repo/)：

```bash
pig repo add pgdg pigsty -u    # 温和方式添加 pgdg 和 pigsty 仓库
pig repo set -u                # 粗暴方式移除并添加所有所需仓库
```

然后你可以搜索并安装 PostgreSQL 扩展：

```bash
pig ext install pg_duckdb
pig ext install pg_partman
pig ext install pg_cron
pig ext install pg_repack
pig ext install pg_stat_statements
pig ext install pg_stat_kcache
```

可用扩展及其名称请查阅 [**扩展列表**](/zh/list/)。

1. 未指定 PostgreSQL 版本时，工具会尝试从 `PATH` 中的 `pg_config` 自动检测当前活动的 PostgreSQL 安装。
2. PostgreSQL 可通过主版本号（`-v`）或 pg_config 路径（`-p`）指定。若指定 `-v`，pig 会使用该版本 PGDG 内核包的默认路径。- EL 发行版为 `/usr/pgsql-$v/bin/pg_config`，- DEB 发行版为 `/usr/lib/postgresql/$v/bin/pg_config` 等。若指定 `-p`，则直接用该路径定位 PostgreSQL。
3. 扩展管理器会根据操作系统自动适配不同的包格式：
- RHEL/CentOS/Rocky Linux/AlmaLinux 使用 RPM 包
- Debian/Ubuntu 使用 DEB 包
4. 某些扩展可能有依赖项，安装时会自动解决。
5. 谨慎使用 `-y` 参数，它会自动确认所有提示。

Pigsty 假定你已安装官方 PGDG 内核包，如未安装，可用如下命令：

```bash
pig ext install pg17          # 安装 PostgreSQL 17 内核（除 devel 包）
```

------

## `ext list`

列出（或搜索）扩展目录中的可用扩展。

```bash
list & search available extensions

用法：
  pig ext list [query] [参数]

别名：
  list, l, ls, find

示例：

  pig ext list                # 列出所有扩展
  pig ext list postgis        # 按名称/描述搜索扩展
  pig ext ls olap             # 列出 olap 类别扩展
  pig ext ls gis -v 16        # 列出 PG 16 的 GIS 类扩展
```

默认扩展目录定义在 [**`cli/ext/assets/extension.csv`**](https://github.com/pgsty/pig/blob/main/cli/ext/assets/extension.csv)

可用 `pig ext upgrade` 命令更新到最新扩展目录，数据将下载到 `~/.pig/extension.csv`。

------

## `ext info`

显示指定扩展的详细信息。

```bash
pig ext info [ext...]
```

**示例：**

```bash
pig ext info postgis        # 显示 PostGIS 详细信息
pig ext info timescaledb    # 显示 TimescaleDB 信息
$ pig ext info postgis        # 显示 PostGIS 详细信息

╭────────────────────────────────────────────────────────────────────────────╮
│ postgis                                                                    │
├────────────────────────────────────────────────────────────────────────────┤
│ PostGIS geometry and geography spatial types and functions                 │
├────────────────────────────────────────────────────────────────────────────┤
│ 扩展名   : postgis                                                        │
│ 别名     : postgis                                                        │
│ 类别     : GIS                                                            │
│ 版本     : 3.5.2                                                          │
│ 许可证   : GPL-2.0                                                        │
│ 官网     : https://git.osgeo.org/gitea/postgis/postgis                    │
│ 详情     : https://pigsty.io/gis/postgis                                  │
├────────────────────────────────────────────────────────────────────────────┤
│ 扩展属性                                                                 │
├────────────────────────────────────────────────────────────────────────────┤
│ PostgreSQL 版本 │  可用版本: 17, 16, 15, 14, 13                         │
│ CREATE  :  是   │  CREATE EXTENSION postgis;                            │
│ DYLOAD  :  否   │  无需加载共享库                                       │
│ TRUST   :  否   │  需超级用户安装                                       │
│ Reloc   :  否   │  Schemas: []                                          │
│ Depend  :  否   │                                                       │
├────────────────────────────────────────────────────────────────────────────┤
│ 依赖关系                                                                   │
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
│ RPM 包                                                                     │
├────────────────────────────────────────────────────────────────────────────┤
│ 仓库         │  PGDG                                                     │
│ 包名         │  postgis35_$v*                                            │
│ 版本         │  3.5.2                                                    │
│ 可用版本     │  17, 16, 15, 14, 13                                       │
├────────────────────────────────────────────────────────────────────────────┤
│ DEB 包                                                                     │
├────────────────────────────────────────────────────────────────────────────┤
│ 仓库         │  PGDG                                                     │
│ 包名         │  postgresql-$v-postgis-3 postgresql-$v-postgis-3-scripts  │
│ 版本         │  3.5.2                                                    │
│ 可用版本     │  17, 16, 15, 14, 13                                       │
╰────────────────────────────────────────────────────────────────────────────╯
```

------

## `status`

显示当前 PostgreSQL 实例已安装扩展的状态。

```bash
pig ext status [-c]
```

**参数：**

- `-c, --contrib`: 结果中包含 contrib 扩展

**示例：**

```bash
pig ext status              # 显示已安装扩展
pig ext status -c           # 包含 contrib 扩展
pig ext status -v 16        # 显示 PG 16 已安装扩展
```

------

## `ext scan`

扫描当前 PostgreSQL 实例已安装的扩展。

```bash
pig ext scan [-v version]
```

该命令会扫描 postgres 扩展目录，查找所有实际已安装的扩展。

```
$ pig ext status

已安装：
* PostgreSQL 17.4 (Debian 17.4-1.pgdg120+2)  85  扩展

活动实例：
PG 版本        :  PostgreSQL 17.4 (Debian 17.4-1.pgdg120+2)
配置路径       :  /usr/lib/postgresql/17/bin/pg_config
二进制路径     :  /usr/lib/postgresql/17/bin
库文件路径     :  /usr/lib/postgresql/17/lib
扩展目录       :  /usr/share/postgresql/17/extension
扩展统计       :  18 已安装（PIGSTY 8, PGDG 10）+ 67 CONTRIB = 85 总计

名称                          版本     类别   标志    许可证         仓库    包名                                                    描述
----                          -------  ----   ------  -------       ------  ------------                                             ---------------------
timescaledb                   2.18.2   TIME   -dsl--  Timescale     PIGSTY  postgresql-17-timescaledb-tsl                            支持大规模写入和复杂时序查询
timescaledb                   2.18.2   TIME   -dsl--  Timescale     PIGSTY  postgresql-17-timescaledb-tsl                            支持大规模写入和复杂时序查询
postgis                       3.5.2    GIS    -ds---  GPL-2.0       PGDG    postgresql-17-postgis-3 postgresql-$v-postgis-3-scripts  PostGIS 几何与地理空间类型与函数
postgis_topology              3.5.2    GIS    -ds---  GPL-2.0       PGDG    postgresql-17-postgis-3 postgresql-$v-postgis-3-scripts  PostGIS 拓扑空间类型与函数
postgis_raster                3.5.2    GIS    -ds---  GPL-2.0       PGDG    postgresql-17-postgis-3 postgresql-$v-postgis-3-scripts  PostGIS 栅格类型与函数
postgis_sfcgal                3.5.2    GIS    -ds--r  GPL-2.0       PGDG    postgresql-17-postgis-3 postgresql-$v-postgis-3-scripts  PostGIS SFCGAL 函数
postgis_tiger_geocoder        3.5.2    GIS    -ds-t-  GPL-2.0       PGDG    postgresql-17-postgis-3 postgresql-$v-postgis-3-scripts  PostGIS Tiger 地理编码与逆地理编码
address_standardizer          3.5.2    GIS    -ds--r  GPL-2.0       PGDG    postgresql-17-postgis-3 postgresql-$v-postgis-3-scripts  地址标准化工具
address_standardizer_data_us  3.5.2    GIS    -ds--r  GPL-2.0       PGDG    postgresql-17-postgis-3 postgresql-$v-postgis-3-scripts  美国地址标准化数据集
vector                        0.8.0    RAG    -ds--r  PostgreSQL    PGDG    postgresql-17-pgvector                                   向量数据类型及 ivfflat/hnsw 索引
pg_search                     0.15.2   FTS    -ds-t-  AGPL-3.0      PIGSTY  postgresql-17-pg-search                                  pg_search：基于 BM25 的全文检索
pgroonga                      4.0.0    FTS    -ds-tr  PostgreSQL    PIGSTY  postgresql-17-pgroonga                                   Groonga 引擎全文检索，支持多语言
pgroonga_database             4.0.0    FTS    -ds-tr  PostgreSQL    PIGSTY  postgresql-17-pgroonga                                   PGroonga 数据库管理模块
citus                         13.0.1   OLAP   -dsl--  AGPL-3.0      PIGSTY  postgresql-17-citus                                      分布式 PostgreSQL 扩展
citus_columnar                11.3-1   OLAP   -ds---  AGPL-3.0      PIGSTY  postgresql-17-citus                                      Citus 列存储引擎
pg_mooncake                   0.1.2    OLAP   ------  MIT           PIGSTY  postgresql-17-pg-mooncake                                Postgres 列存表
plv8                          3.2.3    LANG   -ds---  PostgreSQL    PIGSTY  postgresql-17-plv8                                       PL/JavaScript (v8) 过程语言
pg_repack                     1.5.2    ADMIN  bds---  BSD 3-Clause  PGDG    postgresql-17-repack                                     在线重整表结构，最小锁表
wal2json                      2.5.3    ETL    --s--x  BSD 3-Clause  PGDG    postgresql-17-wal2json                                   以 JSON 格式变更数据捕获

（18 行）（标志：b = 有二进制，d = 有 DDL，s = 有共享库，l = 需加载，t = 可信，r = 可迁移，x = 未知）
```

------

## `ext add`

安装一个或多个 PostgreSQL 扩展。

```bash
install postgres extension

用法：
  pig ext add [参数]

别名：
  add, a, install, ins

示例：

说明：
  pig ext install pg_duckdb                  # 安装单个扩展
  pig ext install postgis timescaledb        # 安装多个扩展
  pig ext add     pgvector pgvectorscale     # 其他别名：add, ins, i, a
  pig ext ins     pg_search -y               # 自动确认安装
  pig ext install pgsql                      # 安装最新版 postgresql 内核
  pig ext a pg17                             # 安装 postgresql 17 内核包
  pig ext ins pg16                           # 安装 postgresql 16 内核包
  pig ext install pg15-core                  # 安装 postgresql 15 核心包
  pig ext install pg14-main -y               # 安装 pg 14 + 常用扩展（vector, repack, wal2json）
  pig ext install pg13-devel --yes           # 安装 pg 13 devel 包（自动确认）
  pig ext install pgsql-common               # 安装常用工具包（patroni、pgbouncer、pgbackrest...）

参数：
  -h, --help   获取 add 帮助
  -y, --yes    自动确认安装
```

------

## `ext rm`

移除一个或多个 PostgreSQL 扩展。

```bash
pig ext rm [ext...] [-y]
```

**参数：**

- `-y, --yes`: 自动确认移除

**示例：**

```bash
pig ext rm pg_duckdb                   # 移除指定扩展
pig ext rm postgis timescaledb         # 移除多个扩展
pig ext rm pgvector -y                 # 自动确认移除
```

------

## `ext update`

将已安装扩展更新到最新版。

```bash
pig ext update [ext...] [-y]
```

**参数：**

- `-y, --yes`: 自动确认更新

**示例：**

```bash
pig ext update                         # 更新所有已安装扩展
pig ext update postgis                 # 更新指定扩展
pig ext update postgis timescaledb     # 更新多个扩展
pig ext update -y                      # 自动确认更新
```

------

## `pig import`

下载扩展包到本地仓库，便于离线安装。

```bash
用法：
  pig ext import [ext...] [参数]

别名：
  import, get

示例：

  pig ext import postgis                # 导入 postgis 扩展包
  pig ext import timescaledb pg_cron    # 导入多个扩展包
  pig ext import pg16                   # 导入 postgresql 16 包
  pig ext import pgsql-common           # 导入常用工具包
  pig ext import -d /www/pigsty postgis # 指定路径导入

参数：
  -h, --help          获取 import 帮助
  -d, --repo string   指定仓库目录（默认 "/www/pigsty"）
```

**参数：**

- `-d, --repo`: 指定仓库目录（默认：/www/pigsty）

**示例：**

```bash
pig ext import postgis                 # 导入 PostGIS 包
pig ext import timescaledb pg_cron     # 导入多个扩展包
pig ext import pg16                    # 导入 PostgreSQL 16 包
pig ext import pgsql-common            # 导入常用工具包
```

------

## `ext link`

将 PostgreSQL 安装链接到系统 PATH。

```bash
link postgres to active PATH

用法：
  pig ext link <-v pgver|-p pgpath> [参数]

别名：
  link, ln

示例：

  pig ext link 16                      # 链接 pgdg postgresql 16 到 /usr/pgsql
  pig ext link /usr/pgsql-16           # 链接指定 pg 到 /usr/pgsql
  pig ext link /u01/polardb_pg         # 链接 polardb pg 到 /usr/pgsql
  pig ext link null|none|nil|nop|no    # 取消当前 postgres 链接

参数：
  -h, --help   获取 link 帮助
```

**示例：**

```bash
pig ext link 17                        # 链接 PostgreSQL 17 到 /usr/pgsql
pig ext link 16                        # 链接 PostgreSQL 16 到 /usr/pgsql
pig ext link /usr/pgsql-16             # 从指定路径链接到 /usr/pgsql
pig ext link null                      # 取消当前 PostgreSQL 链接
```

------

## `upgrade`

升级扩展目录到最新版。

```bash
pig ext upgrade
```


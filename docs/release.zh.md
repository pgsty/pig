---
title: 版本
description: pig 发布说明与变更记录
icon: ClipboardList
weight: 900
breadcrumbs: false
---

|       版本        |     日期     | 摘要                             |                           GitHub                           |
|:---------------:|:----------:|--------------------------------|:----------------------------------------------------------:|
| [v0.7.0](#v070) | 2025-11-05 | 强化 build 能力，大批量包更新             | [v0.7.0](https://github.com/pgsty/pig/releases/tag/v0.7.0) |
| [v0.6.2](#v062) | 2025-10-03 | 正式提供 PG 18 支持                  | [v0.6.2](https://github.com/pgsty/pig/releases/tag/v0.6.2) |
| [v0.6.1](#v061) | 2025-08-13 | 添加 CI/CD 管道，使用 PIGSTY PGDG 仓库  | [v0.6.1](https://github.com/pgsty/pig/releases/tag/v0.6.1) |
| [v0.6.0](#v060) | 2025-07-17 | 423 个扩展，percona pg_tde，mcp 工具箱 | [v0.6.0](https://github.com/pgsty/pig/releases/tag/v0.6.0) |
| [v0.5.0](#v050) | 2025-06-30 | 422 个扩展，新的扩展目录                 | [v0.6.0](https://github.com/pgsty/pig/releases/tag/v0.5.0) |
| [v0.4.2](#v042) | 2025-05-27 | 421 个扩展，halo 和 oriole deb      | [v0.4.2](https://github.com/pgsty/pig/releases/tag/v0.4.2) |
| [v0.4.1](#v041) | 2025-05-07 | 414 个扩展，pg18 别名支持              | [v0.4.1](https://github.com/pgsty/pig/releases/tag/v0.4.1) |
| [v0.4.0](#v040) | 2025-05-01 | do 和 pt 子命令，halo 和 orioledb    | [v0.4.0](https://github.com/pgsty/pig/releases/tag/v0.4.0) |
| [v0.3.4](#v034) | 2025-04-05 | 常规更新                           | [v0.3.4](https://github.com/pgsty/pig/releases/tag/v0.3.4) |
| [v0.3.3](#v033) | 2025-03-25 | 别名、仓库、依赖                       | [v0.3.3](https://github.com/pgsty/pig/releases/tag/v0.3.3) |
| [v0.3.2](#v032) | 2025-03-21 | 新扩展                            | [v0.3.2](https://github.com/pgsty/pig/releases/tag/v0.3.2) |
| [v0.3.1](#v031) | 2025-03-19 | 轻微错误修复                         | [v0.3.1](https://github.com/pgsty/pig/releases/tag/v0.3.1) |
| [v0.3.0](#v030) | 2025-02-24 | 新主页和扩展目录                       | [v0.3.0](https://github.com/pgsty/pig/releases/tag/v0.3.0) |
| [v0.2.2](#v022) | 2025-02-22 | 404 个扩展                        | [v0.2.2](https://github.com/pgsty/pig/releases/tag/v0.2.2) |
| [v0.2.0](#v020) | 2025-02-14 | 400 个扩展                        | [v0.2.0](https://github.com/pgsty/pig/releases/tag/v0.2.0) |
| [v0.1.4](#v014) | 2025-02-12 | 常规错误修复                         | [v0.1.4](https://github.com/pgsty/pig/releases/tag/v0.1.4) |
| [v0.1.3](#v013) | 2025-01-23 | 390 个扩展                        | [v0.1.3](https://github.com/pgsty/pig/releases/tag/v0.1.3) |
| [v0.1.2](#v012) | 2025-01-12 | anon 扩展和其他 350 个扩展             | [v0.1.2](https://github.com/pgsty/pig/releases/tag/v0.1.2) |
| [v0.1.1](#v011) | 2025-01-09 | 更新扩展列表                         | [v0.1.1](https://github.com/pgsty/pig/releases/tag/v0.1.1) |
| [v0.1.0](#v010) | 2024-12-29 | repo、ext、sty 和自更新              | [v0.1.0](https://github.com/pgsty/pig/releases/tag/v0.1.0) |
| [v0.0.1](#v001) | 2024-12-23 | 创世发布                           | [v0.0.1](https://github.com/pgsty/pig/releases/tag/v0.0.1) |


## v0.7.0

- 大批量扩展更新至最新版本，带有 PostgreSQL 18 支持。
- 几乎所有 Rust 扩展现已通过 pgrx 0.16.1 支持 PG 18
- `pig build` 命令彻底重做
    - `pig build pkg <pkg>` 现在会一条龙完成扩展的下载，依赖安装，构建
    - `pig build pgrx` 命令现在从 `pig build rust` 中分离
    - `pig build pgrx [-v pgrx_version]` 现在可以直接使用现有的 PG 安装
    - `pig build dep` 现在会处理 EL 和 Debian 系统下的扩展依赖
    - `pig build ext` 命令现在有了更为紧凑和美观的输出，可在 EL 下不依赖 build 脚本直接构建 RPM
    - `pig build spec` 现在支持直接从Pigsty仓库下载 spec 文件包
    - `pig build repo` / `pig repo add` / `pig repo set` 现在默认使用 `node,pgsql,infra` 仓库模块，取代原本的 `node,pgdg,pigsty`
- 大量优化了错误日志记录。
- 基于 hugo 与 hextra 全新目录网站



## v0.6.2

- 使用 PG 18 官方正式仓库取代原本的 Testing Beta 仓库 instead of testing repo
- 在接收 Pigsty 版本字符串的时候，自动添加 `v` 前缀
- 改进了网络检查与下载的逻辑

## v0.6.1

- 新增 el10 与 debian 13 trixie 的支持存根
- 专门的新文档网站： https://pig.pgsty.com
- 使用 go 1.25 重新构建，新增 CI/CD 管道
- 在中国大陆使用 PIGSTY PGDG 镜像
- 移除空的 `pgdg-el10fix` 仓库
- 使用 Pigsty WiltonDB 镜像
- 修复 EL 10 专用的 EPEL 仓库
- pig version 输出构建环境信息

## v0.6.0

- 新扩展目录：[https://ext.pgsty.com](https://ext.pgsty.com)
- 新子命令：`pig install` 简化 `pig ext install`
- 添加新内核支持：带 pg_tde 的 percona
- 添加新包：Google GenAI MCP 数据库工具箱
- 添加新仓库：percona 仓库和 clickhouse 仓库
- 将扩展摘要信息链接更改为 https://ext.pgsty.com
- 修复 orioledb 在 Debian/Ubuntu 系统上的问题
- 修复 EL 发行版上的 epel 仓库
- 将 golang 升级到 1.24.5
- 将 pigsty 升级到 v3.6.0

**校验和**

```bash
1804766d235b9267701a08f95903bc3b  pig_0.6.0-1_amd64.deb
35f4efa35c1eaecdd12aa680d29eadcb  pig_0.6.0-1_arm64.deb
b523b54d9f2d7dcc5999bcc6bd046b1d  pig-0.6.0-1.aarch64.rpm
9434d9dca7fd9725ea574c5fae1a7f52  pig-0.6.0-1.x86_64.rpm
f635c12d9ad46a779aa7174552977d11  pig-v0.6.0.linux-amd64.tar.gz
165af4e63ec0031d303fe8b6c35c5732  pig-v0.6.0.linux-arm64.tar.gz
```


## v0.5.0

- 将扩展列表更新至 422 个
- 新扩展：来自 AWS 的 [pgactive](https://github.com/aws/pgactive)
- 将 timescaledb 升级到 2.20.3
- 将 citus 升级到 13.1.0
- 将 vchord 升级到 0.4.3
- 修复错误：pgvectorscale debian/ubuntu pg17 失败
- 将 kubernetes 仓库升级到 1.33
- 将默认 pigsty 版本升级到 3.5.0

**校验和**

```bash
9ec6f3caf3edbe867caab5de0e0ccb33  pig_0.5.0-1_amd64.deb
4fbb0a42cd8a88bce50b3c9d85745d77  pig_0.5.0-1_arm64.deb
9cf8208396b068cab438f72c90d39efe  pig-0.5.0-1.aarch64.rpm
d9a8d78c30f45e098b29c3d16471aa8d  pig-0.5.0-1.x86_64.rpm
761df804ff7b83965c41492700717674  pig-v0.5.0.linux-amd64.tar.gz
5d1830069d98030728f08835f883ea39  pig-v0.5.0.linux-arm64.tar.gz
```

发布：https://github.com/pgsty/pig/releases/tag/v0.6.0



## v0.4.2

- 将扩展列表更新至 421 个
- 为 Debian / Ubuntu 添加 openhalo/orioledb 支持
- pgdd [0.6.0](https://github.com/rustprooflabs/pgdd) (pgrx 0.14.1)
- convert [0.0.4](https://github.com/rustprooflabs/convert) (pgrx 0.14.1)
- pg_idkit [0.3.0](https://github.com/VADOSWARE/pg_idkit) (pgrx 0.14.1)
- pg_tokenizer.rs [0.1.0](https://github.com/tensorchord/pg_tokenizer.rs) (pgrx 0.13.1)
- pg_render [0.1.2](https://github.com/mkaski/pg_render) (pgrx 0.12.8)
- pgx_ulid [0.2.0](https://github.com/pksunkara/pgx_ulid) (pgrx 0.12.7)
- pg_ivm [1.11.0](https://github.com/sraoss/pg_ivm) 适用于 debian/ubuntu
- orioledb [1.4.0 beta11](https://github.com/orioledb/orioledb)
- 重新添加 el7 仓库

**校验和**

```bash
bbf83fa3e3ec9a4dca82eeed921ae90a  pig_0.4.2-1_amd64.deb
e45753335faf80a70d4f2ef1d3100d72  pig_0.4.2-1_arm64.deb
966d60bbc2025ba9cc53393011605f9f  pig-0.4.2-1.aarch64.rpm
1f31f54da144f10039fa026b7b6e75ad  pig-0.4.2-1.x86_64.rpm
1eec26c4e69b40921e209bcaa4fe257a  pig-v0.4.2.linux-amd64.tar.gz
768d43441917a3625c462ce9f2b9d4ef  pig-v0.4.2.linux-arm64.tar.gz
```

发布：https://github.com/pgsty/pig/releases/tag/v0.4.2


## v0.4.1

- 将扩展列表更新至 414 个
- 在 `pig ext scan` 映射中添加 `citus_wal2json` 和 `citus_pgoutput`
- 添加 PG 18 beta 仓库
- 添加 PG 18 包别名

**扩展包更新**

- omnigres 20250507
- citus [12.0.3](https://github.com/citusdata/citus/releases/tag/v13.0.3)
- timescaledb [2.19.3](https://github.com/timescale/timescaledb/releases/tag/2.19.3)
- supautils [2.9.1](https://github.com/supabase/supautils/releases/tag/v2.9.1)
- pg_envvar [1.0.1](https://github.com/theory/pg-envvar/releases/tag/v1.0.1)
- pgcollection [1.0.0](https://github.com/aws/pgcollection/releases/tag/v1.0.0)
- aggs_for_vecs [1.4.0](https://github.com/pjungwir/aggs_for_vecs/releases/tag/1.4.0)
- pg_tracing [0.1.3](https://github.com/DataDog/pg_tracing/releases/tag/v0.1.3)
- pgmq [1.5.1](https://github.com/pgmq/pgmq/releases/tag/v1.5.1)
- tzf-pg [0.2.0](https://github.com/ringsaturn/tzf-pg/releases/tag/v0.2.0) (pgrx 0.14.1)
- pg_search [0.15.18](https://github.com/paradedb/paradedb/releases/tag/v0.15.18) (pgrx 0.14.1)
- anon [2.1.1](https://gitlab.com/dalibo/postgresql_anonymizer/-/tree/latest/debian?ref_type=heads) (pgrx 0.14.1)
- pg_parquet [0.4.0](https://github.com/CrunchyData/pg_parquet/releases/tag/v0.3.2) (0.14.1)
- pg_cardano [1.0.5](https://github.com/Fell-x27/pg_cardano/commits/master/) (pgrx 0.12) -> 0.14.1
- pglite_fusion [0.0.5](https://github.com/frectonz/pglite-fusion/releases/tag/v0.0.5) (pgrx 0.12.8) -> 14.1
- vchord_bm25 [0.2.1](https://github.com/tensorchord/VectorChord-bm25/releases/tag/0.2.1) (pgrx 0.13.1)
- vchord [0.3.0](https://github.com/tensorchord/VectorChord/releases/tag/0.3.0) (pgrx 0.13.1)
- pg_vectorize [0.22.1](https://github.com/ChuckHend/pg_vectorize/releases/tag/v0.22.1) (pgrx 0.13.1)
- wrappers [0.4.6](https://github.com/supabase/wrappers/releases/tag/v0.4.6) (pgrx 0.12.9)
- timescaledb-toolkit [1.21.0](https://github.com/timescale/timescaledb-toolkit/releases/tag/1.21.0) (0.12.9)
- pgvectorscale [0.7.1](https://github.com/timescale/pgvectorscale/releases/tag/0.7.1) (pgrx 0.12.9)
- pg_session_jwt [0.3.1](https://github.com/neondatabase/pg_session_jwt/releases/tag/v0.3.1) (pgrx 0.12.6) -> 0.12.9

**校验和**

```
e2c1037c20f97c6f5930876ee82b6392  pig_0.4.1-1_amd64.deb
8197b6b5b95d1d1ae95e0a0e50355ecb  pig_0.4.1-1_arm64.deb
9d3a261d31c92fc73fe5bbfcd5b8e8ba  pig-0.4.1-1.aarch64.rpm
ffcec2a2ae965d14b9d3d80278fd340c  pig-0.4.1-1.x86_64.rpm
01d3128e782f35a20f0c81480cbe9025  pig-v0.4.1.linux-amd64.tar.gz
b2655628df326a1d0ed13f3dd8762c65  pig-v0.4.1.linux-arm64.tar.gz
```

发布：https://github.com/pgsty/pig/releases/tag/v0.4.1




## v0.4.0

- 更新扩展列表，可用扩展达到 **407** 个
- 添加 `pig do` 子命令用于执行 Pigsty playbook 任务
- 添加 `pig pt` 子命令用于包装 Patroni 命令行工具
- 添加扩展别名：`openhalo` 和 `orioledb`
- 添加 `gitlab-ce` / `gitlab-ee` 仓库区分
- 使用最新 Go 1.24.2 构建并升级依赖项版本
- 修复特定条件下 `pig ext status` 的 panic 问题
- 修复 `pig ext scan` 无法匹配多个扩展的问题

**扩展包更新**

- 将 pg_search 更新到 0.15.13
- 将 citus 更新到 13.0.3
- 将 timescaledb 更新到 2.19.1
- 将 pgcollection RPM 更新到 1.0.0
- 将 pg_vectorize RPM 更新到 0.22.1
- 将 pglite_fusion RPM 更新到 0.0.4
- 将 aggs_for_vecs RPM 更新到 1.4.0
- 将 pg_tracing RPM 更新到 0.1.3
- 将 pgmq RPM 更新到 1.5.1

**校验和**

```
bbc0adf94b342ac450c7999ea1c5ab76  pig_0.4.0-1_amd64.deb
7445b819624e7498b496edb12a36f426  pig_0.4.0-1_arm64.deb
835ce929afac0fb1f249f55571fbed97  pig-0.4.0-1.aarch64.rpm
25ba5a846095e17d2bfa2f15fe4e4b44  pig-0.4.0-1.x86_64.rpm
1568b163ffa23cb921ee439452ca4de9  pig-v0.4.0.linux-amd64.tar.gz
9f2ab3f5d1e29807a9642dfbe1dc9b0e  pig-v0.4.0.linux-arm64.tar.gz
```

发布：https://github.com/pgsty/pig/releases/tag/v0.4.0




## v0.3.4

```bash
curl https://repo.pigsty.io/pig | bash -s 0.3.4
```

- 常规扩展元数据更新
- 使用阿里云 epel 镜像代替损坏的清华大学 tuna 镜像
- 升级 pigsty 版本字符串
- 在仓库列表中添加 `gitlab` 仓库

**校验和**

```
5c0bba04d955bbe6a29d24d31aa17c6b  pig-0.3.4-1.aarch64.rpm
42636b9fc64d7882391d856d36d715e7  pig-0.3.4-1.x86_64.rpm
1a6296421d642000ad75a5a41bc9ab96  pig-v0.3.4.linux-amd64.tar.gz
f7ea5ba8abaa89e866811e5b2508e82f  pig-v0.3.4.linux-arm64.tar.gz
2dd63cdb5965f78a48da462a0453001d  pig_0.3.4-1_amd64.deb
094b9e028e81c46d71ee315d8a223ada  pig_0.3.4-1_arm64.deb
```

发布：https://github.com/pgsty/pig/releases/tag/v0.3.4




## v0.3.3

- 添加 `pig build dep` 命令安装扩展构建依赖项
- 更新默认仓库列表
- 为 `mssql` 模块（wiltondb/babelfish）使用 pigsty.io 镜像
- 将 docker 模块合并到 `infra`
- 从 el7 目标中移除 pg16/17
- 允许在 el7 中安装扩展
- 更新包别名
- `pgsql`、`pgsql-main`、`pgsql-core`、`pgsql-mini`、`pgsql-full`
- `ivorysql` 现在映射到 `ivorysql4`
- `timescaledb-utils`
- `pgbackrest_exporter`
- 移除 `pgsql-simple`
- 拉取 [#13 将 github.com/golang-jwt/jwt/v5 从 5.2.1 升级到 5.2.2](https://github.com/pgsty/pig/pull/13)
- 将 polardb 升级到 15.12.3.0-e1e6d85b
- `pig repo set` 现在会自动更新元缓存
- 清理嵌入的 pigsty tarball

**更改内容**

- 由 @dependabot 在 https://github.com/pgsty/pig/pull/13 中将 github.com/golang-jwt/jwt/v5 从 5.2.1 升级到 5.2.2

**新贡献者**

- @dependabot 在 https://github.com/pgsty/pig/pull/13 中首次贡献

**完整变更日志**：https://github.com/pgsty/pig/compare/v0.3.2...v0.3.3

发布：https://github.com/pgsty/pig/releases/tag/v0.3.3

**校验和**

```
4e10567077e5d8cefd94d1c7aeb9478b  pig-0.3.3-1.aarch64.rpm
cc8a423abeb0f5316b427097993b9c6e  pig-0.3.3-1.x86_64.rpm
835d4f63b4ee0b36e2322a4ffef6527a  pig-v0.3.3.linux-amd64.tar.gz
c43e082c661e75d91f1c726e60911ea3  pig-v0.3.3.linux-arm64.tar.gz
938db83c5ca065419b8185adb285ed5a  pig_0.3.3-1_amd64.deb
75af6731adc4d31aa3458d70fc7f4e42  pig_0.3.3-1_arm64.deb
```

发布：https://github.com/pgsty/pig/releases/tag/v0.3.3




## v0.3.2

**增强功能**

- 新扩展
- 使用 `upx` 减少二进制大小
- 移除嵌入的 pigsty 以减少二进制大小
- 允许指定 `-y` 强制重新安装 rust 在 `pig build rust` 中
- 允许指定 `-v` 指定要安装的 `pgrx` 版本

**新扩展**

**405** 个 PG 扩展列表：

- apache age 13 - 17 el rpm (1.5.0)
- pgspider_ext 1.3.0 (新扩展)
- timescaledb 2.18.2 -> 2.19.0
- citus 13.0.1 -> 13.0.2
- documentdb 1.101-0 -> 1.102-0
- pg_analytics: 0.3.4 -> 0.3.7
- pg_search: 0.15.2 -> 0.15.8
- pg_ivm 1.9 -> 1.10
- emaj 4.4.0 -> 4.6.0
- pgsql_tweaks 0.10.0 -> 0.11.0
- pgvectorscale 0.4.0 -> 0.6.0 (pgrx 0.12.5)
- pg_session_jwt 0.1.2 -> 0.2.0 (pgrx 0.12.6)
- wrappers 0.4.4 -> 0.4.5 (pgrx 0.12.9)
- pg_parquet 0.2.0 -> 0.3.1 (pgrx 0.13.1)
- vchord 0.2.1 -> 0.2.2 (pgrx 0.13.1)
- pg_tle 1.2.0 -> 1.5.0
- supautils 2.5.0 -> 2.6.0
- sslutils 1.3 -> 1.4
- pg_profile 4.7 -> 4.8
- pg_snakeoil 1.3 -> 1.4
- pg_jsonschema 0.3.2 -> 0.3.3
- pg_incremental: 1.1.1 -> 1.2.0
- pg_stat_monitor 2.1.0 -> 2.1.1
- 修复 ddl_historization 版本 0.7 -> 0.0.7
- 修复 pg_sqlog 3.1.7 -> 1.6
- 修复 pg_random 移除 dev 后缀
- asn1oid 1.5 -> 1.6
- table_log 0.6.1 -> 0.6.4

**校验和**

```
f773aedf4a76d031f411cb38bc623134  pig-0.3.2-1.aarch64.rpm
fa9084877deb57d4882b7d9531ea0369  pig-0.3.2-1.x86_64.rpm
7f9a03c9dd23cba094191a8044fa0263  pig-v0.3.2.linux-amd64.tar.gz
adda8986efc048565834cda1ef206a20  pig-v0.3.2.linux-arm64.tar.gz
5b27cefdc716629db8f1fbc534f58691  pig_0.3.2-1_amd64.deb
936e85bda5818da4c20b758ebd65e618  pig_0.3.2-1_arm64.deb
```

发布：https://github.com/pgsty/pig/releases/tag/v0.3.2




## v0.3.1

常规错误修复

- 修复仓库格式字符串
- 修复扩展信息链接
- 更新 pg_mooncake 元数据

**校验和**

```
9251aa18e663f1ecf239adcba3a798b9  pig-0.3.1-1.aarch64.rpm
3b91e7faa78c5f0283d27ffe632dda46  pig-0.3.1-1.x86_64.rpm
87c75dfd114252230c53ee8c5d60dac4  pig-v0.3.1.linux-amd64.tar.gz
82832ae767e226627087b97a87982daf  pig-v0.3.1.linux-arm64.tar.gz
4d99f9c03915accf413b6374b75f1bdb  pig_0.3.1-1_amd64.deb
e38e8a21ed73a37d4588053f8c900f7c  pig_0.3.1-1_arm64.deb
```

发布：https://github.com/pgsty/pig/releases/tag/v0.3.1




## v0.3.0

[`pig`](/zh/pig/) 项目现在有了新的 [主页](https://pig.pgsty.com)，以及 PostgreSQL 扩展 [目录](https://ext.pgsty.com/list)。

```bash
curl https://repo.pigsty.io/pig | bash    # cloudflare
curl https://repo.pigsty.cc/pig | bash    # 中国 cdn
```

您可以使用简单的命令安装 PostgreSQL 内核以及 [**404 个扩展**](https://ext.pgsty.com/list)。此外，
pig v0.3 也嵌入并随最新的 Pigsty [v3.3.0](https://doc.pgsty.com/release) 一起发布。

**新功能**

`pig build` 子命令具有设置扩展构建环境的 [能力](https://pig.pgsty.com/cmd/build/)

```bash
pig build repo     # 初始化构建仓库 (=repo set -ru)
pig build tool     # 初始化构建工具集
pig build rust     # 初始化 rustc 和 pgrx (0.12.9)
pig build spec     # 初始化 rpm/deb spec 仓库
pig build get      # 获取扩展源码 tarball
pig build ext      # 构建扩展
## 下载大型 tarball
pig build get std          # 下载 std 小型 tarball
pig build get all          # 下载所有源码 tarball
pig build get pg_mooncake
pig build get pg_duckdb
pig build get omnigres
pig build get plv8
pig build get citus

pig build ext citus
pig build ext timescaledb
```

以及其他工具，如构建代理：

```bash
pig build proxy                  # 安装 v2ray 代理
pig build proxy [user@host:port] # 初始化并设置代理
```

pig 0.3.0 随 Pigsty [3.3.0](https://github.com/Vonng/pigsty/releases/tag/v3.3.0) 一起发布

**新扩展**

[`ext.pgsty.com`](https://ext.pgsty.com/) 目录正在迁移到 [`https://ext.pgsty.com/list`](https://ext.pgsty.com/ext)，包含更多信息！

**校验和**

```bash
9cc3848ab13c41a0415f1fea6294ad2d  pig-0.3.0-1.aarch64.rpm
ee99a6c1ff17975ed184f009a4b1aac5  pig-0.3.0-1.x86_64.rpm
b06f6b5aeaa83a9d76c9b563b2516e1c  pig-v0.3.0.linux-amd64.tar.gz
d783732413e4f32074adeab2d5d092c3  pig-v0.3.0.linux-arm64.tar.gz
7c942b8dbd78458d5371c1abca2571c6  pig_0.3.0-1_amd64.deb
c0a411cf53cb58706ca81b49b4fc840e  pig_0.3.0-1_arm64.deb
```

发布：https://github.com/pgsty/pig/releases/tag/v0.3.0




## v0.2.2

Pig v0.2.2 中提供 [**404**](/zh/list) 个扩展

```bash
curl https://repo.pigsty.io/pig | bash -s v0.2.2
```

- **documentdb** 0.101-0
- **pgcollection** (新) 0.9.1
- **pg_bzip** (新) 1.0.0
- pg_net 0.14.0 (部分发行版)
- pg_curl 2.4.2
- vault 0.3.1 (SQL -> C)
- table_version 1.10.3 -> 1.11.0
- pg_duration 1.0.2
- timescaledb 2.18.2
- pg_analytics 0.3.4
- pg_search 0.15.2
- pg_graphql 1.5.11
- vchord 0.1.1 -> 0.2.1 ((+13))
- vchord_bm25 0.1.0 -> 0.1.1
- **pg_mooncake** 0.1.1 -> 0.1.2
- **pg_duckdb** 0.2.0 -> 0.3.1
- pgddl 0.29
- pgsql_tweaks 0.11.0

发布：https://github.com/pgsty/pig/releases/tag/v0.2.2



## v0.2.0

使用以下命令安装最新的 `pig` 版本：

```bash
curl -fsSL https://repo.pigsty.io/pig | bash
```

**新扩展**

- [pg_documentdb_core](https://github.com/microsoft/documentdb/tree/main/pg_documentdb_core) 和 ferretdb
- [VectorChord-bm25](https://github.com/tensorchord/VectorChord-bm25) (vchord_bm25) 0.1.0
- [pg_tracing](https://github.com/DataDog/pg_tracing) 0.1.2
- [pg_curl](https://github.com/RekGRpth/pg_curl) 2.4
- [pgxicor](https://github.com/Florents-Tselai/pgxicor) 0.1.0
- [pgsparql](https://github.com/lacanoid/pgsparql) 1.0
- [pgjq](https://github.com/Florents-Tselai/pgJQ) 0.1.0
- [hashtypes](https://github.com/adjust/hashtypes/) 0.1.5
- [db_migrator](https://github.com/cybertec-postgresql/db_migrator) 1.0.0
- [pg_cooldown](https://github.com/rbergm/pg_cooldown) 0.1

**更新扩展版本**

- citus 13.0.0 -> 13.0.1
- pg_mooncake 0.1.0 -> 0.1.1
- timescaledb 2.17.2 -> 2.18.1
- supautils 2.5.0 -> 2.6.0
- VectorChord 0.1.0 -> 0.2.0
- pg_bulkload 3.1.22 (+pg17)
- pg_store_plan 1.8 (+pg17)
- pg_search 0.14 -> 0.15.1
- pg_analytics 0.3.0 -> 0.3.2
- pgroonga 3.2.5 -> 4.0.0
- zhparser 2.2 -> 2.3
- pg_vectorize 0.20.0 -> 0.21.1

发布：https://github.com/pgsty/pig/releases/tag/v0.2.0




## v0.1.4

使用以下命令安装最新的 `pig` 版本：

```bash
curl -fsSL https://repo.pigsty.io/pig | bash
```

**新扩展**

- [pg_documentdb_core](https://github.com/microsoft/documentdb/tree/main/pg_documentdb_core) 和 ferretdb
- [VectorChord-bm25](https://github.com/tensorchord/VectorChord-bm25) (vchord_bm25) 0.1.0
- [pg_tracing](https://github.com/DataDog/pg_tracing) 0.1.2
- [pg_curl](https://github.com/RekGRpth/pg_curl) 2.4
- [pgxicor](https://github.com/Florents-Tselai/pgxicor) 0.1.0
- [pgsparql](https://github.com/lacanoid/pgsparql) 1.0
- [pgjq](https://github.com/Florents-Tselai/pgJQ) 0.1.0
- [hashtypes](https://github.com/adjust/hashtypes/) 0.1.5
- [db_migrator](https://github.com/cybertec-postgresql/db_migrator) 1.0.0
- [pg_cooldown](https://github.com/rbergm/pg_cooldown) 0.1

**更新扩展版本**

- citus 13.0.0 -> 13.0.1
- pg_mooncake 0.1.0 -> 0.1.1
- timescaledb 2.17.2 -> 2.18.1
- supautils 2.5.0 -> 2.6.0
- VectorChord 0.1.0 -> 0.2.0
- pg_bulkload 3.1.22 (+pg17)
- pg_store_plan 1.8 (+pg17)
- pg_search 0.14 -> 0.15.1
- pg_analytics 0.3.0 -> 0.3.2
- pgroonga 3.2.5 -> 4.0.0
- zhparser 2.2 -> 2.3
- pg_vectorize 0.20.0 -> 0.21.1

**校验和**

```
6da06705be1c179941327c836d455d35  pig-0.1.4-1.aarch64.rpm
9fa5712e3cfe56e0dcf22a11320b01b1  pig-0.1.4-1.x86_64.rpm
af506dc37f955a7a2e31ff11e227450c  pig-v0.1.4.linux-amd64.tar.gz
1e6eb3dc1ad26f49b07afabdd9142d4e  pig-v0.1.4.linux-arm64.tar.gz
83ae89b58bff003da5c3022eeac1786e  pig_0.1.4_amd64.deb
d6778e628d82bddf3fae1e058e1e05e4  pig_0.1.4_arm64.deb
```

发布：https://github.com/pgsty/pig/releases/tag/v0.1.4




## v0.1.3

v0.1.3，常规更新，现在可用 390 个扩展！

- 新扩展：[`Omnigres`](https://ext.pgsty.com/e/omni) 33 个扩展，postgres 作为平台
- 新扩展：[`pg_mooncake`](https://ext.pgsty.com/e/pg_mooncake)：postgres 中的 duckdb
- 新扩展：[`pg_xxhash`](https://ext.pgsty.com/e/xxhash)
- 新扩展：[`timescaledb_toolkit`](https://ext.pgsty.com/e/timescaledb_toolkit)
- 新扩展：[`pg_xenophile`](https://ext.pgsty.com/e/pg_xenophile)
- 新扩展：[`pg_drop_events`](https://ext.pgsty.com/e/pg_drop_events)
- 新扩展：[`pg_incremental`](https://ext.pgsty.com/e/pg_incremental)
- 将 [`citus`](https://github.com/citusdata/citus/tree/v13.0.0) 升级到 13.0.0，支持 PostgreSQL 17。
- 将 [`pgml`](https://github.com/postgresml/postgresml/releases/tag/v2.10.0) 升级到 2.10.0
- 将 [`pg_extra_time`](https://ext.pgsty.com/e/pg_extra_time) 升级到 2.0.0
- 将 [`pg_vectorize`](https://ext.pgsty.com/e/pg_vectorize) 升级到 0.20.0

```bash
curl https://repo.pigsty.io/pig | bash
curl https://repo.pigsty.cc/pig | bash
```

**校验和**

```
c79b74f676b03482859f5519b279b657  pig-0.1.3-1.aarch64.rpm
1d00a7cd5855a65e4db964075a5e49f6  pig-0.1.3-1.x86_64.rpm
6cd8507b130fca093247278e36d9478b  pig-v0.1.3.linux-amd64.tar.gz
5eee92908701b0d456ec3c15bc817c0b  pig-v0.1.3.linux-arm64.tar.gz
cb376ef2c3512ad35ff43132942c0052  pig_0.1.3_amd64.deb
2b545abc617670a96c2edd13878e0227  pig_0.1.3_arm64.deb
```

发布：https://github.com/pgsty/pig/releases/tag/v0.1.3




## v0.1.2

[**351**](https://ext.pgsty.com/list) 个 PostgreSQL 扩展，包括强大的 [postgresql-anonymizer 2.0](https://postgresql-anonymizer.readthedocs.io/en/stable/)

现在您可以使用以下方式安装 pig：

```bash
curl -fsSL https://repo.pigsty.io/pig | bash
curl -fsSL https://repo.pigsty.cc/pig | bash
```

**添加新扩展**

- 添加 pg_anon 2.0.0
- 添加 omnisketch 1.0.2
- 添加 ddsketch 1.0.1
- 添加 pg_duration 1.0.1
- 添加 ddl_historization 0.0.7
- 添加 data_historization 1.1.0
- 添加 schedoc 0.0.1
- 添加 floatfile 1.3.1
- 添加 pg_upless 0.0.3
- 添加 pg_task 1.0.0
- 添加 pg_readme 0.7.0
- 添加 vasco 0.1.0
- 添加 pg_xxhash 0.0.1

**更新扩展**

- lower_quantile 1.0.3
- quantile 1.1.8
- sequential_uuids 1.0.3
- pgmq 1.5.0 (subdir)
- floatvec 1.1.1
- pg_parquet 0.2.0
- wrappers 0.4.4
- pg_later 0.3.0
- 修复 topn for deb.arm64
- 在 debian 上添加 age 17
- powa + pg17, 5.0.1
- h3 + pg17
- ogr_fdw + pg17
- age + pg17 1.5 在 debian
- pgtap + pg17 1.3.3
- repmgr
- topn + pg17
- pg_partman 5.2.4
- credcheck 3.0
- ogr_fdw 1.1.5
- ddlx 0.29
- postgis 3.5.1
- tdigest 1.4.3
- pg_repack 1.5.2

发布：https://github.com/pgsty/pig/releases/tag/v0.1.2




## v0.1.0

pig CLI v0.1 发布，具有以下新功能：

**安装脚本**

```bash
curl -fsSL https://repo.pigsty.io/pig | bash     # cloudflare，默认
curl -fsSL https://repo.pigsty.cc/pig | bash     # 中国大陆镜像
```

**扩展管理**

您可以使用 `import` 子命令下载扩展及其依赖项，使用 `link` 激活不同的 postgres 主版本，并使用 `build` 子命令准备构建环境

```bash
pig ext list    [query]      # 列出和搜索扩展
pig ext info    [ext...]     # 获取特定扩展的信息
pig ext status  [-v]         # 显示已安装的扩展和 pg 状态
pig ext add     [ext...]     # 为当前 pg 版本安装扩展
pig ext rm      [ext...]     # 为当前 pg 版本移除扩展
pig ext update  [ext...]     # 将扩展更新到最新版本
pig ext import  [ext...]     # 将扩展下载到本地仓库
pig ext link    [ext...]     # 将 postgres 安装链接到路径
pig ext build   [ext...]     # 为扩展设置构建环境
```

**仓库管理**

您现在可以创建本地仓库并从中创建 tarball（离线包），将其复制到某处（例如，没有互联网访问），并从该离线包创建仓库：

```bash
pig repo list                    # 可用仓库列表              (info)
pig repo info   [repo|module...] # 显示仓库信息               (info)
pig repo status                  # 显示当前仓库状态           (info)
pig repo add    [repo|module...] # 添加仓库和模块             (root)
pig repo rm     [repo|module...] # 移除仓库和模块             (root)
pig repo update                  # 更新仓库包缓存             (root)
pig repo create                  # 在当前系统上创建仓库       (root)
pig repo boot                    # 从离线包启动仓库           (root)
pig repo cache                   # 将仓库缓存为离线包         (root)
```

**Pigsty 管理**

**pig** 也可以用作 [Pigsty](https://doc.pgsty.com/) 的 CLI 工具——电池级免费 PostgreSQL RDS

```bash
pig sty init     # 将嵌入的 pigsty 安装到 ~/pigsty
pig sty boot     # 安装 ansible 和其他预依赖项
pig sty conf     # 自动生成 pigsty.yml 配置文件
pig sty install  # 运行 install.yml playbook
```

**自更新**

要将 pig 本身更新到最新版本，您可以使用以下命令：

```bash
pig update
```

**信息**

现在 pig info 提供有关您的 OS 和 PG 环境的更多详细信息：

```
$ pig info

# [Configuration] ================================
Pig Version      : 0.1.0
Pig Config       : /home/vagrant/.pig/config.yml
Log Level        : info
Log Path         : stderr

# [OS Environment] ===============================
OS Distro Code   : el9
OS Architecture  : amd64
OS Package Type  : rpm
OS Vendor ID     : rocky
OS Version       : 9
OS Version Full  : 9.3
OS Version Code  : el9

# [PG Environment] ===============================
Installed:
* PostgreSQL 17.2  74  Extensions

Active:
PG Version      :  PostgreSQL 17.2
Config Path     :  /usr/pgsql-17/bin/pg_config
Binary Path     :  /usr/pgsql-17/bin
Library Path    :  /usr/pgsql-17/lib
Extension Path  :  /usr/pgsql-17/share/extension

# [Pigsty Environment] ===========================
Inventory Path   : /home/vagrant/pigsty/pigsty.yml
Pigsty Home      : /home/vagrant/pigsty
Embedded Version : 3.2.0

# [Network Conditions] ===========================
pigsty.cc  ping ok: 141 ms
pigsty.io  ping ok: 930 ms
google.com request error
Internet Access   :  true
Pigsty Repo       :  pigsty.io
Inferred Region   :  china
Latest Pigsty Ver :  v3.2.0
```

享受 PostgreSQL！

**更改内容**

- 由 @kianmeng 在 https://github.com/pgsty/pig/pull/4 中修复拼写错误

**新贡献者**

- @kianmeng 在 https://github.com/pgsty/pig/pull/4 中首次贡献

**完整变更日志**：https://github.com/pgsty/pig/compare/v0.0.1...v0.1.0

**校验和**

```
46165beec97ab9ff1314f80af953bd59  pig-0.1.0-1.aarch64.rpm
1320a6f9bfbd79948515657d6becbf37  pig-0.1.0-1.x86_64.rpm
bd078a5dc0c41454fcbbe0d8693d5fa0  pig-v0.1.0.linux-amd64.tar.gz
8a15e52f96735b78afa7da42843f1504  pig-v0.1.0.linux-arm64.tar.gz
4d25597cff8425c7e52a2b411344aa4a  pig_0.1.0_amd64.deb
d5f0874601bc1bbd0dd40b5c9982ea9f  pig_0.1.0_arm64.deb
```

发布：https://github.com/pgsty/pig/releases/tag/v0.1.0




## v0.0.1

**入门**

首先安装 `pig` 包，您也可以通过命令安装：

```bash
curl -fsSL https://repo.pigsty.io/pig | bash     # cloudflare，默认
curl -fsSL https://repo.pigsty.cc/pig | bash     # 中国大陆镜像
```

然后就可以使用了，假设您想安装 [`pg_duckdb`](https://ext.pgsty.com/e/pg_duckdb) 扩展：

```bash
$ pig repo add pigsty pgdg -u  # 添加 pgdg 和 pigsty 仓库，更新缓存
$ pig ext install pg17         # 使用 PGDG 原生包安装 PostgreSQL 17 内核
$ pig ext install pg_duckdb    # 安装 pg_duckdb 扩展（用于当前 pg17）
```

就是这样！全部设置好了！您可以使用 `pig ext status` 子命令检查：

```bash
$ pig ext status               # 显示已安装的扩展和 pg 状态
                               # 要打印内置 contrib 扩展，使用 -c|--contrib 标志
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

查看高级用法详情和 [列出 340 个可用扩展](https://ext.pgsty.com/list)。


**安装**

`pig` 工具是一个独立的 go 二进制文件，没有依赖项。您可以直接下载二进制文件或使用以下命令添加仓库并通过包管理器安装（推荐）。

对于 Ubuntu 22.04 / 24.04 和 Debian 12 或任何兼容平台：

```bash
sudo tee /etc/apt/sources.list.d/pigsty.list > /dev/null <<EOF
deb [trusted=yes] https://repo.pigsty.io/apt/infra generic main
EOF
sudo apt update; sudo apt install -y pig
```

对于 EL 8/9 和兼容平台：

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

> 对于中国大陆用户：考虑将 `repo.pigsty.io` 替换为 `repo.pigsty.cc`

**兼容性**

`pig` 运行在：RHEL 8/9、Ubuntu 22.04/24.04 和 Debian 12，支持 `amd64/arm64` 架构

|   代码    | 发行版                         |  `x86_64`  | `aarch64`  |
|:-------:|-----------------------------|:----------:|:----------:|
| **el9** | RHEL 9 / Rocky9 / Alma9 / … | PG 17 - 13 | PG 17 - 13 |
| **el8** | RHEL 8 / Rocky8 / Alma8 / … | PG 17 - 13 | PG 17 - 13 |
| **u24** | Ubuntu 24.04 (`noble`)      | PG 17 - 13 | PG 17 - 13 |
| **u22** | Ubuntu 22.04 (`jammy`)      | PG 17 - 13 | PG 17 - 13 |
| **d12** | Debian 12 (`bookworm`)      | PG 17 - 13 | PG 17 - 13 |

以下是上述发行版的一些坏情况和限制：

- [`citus`](https://ext.pgsty.com/e/citus) 在 `aarch64` 和 ubuntu 24.04 上不可用
- [`pljava`](https://ext.pgsty.com/e/pljava) 在 `el8` 上缺失
- [`jdbc_fdw`](https://ext.pgsty.com/e/jdbc_fdw) 在 `el8.aarch64` 和 `el9.aarch64` 上缺失
- [`pllua`](https://ext.pgsty.com/e/pllua) 在 `el8.aarch64` 上对 pg 13,14,15 缺失
- [`topn`](https://ext.pgsty.com/e/topn) 在 `el8.aarch64` 和 `el9.aarch64` 上对 pg13 缺失，以及所有 `deb.aarch64`
- [`pg_partman`](https://ext.pgsty.com/e/pg_partman) 和 [`timeseries`](https://ext.pgsty.com/e/timeseries) 在 `u24` 上对 pg13 缺失
- [`wiltondb`](https://ext.pgsty.com/e/wiltondb) 在 `d12` 上缺失

发布：https://github.com/pgsty/pig/releases/tag/v0.0.1
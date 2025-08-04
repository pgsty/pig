---
title: "CMD: build"
description: 如何使用 pig build 子命令搭建构建基础设施？
icon: SquareTerminal
weight: 640
---


`pig build` 是一款强大的命令行工具，简化了 PostgreSQL 扩展从构建环境搭建到跨操作系统编译的全流程管理。

------

## 概述

```
构建 Postgres 扩展

用法：
  pig build [命令]

别名：
  build, b

示例：
pig build - 构建 Postgres 扩展

  pig build repo                   # 初始化构建仓库（=repo set -ru）
  pig build tool  [mini|full|...]  # 初始化构建工具集
  pig build proxy [id@host:port ]  # 初始化构建代理（可选）
  pig build rust  [-v <pgrx_ver>]  # 初始化 rustc & pgrx（0.12.9）
  pig build spec                   # 初始化构建规范仓库
  pig build get   [all|std|..]     # 按前缀获取扩展源码包
  pig build ext   [extname...]     # 构建扩展

可用子命令：
  ext         构建扩展
  get         下载源码包
  proxy       初始化构建代理
  repo        初始化所需仓库
  rust        初始化 Rust 与 pgrx 环境
  spec        初始化构建规范仓库
  tool        初始化构建工具

参数：
  -h, --help   查看 build 帮助

全局参数：
      --debug              启用调试模式
  -i, --inventory string   配置清单路径
      --log-level string   日志级别：debug, info, warn, error, fatal, panic（默认 "info"）
      --log-path string    日志文件路径，默认输出到终端
```

------

## 示例

搭建构建环境

```bash
# 初始化仓库与工具
pig build repo
pig build tool
pig build spec

# 构建 Rust 扩展
pig build rust

# 下载标准扩展源码
pig build get std                # 下载全部源码包
pig build get citus timescaledb  # 下载指定源码包

# 构建指定扩展
pig build ext citus
```

------

## 构建流程

使用 `pig build` 构建 PostgreSQL 扩展的典型流程：

1. 初始化仓库：`pig build repo`
2. 安装构建工具：`pig build tool`
3. （可选）配置代理：`pig build proxy id@host:port`
4. （可选，Rust 扩展）配置 Rust 环境：`pig build rust`
5. 初始化构建规范：`pig build spec`
6. 下载源码包：`pig build get [前缀]`
7. 构建扩展：`pig build ext [扩展名...]`

### 注意事项

- EL 与 DEB 发行版构建流程略有差异
- 部分命令需 sudo 权限以安装系统依赖
- Rust 扩展需先配置 Rust 环境
- 代理配置为可选，仅在受限网络环境下需要

### 操作系统支持

`pig build` 支持：

- [**EL 发行版**](/zh/repo/): RHEL、Rocky、CentOS（已在 8/9 测试）
- [**DEB 发行版**](/zh/repo/): Debian（已在 12 测试）、Ubuntu（已在 22.04/24.04 测试）
- 未来可能支持 macOS（通过 homebrew）

------

## `build repo`

**别名**：`r`

通过运行仓库添加命令（自动启用更新与移除标志），为扩展构建添加所需上游仓库。

```bash
pig build repo
```

------

## `build tool`

**别名**：`t`

安装编译 PostgreSQL 扩展所需的构建工具。

```bash
pig build tool
```

**参数**：

- ```
mode
  ```

: 安装模式（默认：“mini”）

- 可用模式取决于操作系统与发行版

此命令会安装如下构建依赖：

**EL 发行版**（RHEL、Rocky、CentOS）：

```
make, cmake, ninja-build, pkg-config, lld, git, lz4, unzip, ncdu, rsync, vray,
rpmdevtools, dnf-utils, pgdg-srpm-macros, postgresql1*-devel, postgresql1*-server, jq,
readline-devel, zlib-devel, libxml2-devel, lz4-devel, libzstd-devel, krb5-devel,
```

**DEB 发行版**（Debian、Ubuntu）：

```fallback
make, cmake, ninja-build, pkg-config, lld, git, lz4, unzip, ncdu, rsync, vray,
debhelper, devscripts, fakeroot, postgresql-all, postgresql-server-dev-all, jq,
libreadline-dev, zlib1g-dev, libxml2-dev, liblz4-dev, libzstd-dev, libkrb5-dev,
```

------

## `build rust`

**别名**：`rs`

为构建 Rust 扩展配置 Rust 环境与 PGRX。

```bash
pig build rust [-v <pgrx_ver>]
```

**参数**：

- `-v <pgrx_ver>`：指定安装的 PGRX 版本（默认：“`0.12.9`”）

此命令将：

1. 若未安装则通过 rustup 安装 Rust
2. 安装指定版本的 cargo-pgrx
3. 按 PostgreSQL 版本（13-17）初始化 PGRX 配置

------

## `build spec`

**别名**：`s`

根据操作系统初始化构建规范仓库：

```bash
pig build spec
```

- **EL 发行版**（RHEL、Rocky、CentOS）：
1. 从 GitHub 克隆 [RPM Spec 仓库](https://github.com/pgsty/rpm)
2. 在 `~/rpmbuild` 下搭建 RPM 构建目录结构
- **DEB 发行版**（Debian、Ubuntu）：
1. 从 GitHub 克隆 [DEB Spec 仓库](https://github.com/pgsty/deb)
2. 在 `~/deb` 下搭建 DEB 构建目录结构

该仓库包含 Pigsty 维护的扩展构建规范（rpmspec 或 debian control 文件）。

- RPM: https://github.com/pgsty/rpm
- DEB: https://github.com/pgsty/deb

------

## `build get`

**别名**：`get`

下载 PostgreSQL 扩展源码包。

```bash
pig build get [前缀|all|std]
```

**参数**：

- `std`：下载标准包（默认，排除大体积包）
- `all`：下载全部可用源码包（批量构建用）
- `[前缀]`：指定一个或多个前缀，筛选需下载的包

下载文件存放路径：

- EL: `~/rpmbuild/SOURCES/`
- DEB: `~/deb/tarball/`

示例：

```bash
pig build get std                # 下载标准包（不含 pg_duckdb/pg_mooncake/omnigres/plv8，体积过大）
pig build get all                # 下载全部源码包

pig build get pg_mooncake        # 下载 pg_mooncake 源码
pig build get pg_duckdb          # 下载 pg_duckdb 源码
pig build get omnigres           # 下载 omnigres 源码
pig build get plv8               # 下载 plv8 源码

pig build get citus              # 下载 citus 源码
pig build get timescaledb        # 下载 timescaledb 源码
pig build get hydra              # 下载 hydra 源码
pig build get pgjwt              # 下载 pgjwt 源码
....
```

------

## `build ext`

**别名**：`e`

使用当前操作系统的构建环境编译 PostgreSQL 扩展。

```bash
pig build ext [扩展名...]
```

**参数**：

- `扩展名`：一个或多个需构建的扩展名

对于每个扩展，命令将：

1. 切换至对应构建目录（`~/rpmbuild/BUILD` 或 `~/deb/build`）
2. 执行 make 命令（如 `make citus`）
3. 汇报每个扩展的构建结果（成功/失败）

实际构建过程依赖规范仓库中的 `make <ext>` 命令。

------

## `build proxy`

**别名**：`p`

为受限网络环境配置代理服务器以访问外部资源。

此步骤**可选**，如无网络限制可跳过。

```bash
pig build proxy [id@host:port] [local]
```

**参数**：

- `id@host:port`：远程 `v2ray` 代理规范，格式为 `user-id@host:port`
- `local`：本地 `v2ray` 绑定地址（默认：“127.0.0.1:12345”）

此命令将：

1. 若未安装则自动安装代理软件（/usr/local/bin/v2ray）
2. 按指定远程与本地参数配置代理
3. 在 `/etc/profile.d/proxy.sh` 创建环境变量脚本
4. 配置代理服务
5. 通过 curl google 测试代理连通性

可通过如下命令加载代理环境变量：

```bash
. /etc/profile.d/proxy.sh
```

配置后可用以下别名：

- `po`：启用代理（在当前 shell 设置代理环境变量）
- `px`：禁用代理（取消代理环境变量）
- `pck`：检测代理状态（通过 ping google）
---
title: "CMD: repo"
description: 如何使用 pig repo 子命令管理软件仓库？
icon: SquareTerminal
weight: 610
---

`pig repo` 命令是一个用于管理软件包仓库的综合性工具。
它支持管理、添加、移除、创建及操作操作系统软件仓库，兼容 RPM 系（EL）与 Debian 系统。

------

## 概览

```bash
pig repo - 管理 Linux APT/YUM 仓库

  pig repo list                    # 查看可用仓库列表             （信息）
  pig repo info   [repo|module...] # 显示仓库详细信息              （信息）
  pig repo status                  # 显示当前仓库状态              （信息）
  pig repo add    [repo|module...] # 添加仓库及模块                （需 root）
  pig repo rm     [repo|module...] # 移除仓库及模块                （需 root）
  pig repo update                  # 更新仓库包缓存                （需 root）
  pig repo create                  # 在本地创建仓库                （需 root）
  pig repo boot                    # 通过离线包引导仓库            （需 root）
  pig repo cache                   # 缓存仓库为离线包              （需 root）

用法：
  pig repo [命令]

别名：
  repo, r

示例：

  入门指南：https://pigsty.io/ext/pig/
  pig repo add -ru                 # 添加全部仓库并更新缓存（简单粗暴）
  pig repo add pigsty -u           # 仅添加 pigsty 仓库并更新缓存（推荐）
  pig repo add node pgdg pigsty    # 安装 PostgreSQL 所需核心仓库
  pig repo add all                 # all = node + pgdg + pigsty
  pig repo add all extra           # extra 模块含非自由及部分第三方扩展仓库
  pig repo update                  # 更新仓库缓存
  pig repo create                  # 更新本地仓库 /www/pigsty 元数据
  pig repo boot                    # 解包 /tmp/pkg.tgz 至 /www/pigsty
  pig repo cache                   # 打包 /www/pigsty 为 /tmp/pkg.tgz

可用子命令：
  add         添加新仓库
  boot        通过离线包引导仓库
  cache       从本地仓库创建离线包
  create      创建本地 YUM/APT 仓库
  info        获取仓库详细信息
  list        显示可用仓库列表
  rm          移除仓库
  set         清空并覆盖仓库
  status      显示当前仓库状态
  update      更新仓库缓存

参数：
  -h, --help   显示帮助

全局参数：
      --debug              启用调试模式
  -i, --inventory string   指定配置清单路径
      --log-level string   日志级别：debug, info, warn, error, fatal, panic（默认 "info"）
      --log-path string    日志文件路径，默认输出至终端
```

------

## 示例

列出可用仓库，添加 PGDG 与 Pigsty 仓库，并更新本地缓存。

```bash
# 列出可用模块
pig repo list

# 添加 PGDG 与 Pigsty 仓库
pig repo add pgdg pigsty

# yum makecache 或 apt update
pig repo update
```

添加新仓库后需刷新仓库元数据缓存，可通过专用的 `pig repo update` 命令，或在 `pig repo add` 时加 `-u|--update` 参数。

```bash
pig repo add pigsty -u    # 添加 pigsty 仓库并更新缓存
```

如需**清空**现有仓库后再添加新仓库，可加 `-r|--remove` 参数，或直接用 `pig repo set` 子命令。

```bash
pig repo add all --remove # 移除所有现有仓库后添加 node, pgdg, pigsty 并更新缓存
pig repo add -r           # 同上，未指定模块时默认添加 all
pig repo set              # 等同于 add --remove，移除的仓库文件备份至 /etc/yum.repos.d/backup 或 /etc/apt/sources.list.d/backup
```

最彻底且可靠的 PostgreSQL 仓库配置方式：

```bash
pig repo set -u           # 清空所有仓库，添加所需仓库并更新缓存
```

------

## 模块说明

在 Pigsty 中，所有仓库按模块组织。模块是仓库的集合。

模块名会根据操作系统发行版、主版本、架构及地理区域映射到不同的实际仓库。

Pigsty 会自动处理所有细节，可用 `pig repo list` 查看所有仓库与模块。

```yaml
repo_modules:   # 可用模块：19 个
  - all       : pigsty-infra, pigsty-pgsql, pgdg, baseos, appstream, extras, powertools, crb, epel, base, updates, security, backports
  - pigsty    : pigsty-infra, pigsty-pgsql
  - pgdg      : pgdg
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

通常安装 PostgreSQL 及其扩展需以下 3 个模块：

- `pgdg`：官方 PostgreSQL 仓库，含内核包、工具及 100+ 扩展
- `pigsty`：Pigsty 扩展仓库，含 200+ 额外扩展与工具
- `node`：操作系统默认仓库，提供 PostgreSQL 依赖库

有一个便捷的伪模块别名 `all`，包含上述 3 个核心模块。可用 `pig repo add all` 一键添加，或更简写：`pig repo add`。

------

## `repo list`

列出当前系统可用的仓库模块与仓库。

```
显示可用仓库列表

用法：
  pig repo list [参数]

别名：
  list, l, ls

示例：

  pig repo list                # 列出当前系统可用仓库
  pig repo list all            # 列出所有未过滤的原始仓库数据

参数：
  -h, --help   显示帮助
```

可用仓库定义在 [**`cli/repo/assets/repo.yml`**](/zh/cli/repo/assets/repo.yml)，如需自定义仓库列表，可在 `~/.pig/repo.yml` 添加自定义配置。

------

## `repo add`

向系统添加仓库配置文件。

```bash
添加新仓库

用法：
  pig repo add [参数]

别名：
  add, a, append

示例：

  pig repo add                      # = pig repo add all
  pig repo add all                  # 添加 node, pgsql, infra 仓库（推荐）
  pig repo add all -u               # 添加上述仓库并更新缓存（或：--update）
  pig repo add all -r               # 添加全部仓库并移除旧仓库（或：--remove）
  pig repo add pigsty --update      # 添加 pigsty 扩展仓库并更新缓存
  pig repo add pgdg --update        # 添加 pgdg 官方仓库并更新缓存
  pig repo add pgsql node --remove  # 添加操作系统与 PG 仓库并移除旧仓库
  pig repo add infra                # 添加观测、Grafana、Prometheus 及 PG 工具仓库

  （注意：系统仓库管理需 sudo/root 权限）

  可用仓库模块：
  - all      :  pgsql + node + infra（推荐）
    - pigsty :  PostgreSQL 扩展仓库（默认）
    - pgdg   :  PGDG 官方 PostgreSQL 仓库（官方）
    - node   :  操作系统官方仓库（el/debian/ubuntu）
  - pgsql    :  pigsty + pgdg（全部 PG 扩展）
  # 用 pig repo list 查看可用仓库与模块

参数：
  -h, --help            显示帮助
      --region string   区域代码（default|china）
  -r, --remove          添加前移除现有仓库
  -u, --update          添加后自动更新缓存
```

该命令：

1. 校验指定模块是否存在，并根据

- 区域、发行版、主版本、架构

1. 若加 `-r|--remove`，则先将现有仓库移至备份目录：

- EL 系统：`/etc/yum.repos.d/backup`
- Debian 系统：`/etc/apt/sources.list.d/backup`

1. 在系统仓库目录创建仓库文件：

- EL 系统：`/etc/yum.repos.d/<module>.repo`
- Debian 系统：`/etc/apt/sources.list.d/<module>.list`

1. 若加 `-u|--update`，则自动执行 `apt update` 或 `dnf makecache` 刷新缓存。

如非 root 用户，需 sudo 权限。

------

## `repo set`

等同于 `repo add <...> --remove`，即先移除现有仓库再添加新仓库。

```bash
清空并覆盖仓库

用法：
  pig repo set [参数]

别名：
  set, overwrite

示例：

  pig repo set all                  # 设置为 node, pgsql, infra（推荐）
  pig repo set all -u               # 设置上述仓库并更新缓存（或 --update）
  pig repo set pigsty --update      # 设置为 pigsty 扩展仓库并更新缓存
  pig repo set pgdg   --update      # 设置为 pgdg 官方仓库并更新缓存
  pig repo set infra                # 设置为观测、Grafana、Prometheus 及 PG 工具仓库

  （注意：系统仓库管理需 sudo/root 权限）

参数：
  -h, --help            显示帮助
      --region string   区域代码
  -u, --update          添加后自动更新缓存
```

如非 root 用户，需 sudo 权限。

------

## `repo update`

刷新仓库缓存，等同于 `apt update` 或 `yum makecache`。

```bash
更新仓库缓存

用法：
  pig repo update [参数]

别名：
  update, u

示例：

  pig repo update                  # yum makecache 或 apt update

参数：
  -h, --help   显示帮助
```

如非 root 用户，需 sudo 权限。

------

## `repo rm`

移除系统中的仓库配置文件。

```bash
移除仓库

用法：
  pig repo rm [参数]

别名：
  rm, remove

示例：

  pig repo rm                      # 移除（备份）所有现有仓库至备份目录
  pig repo rm all --update         # 移除 all 模块并更新缓存
  pig repo rm node pigsty -u       # 移除 node 与 pigsty 模块并更新缓存

参数：
  -h, --help     显示帮助
  -u, --update   移除后自动更新缓存
```

该命令会从系统中移除仓库文件，若加 `-u|--update`，则移除后自动刷新缓存。

移除前会自动备份现有仓库配置。

如非 root 用户，通常需 sudo 权限。

------

## `repo status`

打印系统仓库目录，并用包管理器列出可用仓库。

```bash
显示当前仓库状态

用法：
  pig repo status [参数]

别名：
  status, s, st

参数：
  -h, --help   显示帮助
```

------

## `repo info`

显示指定仓库或模块的详细信息。

```bash
获取仓库详细信息

用法：
  pig repo info [参数]

别名：
  info, i

参数：
  -h, --help   显示帮助
```

**示例：**

```bash
#-------------------------------------------------
Name       : pgdg
Summary    : PGDG
Available  : Yes (debian d12 amd64)
Module     : pgsql
OS Arch    : [x86_64, aarch64]
OS Distro  : deb [11,12,20,22,24]
Meta       : trusted=yes
Base URL   : http://apt.postgresql.org/pub/repos/apt/ ${distro_codename}-pgdg main
     china : https://mirrors.tuna.tsinghua.edu.cn/postgresql/repos/apt/ ${distro_codename}-pgdg main

# 默认仓库内容
# pgdg PGDG
deb [trusted=yes] http://apt.postgresql.org/pub/repos/apt/ bookworm-pgdg main

# 中国镜像仓库内容
# pgdg PGDG
deb [trusted=yes] https://mirrors.tuna.tsinghua.edu.cn/postgresql/repos/apt/ bookworm-pgdg main
```

会输出指定仓库或模块的详细信息，并支持区域镜像。

------

## `repo create`

在指定目录下创建本地 YUM/APT 仓库。

```bash
创建本地 YUM/APT 仓库

用法：
  pig repo create [路径...]

别名：
  create, cr

示例：

  pig repo create                    # 默认在 /www/pigsty 创建仓库
  pig repo create /www/mssql /www/b  # 在多个目录创建仓库

  （注意：系统仓库管理需 sudo/root 权限）
```

**默认目录：** `/www/pigsty`

该命令：

1. 若目录不存在则自动创建
2. 使用仓库工具创建本地仓库（需确保已安装相关工具）

- EL 系统：`createrepo_c`
- Debian 系统：`dpkg-dev`

如非 root 用户，需对目标目录有读写权限。

------

## `repo cache`

将仓库内容打包为离线压缩包，便于离线部署。

```bash
pig repo cache [目录路径] [包路径] [repo1,repo2,...]
```

**参数：**

- `目录路径`：源仓库目录（默认：`/www`）
- `包路径`：输出压缩包路径（默认：当前目录下 `pigsty-pkg-<os>-<arch>.tgz`）
- `repos`：要包含的仓库子目录（默认：全部）

**示例：**

```bash
pig repo cache /www /tmp/pkg.tgz pigsty
pig repo cache /www /tmp/pkg.tgz pigsty mssql ivory
```

可对本地仓库打包，便于新系统离线引导。

------

## `repo cache`

从本地仓库创建离线包

```bash
从本地仓库创建离线包

用法：
  pig repo cache [参数]

别名：
  cache, c

示例：

  pig repo cache                    # 从 /www/pigsty 创建 /tmp/pkg.tgz 离线包
  pig repo cache -f                 # 强制覆盖已存在的离线包
  pig repo cache -d /srv            # 指定内容目录为 /srv
  pig repo cache pigsty mssql       # 仅打包 pigsty 与 mssql 仓库
  pig repo c -f                     # 最简用法，强制打包

  （注意：系统仓库管理需 sudo/root 权限）

参数：
  -d, --dir string    源仓库路径（默认 "/www/")
  -h, --help          显示帮助
  -p, --path string   离线包路径（默认 "/tmp/pkg.tgz")
```

------

## `repo boot`

通过离线包引导本地仓库。

```bash
通过离线包引导仓库

用法：
  pig repo boot [参数]

别名：
  boot, b, bt

示例：

  pig repo boot                    # 从 /tmp/pkg.tgz 引导至 /www
  pig repo boot -p /tmp/pkg.tgz    # 指定离线包路径
  pig repo boot -d /srv            # 指定目标目录为 /srv

参数：
  -d, --dir string    目标仓库路径（默认 "/www/")
  -h, --help          显示帮助
  -p, --path string   离线包路径（默认 "/tmp/pkg.tgz")
```

**参数说明：**

- `offline_package`：由 `pig repo cache` 生成的离线包路径
- `target_directory`：解包目标目录（默认：`/www`）

**示例：**

```bash
pig repo boot /tmp/pkg.tgz /www
```

该命令：

1. 解包至目标目录
2. 配置本地仓库
3. 更新仓库元数据

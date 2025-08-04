---
title: "CMD: sty"
description: 如何使用 pig sty 子命令管理 Pigsty
icon: SquareTerminal
weight: 630
---


**pig** 也可作为 Pigsty 的命令行工具使用 —— 这是一款开箱即用的免费 PostgreSQL RDS 解决方案。
它为你的 PostgreSQL 集群带来高可用（HA）、PITR、监控、基础设施即代码（IaC）以及丰富的扩展支持。

```bash
pig sty - 初始化（下载）、引导、配置与安装 Pigsty

  pig sty init    [-pfvd]      # 安装 pigsty（默认目录 ~/pigsty）
  pig sty boot    [-rpk]       # 安装 ansible 并准备离线包
  pig sty conf    [-civrsxn]   # 配置 pigsty 并生成配置文件
  pig sty install              # 使用 pigsty 安装与环境部署（危险操作！）
  pig sty get                  # 下载 pigsty 源码压缩包
  pig sty list                 # 列出可用 pigsty 版本

用法：
  pig sty [命令]

别名：
  sty, s, pigsty

示例：
  入门指南：https://pgsty.com/zh/docs/install
  pig sty init                 # 解压并初始化 ~/pigsty
  pig sty boot                 # 安装 ansible 及其他依赖
  pig sty conf                 # 生成 pigsty.yml 配置文件
  pig sty install              # 执行 pigsty/install.yml 剧本

可用命令：
  boot        引导 Pigsty
  conf        配置 Pigsty
  get         下载 pigsty 可用版本
  init        安装 Pigsty
  install     执行 pigsty install.yml 剧本
  list        列出 pigsty 可用版本

参数：
  -h, --help   获取 sty 帮助
```

你可以使用 `pig sty` 子命令在当前节点引导部署 Pigsty。


------

## `sty init`

```bash
pig sty init
  -p | --path    : 安装路径，默认为 ~/pigsty
  -f | --force   : 强制覆盖已存在的 pigsty 目录
  -v | --version : pigsty 版本，默认内置版本
  -d | --dir     : 下载目录，默认为 /tmp

用法：
  pig sty init [参数]

别名：
  init, i

示例：

  pig sty init                   # 使用内置版本安装到 ~/pigsty
  pig sty init -f                # 安装并覆盖已有 pigsty 目录
  pig sty init -p /tmp/pigsty    # 安装到指定目录 /tmp/pigsty
  pig sty init -v 3.3            # 获取并安装指定版本 v3.3.0
  pig sty init 3                 # 获取并安装指定主版本 v3 最新


参数：
  -d, --dir string       pigsty 下载目录（默认 "/tmp"）
  -f, --force            覆盖已存在 pigsty（默认不覆盖）
  -h, --help             获取 init 帮助
  -p, --path string      目标安装目录（默认 "~/pigsty"）
  -v, --version string   pigsty 版本号
```

------

## `sty boot`

```bash
pig sty boot
  [-r|--region <region>]   [default,china,europe]
  [-p|--path <path>]       指定离线包路径
  [-k|--keep]              引导时保留已有上游仓库

详见：https://pigsty.io/zh/docs/setup/offline/#bootstrap

用法：
  pig sty boot [参数]

别名：
  boot, b, bootstrap

参数：
  -h, --help            获取 boot 帮助
  -k, --keep            保留已有仓库
  -p, --path string     离线包路径
  -r, --region string   区域（default, china, europe...）
```

------

## `sty conf`

```bash
使用 ./configure 配置 pigsty

pig sty conf
  [-c|--conf <name>]       # [meta|dual|trio|full|prod]
  [--ip <ip>]              # 主节点 IP 地址（-s 跳过）
  [-v|--version <pgver>]   # [17|16|15|14|13]
  [-r|--region <region>]   # [default|china|europe]
  [-s|--skip]              # 跳过 IP 探测
  [-x|--proxy]             # 从环境变量写入代理配置
  [-n|--non-interactive]   # 非交互模式

详见：https://pigsty.io/zh/docs/setup/install/#configure

用法：
  pig sty conf [参数]

别名：
  conf, c, configure

示例：

  pig sty conf                       # 使用默认 conf/meta.yml 配置
  pig sty conf -c rich -x            # 使用 rich.yml 模板，并写入代理配置
  pig sty conf -c supa --ip=10.9.8.7 # 使用 supa 模板，主节点 IP 为 10.9.8.7
  pig sty conf -c full -v 16         # 使用 4 节点 full 模板，默认 PG16
  pig sty conf -c oss -s             # 使用 oss 模板，跳过 IP 探测与替换
  pig sty conf -c slim -s -r china   # 使用 2 节点 slim 模板，指定区域为 china


参数：
  -c, --conf string       配置模板名称
  -h, --help              获取 conf 帮助
      --ip string         主节点 IP 地址
  -n, --non-interactive   非交互式配置
  -p, --proxy             配置代理环境变量
  -r, --region string     上游仓库区域
  -s, --skip              跳过 IP 探测
  -v, --version string    PostgreSQL 主版本
```

------

## `sty install`

```bash
执行 pigsty install.yml 剧本

用法：
  pig sty install [参数]

别名：
  install, ins, install

参数：
  -h, --help   获取 install 帮助
```

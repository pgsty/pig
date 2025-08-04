---
title: 简介
description: 为什么我们还需要一个新的包管理器？尤其是针对 Postgres 扩展？
icon: CircleHelp
weight: 100
breadcrumbs: false
---

你是否曾因安装或升级 PostgreSQL 扩展而头疼？翻查过时的文档、晦涩难懂的配置脚本，或是在 GitHub 上苦寻分支与补丁？
Postgres 丰富的扩展生态同时意味着复杂的部署流程 —— 在多发行版、多架构环境下尤为棘手。而 PIG 可以为您解决这些烦恼。

这正是 **Pig** 诞生的初衷。Pig 由 Go 语言开发，致力于一站式管理 Postgres 及其 [420+](/zh/list/) 扩展。
无论是 TimescaleDB、Citus、PGVector，还是 30+ Rust 扩展，亦或 [自建 Supabase](/zh/app/supabase) 所需的全部组件 —— Pig 统一的 CLI 让一切触手可及。
它彻底告别源码编译与杂乱仓库，直接提供版本对齐的 RPM/DEB 包，完美兼容 Debian、Ubuntu、RedHat 等主流发行版，支持 x86 与 Arm 架构，无需猜测，无需折腾。

Pig 并非重复造轮子，而是充分利用系统原生包管理器（APT、YUM、DNF），严格遵循 PGDG 官方打包规范，确保无缝集成。
你无需在“标准做法”与“快捷方式”之间权衡；Pig 尊重现有仓库，遵循操作系统最佳实践，与现有仓库和软件包和谐共存。
如果你的 Linux 系统和 PostgreSQL 大版本不在支持的列表中，你还可以使用 `pig` 直接针对特定组合编译扩展。

想让你的 Postgres 如虎添翼、远离繁琐？欢迎访问 [PIG 官方文档](https://pig.pgsty.com) 获取文档、指南，并查阅庞大的 [扩展列表](https://ext.pgsty.com/zh/list/)，
让你的本地 Postgres 数据库一键进化为全能的多模态数据中台。
如果说[Postgres 的未来是无可匹敌的可扩展性](https://medium.com/@fengruohang/postgres-is-eating-the-database-world-157c204dcfc4)，那么 Pig 就是帮你解锁它的神灯。毕竟，从没有人抱怨 “扩展太多”。

![](/img/pigsty/ecosystem.gif)

> 《[ANNOUNCE pig: The Postgres Extension Wizard](https://www.postgresql.org/about/news/announce-pig-the-postgres-extension-wizard-2988/)》


--------

## Linux兼容性

| Linux 发行版              | 版本  |   架构    |                                             系统代码                                              |   PostgreSQL 版本    |
|:-----------------------|:---:|:-------:|:---------------------------------------------------------------------------------------------:|:------------------:|
| RHEL9 / Rocky9 / Alma9 | EL9 | x86_64  |  [`el9.x86_64`](https://github.com/pgsty/pigsty/blob/main/roles/node_id/vars/el9.x86_64.yml)  | 17, 16, 15, 14, 13 |
| RHEL9 / Rocky9 / Alma9 | EL9 | aarch64 | [`el9.aarch64`](https://github.com/pgsty/pigsty/blob/main/roles/node_id/vars/el9.aarch64.yml) | 17, 16, 15, 14, 13 |
| RHEL8 / Rocky8 / Alma8 | EL8 | x86_64  |  [`el8.x86_64`](https://github.com/pgsty/pigsty/blob/main/roles/node_id/vars/el8.x86_64.yml)  | 17, 16, 15, 14, 13 |
| RHEL8 / Rocky8 / Alma8 | EL8 | aarch64 | [`el8.aarch64`](https://github.com/pgsty/pigsty/blob/main/roles/node_id/vars/el8.aarch64.yml) | 17, 16, 15, 14, 13 |
| Ubuntu 24.04 (`noble`) | U24 | x86_64  |  [`u24.x86_64`](https://github.com/pgsty/pigsty/blob/main/roles/node_id/vars/u24.x86_64.yml)  | 17, 16, 15, 14, 13 |
| Ubuntu 24.04 (`noble`) | U24 | aarch64 | [`u24.aarch64`](https://github.com/pgsty/pigsty/blob/main/roles/node_id/vars/u24.aarch64.yml) | 17, 16, 15, 14, 13 |
| Ubuntu 22.04 (`jammy`) | U22 | x86_64  |  [`u22.x86_64`](https://github.com/pgsty/pigsty/blob/main/roles/node_id/vars/u22.x86_64.yml)  | 17, 16, 15, 14, 13 |
| Ubuntu 22.04 (`jammy`) | U22 | aarch64 | [`u22.aarch64`](https://github.com/pgsty/pigsty/blob/main/roles/node_id/vars/u22.aarch64.yml) | 17, 16, 15, 14, 13 |
| Debian 12 (`bookworm`) | D12 | x86_64  |  [`d12.x86_64`](https://github.com/pgsty/pigsty/blob/main/roles/node_id/vars/d12.x86_64.yml)  | 17, 16, 15, 14, 13 |
| Debian 12 (`bookworm`) | D12 | aarch64 | [`d12.aarch64`](https://github.com/pgsty/pigsty/blob/main/roles/node_id/vars/d12.aarch64.yml) | 17, 16, 15, 14, 13 |

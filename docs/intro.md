---
title: Introduction
description: Why would we need yet another package manager? especially for Postgres extensions?
weight: 100
icon: CircleHelp
breadcrumbs: false
---

Ever wished installing or upgrading PostgreSQL extensions didn’t feel like digging through outdated readmes, cryptic configure scripts, or random GitHub forks & patches? 
The painful truth is that Postgres’s richness of extension often comes at the cost of complicated setups—especially if you’re juggling multiple distros or CPU architectures.


Enter **Pig**, a Go-based package manager built to tame Postgres and its ecosystem of [420+](https://ext.pgsty.com/list) extensions in one fell swoop.
TimescaleDB, Citus, PGVector, 20+ Rust extensions, plus every must-have pieces to [Self-hosting Supabase](https://doc.pgsty.com/app/supabase) —
Pig’s unified CLI makes them all effortlessly accessible. It cuts out messy source builds and half-baked repos,
offering version-aligned RPM / DEB packages that work seamlessly across Debian, Ubuntu, and RedHat flavors Linuxl, as well as x86 & ARM arch. No guesswork, no drama.

Instead of reinventing the wheel, Pig piggyback your system’s native package manager (APT, YUM, DNF) and follow official PGDG packaging conventions to ensure a glitch-free fit.
That means you don’t have to choose between “the right way” and “the quick way”; Pig respects your existing repos, aligns with standard OS best practices, and fits neatly alongside other packages you already use.

Ready to give your Postgres superpowers without the usual hassle? Check out [GitHub](https://github.com/pgsty/pig) for documentation,
installation steps, and a peek at its massive [extension list](https://ext.pgsty.com/list/). Then, watch your local Postgres instance transform into a powerhouse of specialized modules.
If [the future of Postgres is unstoppable extensibility](https://medium.com/@fengruohang/postgres-is-eating-the-database-world-157c204dcfc4),
Pig is the genie that helps you unlock it. Honestly, nobody ever complained that they had *too many* extensions.

![](/img/pigsty/ecosystem.gif)


## Linux Compatibility

| Linux Distro           | Ver |  Arch   |                                            OS Code                                            |  PostgreSQL Major  |
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


## Featured

![](/featured.jpg)
# pig - CLI tools for PostgreSQL & Pigsty

> Manage PostgreSQL extensions and clusters with ease

--------

## Why Pig?

The `pig` is a CLI tool for managing PostgreSQL extensions. 

It can install 340 PostgreSQL extensions with a single command for your


```bash
pig ext  list                       # list all available extensions
pig ext  info    pg_duckdb          # show detailed information of pg_duckdb
pig ext  install pg_duckdb          # install pg_duckdb extension
pig ext  remove  pg_duckdb          # remove pg_duckdb extension
pig ext  update  pg_duckdb          # update extension to the latest version

pig repo add     pgsql              # add pgsql repo (PGDG + PIGSTY) to your node
pig repo set     infra,pgsql,node   # overwrite default repos with infra, pgsql, node
pig repo rm      
pig repo         info | add  | rm | set | build | cache | create
```








--------

## Get Started

```bash

```


--------

## Installation



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
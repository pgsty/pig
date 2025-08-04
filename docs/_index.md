---
title: PIG CLI
description: The Missing Package Manager for PostgreSQL Extensions
icon: PiggyBank
full: true
cascade:
  type: docs
breadcrumbs: false
comments: false
---


—— ***Postgres Install Genius, the missing extension package manager for PostgreSQL ecosystem***

{{< cards >}}
{{< card link="/intro"   title="Introduction" subtitle="Why we need a package manager" icon="sparkles" >}}
{{< card link="/start"   title="Get Started"  subtitle="Tutorial and examples"         icon="play" >}}
{{< card link="/install" title="Installation" subtitle="Install in different ways" icon="save" >}}
{{< /cards >}}



## Quick Start

[Install](/install) `pig` with a single command

```bash tab="Global"
curl -fsSL https://repo.pigsty.io/pig | bash
```


Then it’s ready to use, assume you want to install the [**`pg_duckdb`**](/e/pg_duckdb/) extension:

```bash
$ pig repo add pigsty pgdg -u  # add pgdg & pigsty repo, then update repo cache
$ pig ext install pg17         # install PostgreSQL 17 kernels with native PGDG packages
$ pig ext install pg_duckdb    # install the pg_duckdb extension (for current pg17)
```


## CLI Usage

Check sub-commands [documentation](/cmd) with `pig help <command>`

{{< cards cols="5"  >}}
{{< card link="/cmd/repo"  title="pig repo"  subtitle="Manage software repositories" icon="library" >}}
{{< card link="/cmd/ext"   title="pig ext"   subtitle="Manage postgres extensions"   icon="cube" >}}
{{< card link="/cmd/build" title="pig build" subtitle="Build extension from source"  icon="view-grid" >}}
{{< card link="/cmd/sty"   title="pig sty"   subtitle="Manage pigsty installation"   icon="cloud-download" >}}
{{< /cards >}}



## Source

The `pig` CLI is developed by [Vonng](https://blog.vonng.com/en/), and open-sourced under the [Apache License 2.0](https://github.com/pgsty/pig/?tab=Apache-2.0-1-ov-file#readme).

You can also check the [pigsty](https://pgsty.com) project, which makes it even smoother to deliver all these [extensions](/usage) in an IaC way:

- https://github.com/pgsty/pigsty
- https://github.com/pgsty/pig

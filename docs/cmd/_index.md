---
title: "CLI Usage"
description: Overview of the pig cli tool
icon: SquareTerminal
weight: 600
---

## Overview

```bash
pig - the Linux Package Manager for PostgreSQL and CLI tool for Pigsty

Usage:
  pig [command]

Examples:

  # get started: check https://github.com/pgsty/pig for details
  pig repo add -ru        # overwrite existing repo & update cache
  pig ext  add pg17       # install optional postgresql 17 package
  pig ext  add pg_duckdb  # install certain postgresql extension

  pig repo : add rm update list info status create boot cache
  pig ext  : add rm update list info status import link build
  pig sty  : init boot conf install get list


PostgreSQL Extension Manager
  ext         Manage PostgreSQL Extensions (pgext)
  repo        Manage Linux Software Repo (apt/dnf)

Pigsty Management Commands
  sty         Manage Pigsty Installation

Additional Commands:
  build       Build Postgres Extension
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  status      Show Environment Status
  update      Upgrade pig itself
  version     Show pig version info

Flags:
      --debug              enable debug mode
  -h, --help               help for pig
  -i, --inventory string   config inventory path
      --log-level string   log level: debug, info, warn, error, fatal, panic (default "info")
      --log-path string    log file path, terminal by default
  -t, --toggle             Help message for toggle

Use "pig [command] --help" for more information about a command.
```

------

## Examples

### Environment Status

```bash
pig status                    # show os & pg & pig status
pig repo status               # show upstream repo status
pig ext  status               # show pg extensions status 
```

### Extension Management

Check [`pig ext`](/cmd/ext/) for details.

```bash
pig ext list    [query]       # list & search extension      
pig ext info    [ext...]      # get information of a specific extension
pig ext status  [-v]          # show installed extension and pg status
pig ext add     [ext...]      # install extension for current pg version
pig ext rm      [ext...]      # remove extension for current pg version
pig ext update  [ext...]      # update extension to the latest version
pig ext import  [ext...]      # download extension to local repo
pig ext link    [ext...]      # link postgres installation to path
pig ext upgrade               # fetch the latest extension catalog
```

### Repo Management

Check [`pig repo`](/cmd/repo/) for details.

```bash
pig repo list                    # available repo list
pig repo info   [repo|module...] # show repo info
pig repo status                  # show current repo status
pig repo add    [repo|module...] # add repo and modules
pig repo rm     [repo|module...] # remove repo & modules
pig repo update                  # update repo pkg cache
pig repo create                  # create repo on current system
pig repo boot                    # boot repo from offline package
pig repo cache                   # cache repo as offline package
```

### Pigsty Management

Check [`pig sty`](/cmd/sty/) for details.

The pig can also be used as a cli tool for Pigsty — the battery-include free PostgreSQL RDS.
Which brings HA, PITR, Monitoring, IaC, and all the extensions to your PostgreSQL cluster.

```bash
pig sty init     # install pigsty to ~/pigsty 
pig sty boot     # install ansible and other pre-deps 
pig sty conf     # auto-generate pigsty.yml config file
pig sty install  # run the install.yml playbook
```

You can use the `pig sty` subcommand to bootstrap pigsty on current node.
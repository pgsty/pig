---
title: "CMD: repo"
description: How to manage repositories with pig repo subcommand?
icon: SquareTerminal
weight: 610
---

The `pig repo` command is a comprehensive tool for managing package repositories.
It provides functionality to manage, add, remove, create and interact with os software repos. It works on both RPM-based (EL) and Debian-based systems.

------

## Overview

```bash
pig repo - Manage Linux APT/YUM Repo

  pig repo list                    # available repo list             (info)
  pig repo info   [repo|module...] # show repo info                  (info)
  pig repo status                  # show current repo status        (info)
  pig repo add    [repo|module...] # add repo and modules            (root)
  pig repo rm     [repo|module...] # remove repo & modules           (root)
  pig repo update                  # update repo pkg cache           (root)
  pig repo create                  # create repo on current system   (root)
  pig repo boot                    # boot repo from offline package  (root)
  pig repo cache                   # cache repo as offline package   (root)

Usage:
  pig repo [command]

Aliases:
  repo, r

Examples:

  Get Started: https://pigsty.io/ext/pig/
  pig repo add -ru                 # add all repo and update cache (brute but effective)
  pig repo add pigsty -u           # gentle version, only add pigsty repo and update cache
  pig repo add node pgdg pigsty    # essential repo to install postgres packages
  pig repo add all                 # all = node + pgdg + pigsty
  pig repo add all extra           # extra module has non-free and some 3rd repo for certain extensions
  pig repo update                  # update repo cache
  pig repo create                  # update local repo /www/pigsty meta
  pig repo boot                    # extract /tmp/pkg.tgz to /www/pigsty
  pig repo cache                   # cache /www/pigsty into /tmp/pkg.tgz


Available Commands:
  add         add new repository
  boot        bootstrap repo from offline package
  cache       create offline package from local repo
  create      create local YUM/APT repository
  info        get repo detailed information
  list        print available repo list
  rm          remove repository
  set         wipe and overwrite repository
  status      show current repo status
  update      update repo cache

Flags:
  -h, --help   help for repo

Global Flags:
      --debug              enable debug mode
  -i, --inventory string   config inventory path
      --log-level string   log level: debug, info, warn, error, fatal, panic (default "info")
      --log-path string    log file path, terminal by default
```

------

## Examples

List available repo and add PGDG & Pigsty repo, then update local repo cache.

```bash
# list available modules
pig repo list

# add PGDG & Pigsty repo
pig repo add pgdg pigsty

# yum makecache or apt update
pig repo update
```

You’ll have to update repo metadata cache after adding new repo, you can either use the dedicate `pig repo update` command or use the `-u|--update` flag in `pig repo add` command.

```bash
pig repo add pigsty -u    # add pigsty repo and update repo cache
```

If you wish to **WIPE** all the existing repo before adding new repo, you can use the extra `-r|--remove` flag, or use the dedicate `pig repo set` subcommand instead of `pig repo add`.

```bash
pig repo add all --remove # REMOVE all existing repo and add node, pgdg, pigsty repo and update repo cache
pig repo add -r           # same as above, and missing repo/module will use the default `all` alias to add node, pgdg, pigsty repo
pig repo set              # same as above, set is a shortcut for `add --remove`, REMOVED repo files are backupped to `/etc/yum.repos.d/backup` or `/etc/apt/sources.list.d/backup`
```

The most brutal but reliable way to setup repo for PostgreSQL installation is to wipe all existing repo and add all the required repo with:

```bash
pig repo set -u           # wipe all existing repo and add all the required repo and update repo cache
```

------

## Modules

In pigsty, all repos are organized into modules, a module is a collection of repos.

Module names may maps to different real repos on different OS distro, major version and architecture, and geo region.

Pigsty will handle all the details, you can list all repos & modules with `pig repo list`.

```yaml
repo_modules:   # Available Modules: 19
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

Usually these 3 modules are required to install PostgreSQL & all the extensions:

- `pgdg`: Official PostgreSQL Repo, with PG kernel packages, utils, and 100+ extensions.
- `pigsty`: Pigsty Extension Repo, with 200+ extra extensions and utils.
- `node`: Operating System Default Repo, which brings all the libraries and dependencies for PostgreSQL.

There’s a convient pesudo module alias `all` which includes all the 3 essential modules above. You can add all of them with `pig repo add all`, or a even simpler abbreviation: `pig repo add`.

------

## `repo list`

Lists available repository modules and repositories that can be added to the current system.

```
print available repo list

Usage:
  pig repo list [flags]

Aliases:
  list, l, ls

Examples:

  pig repo list                # list available repos on current system
  pig repo list all            # list all unfiltered repo raw data


Flags:
  -h, --help   help for list
```

Available repos are defined in the [**`cli/repo/assets/repo.yml`**](https://github.com/pgsty/pig/blob/main/cli/repo/assets/repo.yml), if you wish to modify the repo list, you can do so by adding your own `repo.yml` file to `~/.pig/repo.yml`.

------

## `repo add`

Add repository configuration files to the system.

```bash
add new repository

Usage:
  pig repo add [flags]

Aliases:
  add, a, append

Examples:

  pig repo add                      # = pig repo add all
  pig repo add all                  # add node,pgsql,infra repo (recommended)
  pig repo add all -u               # add above repo and update repo cache (or: --update)
  pig repo add all -r               # add all repo, remove old repos       (or: --remove)
  pig repo add pigsty --update      # add pigsty extension repo and update repo cache
  pig repo add pgdg --update        # add pgdg official repo and update repo cache
  pig repo add pgsql node --remove  # add os + postgres repo, remove old repos
  pig repo add infra                # add observability, grafana & prometheus stack, pg bin utils

  (Beware that system repo management require sudo / root privilege)

  available repo modules:
  - all      :  pgsql + node + infra (recommended)
    - pigsty :  PostgreSQL Extension Repo (default)
    - pgdg   :  PGDG the Official PostgreSQL Repo (official)
    - node   :  operating system official repo (el/debian/ubuntu)
  - pgsql    :  pigsty + pgdg (all available pg extensions)
  # check available repo & modules with pig repo list

Flags:
  -h, --help            help for add
      --region string   region code (default|china)
  -r, --remove          remove existing repo before adding new repo
  -u, --update          run apt update or dnf makecache
```

This command:

1. Verifies if specified modules exist and translate to real repos according to

- region, distro, os major version, arch

1. If `-r|--remove` flag is provided, it will move the existing repo to backup folder:

- `/etc/yum.repos.d/backup` for EL systems
- `/etc/apt/sources.list.d/backup` for Debian systems

1. Creates repo files in the system’s repository directory

- `/etc/yum.repos.d/<module>.repo` for EL systems
- `/etc/apt/sources.list.d/<module>.list` for Debian systems

1. If `-u|--update` flag is provided, it will run `apt update` or `dnf makecache` to update the repo cache.

If not running as `root`, sudo privilege is required.

------

## `repo set`

Same as `repo add <...> --remove`, remove existing repo before adding new repo.

```bash
wipe and overwrite repository

Usage:
  pig repo set [flags]

Aliases:
  set, overwrite

Examples:

  pig repo set all                  # set repo to node,pgsql,infra  (recommended)
  pig repo set all -u               # set repo to above repo and update repo cache (or --update)
  pig repo set pigsty --update      # set repo to pigsty extension repo and update repo cache
  pig repo set pgdg   --update      # set repo to pgdg official repo and update repo cache
  pig repo set infra                # set repo to observability, grafana & prometheus stack, pg bin utils

  (Beware that system repo management require sudo/root privilege)


Flags:
  -h, --help            help for set
      --region string   region code
  -u, --update          run apt update or dnf makecache
```

If not running as `root`, sudo privilege is required.

------

## `repo update`

Update repo cache, same as `apt update` or `yum makecache`.

```bash
update repo cache

Usage:
  pig repo update [flags]

Aliases:
  update, u

Examples:

  pig repo update                  # yum makecache or apt update


Flags:
  -h, --help   help for update
```

If not running as `root`, sudo privilege is required.

------

## `repo rm`

Removes repository files from the system.

```bash
remove repository

Usage:
  pig repo rm [flags]

Aliases:
  rm, remove

Examples:

  pig repo rm                      # remove (backup) all existing repo to backup dir
  pig repo rm all --update         # remove module 'all' and update repo cache
  pig repo rm node pigsty -u       # remove module 'node' & 'pigsty' and update repo cache


Flags:
  -h, --help     help for rm
  -u, --update   run apt update or dnf makecache
```

It will remove the repo files from the system, and if `-u|--update` flag is provided, it will run `apt update` or `dnf makecache` to update the repo cache after removing the repo files.

Before removing files, the command creates a backup of existing repository configurations.

If not running as `root`, sudo privilege is usually required.

------

## `repo status`

Print the system repo directory and list available repos with system package manager.

```bash
show current repo status

Usage:
  pig repo status [flags]

Aliases:
  status, s, st

Flags:
  -h, --help   help for status
```

------

## `repo info`

Provides detailed information about specific repositories or modules.

```bash
get repo detailed information

Usage:
  pig repo info [flags]

Aliases:
  info, i

Flags:
  -h, --help   help for info
```

**Example:**

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

# default repo content
# pgdg PGDG
deb [trusted=yes] http://apt.postgresql.org/pub/repos/apt/ bookworm-pgdg main

# china mirror repo content
# pgdg PGDG
deb [trusted=yes] https://mirrors.tuna.tsinghua.edu.cn/postgresql/repos/apt/ bookworm-pgdg main
```

It will print the repo information for the given repo name or module name. And regional mirrors are also supported.

------

## `repo create`

Create a local YUM/APT repository in specified directories

```bash
create local YUM/APT repository

Usage:
  pig repo create [path...]

Aliases:
  create, cr

Examples:

  pig repo create                    # create repo on /www/pigsty by default
  pig repo create /www/mssql /www/b  # create repo on multiple locations

  (Beware that system repo management require sudo/root privilege)
```

**Default directory:** `/www/pigsty`

This command:

1. Create the directory structure if it doesn’t exist
2. Create local repo with repo utils (make sure they are installed on the system)

- `createrepo_c` for EL systems
- `dpkg-dev` for Debian systems

If not running as `root`, read/write permission on that directory is required.

------

## `repo cache`

Creates a compressed tarball of repository contents for offline use.

```bash
pig repo cache [directory_path] [package_path] [repo1,repo2,...]
```

**Parameters:**

- `directory_path`: Source directory containing repositories (default: `/www`)
- `package_path`: Output tarball path (default: `pigsty-pkg-<os>-<arch>.tgz` in current directory)
- `repos`: Comma-separated list of repository subdirectories to include (default: all)

**Example:**

```bash
pig repo cache /www /tmp/pkg.tgz pigsty
pig repo cache /www /tmp/pkg.tgz pigsty mssql ivory
```

You can create a tarball on created local repo, and use it to boot a new system from offline package.

------

## `repo cache`

create offline package from local repo

```bash
create offline package from local repo

Usage:
  pig repo cache [flags]

Aliases:
  cache, c

Examples:

  pig repo cache                    # create /tmp/pkg.tgz offline package from /www/pigsty
  pig repo cache -f                 # force overwrite existing package
  pig repo cache -d /srv            # overwrite default content dir /www to /srv
  pig repo cache pigsty mssql       # create the tarball with both pigsty & mssql repo
  pig repo c -f                     # the simplest use case to make offline package

  (Beware that system repo management require sudo/root privilege)


Flags:
  -d, --dir string    source repo path (default "/www/")
  -h, --help          help for cache
  -p, --path string   offline package path (default "/tmp/pkg.tgz")
```

------

## `repo boot`

Bootstraps a local repository from an offline package.

```bash
bootstrap repo from offline package

Usage:
  pig repo boot [flags]

Aliases:
  boot, b, bt

Examples:

  pig repo boot                    # boot repo from /tmp/pkg.tgz to /www
  pig repo boot -p /tmp/pkg.tgz    # boot repo from given package path
  pig repo boot -d /srv            # boot repo to another directory /srv


Flags:
  -d, --dir string    target repo path (default "/www/")
  -h, --help          help for boot
  -p, --path string   offline package path (default "/tmp/pkg.tgz")
```

**Parameters:**

- `offline_package`: Path to the tarball created by `pig repo cache`
- `target_directory`: Directory to extract repositories to (default: `/www`)

**Example:**

```bash
pig repo boot /tmp/pkg.tgz /www
```

This command:

1. Extract the tarball to the target directory
2. Set up local repository configuration
3. Updates repository metadata
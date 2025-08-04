---
title: "CMD: sty"
description: How to manage pigsty with pig sty subcommand
icon: SquareTerminal
weight: 630
---


The **pig** can also be used as a cli tool for Pigsty — the battery-include free PostgreSQL RDS.
Which brings HA, PITR, Monitoring, IaC, and all the extensions to your PostgreSQL cluster.

```bash
pig sty -Init (Download), Bootstrap, Configure, and Install Pigsty

  pig sty init    [-pfvd]      # install pigsty (~/pigsty by default)
  pig sty boot    [-rpk]       # install ansible and prepare offline pkg
  pig sty conf    [-civrsxn]   # configure pigsty and generate config
  pig sty install              # use pigsty to install & provisioning env (DANGEROUS!)
  pig sty get                  # download pigsty source tarball
  pig sty list                 # list available pigsty versions

Usage:
  pig sty [command]

Aliases:
  sty, s, pigsty

Examples:
  Get Started: https://pgsty.com/docs/install
  pig sty init                 # extract and init ~/pigsty
  pig sty boot                 # install ansible & other deps
  pig sty conf                 # generate pigsty.yml config file
  pig sty install              # run pigsty/install.yml playbook

Available Commands:
  boot        Bootstrap Pigsty
  conf        Configure Pigsty
  get         download pigsty available versions
  init        Install Pigsty
  install     run pigsty install.yml playbook
  list        list pigsty available versions

Flags:
  -h, --help   help for sty
```

You can use the `pig sty` subcommand to bootstrap pigsty on current node.



------

## `sty init`

```bash
pig sty init
  -p | --path    : where to install, ~/pigsty by default
  -f | --force   : force overwrite existing pigsty dir
  -v | --version : pigsty version, embedded by default
  -d | --dir     : download directory, /tmp by default

Usage:
  pig sty init [flags]

Aliases:
  init, i

Examples:

  pig sty init                   # install to ~/pigsty with embedded version
  pig sty init -f                # install and OVERWRITE existing pigsty dir
  pig sty init -p /tmp/pigsty    # install to another location /tmp/pigsty
  pig sty init -v 3.3            # get & install specific version v3.3.0
  pig sty init 3                 # get & install specific version v3 latest


Flags:
  -d, --dir string       pigsty download directory (default "/tmp")
  -f, --force            overwrite existing pigsty (false by default)
  -h, --help             help for init
  -p, --path string      target directory (default "~/pigsty")
  -v, --version string   pigsty version string
```

------

## `sty boot`

```bash
pig sty boot
  [-r|--region <region]   [default,china,europe]
  [-p|--path <path>]      specify another offline pkg path
  [-k|--keep]             keep existing upstream repo during bootstrap

Check https://pigsty.io/docs/setup/offline/#bootstrap for details

Usage:
  pig sty boot [flags]

Aliases:
  boot, b, bootstrap

Flags:
  -h, --help            help for boot
  -k, --keep            keep existing repo
  -p, --path string     offline package path
  -r, --region string   default,china,europe,...
```

------

## `sty conf`

```bash
Configure pigsty with ./configure

pig sty conf
  [-c|--conf <name>       # [meta|dual|trio|full|prod]
  [--ip <ip>]             # primary IP address (skip with -s)
  [-v|--version <pgver>   # [17|16|15|14|13]
  [-r|--region <region>   # [default|china|europe]
  [-s|--skip]             # skip IP address probing
  [-x|--proxy]            # write proxy env from environment
  [-n|--non-interactive]  # non-interactively mode

Check https://pigsty.io/docs/setup/install/#configure for details

Usage:
  pig sty conf [flags]

Aliases:
  conf, c, configure

Examples:

  pig sty conf                       # use the default conf/meta.yml config
  pig sty conf -c rich -x            # use the rich.yml template, add your proxy env to config
  pig sty conf -c supa --ip=10.9.8.7 # use the supa template with 10.9.8.7 as primary IP
  pig sty conf -c full -v 16         # use the 4-node full template with pg16 as default
  pig sty conf -c oss -s             # use the oss template, skip IP probing and replacement
  pig sty conf -c slim -s -r china   # use the 2-node slim template, designate china as region


Flags:
  -c, --conf string       config template name
  -h, --help              help for conf
      --ip string         primary ip address
  -n, --non-interactive   configure non-interactive
  -p, --proxy             configure proxy env
  -r, --region string     upstream repo region
  -s, --skip              skip ip probe
  -v, --version string    postgres major version
```

------

## `sty install`

```bash
run pigsty install.yml playbook

Usage:
  pig sty install [flags]

Aliases:
  install, ins, install

Flags:
  -h, --help   help for install
```

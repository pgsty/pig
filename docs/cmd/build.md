---
title: "CMD: build"
description: How to setup building infrastructure with pig build subcommand?
icon: SquareTerminal
weight: 640
---


`pig build` is a powerful command-line tool that simplifies the entire workflow of building and managing PostgreSQL extensions - from setting up the build environment to compiling extensions across different operating systems.

------

## Overview

```
Build Postgres Extension

Usage:
  pig build [command]

Aliases:
  build, b

Examples:
pig build - Build Postgres Extension

  pig build repo                   # init build repo (=repo set -ru)
  pig build tool  [mini|full|...]  # init build toolset
  pig build proxy [id@host:port ]  # init build proxy (optional)
  pig build rust  [-v <pgrx_ver>]  # init rustc & pgrx (0.12.9)
  pig build spec                   # init build spec repo
  pig build get   [all|std|..]     # get ext code tarball with prefixes
  pig build ext   [extname...]     # build extension


Available Commands:
  ext         Build extension
  get         Download source code tarball
  proxy       Initialize build proxy
  repo        Initialize required repos
  rust        Initialize rust and pgrx environment
  spec        Initialize building spec repo
  tool        Initialize build tools

Flags:
  -h, --help   help for build

Global Flags:
      --debug              enable debug mode
  -i, --inventory string   config inventory path
      --log-level string   log level: debug, info, warn, error, fatal, panic (default "info")
      --log-path string    log file path, terminal by default
```

------

## Examples

Setting up the build environment

```bash
# Initialize repositories and tools
pig build repo
pig build tool
pig build spec

# For Rust-based extensions
pig build rust

# Download standard extensions
pig build get std                # download all tarballs
pig build get citus timescaledb  # download specific tarballs

# Build specific extensions
pig build ext citus
```

------

## Build Workflow

A typical workflow for building PostgreSQL extensions with `pig build`:

1. Set up repositories: `pig build repo`
2. Install build tools: `pig build tool`
3. (Optional) Set up proxy: `pig build proxy id@host:port`
4. (Optional, for Rust extensions) Set up Rust: `pig build rust`
5. Initialize build specs: `pig build spec`
6. Download source code: `pig build get [prefixes]`
7. Build extensions: `pig build ext [extname...]`

### Notes

- The build process differs between EL and DEB-based distributions
- Some commands may require sudo privileges to install system packages
- For Rust-based extensions, set up the Rust environment first
- Proxy setup is optional and only needed in environments with restricted internet access

### OS Support

The `pig build` command supports:

- [**EL distributions**](/repo/): RHEL, Rocky, CentOS (tested on versions 8 and 9)
- [**DEB distributions**](/repo/): Debian (tested on versions 12), Ubuntu (tested on versions 22.04 and 24.04)
- May have macOS support via homebrew in the future

------

## `build repo`

**Aliases**: `r`

Adding required upstream repo for building PostgreSQL extensions by running the repository add command with update and remove flags enabled.

```bash
pig build repo
```

------

## `build tool`

**Aliases**: `t`

Installs the build tools required for compiling PostgreSQL extensions.

```bash
pig build tool
```

**Parameters**:

- ```
mode
  ```

: The installation mode (default: “mini”)

- Available modes depend on the operating system and distribution

This command installs necessary build dependencies including:

**For EL distributions** (RHEL, Rocky, CentOS):

```
make, cmake, ninja-build, pkg-config, lld, git, lz4, unzip, ncdu, rsync, vray,
rpmdevtools, dnf-utils, pgdg-srpm-macros, postgresql1*-devel, postgresql1*-server, jq,
readline-devel, zlib-devel, libxml2-devel, lz4-devel, libzstd-devel, krb5-devel,
```

**For DEB distributions** (Debian, Ubuntu):

```fallback
make, cmake, ninja-build, pkg-config, lld, git, lz4, unzip, ncdu, rsync, vray,
debhelper, devscripts, fakeroot, postgresql-all, postgresql-server-dev-all, jq,
libreadline-dev, zlib1g-dev, libxml2-dev, liblz4-dev, libzstd-dev, libkrb5-dev,
```

------

## `build rust`

**Aliases**: `rs`

Set up the Rust environment and PGRX for building PostgreSQL extensions in Rust.

```bash
pig build rust [-v <pgrx_ver>]
```

**Parameters**:

- `-v <pgrx_ver>`: The PGRX version to install (default: “`0.12.9`”)

This command:

1. Install Rust using rustup if not already installed
2. Install the specified version of cargo-pgrx
3. Initializes PGRX with appropriate PostgreSQL configurations (versions 13-17)

------

## `build spec`

**Aliases**: `s`

Initializes the build specification repository based on the operating system:

```bash
pig build spec
```

- **For EL distributions** (RHEL, Rocky, CentOS):
1. Clones the [RPM Spec Repo](https://github.com/pgsty/rpm) from GitHub
2. Sets up the RPM build fhs on `~/rpmbuild`
- **For DEB distributions** (Debian, Ubuntu):
1. Clones the [DEB Spec Repo](https://github.com/pgsty/deb) from GitHub
2. Setup building directory fhs on `~/deb`

The repo contains building specs (rpmspecs or debian control files) for PIGSTY maintained extensions.

- RPM: https://github.com/pgsty/rpm
- DEB: https://github.com/pgsty/deb

------

## `build get`

**Aliases**: `get`

Download source code tarballs for PostgreSQL extensions.

```bash
pig build get [prefixes|all|std]
```

**Parameters**:

- `std`: Download standard packages (excluding large ones, default behavior)
- `all`: Download all available source code tarballs (for batch building)
- `[prefixes]`: One or more prefixes to filter the packages to download

Downloaded files are stored in:

- EL: `~/rpmbuild/SOURCES/`
- DEB: `~/deb/tarball/`

Example:

```bash
pig build get std                # get standard packages (except for pg_duckdb/pg_mooncake/omnigres/plv8, too large)
pig build get all                # get all available source code tarballs

pig build get pg_mooncake        # get pg_mooncake source code
pig build get pg_duckdb          # get pg_duckdb source code
pig build get omnigres           # get omnigres source code
pig build get plv8               # get plv8 source code

pig build get citus              # get citus source code
pig build get timescaledb        # get timescaledb source code
pig build get hydra              # get hydra source code
pig build get pgjwt              # get pgjwt source code
....
```

------

## `build ext`

**Aliases**: `e`

Builds PostgreSQL extensions using the appropriate build environment for the current operating system.

```bash
pig build ext [extname...]
```

**Parameters**:

- `extname`: One or more extension names to build

For each extension, the command:

1. Changes to the appropriate build directory (`~/rpmbuild/BUILD` or `~/deb/build`)
2. Runs the make command with the extension name (e.g. `make citus`)
3. Reports success or failure for each extension build

It actually leverages the `make <ext>` command in the spec repo to build the extension.

------

## `build proxy`

**Aliases**: `p`

Setup a proxy server for accessing external resources, useful in environments with restricted internet access.

This is purely **optional**, and you don't need to it if you don’t have any connection problems.

```bash
pig build proxy [id@host:port] [local]
```

**Parameters**:

- `id@host:port`: The remote `v2ray` proxy specification in the format `user-id@host:port`
- `local`: The local address for `v2ray` to bind to (default: “127.0.0.1:12345”)

This command:

1. Installs proxy software if not already present (`/usr/local/bin/v2ray`)
2. Configure the proxy with specified remote and local settings
3. Creates environment setup scripts in `/etc/profile.d/proxy.sh`
4. Configure the proxy service
5. Test the proxy connectivity via curl google

You can load the proxy environment variables by running:

```bash
. /etc/profile.d/proxy.sh
```

After setup, you can use these following aliases:

- `po`: Enable proxy (set proxy environment variables in your current shell session)
- `px`: Disable proxy (unset proxy environment variables)
- `pck`: Check proxy status (via ping google)
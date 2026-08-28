# PIG

[![Website](https://img.shields.io/badge/Website-pig.pgsty.com-slategray?style=flat)](https://pig.pgsty.com/)
[![Documentation](https://img.shields.io/badge/Docs-PIG-slategray?style=flat)](https://pig.pgsty.com/docs/)
[![Release](https://img.shields.io/github/v/release/pgsty/pig?style=flat&logo=github)](https://github.com/pgsty/pig/releases)
[![License](https://img.shields.io/github/license/pgsty/pig?style=flat)](LICENSE)

**PIG** is a self-contained PostgreSQL package manager and operations CLI. It resolves PostgreSQL
kernels and extensions to native APT/DNF packages, manages package repositories, builds extension
packages, and provides selected PostgreSQL and Pigsty workflows.

- Website: <https://pig.pgsty.com/>
- Documentation: <https://pig.pgsty.com/docs/>
- Design records: <https://pig.pgsty.com/design/>
- Release notes: <https://pig.pgsty.com/release/>
- Extension catalog: <https://pigsty.io/ext/>

The website is the canonical documentation source. This repository keeps source code, tests,
packaging metadata, and only the short project entry point you are reading now.

## Install

On a supported RPM or DEB Linux distribution:

```bash
curl -fsSL https://repo.pigsty.io/pig | bash
```

For mainland China:

```bash
curl -fsSL https://repo.pigsty.cc/pig | bash
```

Release RPM, DEB, Linux, and macOS archives are available from
[GitHub Releases](https://github.com/pgsty/pig/releases). See the current
[installation guide](https://pig.pgsty.com/install/) for package names, checksums, upgrades, and
removal.

## Quick start

```bash
pig repo set                        # configure PostgreSQL package repositories
pig install pg18                    # install PostgreSQL 18 packages
pig ext list duck                   # search the extension catalog
pig install pg_duckdb vector        # install extension packages
pig status                          # inspect the current host
```

PIG installs host packages. Extension-specific preload, restart, `CREATE EXTENSION`, and SQL
upgrade steps remain the operator's responsibility. Follow the extension's own documentation and
the [PIG getting-started guide](https://pig.pgsty.com/start/).

## Command families

| Command | Purpose | Reference |
|:---|:---|:---|
| `pig repo` | Configure and inspect APT/DNF repositories | [repo](https://pig.pgsty.com/repo/) |
| `pig ext` | Search and manage PostgreSQL extension packages | [ext](https://pig.pgsty.com/ext/) |
| `pig build` | Build PostgreSQL extensions from source | [build](https://pig.pgsty.com/build/) |
| `pig install` | Install translated aliases or native package names | [commands](https://pig.pgsty.com/cmd/#pig-install) |
| `pig sty` | Initialize and operate a Pigsty controller | [sty](https://pig.pgsty.com/sty/) |
| `pig inventory` | Inspect, edit, validate, and exchange Pigsty Inventory | [inventory](https://pig.pgsty.com/inventory/) |
| `pig do` | Run bounded Pigsty administrative playbooks | [do](https://pig.pgsty.com/do/) |
| `pig pg` | Operate a local PostgreSQL instance | [pg](https://pig.pgsty.com/pg/) |
| `pig pt` | Run Patronictl transparently with local helpers | [pt](https://pig.pgsty.com/pt/) |
| `pig pe` | Inspect and reload pg_exporter | [pe](https://pig.pgsty.com/pe/) |
| `pig pb` | Run pgBackRest backup and restore primitives | [pb](https://pig.pgsty.com/pb/) |
| `pig pitr` | Run the orchestrated PITR workflow | [pitr](https://pig.pgsty.com/pitr/) |

Run `pig help COMMAND` for the command's embedded help. Use the linked website pages for the
maintained bilingual reference and the [Design Records](https://pig.pgsty.com/design/) for the
reasoning behind major contracts.

## Development

PIG is written in Go. Read [AGENTS.md](AGENTS.md) before changing the Cobra command layer.

```bash
go test ./...
go vet ./...
make build
make docs-check                    # validate the sibling pig.pgsty.com checkout
```

Keep Cobra entry points in `cmd/`, concrete command behavior in `cli/*`, and shared foundations in
`internal/*`. Do not add a local `docs/` tree: current documentation and design history belong in
the dedicated [`pig.pgsty.com`](https://github.com/pgsty/pig.pgsty.com) repository. Override
`DOCS_DIR` when that checkout is not at `../pig.pgsty.com`.

## License

Copyright 2018-2026 Ruohang Feng. Licensed under the [Apache License 2.0](LICENSE).

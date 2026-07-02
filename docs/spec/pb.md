---
title: "pig pgbackrest"
description: "Manage pgBackRest backup and PITR with pig pgbackrest subcommand"
weight: 180
icon: fas fa-database
module: [PIG]
categories: [Reference]
---

The `pig pgbackrest` command (alias `pig pb`) manages pgBackRest backup and point-in-time recovery (PITR). It wraps common `pgbackrest` operations for simplified backup management. All commands execute as database superuser (default `postgres`).

```bash
pig pb - Manage pgBackRest backup & restore commands.

Usage: pig pb <command>

Info Commands:
  pig pb info                      show backup info
  pig pb ls                        list backups (alias for info)
  pig pb ls repo                   list configured repos
  pig pb ls stanza                 list all stanzas

Backup Commands (Primary Only):
  pig pb backup                    create backup (auto mode)
  pig pb backup full               full backup
  pig pb backup diff               differential backup
  pig pb backup incr               incremental backup

Restore Commands:
  pig pb restore -d                restore to latest (default)
  pig pb restore -I                restore to backup consistency point
  pig pb restore -t <time>         restore to specific time
  pig pb restore -n <name>         restore to named restore point
  pig pb restore -b <set>          restore from specific backup set

Stanza Management:
  pig pb create                    create stanza (first-time setup)
  pig pb upgrade                   upgrade stanza after PG major upgrade
  pig pb delete --force            delete stanza (dangerous!)

Control Commands:
  pig pb check                     verify backup repository
  pig pb start                     enable pgBackRest
  pig pb stop                      disable pgBackRest
  pig pb expire                    cleanup expired backups

Log Commands:
  pig pb log                       show latest log lines
  pig pb log -f                    tail -f latest log
  pig pb log list                  list log files
  pig pb log tail                  tail -f latest log
  pig pb log show                  show latest log snapshot
```

## Primitive Contract

`pig pb` is the pgBackRest primitive layer. It manages repository, stanza, backup, expire, and restore operations through pgBackRest only. `pb restore` restores files and recovery targets; it does not stop Patroni, stop PostgreSQL, start PostgreSQL, promote, rejoin HA, or verify cluster health. Use `pig pitr` for managed recovery orchestration.

High-risk pgBackRest actions expose a structured `state -> plan -> precheck -> execute -> verify -> result -> next_actions` contract. In JSON/YAML mode, destructive execution requires explicit `--yes` or `--force`; missing confirmation returns a structured parameter error with `next_actions` instead of prompting. Plan output is side-effect-free and may include `boundary`, `confirmation`, `preconditions`, `verifications`, and `next_actions` in addition to the existing plan fields.


## Command Overview

**Information Query**:

| Command | Description | Implementation |
|:---|:---|:---|
| `pb info` | Show backup repository info | `pgbackrest info` |
| `pb ls` | List backup sets | `pgbackrest info` |
| `pb ls repo` | List configured repos | Parse pgbackrest.conf |
| `pb ls stanza` | List all stanzas | Parse pgbackrest.conf |
{.full-width}

**Backup & Restore**:

| Command | Description | Implementation |
|:---|:---|:---|
| `pb backup` | Create backup | `pgbackrest backup` |
| `pb restore` | Restore from backup (PITR) | `pgbackrest restore` |
| `pb expire` | Clean up expired backups | `pgbackrest expire` |
{.full-width}

**Stanza Management**:

| Command | Description | Implementation |
|:---|:---|:---|
| `pb create` | Create stanza (first-time setup) | `pgbackrest stanza-create` |
| `pb upgrade` | Upgrade stanza (after PG major upgrade) | `pgbackrest stanza-upgrade` |
| `pb delete` | Delete stanza (dangerous!) | `pgbackrest stanza-delete` |
{.full-width}

**Control Commands**:

| Command | Alias | Description | Implementation |
|:--------|:------|:------------|:---------------|
| `pb check` | | Verify backup repository integrity | `pgbackrest check` |
| `pb start` | | Enable pgBackRest operations | `pgbackrest start` |
| `pb stop` | | Disable pgBackRest operations | `pgbackrest stop` |
| `pb log` | `l, lg` | View logs | `tail/cat` log files |
{.full-width}


## Quick Start

```bash
# View backup info
pig pb info                          # Show all backup info
pig pb info -o json                  # JSON format output
pig pb ls                            # List all backups
pig pb ls repo                       # List configured repos
pig pb ls stanza                     # List all stanzas

# Create backup (must run on primary)
pig pb backup                        # Auto mode: full if none, else incr
pig pb backup full                   # Full backup
pig pb backup diff                   # Differential backup
pig pb backup incr                   # Incremental backup

# Restore (PITR)
pig pb restore -d                    # Restore to latest (end of WAL)
pig pb restore -I                    # Restore to backup consistency point
pig pb restore -t "2025-01-01 12:00:00+08"  # Restore to specific time
pig pb restore -n savepoint          # Restore to named restore point

# Stanza management
pig pb create                        # Initialize stanza
pig pb upgrade                       # Upgrade stanza after PG major upgrade
pig pb check                         # Verify repository integrity

# Cleanup
pig pb expire                        # Clean up per retention policy
pig pb expire --plan                 # Preview cleanup plan
pig pb expire --set 20250101-* --yes # Delete a specific backup set
```


## Global Options

These options apply to all `pig pb` subcommands:

| Option | Short | Description |
|:---|:---|:---|
| `--stanza` | `-s` | pgBackRest stanza name (auto-detected) |
| `--config` | `-c` | Config file path |
| `--repo` | `-r` | Repository number (multi-repo scenario) |
| `--dbsu` | `-U` | Database superuser (default: `$PIG_DBSU` or `postgres`) |
{.full-width}

**Stanza Auto-Detection:**

If `-s` not specified, pig auto-detects stanza name from config file:

1. Read config file (default `/etc/pgbackrest/pgbackrest.conf`)
2. Find sections not starting with `[global*]`
3. Use first stanza found

If config has multiple stanzas, a warning is issued and first one is used. Explicitly specify `--stanza` in this case.

**Multi-Repo Support:**

pgBackRest supports multiple repositories (repo1, repo2, etc.). Use `-r` to specify target repo:

```bash
pig pb backup -r 1                   # Backup to repo1
pig pb backup -r 2                   # Backup to repo2
pig pb info -r 2                     # View repo2 backup info
```


## Information Commands

### pb info

Show detailed backup repository info including all backup sets and WAL archive status.

```bash
pig pb info                          # Show all backup info
pig pb info -o json                  # JSON format output
pig pb info --set 20250101-120000F   # Show specific backup set details
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--output` | `-o` | Output format: text, json |
| `--set` | | Show specific backup set details |
{.full-width}


### pb ls

List resources in backup repository.

```bash
pig pb ls                            # List all backups (default)
pig pb ls backup                     # List all backups (explicit)
pig pb ls repo                       # List configured repos
pig pb ls stanza                     # List all stanzas
pig pb ls cluster                    # Alias for stanza
```

**Types:**

| Type | Description | Data Source |
|:---|:---|:---|
| backup | List all backup sets (default) | pgbackrest info |
| repo | List configured repos | Parse pgbackrest.conf |
| stanza | List all stanzas | Parse pgbackrest.conf |
{.full-width}


## Backup Commands

### pb backup

Create physical backup. Backups can only run on primary instance.

```bash
pig pb backup                        # Auto mode
pig pb backup full                   # Full backup
pig pb backup diff                   # Differential backup
pig pb backup incr                   # Incremental backup
pig pb backup --force                # Skip primary role check
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--force` | `-f` | Skip primary role check |
{.full-width}

**Backup Types:**

| Type | Description |
|:---|:---|
| (empty) | Auto mode: full if no backup exists, else incremental |
| full | Full backup: backup all data |
| diff | Differential: changes since last full backup |
| incr | Incremental: changes since last any backup |
{.full-width}

**Primary Check:**

Before backup, command auto-checks if current instance is primary. If replica, command exits with error. Use `--force` to skip this check.


### pb expire

Clean up expired backups and WAL archives per retention policy.

```bash
pig pb expire                        # Clean up per policy
pig pb expire --set 20250101-*       # Delete specific backup set
pig pb expire --set 20250101-* --yes # Skip set-delete confirmation
pig pb expire --plan                 # Preview expire plan
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--set` | | Delete specific backup set |
| `--plan` | | Preview cleanup plan without deleting backups |
| `--yes` | `-y` | Skip confirmation when `--set` deletes a backup set |
{.full-width}

**Safety:** `pb expire --set` can delete a specific backup set. Text mode prompts for confirmation unless `--yes` is provided. JSON/YAML mode requires `--yes`; otherwise it returns a structured confirmation-required error with a `--plan` next action.

**Retention Policy:**

Configured in `pgbackrest.conf`:

```ini
[global]
repo1-retention-full=2               # Full backups to retain
repo1-retention-diff=4               # Differential backups to retain
repo1-retention-archive=2            # WAL archive retention policy
```


## Restore Commands

### pb restore

Restore from backup with point-in-time recovery (PITR) support.

```bash
# Recovery target (mutually exclusive, one is required)
pig pb restore -d                    # Restore to latest (explicit)
pig pb restore -I                    # Restore to backup consistency point
pig pb restore -t "2025-01-01 12:00:00+08"  # Restore to specific time
pig pb restore -t "2025-01-01"       # Restore to date (00:00:00 that day)
pig pb restore -t "12:00:00"         # Restore to time (today)
pig pb restore -n my-savepoint       # Restore to named restore point
pig pb restore -l "0/7C82CB8"        # Restore to LSN
pig pb restore -x 12345              # Restore to transaction ID

# Backup set selection (can combine with recovery target)
pig pb restore -b 20251225-120000F   # Restore from specific backup set

# Other options
pig pb restore -t "..." -X           # Exclusive mode (stop before target)
pig pb restore -t "..." -P           # Auto-promote after restore
pig pb restore -t "..." --target-action shutdown
pig pb restore -d -- --delta         # Raw pgBackRest args after --
pig pb restore -y                    # Skip confirmation prompt
```

**Recovery Target Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--default` | `-d` | Restore to end of WAL stream (latest data) |
| `--immediate` | `-I` | Restore to backup consistency point |
| `--time` | `-t` | Restore to specific timestamp |
| `--name` | `-n` | Restore to named restore point |
| `--lsn` | `-l` | Restore to specific LSN |
| `--xid` | `-x` | Restore to specific transaction ID |
{.full-width}

**Backup Set and Other Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--set` | `-b` | Restore from specific backup set (can combine with target) |
| `--data` | `-D` | Target data directory |
| `--exclusive` | `-X` | Exclusive mode: stop before target |
| `--promote` | `-P` | Auto-promote to primary after restore |
| `--target-action` | | Action at target: `pause`, `promote`, or `shutdown` |
| `--target-timeline` | `-T` | Timeline: `latest`, `current`, integer, or `0xHEX` |
| `--yes` | `-y` | Skip confirmation prompt |
{.full-width}

Raw pgBackRest restore arguments must be placed after `--`. Pig rejects passthrough arguments that override restore target or lifecycle flags, including `--type`, `--target`, `--target-action`, `--target-exclusive`, `--target-timeline`, `--set`, and `--pg1-path`; use Pig's first-class flags instead.

**Time Formats:**

Supports multiple time format inputs with strict validation and timezone auto-completion (including non-integer-hour zones like +05:30):

| Format | Example | Description |
|:---|:---|:---|
| Full format | `2025-01-01 12:00:00+08` | Complete timestamp with timezone |
| Date only | `2025-01-01` | Auto-completes to 00:00:00 that day (local timezone) |
| Time only | `12:00:00` | Auto-completes to today (local timezone) |
{.full-width}

**Restore Flow:**

1. Validate parameters and environment
2. Refuse managed `/pg/data` restore when Patroni is active; use `pig pitr`
3. Check PostgreSQL is stopped
4. Display restore plan and require typed `yes` confirmation
5. Execute pgbackrest restore
6. Provide post-restore guidance

**Important:** `pb restore` is a low-level primitive. Stop PostgreSQL before restore, and do not use it against Patroni-managed PGDATA while Patroni is active:

```bash
pig pg stop                          # Stop PostgreSQL
pig pb restore -t "..."              # Execute restore
pig pg start                         # Start PostgreSQL
```

For Patroni-managed Pigsty clusters, prefer `pig pitr`, which coordinates restore safety and post-restore guidance.


## Stanza Management Commands

### pb create

Initialize new stanza. Must run before first backup.

```bash
pig pb create                        # Create stanza
pig pb create --no-online            # Create when PostgreSQL not running
pig pb create --force                # Force create
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--no-online` | | Create when PostgreSQL not running |
| `--force` | `-f` | Force create |
{.full-width}


### pb upgrade

Update stanza after PostgreSQL major version upgrade.

```bash
pig pb upgrade                       # Upgrade stanza
pig pb upgrade --no-online           # Upgrade when PostgreSQL not running
```

**Options:**

| Option | Description |
|:---|:---|
| `--no-online` | Upgrade when PostgreSQL not running |
{.full-width}

**Use Case:**

After PostgreSQL major version upgrade (e.g., 16 -> 17), run this command to update stanza metadata.


### pb delete

Delete stanza and all its backups.

```bash
pig pb delete --force                # Delete stanza (requires --force)
pig pb delete --force --yes          # Skip countdown confirmation
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--force` | `-f` | Confirm delete (required) |
| `--yes` | `-y` | Skip countdown confirmation |
{.full-width}

**Warning:** This is a **destructive and irreversible** operation! All backups will be permanently deleted.

Multiple safety mechanisms:
1. Must provide `--force` parameter
2. 5-second countdown (press Ctrl+C to cancel)
3. Use `--yes` to skip countdown


## Control Commands

### pb check

Verify backup repository integrity and configuration.

```bash
pig pb check                         # Verify repository
```

This command checks:
- WAL archive configuration correctness
- Repository accessibility
- Stanza configuration validity


### pb start

Enable pgBackRest operations.

```bash
pig pb start                         # Enable operations
```

Use after `pb stop` to resume normal operations.


### pb stop

Disable pgBackRest operations (for maintenance).

```bash
pig pb stop                          # Disable operations
pig pb stop --force                  # Terminate running operations
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--force` | `-f` | Terminate running operations |
{.full-width}

**Use Case:**

During system maintenance, use this command to prevent new backup operations from starting.


## Log Commands

### pb log

View pgBackRest log files. Log directory is read from pgBackRest `log-path` when configured, otherwise `/pg/log/pgbackrest/`. Latest log selection follows modification-time order. Use `-o json` for JSONL log records; `yaml` and `json-pretty` are not supported for log snapshots.

```bash
pig pb log                           # Show latest 50 lines
pig pb log -f                        # Real-time view latest log
pig pb log list                      # List log files
pig pb log tail                      # Real-time view latest log
pig pb log tail -n 100               # Show last 100 lines and follow
pig pb log show                      # Show latest log snapshot
pig pb log show -n 50                # Show last 50 lines
```

**Subcommands:**

| Subcommand | Aliases | Description |
|:---|:---|:---|
| list | ls | List log files |
| tail | follow, f | Real-time follow latest log |
| show | cat, c | Show latest log content |
{.full-width}

**Options:**

| Option | Short | Default | Description |
|:---|:---|:---|:---|
| `--lines` | `-n` | 50 | Positive number of lines to show |
{.full-width}

**Permission Handling:**

Log file reads are executed as the configured database superuser (`--dbsu`, default `postgres`) when direct access is not available.


## Design Notes

**Command Execution:**

All `pig pb` commands execute as database superuser (DBSU). This is because pgBackRest needs access to PostgreSQL data files and WAL archives.

Execution logic:
- If current user is DBSU: execute directly
- If current user is root: use `su - postgres -c "..."` to execute
- Other users: use `sudo -inu postgres -- ...` to execute

**Relationship with pgbackrest:**

`pig pb` is not a complete wrapper of `pgbackrest`, but a higher-level abstraction for common operations:

- Auto-detect stanza name, no need to specify each time
- Auto-check primary role before backup
- Display plan and require confirmation before restore
- Reject low-level restore when Patroni is active for managed PGDATA; use `pig pitr`
- Provide user-friendly time format input
- Provide post-restore guidance

For full `pgbackrest` functionality, use `pgbackrest` command directly.

**Default Configuration Paths:**

| Config | Default |
|:---|:---|
| Config file | `/etc/pgbackrest/pgbackrest.conf` |
| Log directory | `/pg/log/pgbackrest` |
| Data directory | `pg1-path` from config, or `$PGDATA` env, or `/pg/data` |
{.full-width}

**Security Considerations:**

- `pb delete` requires `--force` confirmation, with 5-second countdown
- `pb expire --set` requires confirmation or `--yes`
- `pb restore` requires an explicit recovery target, validates `--time`, and requires typed `yes` confirmation unless `--yes` is used
- `pb backup` checks primary role by default, prevents running on replica
- Log command filename parameter filters paths to prevent path traversal attacks

**Platform Support:**

This command is designed for Linux systems, depends on Pigsty default directory structure.

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
  pig pb ls                        list backups
  pig pb ls repo                   list configured repos
  pig pb ls stanza                 list all stanzas

Backup Commands (Primary Only):
  pig pb backup                    create backup (auto mode)
  pig pb backup full               full backup
  pig pb backup diff               differential backup
  pig pb backup incr               incremental backup

Restore Commands (low-level primitive):
  pig pb restore -d                restore to latest (end of WAL)
  pig pb restore -I                restore to backup consistency point
  pig pb restore -t <time>         restore to specific time
  pig pb restore --name <name>     restore to named restore point
  pig pb restore -b <set> -d       restore specific backup set (requires a target)

Stanza Management:
  pig pb create                    create stanza (first-time setup)
  pig pb upgrade                   upgrade stanza after PG major upgrade
  pig pb delete                    delete stanza (dangerous!)

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

High-risk pgBackRest actions expose a structured `state -> plan -> precheck -> execute -> verify -> result -> next_actions` contract. In JSON/YAML mode, destructive execution requires explicit `--yes`; missing confirmation returns a structured parameter error with `next_actions` instead of prompting. Confirmation errors and plans carry concrete, replayable commands that preserve `--stanza/--config/--repo/--dbsu` (the stanza is pinned from auto-detection when unspecified). Plan output never deletes or mutates managed data, and may include `boundary`, `confirmation`, `preconditions`, `verifications`, `next_actions`, and `dry_run_output` in addition to the existing plan fields. `dry_run_output` (currently on `pb expire --plan`) embeds a non-deleting native `pgbackrest expire --dry-run` preview, which does execute pgBackRest as DBSU (reads the repository and may write pgBackRest logs/locks). Plans resolve the effective config (read-only) so the displayed stanza and data directory match what execution would use; unresolvable configs are marked with a `config resolution: unresolved` precondition. Replayable commands quote arguments with POSIX single-quote escaping and canonicalize `--time` to its normalized, timezone-completed form so replays are deterministic. Successful `pb restore` results include `next_actions` mirroring the text-mode post-restore hints.


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
| `pb restore` | Restore from backup (low-level primitive) | `pgbackrest restore` |
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

# Restore (low-level primitive; use pig pitr for orchestrated recovery)
pig pb restore -d                    # Restore to latest (end of WAL)
pig pb restore -I                    # Restore to backup consistency point
pig pb restore -t "2025-01-01 12:00:00+08"  # Restore to specific time
pig pb restore --name savepoint      # Restore to named restore point

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
pig pb info                          # Show all backup info (parsed view)
pig pb info -o json                  # Structured output (Result wrapper, native JSON embedded)
pig pb info -R                       # Raw pgbackrest text output
pig pb info --raw --raw-output json  # Raw pgbackrest native JSON output
pig pb info --set 20250101-120000F   # Show specific backup set details
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--raw` | `-R` | Raw mode: pass through native pgbackrest output |
| `--raw-output` | | Raw output format: text, json (only with `--raw`) |
| `--set` | | Show specific backup set details |
{.full-width}

Structured output uses the global `-o/--output` flag (json/yaml/json-pretty): the Result envelope embeds pgBackRest's native info JSON in `data`. Raw mode bypasses the Result envelope and does not support YAML; invalid raw parameters return a structured `pb` parameter error in structured mode.


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

Before backup, command auto-checks if the instance is primary. If replica, command exits with error. Use `--force` to skip this check. The role probe targets the stanza's `pg1-path` (and the configured `--dbsu`) rather than the ambient default instance, so the check and the backup target cannot diverge on hosts with non-default data directories.


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

**Plan semantics:** `--plan` in text mode executes the native `pgbackrest expire --dry-run` directly. In structured mode it renders a Plan whose `dry_run_output` field embeds the same native dry-run output (when pgbackrest is available), so agents see the real expiration scope; a `dry run: unavailable` verification is reported otherwise.

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
pig pb restore --name my-savepoint   # Restore to named restore point
pig pb restore --lsn "0/7C82CB8"     # Restore to LSN
pig pb restore --xid 12345           # Restore to transaction ID

# Backup set selection (can combine with recovery target)
pig pb restore -b 20251225-120000F   # Restore from specific backup set

# Other options
pig pb restore -t "..." -X           # Exclusive mode (stop before target)
pig pb restore -t "..." --target-action promote   # Promote after reaching target
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
| `--name` | | Restore to named restore point |
| `--lsn` | | Restore to specific LSN |
| `--xid` | | Restore to specific transaction ID |
{.full-width}

**Backup Set and Other Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--set` | `-b` | Restore from specific backup set (can combine with target) |
| `--data` | `-D` | Target data directory |
| `--exclusive` | `-X` | Exclusive mode: stop before target |
| `--target-action` | | Action at target: `pause`, `promote`, or `shutdown` |
| `--target-timeline` | `-T` | Timeline: `latest`, `current`, integer, or `0xHEX` |
| `--yes` | `-y` | Skip confirmation prompt |
| `--plan` | | Preview restore plan without executing |
{.full-width}

Raw pgBackRest restore arguments must be placed after `--`; stray positionals before the separator are rejected. Pig rejects passthrough arguments that override restore target, lifecycle, or selection flags, including `--type`, `--target`, `--target-action`, `--target-exclusive`, `--target-timeline`, `--set`, `--recovery-option`, every spelling of the data directory option (`--pg-path`, `--pgN-path`, deprecated `--db[N]-path`), the **entire** repository option family (`--repo[N]-*`: path, host, type, s3/gcs/azure/sftp, cipher, ...), and config/selection redirection (`--stanza`, `--config`, `--config-path`, `--config-include-path`, `--repo`); repository identity comes from the config file plus Pig's `-r`/`-c` flags only. Relocation escape hatches (`--tablespace-map`, `--link-map`, `--link-all`) remain allowed: they neither move the declared PGDATA nor change the backup source.

**Time Formats:**

Supports multiple time format inputs with strict validation and timezone auto-completion (including non-integer-hour zones like +05:30). Every timezone-less input gets the operator's local offset appended, so the recovery point never silently shifts to the server timezone:

| Format | Example | Description |
|:---|:---|:---|
| Full format | `2025-01-01 12:00:00+08` | Complete timestamp with timezone |
| Datetime, no timezone | `2025-01-01 12:00:00` | Local timezone offset appended (`T` separator also accepted) |
| Date only | `2025-01-01` | Auto-completes to 00:00:00 that day (local timezone) |
| Time only | `12:00:00` | Auto-completes to today (local timezone) |
{.full-width}

Every accepted form is canonicalized to the space-separated `YYYY-MM-DD HH:MM:SS±HH[:MM]` spelling pgBackRest documents for `--target`: the `T` separator and `Z`/`±HHMM` offset spellings are rewritten (input offset preserved), because pgBackRest parses the value itself for backup-set selection and rejects non-canonical forms with `[029] time format must be ...`.

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
pig pb delete                        # Delete stanza (interactive y/N confirmation)
pig pb delete --yes                  # Skip confirmation prompt
pig pb delete --plan                 # Preview stanza deletion plan
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--yes` | `-y` | Skip confirmation prompt |
| `--plan` | | Preview stanza deletion plan without executing |
{.full-width}

**Warning:** This is a **destructive and irreversible** operation! All backups will be permanently deleted.

Safety mechanism: text mode prompts for typed `y`/`yes` confirmation unless `--yes` is provided (EOF aborts). JSON/YAML mode never prompts; without `--yes` it returns a structured confirmation-required error with replayable `--yes`/`--plan` next actions.

**Multi-stanza guard:** when the config file defines more than one stanza and `--stanza` was not given, `pb delete` refuses in all modes (`CodePbAmbiguousStanza`) and lists per-stanza `--plan` preview commands — auto-detection never selects a deletion target. The `--plan` output marks stanza selection as `blocked` in this case.

**Native `--force`:** pig always passes pgBackRest's native `--force` to `stanza-delete`, replacing the native stop-first interlock with pig's own `--yes` gate. This keeps `pig pb delete --yes` working while PostgreSQL is stopped without requiring a prior `pgbackrest stop`.


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

- `pb delete` prompts for interactive `y`/`yes` confirmation unless `--yes` is given, and refuses without explicit `--stanza` when multiple stanzas are configured
- `pb expire --set` requires confirmation or `--yes`
- `pb restore` requires an explicit recovery target, validates `--time`, and requires typed `yes` confirmation unless `--yes` is used; the structured result builder re-checks `--yes` independently of the cmd-layer gate
- `pb backup` checks primary role by default, prevents running on replica
- Restore passthrough (`-- args`) rejects target/lifecycle/selection overrides and stray positionals before the `--` separator
- Log command filename parameter filters paths to prevent path traversal attacks

**Platform Support:**

This command is designed for Linux systems, depends on Pigsty default directory structure.

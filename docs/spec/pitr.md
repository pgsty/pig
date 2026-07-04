---
title: "pig pitr"
description: "Perform orchestrated Point-In-Time Recovery (PITR) with pig pitr command"
weight: 185
icon: fas fa-clock-rotate-left
module: [PIG]
categories: [Reference]
---

The `pig pitr` command performs **Orchestrated Point-In-Time Recovery**. Unlike low-level `pig pb restore`, this command coordinates restore safety across Patroni, PostgreSQL, and pgBackRest. It stops Patroni when needed and can restart PostgreSQL after restore, but it does not automatically rejoin Patroni or validate HA routing after recovery.

```bash
pig pitr - Perform PITR with Patroni/PostgreSQL restore-safety orchestration.

This command orchestrates a complete PITR workflow:
  1. Stop Patroni service (if running)
  2. Ensure PostgreSQL is stopped (fast stop with retry; destructive fallback only with --force-stop)
  3. Execute pgbackrest restore
  4. Start PostgreSQL unless --no-restart is used
  5. Leave Patroni stopped; provide post-restore guidance

Recovery Targets (at least one required):
  --default, -d      Recover to end of WAL stream (latest)
  --immediate, -I    Recover to backup consistency point
  --time, -t         Recover to specific timestamp
  --name             Recover to named restore point
  --lsn              Recover to specific LSN
  --xid              Recover to specific transaction ID

Time Format:
  - Full: "2025-01-01 12:00:00+08"
  - Date only: "2025-01-01" (defaults to 00:00:00)
  - Time only: "12:00:00" (defaults to today)

Examples:
  pig pitr -d                      # Recover to latest (most common)
  pig pitr -t "2025-01-01 12:00:00" # Recover to specific time
  pig pitr -I                      # Recover to backup consistency point
  pig pitr -d --plan               # Show execution plan without running
  pig pitr -d -y                   # Skip y/yes confirmation (for automation)
  pig pitr -d --no-restart         # Don't auto-start PostgreSQL after restore
  pig pitr -d --target-timeline current
```


## Overview

`pig pitr` is a recovery command that:

1. Automatically stops Patroni service (if running)
2. Ensures PostgreSQL is stopped (with retry and fallback strategies)
3. Executes pgBackRest restore
4. Starts PostgreSQL unless `--no-restart` is used
5. Provides post-recovery guidance while leaving Patroni rejoin under operator control

**Comparison with `pig pb restore`:**

| Feature | `pig pitr` | `pig pb restore` |
|:--------|:-----------|:-----------------|
| Stop Patroni | Automatic | Manual |
| Stop PostgreSQL | Automatic (with retry) | Must be pre-stopped |
| Start PostgreSQL | Automatic unless `--no-restart` | Manual |
| Patroni rejoin | Manual after verification | Manual |
| Post-recovery guidance | Detailed guidance | Basic low-level hints |
| Use case | Production full recovery | Low-level ops or scripting |
{.full-width}


## Quick Start

```bash
# Most common: recover to latest data
pig pitr -d

# Recover to specific point in time
pig pitr -t "2025-01-01 12:00:00+08"

# Recover to backup consistency point (fastest)
pig pitr -I

# View execution plan (plan mode)
pig pitr -d --plan

# Skip confirmation (for automation)
pig pitr -d -y

# Recover from specific backup set
pig pitr -d -b 20251225-120000F

# Don't auto-start PostgreSQL after recovery
pig pitr -d --no-restart
```


## Parameters

### Recovery Target (choose one)

| Param | Short | Description |
|:------|:------|:------------|
| `--default` | `-d` | Recover to end of WAL stream (latest data) |
| `--immediate` | `-I` | Recover to backup consistency point |
| `--time` | `-t` | Recover to specific timestamp |
| `--name` | | Recover to named restore point |
| `--lsn` | | Recover to specific LSN |
| `--xid` | | Recover to specific transaction ID |
{.full-width}

### Backup Selection

| Param | Short | Description |
|:------|:------|:------------|
| `--set` | `-b` | Recover from specific backup set |
{.full-width}

### Flow Control

| Param | Short | Description |
|:------|:------|:------------|
| `--no-restart` | | Don't auto-start PostgreSQL after recovery |
| `--plan` | | Show execution plan only, don't execute |
| `--yes` | `-y` | Skip interactive y/yes confirmation |
| `--timeout` | | PostgreSQL start/recovery timeout in seconds |
| `--force-stop` | | Allow immediate shutdown and kill fallback if fast stop fails |
{.full-width}

### Recovery Options

| Param | Short | Description |
|:------|:------|:------------|
| `--exclusive` | `-X` | Exclusive mode: stop before target |
| `--target-action` | | Action at recovery target: `pause`, `promote`, or `shutdown` |
| `--target-timeline` | `-T` | Timeline: `latest`, `current`, integer, or `0xHEX` |
{.full-width}

Use `--no-restart` with `--target-action=shutdown`, because PostgreSQL exits when it reaches that recovery target. `--target-action` cannot be used with `--default`, because `--default` already recovers to the end of WAL. `--exclusive/-X` requires a precise stop-before target: `--time`, `--lsn`, or `--xid`.

Raw pgBackRest restore arguments may be placed after `--`, but Pig rejects passthrough arguments that override restore target, lifecycle, data directory, repository, config, or selection flags; use Pig's first-class flags instead. This uses the same restore passthrough blocklist as `pig pb restore`.

### Configuration

| Param | Short | Description |
|:------|:------|:------------|
| `--stanza` | `-s` | pgBackRest stanza name (auto-detected) |
| `--config` | `-c` | pgBackRest config file path |
| `--repo` | `-r` | Repository number (multi-repo scenario) |
| `--dbsu` | `-U` | Database superuser (default: `postgres`) |
| `--data` | `-D` | Target data directory |
{.full-width}


## Time Format

The `--time` parameter supports multiple formats with strict validation and automatic timezone completion:

| Format | Example | Description |
|:-------|:--------|:------------|
| Full format | `2025-01-01 12:00:00+08` | Complete timestamp with timezone |
| Datetime, no timezone | `2025-01-01 12:00:00` | Local timezone offset appended (`T` separator also accepted) |
| Date only | `2025-01-01` | Auto-complete to 00:00:00 (current timezone) |
| Time only | `12:00:00` | Auto-complete to today (current timezone) |
{.full-width}

Plan output and replayable next-action commands normalize date-only and time-only targets to a deterministic timestamp with timezone, using the same normalization contract as `pig pb restore --plan`.


## Managed vs Side Restore

The managed PostgreSQL data directory is resolved from the effective pgBackRest config (`pg1-path`) plus command flags. It is not hardcoded to `/pg/data`; a cluster whose managed PGDATA is `/var/lib/pgsql/18/data` is still treated as a managed restore. Path comparisons resolve symlinks as the database superuser where needed, so a symlink to managed PGDATA is not treated as a side restore.

An explicit `-D/--data` that resolves to a different directory is a side restore. Side restores require `--no-restart`, do not stop Patroni, and post-restore guidance uses `pg_ctl -D <dir> -o "-p 5433" start`, `pg_ctl -D <dir> status`, and side-directory log inspection. Port `5433` is only an example alternate port; replace it if occupied. When recovery stops at `--target-action=shutdown`, side-restore log guidance points at `<dir>/log` instead of the managed `/pg/log/postgres` log directory. The side-directory `pgbackrest --pg1-path=<dir> stanza-create` guidance preserves non-default `--stanza=<stanza>` and `--config=<path>`, but is only relevant if the side directory will be converted into a managed cluster.

The side-restore directory must exist and be owned by the configured DBSU; unlike managed PGDATA, it does not need to be initialized with `PG_VERSION` before restore. If the requested side directory cannot be canonicalized, Pig treats it as a side restore and lets the normal data-directory precheck report the concrete existence or owner error. If the requested side directory resolves successfully but the managed PGDATA cannot be resolved, the explicit `-D` is still treated as a side restore rather than failing the precheck on the unrelated managed path.

For managed PGDATA outside `/pg/data`, follow-up runbook commands include the effective data directory, for example `pig pg start -D /var/lib/pgsql/18/data`, `pig pg psql -D /var/lib/pgsql/18/data`, and `pig pg promote -D /var/lib/pgsql/18/data`. `pig pg psql -D <dir>` reads that directory's `postmaster.pid` and binds to its port and socket directory when available; it does not silently fall back to the default psql target when postmaster information cannot be parsed.


## Execution Flow

### Phase 1: Pre-check

- Validate recovery target parameters (must specify exactly one)
- Resolve effective pgBackRest config, stanza, repository, and managed `pg1-path`
- Check managed data directory exists and is initialized, or exists as an empty directory owned by DBSU
- For side restores, check the custom data directory exists and is owned by DBSU
- Verify the selected stanza is OK and has backups; when `--set` is given, verify that backup set exists
- Detect Patroni service status
- Detect PostgreSQL running status

### Phase 2: Stop Patroni

If Patroni service is running and the restore targets the managed data
directory, Patroni is always stopped (side restores to a custom `-D`
directory never touch Patroni):
- Execute `systemctl stop patroni`
- Wait for PostgreSQL to auto-stop with Patroni

### Phase 3: Ensure PostgreSQL Stopped

Progressive strategy to ensure PostgreSQL is fully stopped:

1. **Wait for auto-stop**: Wait 30 seconds after Patroni stops
2. **Graceful stop**: Use `pg_ctl stop -m fast` (retry 3 times with exponential backoff)
3. **Immediate stop**: Use `pg_ctl stop -m immediate` only when `--force-stop` is set
4. **Force kill**: Use `kill -9` only when `--force-stop` is set and immediate stop still fails

### Phase 4: Execute Recovery

Call pgBackRest for actual data recovery:
```bash
pgbackrest restore ...
```

If restore fails after Patroni has been stopped, Pig reports that Patroni remains stopped and that the target data directory may be partially restored. Structured failure results include `next_actions` for Patroni status, pgBackRest log inspection, and a concrete replayable PITR command ending in `--yes` after the underlying failure is fixed.

### Phase 5: Start PostgreSQL

Unless `--no-restart` specified, auto-start PostgreSQL:
- Wait for startup completion (timeout 120 seconds)
- Verify process is actually running
- For `--default` and `--target-action=promote`, wait for `pg_is_in_recovery()` to become false on the restored instance
- Recovery and post-restore SQL probes bind to the restored data directory's `postmaster.pid` port, and include the socket directory with `-h` when present

Patroni is not restarted or rejoined automatically. Keep Patroni stopped until recovered data, timeline, and intended primary/replica role are verified.

If PostgreSQL fails to start after restore, Pig reports that the restore has already run and Patroni remains stopped when PITR stopped it. Structured failure results include log-inspection and replay actions; do not start Patroni until the restored data directory has been validated or PITR has been rerun cleanly.

### Phase 6: Post-Recovery Guidance

Display detailed follow-up instructions including:
- How to verify recovered data
- How to promote to primary
- How to resume Patroni cluster management
- How to recreate pgBackRest stanza


## Usage Examples

### Scenario 1: Accidental Data Deletion Recovery

```bash
# 1. View available backups
pig pb info

# 2. Recover to time before deletion
pig pitr -t "2025-01-15 09:30:00+08"

# 3. Verify data
pig pg psql
SELECT * FROM important_table;

# 4. Promote to primary if satisfied
pig pg promote
```

### Scenario 2: Recover to Latest State

```bash
# Recover to latest data after server failure
pig pitr -d
```

### Scenario 3: Fast Recovery to Backup Point

```bash
# Recover to backup consistency point (no WAL replay needed)
pig pitr -I
```

### Scenario 4: Automation Scripts

```bash
# Skip destructive confirmation, suitable for automation
pig pitr -d -y
```

### Scenario 5: Standalone PostgreSQL Instance

```bash
# Non-Patroni managed instance: inactive Patroni is detected automatically,
# no extra flag is needed
pig pitr -d
```

### Scenario 6: Recover Without Starting

```bash
# Recover then manually inspect before deciding to start
pig pitr -d --no-restart

# Check recovered data directory
ls -la /pg/data/

# Manual start
pig pg start
```


## Execution Plan Example

Running `pig pitr -d --plan` shows a plan like:

```
══════════════════════════════════════════════════════════════════
 PITR Execution Plan
══════════════════════════════════════════════════════════════════

Current State:
  Data Directory:  /pg/data
  Database User:   postgres
  Patroni Service: active
  PostgreSQL:      running (PID: 12345)

Recovery Target:
  Latest (end of WAL stream)

Execution Steps:
  [1] Stop Patroni service
  [2] Ensure PostgreSQL is stopped
  [3] Execute pgBackRest restore
  [4] Start PostgreSQL
  [5] Print post-restore guidance

══════════════════════════════════════════════════════════════════

[Plan mode] No changes made.
```

Structured plan output includes `api`, `boundary`, `confirmation`, `preconditions`, `verifications`, and `next_actions`. Managed restores use boundary `pitr:managed-recovery`; side restores use boundary `pitr:side-restore`. The first `next_actions` entry is the replayable execution command with `--yes`; it preserves only the user-requested `-D` flag and does not pin an inferred managed data directory into the command. Plan `next_actions` intentionally contain preview/execute and read-only inspection commands, not post-restore lifecycle commands such as `pig pg start`, `pig pg promote`, or `systemctl start patroni`. pgBackRest inspection actions preserve the effective stanza/config/repo/DBSU context when it is known.

If structured execution is requested without `--yes`, Pig returns a confirmation-required result with neutral boundary `pitr:restore` and replayable `next_actions` that preserve the requested PITR flags, including `--no-restart`, `-D`, `-s`, and `-c` when present. The primitive preview action is rendered as a concrete `pig pb restore` command ending in `--plan`, using the same target, side directory, stanza, config, repo, and DBSU context instead of a placeholder.


## Post-Recovery Operations

After successful recovery, detailed follow-up instructions are displayed:

```
══════════════════════════════════════════════════════════════════
 PITR Complete
══════════════════════════════════════════════════════════════════

[1] Verify recovered data:
   pig pg psql

[2] If satisfied, promote to primary:
   pig pg promote

[3] To resume Patroni cluster management:
   systemctl start patroni

[4] Re-create pgBackRest stanza if needed:
   pig pb create

══════════════════════════════════════════════════════════════════
```

On a managed cluster whose PGDATA is `/var/lib/pgsql/18/data`, the same post-recovery guidance targets that directory explicitly:

```bash
pig pg start -D /var/lib/pgsql/18/data
pig pg psql -D /var/lib/pgsql/18/data
pig pg promote -D /var/lib/pgsql/18/data
```


## Safety Mechanisms

### Confirmation Prompt

Unless `--yes` is used, `pig pitr` requires an explicit interactive confirmation:

```
WARNING: This will overwrite the current database!
Continue with PITR? [y/N]:
```

### Progressive Stop Strategy

To ensure data safety, PostgreSQL is stopped progressively:
1. First try graceful stop (ensures data consistency)
2. Without `--force-stop`, stop and report an error if fast stop cannot complete
3. With `--force-stop`, try immediate stop and finally `kill -9` if needed

### Recovery Verification

When PostgreSQL is restarted, Pig verifies that it started successfully and prompts to check logs on failure. Patroni rejoin and HA routing checks remain manual follow-up work.

### Structured Output

In structured output mode, execution requires `--yes`; `--plan` remains the preview path. Successful structured PITR results include post-restore `next_actions` on the Result envelope, not inside `data`, so automation can read follow-up commands consistently with other Pig plans/results. Result `data` includes `requested_data_dir`, `effective_data_dir`, `managed_data_dir`, and `side_restore` so callers can distinguish user input from the actual restore target. The `post_restore` object keeps the compatibility boolean `queried`, adds `sql_queried` for actual SQL probe execution, and uses `query_skipped_reason` when PostgreSQL was not running or was not started by PITR. For non-default manual targets, `success=true` can still mean PostgreSQL is running at a recovery pause point; automation must inspect `post_restore.in_recovery`, LSN, and timeline before declaring recovery complete. Restore/start failures after lifecycle changes include envelope `next_actions` for status/log inspection and clean PITR replay.


## Design Notes

**Relationship with other commands:**

- `pig pitr` internally performs Patroni stop, PostgreSQL stop/start, and pgBackRest restore steps
- Provides higher-level restore-safety coordination than individual commands
- Does not automatically rejoin Patroni or validate VIP/client routing
- Suitable for production environment complete PITR workflow

**Error Handling:**

- Detailed error messages at each phase
- Prompts relevant log locations on failure
- Supports manual continuation after interruption

**Permission Execution:**

- If current user is DBSU: execute commands directly
- If current user is root: use `su - postgres -c "..."` to execute
- Other users: use `sudo -inu postgres -- ...` to execute

**Platform Support:**

This command is designed for Linux systems, depends on Pigsty's default directory structure.

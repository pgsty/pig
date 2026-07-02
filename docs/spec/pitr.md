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
  2. Ensure PostgreSQL is stopped (with retry and fallback)
  3. Execute pgbackrest restore
  4. Start PostgreSQL unless --no-restart is used
  5. Leave Patroni stopped; provide post-restore guidance

Recovery Targets (at least one required):
  --default, -d      Recover to end of WAL stream (latest)
  --immediate, -I    Recover to backup consistency point
  --time, -t         Recover to specific timestamp
  --name, -n         Recover to named restore point
  --lsn, -l          Recover to specific LSN
  --xid, -x          Recover to specific transaction ID

Time Format:
  - Full: "2025-01-01 12:00:00+08"
  - Date only: "2025-01-01" (defaults to 00:00:00)
  - Time only: "12:00:00" (defaults to today)

Examples:
  pig pitr -d                      # Recover to latest (most common)
  pig pitr -t "2025-01-01 12:00:00" # Recover to specific time
  pig pitr -I                      # Recover to backup consistency point
  pig pitr -d --plan               # Show execution plan without running
  pig pitr -d -y                   # Skip confirmation (for automation)
  pig pitr -d --skip-patroni       # Skip Patroni management
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

# Standalone PostgreSQL (non-Patroni managed)
pig pitr -d --skip-patroni

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
| `--name` | `-n` | Recover to named restore point |
| `--lsn` | `-l` | Recover to specific LSN |
| `--xid` | `-x` | Recover to specific transaction ID |
{.full-width}

### Backup Selection

| Param | Short | Description |
|:------|:------|:------------|
| `--set` | `-b` | Recover from specific backup set |
{.full-width}

### Flow Control

| Param | Short | Description |
|:------|:------|:------------|
| `--skip-patroni` | `-S` | Skip Patroni stop operation |
| `--no-restart` | `-N` | Don't auto-start PostgreSQL after recovery |
| `--plan` | | Show execution plan only, don't execute |
| `--yes` | `-y` | Skip confirmation countdown |
| `--timeout` | | PostgreSQL start/recovery timeout in seconds |
| `--force-stop` | | Allow immediate shutdown and kill fallback if fast stop fails |
{.full-width}

### Recovery Options

| Param | Short | Description |
|:------|:------|:------------|
| `--exclusive` | `-X` | Exclusive mode: stop before target |
| `--promote` | `-P` | Auto-promote to primary after recovery |
| `--target-action` | | Action at recovery target: `pause`, `promote`, or `shutdown` |
| `--target-timeline` | `-T` | Timeline: `latest`, `current`, integer, or `0xHEX` |
{.full-width}

Use `--no-restart` with `--target-action=shutdown`, because PostgreSQL exits when it reaches that recovery target. Raw pgBackRest restore arguments may be placed after `--`, but Pig rejects passthrough arguments that override restore target or lifecycle flags; use Pig's first-class flags instead.

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
| Date only | `2025-01-01` | Auto-complete to 00:00:00 (current timezone) |
| Time only | `12:00:00` | Auto-complete to today (current timezone) |
{.full-width}


## Execution Flow

### Phase 1: Pre-check

- Validate recovery target parameters (must specify exactly one)
- Check data directory exists and is initialized
- Detect Patroni service status
- Detect PostgreSQL running status

### Phase 2: Stop Patroni

If Patroni service is running and `--skip-patroni` not specified:
- Execute `systemctl stop patroni`
- Wait for PostgreSQL to auto-stop with Patroni

### Phase 3: Ensure PostgreSQL Stopped

Progressive strategy to ensure PostgreSQL is fully stopped:

1. **Wait for auto-stop**: Wait 30 seconds after Patroni stops
2. **Graceful stop**: Use `pg_ctl stop -m fast` (retry 3 times with exponential backoff)
3. **Immediate stop**: Use `pg_ctl stop -m immediate`
4. **Force kill**: Use `kill -9` (last resort)

### Phase 4: Execute Recovery

Call pgBackRest for actual data recovery:
```bash
pgbackrest restore --target-action=promote ...
```

### Phase 5: Start PostgreSQL

Unless `--no-restart` specified, auto-start PostgreSQL:
- Wait for startup completion (timeout 120 seconds)
- Verify process is actually running

Patroni is not restarted or rejoined automatically. Keep Patroni stopped until recovered data, timeline, and intended primary/replica role are verified.

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
# Non-Patroni managed instance
pig pitr -d --skip-patroni
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
   WARNING: Ensure data is correct before starting Patroni!
   systemctl start patroni

   Or if you want this node to be the leader:
   1. Promote PostgreSQL first: pig pg promote
   2. Then start Patroni: systemctl start patroni

[4] Re-create pgBackRest stanza if needed:
   pig pb create

══════════════════════════════════════════════════════════════════
```


## Safety Mechanisms

### Confirmation Countdown

Unless `--yes` is used, a 5-second countdown is displayed before execution:

```
WARNING: This will overwrite the current database!
Press Ctrl+C to cancel, or wait for countdown...
Starting PITR in 5 seconds...
```

### Progressive Stop Strategy

To ensure data safety, PostgreSQL is stopped progressively:
1. First try graceful stop (ensures data consistency)
2. Then try immediate stop on failure
3. Finally use kill -9 (only in extreme cases)

### Recovery Verification

When PostgreSQL is restarted, Pig verifies that it started successfully and prompts to check logs on failure. Patroni rejoin and HA routing checks remain manual follow-up work.


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

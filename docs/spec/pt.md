---
title: "pig patroni"
description: "Manage Patroni service and cluster with pig patroni subcommand"
weight: 170
icon: fas fa-infinity
module: [PIG]
categories: [Reference]
---

The `pig patroni` command (alias `pig pt`) manages Patroni service and PostgreSQL HA clusters. It wraps common `patronictl` and `systemctl` operations for simplified cluster management.

```bash
pig pt - Low-level Patroni primitives (patronictl + systemd unit patroni).
         Orchestrated point-in-time recovery lives in "pig pitr".

Cluster Operations (via patronictl):
  pig pt list [cluster]            list cluster members
  pig pt restart [member]          restart PostgreSQL (rolling restart)
  pig pt reload                    reload PostgreSQL config
  pig pt reinit <member>           reinitialize a member
  pig pt pause                     pause automatic failover
  pig pt resume                    resume automatic failover
  pig pt switchover                perform planned switchover
  pig pt failover                  perform manual failover
  pig pt config <action>           manage cluster config

Service Management (via systemctl):
  pig pt status                    show comprehensive patroni status
  pig pt svc start                 start patroni service
  pig pt svc stop                  stop patroni service
  pig pt svc restart               restart patroni service
  pig pt svc status                show patroni service status

Logs:
  pig pt log [-f] [-n 100]         view patroni logs
  pig pt log tail [-n 100]         follow patroni logs
  pig pt log show [-n 100]         show patroni log snapshot
```

> **B03**: the former top-level shortcuts `pt start` / `pt stop` are removed. They remain as
> hidden stubs that fail with a one-line route: `daemon control moved: use pig pt svc start|stop`.

## Primitive Contract

`pig pt` is the Patroni and DCS primitive layer. It changes cluster state through `patronictl` and Patroni service operations; it does not call `pig pg` to manage local `pg_ctl` lifecycle and does not run pgBackRest restore. Cross-module recovery and lifecycle choreography belongs in `pig pitr`.

High-risk cluster actions and DCS mutations should present a structured `state -> plan -> precheck -> execute -> verify -> result -> next_actions` contract. In JSON/YAML mode, unsafe execution requires explicit `--yes` confirmation, while read-only commands such as `pt list`, `pt status`, and `pt config show` remain confirmation-free. Plan output is side-effect-free and may include `boundary`, `confirmation`, `preconditions`, `verifications`, and `next_actions` in addition to the existing plan fields.

Pig owns confirmation for cluster operations (B04): `patronictl` always runs with its own `--force` flag and never prompts interactively. In text mode, `pt reinit`, `pt switchover`, `pt failover`, and cluster-wide `pt restart` ask a one-line `y/yes` confirmation at the pig layer unless `--yes/-y` is given. In structured (JSON/YAML) mode the same commands are fail-closed: without `--yes` Pig returns a structured confirmation-required result. `pt restart` uses a conditional tier (D2): an explicit single member executes directly in both modes; only the cluster-wide rolling restart (no member argument) requires confirmation. `pt reinit`, `pt switchover`, and `pt failover` also expose `--plan` previews.


## Overview

**Cluster Operations** (patronictl wrapper):

| Command | Alias | Description | Implementation |
|:--------|:------|:------------|:---------------|
| `pt list [cluster]` | `ls, l` | List cluster members | `patronictl list [cluster] -e -t` |
| `pt restart` | `reboot, rt` | Restart PostgreSQL instance | `patronictl restart` |
| `pt reload` | `rl, hup` | Reload PostgreSQL config | `patronictl reload` |
| `pt reinit` | `ri` | Reinitialize member | `patronictl reinit` |
| `pt switchover` | `sw` | Planned switchover | `patronictl switchover` |
| `pt failover` | `fo` | Manual failover | `patronictl failover` |
| `pt pause` | `p` | Pause auto-failover | `patronictl pause` |
| `pt resume` | `r` | Resume auto-failover | `patronictl resume` |
| `pt config` | `cfg, c` | Show or modify cluster config | `patronictl show-config / edit-config` |
{.full-width}

**Service Management** (systemctl wrapper):

| Command | Alias | Description | Implementation |
|:--------|:------|:------------|:---------------|
| `pt status` | `st, stat` | Show service status | `systemctl status patroni` |
| `pt log` | `l, lg` | View Patroni logs | `journalctl -u patroni` |
{.full-width}

The former top-level shortcuts `pt start` / `pt stop` are removed (B03). Hidden stubs keep the
old tokens routable: invoking them fails with `daemon control moved: use pig pt svc start|stop`.

**Service Subcommand** (`pt svc`):

| Command | Alias | Description |
|:--------|:------|:------------|
| `pt svc start` | `boot, up` | Start Patroni service |
| `pt svc stop` | `halt, dn, down` | Stop Patroni service |
| `pt svc restart` | `reboot, rt` | Restart Patroni service |
| `pt svc reload` | `rl, hup` | Reload Patroni service |
| `pt svc status` | `st, stat` | Show service status |
{.full-width}


## Quick Start

```bash
# Check cluster member status
pig pt list                    # List default cluster members
pig pt list pg-meta            # List specific cluster
pig pt list -W                 # Continuous watch mode
pig pt list -w 5               # Refresh every 5 seconds

# View and modify cluster config
pig pt config                  # Show current cluster config
pig pt config ttl=60           # Modify single config item (immediate effect)
pig pt config ttl=60 loop_wait=15  # Modify multiple config items

# Cluster operations
pig pt restart                 # Rolling restart ALL members (asks confirmation)
pig pt restart pg-test-1       # Restart specific member (direct)
pig pt restart -y              # Cluster-wide restart, skip confirmation
pig pt switchover              # Planned switchover (asks confirmation)
pig pt pause                   # Pause auto-failover
pig pt resume                  # Resume auto-failover

# Manage Patroni service
pig pt status                  # Check service status
pig pt svc start               # Start service
pig pt svc stop                # Stop service
pig pt log -f                  # Real-time log viewing
```


## Global Options

These options apply to all `pig pt` subcommands:

| Option | Short | Description |
|:-------|:------|:------------|
| `--dbsu` | `-U` | Database superuser (default: `$PIG_DBSU` or `postgres`) |
{.full-width}


## Cluster Commands

### pt list

List Patroni cluster member status. Wraps `patronictl list` with `-e` (extended output) and `-t` (show timestamp) flags by default. The optional `cluster` positional is passed through to `patronictl list`; without it, Patroni uses the local config.

```bash
pig pt list                    # List default cluster members
pig pt list pg-meta            # List specific cluster
pig pt list -W                 # Continuous watch mode
pig pt list -w 5               # Refresh every 5 seconds
pig pt list pg-test -W -w 3    # Watch pg-test cluster, 3s refresh
```

**Options:**

| Option | Short | Description |
|:-------|:------|:------------|
| `--watch` | `-W` | Enable continuous watch mode |
| `--interval` | `-w` | Watch refresh interval (seconds) |
{.full-width}

**Argument policy:** `pt list` accepts at most one optional cluster positional. `pt restart` accepts at most one member positional. `pt reinit` requires exactly one member positional. `pt reload`, `pt switchover`, `pt failover`, `pt pause`, and `pt resume` do not accept positionals; use `--leader` or `--candidate` for switchover/failover target selection.


### pt restart

Restart PostgreSQL instance via Patroni. This triggers a rolling restart of PostgreSQL, not the Patroni daemon itself.

```bash
pig pt restart                   # Rolling restart ALL members (asks confirmation)
pig pt restart -y                # Cluster-wide restart, skip confirmation
pig pt restart pg-test-1         # Restart specific member (direct execution)
pig pt restart --role=replica    # Restart replicas only
pig pt restart --pending         # Restart pending members
```

**Options:**

| Option | Short | Description |
|:-------|:------|:------------|
| `--yes` | `-y` | Skip confirmation prompt |
| `--role` | `-r` | Filter by role (leader/replica/any) |
| `--pending` | `-p` | Restart only pending members |
{.full-width}

**Confirmation tier (D2, conditional):** an explicit single member executes directly in both output modes. A cluster-wide rolling restart (no member argument) is T2: text mode asks a pig-level confirmation unless `--yes`; JSON/YAML mode is fail-closed and returns a confirmation-required result without `--yes`. `patronictl restart` always receives `--force` and never prompts (B04).


### pt reload

Reload PostgreSQL configuration via Patroni. Triggers config reload on all members.

```bash
pig pt reload
```


### pt reinit

Reinitialize cluster member. This re-syncs data from the primary.

```bash
pig pt reinit pg-test-1          # Reinit specific member (asks confirmation)
pig pt reinit pg-test-1 -y       # Skip confirmation
pig pt reinit pg-test-1 --wait   # Wait for completion
pig pt reinit pg-test-1 --plan   # Preview reinit plan
```

**Options:**

| Option | Short | Description |
|:-------|:------|:------------|
| `--yes` | `-y` | Skip confirmation prompt |
| `--wait` | | Wait for reinit completion (long-only, B12) |
| `--plan` | | Preview reinit plan without executing |
{.full-width}

**Warning:** This operation deletes all data on the target member and re-syncs from primary. Text mode asks a pig-level confirmation ("This will WIPE and rebuild member ...") unless `--yes`; JSON/YAML execution is fail-closed and requires `--yes`. `patronictl reinit` always receives `--force` (B04).


### pt switchover

Perform planned primary-replica switchover.

```bash
pig pt switchover                 # Planned switchover (asks confirmation)
pig pt switchover -y              # Skip confirmation
pig pt switchover --plan          # Show switchover plan without running
pig pt switchover --leader pg-1   # Specify current primary
pig pt switchover --candidate pg-2  # Specify new primary
```

**Options:**

| Option | Short | Description |
|:-------|:------|:------------|
| `--yes` | `-y` | Skip confirmation prompt |
| `--leader` | | Specify current primary (long-only, B17) |
| `--candidate` | | Specify candidate new primary (long-only, B17) |
| `--scheduled` | | Scheduled time for switchover (long-only, B17) |
| `--plan` | | Show execution plan only, don't execute |
{.full-width}

Text mode asks a pig-level confirmation ("This will transfer cluster leadership") unless `--yes`; JSON/YAML execution is fail-closed and requires `--yes`. `patronictl switchover` always receives `--force` (B04).


### pt failover

Perform manual failover. Used when primary is unavailable.

```bash
pig pt failover                   # Manual failover (asks confirmation)
pig pt failover -y                # Skip confirmation
pig pt failover --candidate pg-2  # Specify new primary
```

**Options:**

| Option | Short | Description |
|:-------|:------|:------------|
| `--yes` | `-y` | Skip confirmation prompt |
| `--candidate` | | Specify candidate new primary (long-only, B17) |
| `--plan` | | Show execution plan only, don't execute |
{.full-width}

Text mode asks a pig-level confirmation ("This will force leadership transfer (failover, data loss possible)") unless `--yes`; JSON/YAML execution is fail-closed and requires `--yes`. `patronictl failover` always receives `--force` (B04).


### pt pause

Pause Patroni's automatic failover.

```bash
pig pt pause                      # Pause auto-failover
pig pt pause --wait               # Wait for confirmation
```

**Options:**

| Option | Short | Description |
|:-------|:------|:------------|
| `--wait` | | Wait for operation completion (long-only, B12) |
{.full-width}

**Use case:** Pause auto-failover during maintenance operations (e.g., major version upgrade, storage migration) to prevent accidental triggers.


### pt resume

Resume Patroni's automatic failover.

```bash
pig pt resume                     # Resume auto-failover
pig pt resume --wait              # Wait for confirmation
```

**Options:**

| Option | Short | Description |
|:-------|:------|:------------|
| `--wait` | | Wait for operation completion (long-only, B12) |
{.full-width}


### pt config

Show or modify cluster configuration. Without parameters shows current config, with `key=value` parameters modifies config.

```bash
pig pt config                           # Show current cluster config
pig pt config show                      # Show config (explicit)
pig pt config edit                      # Interactive config edit
pig pt config set ttl=60                # Set TTL to 60 seconds
pig pt config set ttl=60 loop_wait=15   # Modify multiple config items
pig pt config pg max_connections=200    # Modify PostgreSQL parameter
```

**Subcommands:**

| Subcommand | Description |
|:-----------|:------------|
| `show` (default) | Show current config |
| `edit` | Interactive config edit |
| `set key=value` | Directly set config item |
| `pg key=value` | Set PostgreSQL parameter |
{.full-width}

**Common config items:**

| Config | Description | Default |
|:-------|:------------|:--------|
| `ttl` | Leader lock time-to-live (seconds) | 30 |
| `loop_wait` | Main loop sleep time (seconds) | 10 |
| `retry_timeout` | DCS and PostgreSQL operation timeout (seconds) | 10 |
| `maximum_lag_on_failover` | Maximum lag allowed during failover (bytes) | 1048576 |
{.full-width}

**Note:** This command modifies dynamic cluster config stored in DCS (e.g., etcd), not local config file `/etc/patroni/patroni.yml`.


## Service Commands

### pt start / pt stop (removed, B03)

The top-level `pt start` / `pt stop` shortcuts are removed. Daemon control lives under `pt svc`:

```bash
pig pt svc start                 # Start Patroni service
pig pt svc stop                  # Stop Patroni service
```

Hidden landing-pad stubs keep the old tokens (and their aliases `boot/up`, `halt/dn/down`)
routable: invoking them prints nothing to stdout and fails with the exact one-line route
`daemon control moved: use pig pt svc start|stop`.

**Note:** Stopping Patroni service will also stop the PostgreSQL instance on this node (depending on Patroni configuration).


### pt status

Show Patroni service comprehensive status, including:
- systemd service status
- Patroni process info
- Cluster member status

```bash
pig pt status
```


### pt log

View Patroni service logs. Use `-o json` for JSONL log records; `yaml` and `json-pretty` are not supported for log snapshots. JSONL mode reads journal messages with `journalctl -o cat` so each JSONL `message` field contains the raw log message.

```bash
pig pt log                     # Show last 50 log lines
pig pt log -f                  # Real-time log following
pig pt log tail                # Real-time log following
pig pt log show                # Show last 50 log lines
pig pt log -n 100              # Show last 100 log lines
pig pt log -f -n 200           # Show last 200 lines and follow
```

**Options:**

| Option | Short | Default | Description |
|:-------|:------|:--------|:------------|
| `--follow` | `-f` | false | Real-time log following |
| `--lines` | `-n` | 50 | Number of log lines to show |
{.full-width}

`pt log tail` also accepts `--follow/-f` as a documented no-op (B16): tail always follows.

Text mode is equivalent to `journalctl -u patroni [-f] [-n N]`. JSONL mode is equivalent to `journalctl -u patroni -n N --no-pager -o cat` followed by JSONL wrapping.


## pt svc Subcommand

`pt svc` provides the same functionality as top-level service commands, for explicitly operating on the Patroni daemon:

```bash
pig pt svc start                 # Start Patroni service
pig pt svc stop                  # Stop Patroni service
pig pt svc restart               # Restart Patroni service
pig pt svc reload                # Reload Patroni service
pig pt svc status                # Show service status
```

**Alias Reference:**

| Command | Alias |
|:--------|:------|
| `pt svc start` | `boot, up` |
| `pt svc stop` | `halt, dn, down` |
| `pt svc restart` | `reboot, rt` |
| `pt svc reload` | `rl, hup` |
| `pt svc status` | `st, stat` |
{.full-width}


## Design Notes

**Relationship with patronictl:**

`pig pt` wraps common `patronictl` operations:
- Cluster queries: `list`, `config show`
- Cluster management: `restart`, `reload`, `reinit`, `switchover`, `failover`, `pause`, `resume`
- Config modification: `config set`, `config edit`
- Service commands (start/stop/restart/reload/status) call `systemctl`
- `log` command calls `journalctl`

**Default Config Paths:**

| Config | Default |
|:-------|:--------|
| Patroni config file | `/etc/patroni/patroni.yml` |
| Service name | `patroni` |
{.full-width}

**Cluster Scope Resolution:**

`patronictl restart`, `reinit`, `switchover`, and `failover` require a `CLUSTER_NAME` positional argument. `pig pt` reads `scope:` from `/etc/patroni/patroni.yml` and prepends it before member/candidate flags for those subcommands. If the config is not directly readable, `pig pt` retries the read as the configured DBSU.

**Structured Output Error Codes:**

| Code | Meaning |
|:-----|:--------|
| `CodePtConfigNotFound` | Patroni config file was not found |
| `CodePtPermDenied` | Permission denied reading the Patroni config or running `patronictl` |
| `CodePtScopeMissing` | `scope:` is missing or empty in the Patroni config |
| `CodePtConfigResolveFailed` | Cluster scope resolution failed for an unclassified reason |
| `CodePtConfigReadFailed` | Patroni config exists or was attempted but could not be read for a non-permission, non-not-found reason |
| `CodePtConfirmationRequired` | Structured cluster-wide `pt restart` / `pt reinit` / `pt switchover` / `pt failover` invoked without `--yes` |
| `CodePtWatchModeUnsupported` | `pt list --watch/-W` is incompatible with structured output |
{.full-width}

**Permission Handling:**

- If current user is DBSU: execute commands directly
- If current user is root: use `su - postgres -c "..."` to execute
- Other users: use `sudo -inu postgres -- ...` to execute

**Platform Support:**

This command is designed for Linux systems, depends on `systemctl` and `journalctl`.

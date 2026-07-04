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
  pig pt failover [candidate]      perform manual failover
  pig pt config <action>           manage cluster config (edit|show|set|pg)

Service Management (via systemctl):
  pig pt status                    show comprehensive patroni status
  pig pt svc start (pig pt start)  start patroni service
  pig pt svc stop  (pig pt stop)   stop patroni service
  pig pt svc restart               restart patroni service
  pig pt svc reload                reload patroni service
  pig pt svc status                show patroni service status

Logs:
  pig pt log [-f] [-n 50]          view patroni logs
  pig pt log tail [-n 50]          follow patroni logs
  pig pt log show [-n 50]          show patroni log snapshot
```

> **B03**: `pt start` / `pt stop` are hidden shortcuts for `pt svc start` / `pt svc stop`.
> `pt svc` remains the documented Patroni daemon control surface.

## Primitive Contract

`pig pt` is the Patroni and DCS primitive layer. It changes cluster state through `patronictl` and Patroni service operations; it does not call `pig pg` to manage local `pg_ctl` lifecycle and does not run pgBackRest restore. Cross-module recovery and lifecycle choreography belongs in `pig pitr`.

High-risk cluster actions and DCS mutations should present a structured `state -> plan -> precheck -> execute -> verify -> result -> next_actions` contract. In JSON/YAML mode, unsafe execution requires explicit `--yes` confirmation, while read-only commands such as `pt list`, `pt status`, and `pt config show` remain confirmation-free. Plan output is side-effect-free and may include `boundary`, `confirmation`, `preconditions`, `verifications`, and `next_actions` in addition to the existing plan fields.

Pig owns confirmation for cluster operations (B04): `patronictl` always runs with its own `--force` flag and never prompts interactively (this includes `pt reload`, whose underlying `patronictl reload` would otherwise prompt for member confirmation). In text mode, `pt reinit`, `pt switchover`, `pt failover`, and cluster-wide `pt restart` ask a one-line `y/yes` confirmation at the pig layer unless `--yes/-y` is given. In structured (JSON/YAML) mode the same commands are fail-closed through the shared `requireStructuredConfirmation` gate (same envelope as `pg`/`pb`): without `--yes` Pig returns a confirmation-required result whose `data.operation` carries the operation metadata and whose `next_actions` carry replayable `--yes` execute and `--plan` preview commands rendered by the same builders the plans use. `pt restart` uses a conditional tier (D2): an explicit single member and `--pending` (already scoped by a prior config change) execute directly in both modes; only the unscoped cluster-wide rolling restart requires confirmation. `pt restart`, `pt reinit`, `pt switchover`, and `pt failover` all expose `--plan` previews; every `Plan.Command` is the replayable `--plan` preview form, and the first plan next action is the execute form — carrying `--yes` exactly when the scope is gated, so a `confirmation: none` plan never points at a confirmation-flagged command.

Short option contract: `--wait` exposes `-w` whenever the command scope has no local shorthand conflict. This applies to `pt reinit`, `pt pause`, and `pt resume`; `pt list -w` remains the list interval flag because it is a different command scope.

Switch preflight contract: before executing or asking for confirmation for `pt switchover` and `pt failover`, Pig reads the current Patroni topology through the same structured list/config surfaces used by `pig pt list` and `pig pt config show`. The preflight records the cluster name, current leader, observed candidate replicas, and Patroni pause mode. If the cluster is paused, Pig refuses the leader-transfer operation before prompting and returns `CodePtClusterPaused` in structured output with `pig pt resume` as the required next action.


## Overview

**Cluster Operations** (patronictl wrapper):

| Command             | Alias | Description                   | Implementation                         |
|:--------------------|:------|:------------------------------|:---------------------------------------|
| `pt list [cluster]` | `ls`  | List cluster members          | `patronictl list [cluster] -e -t`      |
| `pt restart`        | `rs`  | Restart PostgreSQL instance   | `patronictl restart`                   |
| `pt reload`         | `rl`  | Reload PostgreSQL config      | `patronictl reload <scope> --force`    |
| `pt reinit`         | `ri`  | Reinitialize member           | `patronictl reinit`                    |
| `pt switchover`     | `so`  | Planned switchover            | `patronictl switchover`                |
| `pt failover`       | `fo`  | Manual failover               | `patronictl failover`                  |
| `pt pause`          | `p`   | Pause auto-failover           | `patronictl pause`                     |
| `pt resume`         | `r`   | Resume auto-failover          | `patronictl resume`                    |
| `pt config`         | `c`   | Show or modify cluster config | `patronictl show-config / edit-config` |
{.full-width}

**Service Management** (systemctl wrapper):

| Command | Alias | Description | Implementation |
|:--------|:------|:------------|:---------------|
| `pt start` | `up` | Start Patroni service | `systemctl start patroni` |
| `pt stop` | `dn` | Stop Patroni service | `systemctl stop patroni` |
| `pt status` | `st` | Show comprehensive status | `systemctl status` + `ps` + `patronictl list` |
| `pt log` | `l` | View Patroni logs | `journalctl -u patroni` |
{.full-width}

The top-level `pt start` / `pt stop` shortcuts remain hidden (B03), but execute the same actions as
`pt svc start` / `pt svc stop`. `pt svc` stays the documented, explicit Patroni daemon control surface.

**Service Subcommand** (`pt svc`):

| Command | Alias | Description |
|:--------|:------|:------------|
| `pt svc start` | `up` | Start Patroni service |
| `pt svc stop` | `dn` | Stop Patroni service |
| `pt svc restart` | `rs` | Restart Patroni service |
| `pt svc reload` | `rl` | Reload Patroni service |
| `pt svc status` | `st` | Show service status |
{.full-width}


## Quick Start

```bash
# Check cluster member status
pig pt list                    # List default cluster members
pig pt list pg-meta            # List specific cluster
pig pt list -W                 # Continuous watch mode
pig pt list -w 5               # Refresh every 5 seconds

# View and modify cluster config
pig pt config                  # Show current cluster config (defaults to show)
pig pt config set ttl=60       # Modify single config item (immediate effect)
pig pt config set ttl=60 loop_wait=15  # Modify multiple config items

# Cluster operations
pig pt restart                 # Rolling restart ALL members (asks confirmation)
pig pt restart pg-test-1       # Restart specific member (direct)
pig pt restart --pending       # Apply pending restarts (direct)
pig pt restart -y              # Cluster-wide restart, skip confirmation
pig pt switchover              # Planned switchover (asks confirmation)
pig pt pause                   # Pause auto-failover
pig pt resume                  # Resume auto-failover

# Manage Patroni service
pig pt status                  # Check service status
pig pt start                   # Hidden shortcut for pig pt svc start
pig pt stop                    # Hidden shortcut for pig pt svc stop
pig pt svc start               # Start service
pig pt svc stop                # Stop service
pig pt log -f                  # Real-time log viewing
```


## Global Options

These options apply to all `pig pt` subcommands:

| Option   | Short | Description                                             |
|:---------|:------|:--------------------------------------------------------|
| `--dbsu` | `-U`  | Database superuser (default: `$PIG_DBSU` or `postgres`) |
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

| Option       | Short | Description                                                            |
|:-------------|:------|:-----------------------------------------------------------------------|
| `--watch`    | `-W`  | Enable continuous watch mode                                           |
| `--interval` | `-w`  | Watch refresh interval in seconds; decimals such as `0.5` are accepted |
{.full-width}

Watch mode (`--watch/-W` or a positive `--interval/-w`) is passthrough text output only; structured output returns `CodePtWatchModeUnsupported`.

**Argument policy:** `pt list` accepts at most one optional cluster positional. `pt restart` accepts at most one member positional. `pt reinit` requires exactly one member positional. `pt reload`, `pt switchover`, `pt pause`, and `pt resume` do not accept positionals. `pt reload` resolves the current cluster scope from Patroni config. Switchover target selection uses `--leader/-l` and `--candidate/-c`. `pt failover` accepts at most one optional candidate positional; `pig pt failover <member>` is equivalent to `pig pt failover --candidate <member>`, and a conflicting positional/flag candidate is a parameter error.


### pt restart

Restart PostgreSQL instance via Patroni. This triggers a rolling restart of PostgreSQL, not the Patroni daemon itself.

```bash
pig pt restart                   # Rolling restart ALL members (asks confirmation)
pig pt restart -y                # Cluster-wide restart, skip confirmation
pig pt restart pg-test-1         # Restart specific member (direct execution)
pig pt restart --role=replica    # Restart replicas only (asks confirmation)
pig pt restart --pending         # Apply pending restarts (direct execution)
pig pt restart --plan            # Preview restart plan without executing
```

**Options:**

| Option      | Short | Description                                    |
|:------------|:------|:-----------------------------------------------|
| `--yes`     | `-y`  | Skip confirmation prompt                       |
| `--role`    | `-r`  | Filter by role (leader/replica/any, validated) |
| `--pending` | `-p`  | Restart only pending members                   |
| `--plan`    |       | Preview restart plan without executing         |
{.full-width}

**Confirmation tier (D2, conditional):** an explicit single member executes directly in both output modes, and so does `--pending` — it only restarts members already flagged by a prior (operator-initiated) config change, making it the friction-free follow-up that `pt config pg` suggests in `next_actions`. An unscoped cluster-wide rolling restart (no member, no `--pending`, with or without `--role`) is T2: text mode asks a pig-level confirmation unless `--yes`; JSON/YAML mode is fail-closed and returns a confirmation-required result without `--yes`. `patronictl restart` always receives `--force` and never prompts (B04).


### pt reload

Reload PostgreSQL configuration via Patroni. Triggers config reload on all members.

`pig pt reload` does not accept a cluster positional. It reads `scope:` from
`/etc/patroni/patroni.yml` and executes `patronictl reload <scope> --force`
internally, because `patronictl reload` requires `CLUSTER_NAME` even when `-c`
points at the Patroni config file, and because it would otherwise prompt its own
interactive member confirmation (B04: pig owns confirmation; reload is a
low-risk primitive that runs without one).

```bash
pig pt reload
```


### pt reinit

Reinitialize cluster member. This re-syncs data from the primary.

```bash
pig pt reinit pg-test-1          # Reinit specific member (asks confirmation)
pig pt reinit pg-test-1 -y       # Skip confirmation
pig pt reinit pg-test-1 -w       # Wait for completion
pig pt reinit pg-test-1 --plan   # Preview reinit plan
```

**Options:**

| Option   | Short | Description                           |
|:---------|:------|:--------------------------------------|
| `--yes`  | `-y`  | Skip confirmation prompt              |
| `--wait` | `-w`  | Wait for reinit completion            |
| `--plan` |       | Preview reinit plan without executing |
{.full-width}

**Warning:** This operation deletes all data on the target member and re-syncs from primary. Text mode asks a pig-level confirmation ("This will WIPE and rebuild member ...") unless `--yes`; JSON/YAML execution is fail-closed and requires `--yes`. `patronictl reinit` always receives `--force` (B04).


### pt switchover

Perform planned primary-replica switchover. (alias `so`)

```bash
pig pt switchover                 # Planned switchover (asks confirmation)
pig pt switchover -y              # Skip confirmation
pig pt switchover --plan          # Show switchover plan without running
pig pt switchover -l pg-1 -c pg-2 # Specify current and new primary
pig pt switchover -s "2026-07-04T12:00:00" # Schedule switchover
```

**Options:**

| Option        | Short | Description                             |
|:--------------|:------|:----------------------------------------|
| `--yes`       | `-y`  | Skip confirmation prompt                |
| `--leader`    | `-l`  | Specify current primary                 |
| `--candidate` | `-c`  | Specify candidate new primary           |
| `--scheduled` | `-s`  | Scheduled time for switchover           |
| `--plan`      |       | Show execution plan only, don't execute |
{.full-width}

Execution preflight reads the current cluster topology before confirmation. If Patroni pause mode is enabled, Pig refuses the switchover and tells the operator to run `pig pt resume` first. Otherwise the text confirmation names the cluster, current leader, and observed candidate replicas; without `--candidate/-c` it states that Patroni will choose the most eligible replica and suggests `pig pt switchover -c <instance>` for an explicit target. JSON/YAML execution is fail-closed and requires `--yes`. `patronictl switchover` always receives `--force` (B04).


### pt failover

Perform manual failover. Used when primary is unavailable. A candidate is
required: Patroni's REST API only performs failover to an explicit candidate,
so pig fails fast (structured mode returns a parameter error) instead of
leaving the rejection to patronictl. The candidate can be provided either as
`--candidate/-c <member>` or as the single positional `pig pt failover <member>`.

```bash
pig pt failover --candidate pg-2   # Manual failover (asks confirmation)
pig pt failover pg-2               # Same candidate, positional form
pig pt failover -c pg-2 -y         # Skip confirmation
pig pt failover -c pg-2 --plan     # Preview failover plan
```

**Options:**

| Option | Short | Description |
|:-------|:------|:------------|
| `--yes` | `-y` | Skip confirmation prompt |
| `--candidate` | `-c` | Candidate new primary (required unless positional candidate is used) |
| `--plan` | | Show execution plan only, don't execute |
{.full-width}

Execution preflight reads the current cluster topology before confirmation. If Patroni pause mode is enabled, Pig refuses the failover and tells the operator to run `pig pt resume` first. Otherwise the text confirmation names the cluster, current leader, requested candidate, and observed candidate replicas, while still warning that failover can lose data. JSON/YAML execution is fail-closed and requires `--yes`. `patronictl failover` always receives `--force` (B04).


### pt pause

Pause Patroni's automatic failover.

```bash
pig pt pause                      # Pause auto-failover
pig pt pause -w                   # Wait for confirmation
```

**Options:**

| Option | Short | Description |
|:-------|:------|:------------|
| `--wait` | `-w` | Wait for operation completion |
{.full-width}

**Use case:** Pause auto-failover during maintenance operations (e.g., major version upgrade, storage migration) to prevent accidental triggers.


### pt resume

Resume Patroni's automatic failover.

```bash
pig pt resume                     # Resume auto-failover
pig pt resume -w                  # Wait for confirmation
```

**Options:**

| Option | Short | Description |
|:-------|:------|:------------|
| `--wait` | `-w` | Wait for operation completion |
{.full-width}


### pt config

Show or modify cluster configuration. Without an action it defaults to `show`
in both output modes; modifications go through the explicit `set` (Patroni
config) and `pg` (PostgreSQL parameters) actions with `key=value` pairs.

```bash
pig pt config                           # Show current cluster config
pig pt config show                      # Show config (explicit)
pig pt config edit                      # Interactive config edit
pig pt config set ttl=60                # Set TTL to 60 seconds
pig pt config set ttl=60 loop_wait=15   # Modify multiple config items
pig pt config set ttl=60 --plan         # Preview DCS config change
pig pt config pg max_connections=200    # Modify PostgreSQL parameter
pig pt config pg shared_buffers=4GB --plan  # Preview PostgreSQL parameter change
```

**Subcommands:**

| Subcommand | Description |
|:-----------|:------------|
| `show` (default) | Show current config |
| `edit` | Interactive config edit |
| `set key=value` | Directly set config item |
| `pg key=value` | Set PostgreSQL parameter |
{.full-width}

**Options:**

| Option | Short | Description |
|:-------|:------|:------------|
| `--plan` | | Preview `set` or `pg` config changes without executing |
{.full-width}

`set` and `pg` reject non-`key=value` tokens instead of silently dropping them. `edit` is interactive and is not supported in structured output. `pg` changes analyze known postmaster-context parameters and include `pig pt list` plus `pig pt restart --pending` next actions when a PostgreSQL restart is required.

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

### pt start / pt stop (hidden shortcuts, B03)

The top-level `pt start` / `pt stop` shortcuts are hidden from help, but execute the same service
actions as `pt svc start` / `pt svc stop`:

```bash
pig pt start                     # Hidden shortcut for pig pt svc start
pig pt stop                      # Hidden shortcut for pig pt svc stop
pig pt svc start                 # Start Patroni service
pig pt svc stop                  # Stop Patroni service
```

The aliases stay minimal and aligned with the `pt svc` commands: `up` routes to start, and
`dn` routes to stop.

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

View Patroni service logs. Use `-o json` with `pt log` or `pt log show` for JSONL log records; `yaml` and `json-pretty` are not supported for log snapshots. JSONL mode reads journal messages with `journalctl -o cat` so each JSONL `message` field contains the raw log message. Structured output is not supported for follow/tail mode.

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

`pt svc` (also `pt service`) is the explicit command group for operating on the Patroni daemon. Hidden top-level
`pt start` / `pt stop` shortcuts map to its start/stop actions:

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
| `pt svc start` | `up` |
| `pt svc stop` | `dn` |
| `pt svc restart` | `rs` |
| `pt svc reload` | `rl` |
| `pt svc status` | `st` |
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

`patronictl reload`, `restart`, `reinit`, `switchover`, and `failover` require a `CLUSTER_NAME` positional argument. `pig pt` reads `scope:` from `/etc/patroni/patroni.yml` and prepends it before member/candidate flags for those subcommands. If the config is not directly readable, `pig pt` retries the read as the configured DBSU.

**Structured Output Error Codes:**

| Code | Meaning |
|:-----|:--------|
| `CodePtConfigNotFound` | Patroni config file was not found |
| `CodePtPermDenied` | Permission denied reading the Patroni config or running `patronictl` |
| `CodePtScopeMissing` | `scope:` is missing or empty in the Patroni config |
| `CodePtConfigResolveFailed` | Cluster scope resolution failed for an unclassified reason |
| `CodePtConfigReadFailed` | Patroni config exists or was attempted but could not be read for a non-permission, non-not-found reason |
| `CodePtClusterPaused` | `pt switchover` / `pt failover` preflight found the cluster in Patroni pause mode; run `pig pt resume` before retrying |
| `CodePtConfirmationRequired` | Structured cluster-wide `pt restart` / `pt reinit` / `pt switchover` / `pt failover` invoked without `--yes` |
| `CodePtWatchModeUnsupported` | `pt list --watch/-W` is incompatible with structured output |
| `150199` (generic param error) | Invalid parameters rejected by the cmd envelope: bad `--role`, `failover` without `--candidate`, non-`key=value` config args, unsupported log modes |
| `150899` (generic op failure) | A wrapped patronictl/systemctl operation failed without a more specific classification |
{.full-width}

`NN=99` is reserved in every module's param/operation category for these
generic envelope codes (`output.GenericParamError` / `output.GenericOpFailed`),
so they can never collide with named `CodePt*` constants.

**Permission Handling:**

- If current user is DBSU: execute commands directly
- If current user is root: use `su - postgres -c "..."` to execute
- Other users: use `sudo -inu postgres -- ...` to execute
- systemctl actions escalate via sudo for non-root users, except the read-only
  `status` action, which runs unprivileged (with `--no-pager -l`) so it works
  without sudo rights

**Platform Support:**

This command is designed for Linux systems, depends on `systemctl` and `journalctl`.

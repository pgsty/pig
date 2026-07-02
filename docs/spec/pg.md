---
title: "pig postgres"
description: "Manage local PostgreSQL server with pig postgres subcommand"
weight: 160
icon: fas fa-database
module: [PIG]
categories: [Reference]
---

The `pig pg` command (alias `pig postgres`) manages local PostgreSQL server and databases. It wraps native tools like `pg_ctl`, `psql`, and `pg_repack`, providing a simplified server management experience.

```bash
pig pg - Manage local postgres server (pg_ctl, psql, pg_repack)

Control Commands (via pg_ctl or systemctl):
  pig pg init                      initialize postgres data directory
  pig pg start                     start postgres server
  pig pg stop                      stop postgres server
  pig pg restart                   restart postgres server
  pig pg reload                    reload postgres server
  pig pg status                    show postgres server status
  pig pg promote                   promote replica to primary
  pig pg role                      detect and print postgres role

Connection & Query (via psql):
  pig pg psql [db] [-c sql]        connect to postgres
  pig pg ps                        show current connections
  pig pg kill [-a] [-x] [-u user] [-d db] [-q sql] [-w secs]

Maintenance (via psql & pg_repack):
  pig pg vacuum  [db] [-a]         vacuum database
  pig pg analyze [db] [-a]         analyze database
  pig pg freeze  [db] [-a]         vacuum freeze tables
  pig pg repack  [db] [-a]         online repack database

Log Commands:
  pig pg log                       show latest log lines
  pig pg log -f                    tail -f latest log
  pig pg log list                  list log files
  pig pg log tail [logfile]        tail -f log file
  pig pg log show [logfile]        show log file snapshot
  pig pg log less <logfile>        less log file
  pig pg log grep <pat> [logfile]  grep log file

Service Management (via systemctl):
  pig pg svc start                 start postgres service
  pig pg svc stop                  stop postgres service
  pig pg svc restart               restart postgres service
  pig pg svc reload                reload postgres service
  pig pg svc status                show postgres service status
```

## Primitive Contract

`pig pg` is the local PostgreSQL primitive layer. It operates on the local instance, data directory, local connections, and PostgreSQL utilities only. It does not coordinate Patroni membership, pgBackRest restore state, VIPs, load balancers, or application traffic; use `pig pt`, `pig pb`, or top-level orchestration such as `pig pitr` for those boundaries.

High-risk primitives expose a structured `state -> plan -> precheck -> execute -> verify -> result -> next_actions` contract. In JSON/YAML mode, destructive execution requires explicit confirmation flags such as `--yes`, `--force`, or an execution flag like `pg kill --execute`; safe queries and `--plan` previews do not. Plan output is side-effect-free and may include `boundary`, `confirmation`, `preconditions`, `verifications`, and `next_actions` in addition to the existing `command/actions/affects/expected/risks` fields.

## Command Overview

**Service Control** (pg_ctl wrapper):

| Command | Alias | Description | Notes |
|:--------|:------|:------------|:------|
| `pg init` | `initdb, i` | Initialize data directory | Wraps initdb |
| `pg start` | `boot, up` | Start PostgreSQL | Wraps pg_ctl start |
| `pg stop` | `halt, down` | Stop PostgreSQL | Wraps pg_ctl stop |
| `pg restart` | `reboot` | Restart PostgreSQL | Wraps pg_ctl restart |
| `pg reload` | `hup` | Reload configuration | Wraps pg_ctl reload |
| `pg status` | `st, stat` | Show service status | Shows processes & related services |
| `pg promote` | `pro` | Promote replica to primary | Wraps pg_ctl promote |
| `pg role` | `r` | Detect instance role | Outputs primary/replica |
{.full-width}

**Connection & Query**:

| Command | Alias | Description | Notes |
|:--------|:------|:------------|:------|
| `pg psql` | `sql, connect` | Connect to database | Wraps psql |
| `pg ps` | `activity, act` | Show current connections | Queries pg_stat_activity |
| `pg kill` | `k` | Terminate connections | Default dry-run mode |
{.full-width}

**Database Maintenance**:

| Command | Alias | Description | Notes |
|:--------|:------|:------------|:------|
| `pg vacuum` | `vac, vc` | Vacuum tables | Executes maintenance SQL through psql |
| `pg analyze` | `ana, az` | Analyze tables | Executes maintenance SQL through psql |
| `pg freeze` | `frz` | Freeze vacuum | Executes maintenance SQL through psql |
| `pg repack` | `rp` | Online table repacking | Requires pg_repack extension |
{.full-width}

**Log Tools**:

| Command | Alias | Description | Notes |
|:--------|:------|:------------|:------|
| `pg log` | `l` | Log management | Parent command |
| `pg log list` | `ls` | List log files | |
| `pg log tail` | `t, f` | Real-time log viewing | tail -f |
| `pg log show` | `cat, c` | Show log content | |
| `pg log less` | `vi, v` | View with less | |
| `pg log grep` | `g, search` | Search logs | |
{.full-width}

**Service Subcommand** (`pg svc`):

| Command | Alias | Description |
|:--------|:------|:------------|
| `pg svc start` | `boot, up` | Start postgres service |
| `pg svc stop` | `halt, dn, down` | Stop postgres service |
| `pg svc restart` | `reboot, rt` | Restart postgres service |
| `pg svc reload` | `rl, hup` | Reload postgres service |
| `pg svc status` | `st, stat` | Show service status |
{.full-width}


## Quick Start

```bash
# Service control
pig pg init                       # Initialize data directory
pig pg start                      # Start PostgreSQL
pig pg status                     # Check status
pig pg stop                       # Stop PostgreSQL
pig pg restart                    # Restart PostgreSQL
pig pg reload                     # Reload configuration

# Connection & query
pig pg psql                       # Connect to postgres database
pig pg psql mydb                  # Connect to specific database
pig pg ps                         # View current connections
pig pg kill -x                    # Terminate connections (requires -x to execute)

# Database maintenance
pig pg vacuum mydb                # Vacuum specific database
pig pg analyze mydb               # Analyze specific database
pig pg repack mydb                # Online repack database

# Log viewing
pig pg log                        # Show latest log lines
pig pg log -f                     # Real-time view latest log
pig pg log tail                   # Real-time view latest log
pig pg log grep ERROR             # Search error logs
pig pg log list --log-dir /var/log/pg  # Custom log directory
```


## Global Options

These options apply to all `pig pg` subcommands:

| Option | Short | Default | Description |
|:---|:---|:---|:---|
| `--version` | `-v` | auto-detect | PostgreSQL major version |
| `--data` | `-D` | `/pg/data` | Data directory path |
| `--dbsu` | `-U` | `postgres` | Database superuser (or `$PIG_DBSU` env) |
{.full-width}

Systemd service operations are exposed through `pig pg svc ...`; there is no global `--systemd/-S` switch on `pig pg` primitives.

**Version Detection Logic:**

1. If `-v` specified, use that version
2. Otherwise read from `PG_VERSION` file in data directory
3. If neither available, use default PostgreSQL in PATH


## Service Control Commands

### pg init

Initialize PostgreSQL data directory. Wraps `initdb`.

```bash
pig pg init                       # Initialize with defaults
pig pg init -v 17                 # Specify PostgreSQL 17
pig pg init -D /data/pg17         # Specify data directory
pig pg init -k                    # Enable data checksums
pig pg init -f                    # Force init (remove existing data)
pig pg init -f -y                 # Skip force confirmation prompt
pig pg init -- --waldir=/wal      # Pass extra args to initdb
```

**Options:**

| Option | Short | Default | Description |
|:---|:---|:---|:---|
| `--encoding` | `-E` | UTF8 | Database encoding |
| `--locale` | | C | Locale setting |
| `--data-checksum` | `-k` | false | Enable data checksums |
| `--force` | `-f` | false | Force init, remove existing data (dangerous!) |
| `--yes` | `-y` | false | Skip confirmation when `--force` overwrites data |
{.full-width}

**Safety:** `--force` is destructive and requires confirmation in text mode; JSON/YAML mode requires `--yes`. Even with `--force --yes`, command refuses to run if PostgreSQL is running.


### pg start

Start PostgreSQL server.

```bash
pig pg start                      # Start with defaults
pig pg start -D /data/pg17        # Specify data directory
pig pg start -l /pg/log/pg.log    # Redirect output to log file
pig pg start -O "-p 5433"         # Pass options to postgres
pig pg start -y                   # Force start (skip running check)
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--log` | `-l` | Redirect stdout/stderr to log file |
| `--timeout` | `-t` | Wait timeout (seconds) |
| `--no-wait` | `-W` | Don't wait for startup completion |
| `--options` | `-O` | Options to pass to postgres |
| `--yes` | `-y` | Force start (even if already running) |
{.full-width}


### pg stop

Stop PostgreSQL server.

```bash
pig pg stop                       # Fast shutdown (default)
pig pg stop -m smart              # Wait for clients to disconnect
pig pg stop -m immediate          # Immediate shutdown
```

**Options:**

| Option | Short | Default | Description |
|:---|:---|:---|:---|
| `--mode` | `-m` | fast | Shutdown mode: smart/fast/immediate |
| `--timeout` | `-t` | 60 | Wait timeout (seconds) |
| `--no-wait` | `-W` | false | Don't wait for shutdown completion |
{.full-width}

**Shutdown Modes:**

| Mode | Description |
|:---|:---|
| `smart` | Wait for all clients to disconnect |
| `fast` | Rollback active transactions, disconnect clients, clean shutdown |
| `immediate` | Terminate all processes immediately, requires recovery on next start |
{.full-width}


### pg restart

Restart PostgreSQL server.

```bash
pig pg restart                    # Fast restart
pig pg restart -m immediate       # Immediate restart
pig pg restart -O "-p 5433"       # Restart with new options
```

**Options:** Same as `pg stop`, plus `--options` (`-O`) to pass to postgres.


### pg reload

Reload PostgreSQL configuration. Sends SIGHUP signal to server.

```bash
pig pg reload                     # Reload configuration
pig pg reload -D /data/pg17       # Specify data directory
```


### pg status

Show PostgreSQL server status. Displays not only `pg_ctl status` output, but also postgres processes and Pigsty-related service status.

```bash
pig pg status                     # Check service status
pig pg status -D /data/pg17       # Specify data directory
```

**Output includes:**

1. `pg_ctl status` output (running status, PID, etc.)
2. PostgreSQL process list (`ps -u postgres`)
3. Related service status:
   - `postgres`: PostgreSQL systemd service
   - `patroni`: Patroni HA manager
   - `pgbouncer`: Connection pooler
   - `pgbackrest`: Backup service
   - `vip-manager`: VIP manager
   - `haproxy`: Load balancer


### pg promote

Promote replica to primary.

```bash
pig pg promote                    # Promote replica
pig pg promote -D /data/pg17      # Specify data directory
pig pg promote --plan             # Preview local-only promotion
pig pg promote -y                 # Skip confirmation prompt
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--timeout` | `-t` | Wait timeout (seconds) |
| `--no-wait` | `-W` | Don't wait for promotion completion |
| `--yes` | `-y` | Skip confirmation prompt |
| `--plan` | | Preview local-only promotion without executing |
{.full-width}

**Boundary:** `pg promote` is a local `pg_ctl promote` primitive. It does not coordinate Patroni, DCS, VIPs, replicas, or client routing. If Patroni is active, the command warns about this risk. Use `pig pt switchover`/`pig pt failover` or `pig pitr` for managed cluster workflows.


### pg role

Detect PostgreSQL instance role (primary or replica).

```bash
pig pg role                       # Output: primary, replica, or unknown
pig pg role -V                    # Verbose output, show detection process
pig pg role -D /data/pg17         # Specify data directory
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--verbose` | `-V` | Show detailed detection process |
{.full-width}

**Output:**

- `primary`: Current instance is primary
- `replica`: Current instance is replica
- `unknown`: Cannot determine instance role

**Detection Strategy (by priority):**

1. **Process detection**: Check for `walreceiver`, `recovery` processes
2. **SQL query**: Execute `pg_is_in_recovery()` (requires PostgreSQL running)
3. **Data directory check**: Check for `standby.signal`, `recovery.signal`, `recovery.conf` files


## Connection & Query Commands

### pg psql

Connect to PostgreSQL database via psql.

```bash
pig pg psql                       # Connect to postgres database
pig pg psql mydb                  # Connect to specific database
pig pg psql mydb -c "SELECT 1"    # Execute single command
pig pg psql -f script.sql         # Execute SQL script file
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--command` | `-c` | Execute single SQL command |
| `--file` | `-f` | Execute SQL script file |
{.full-width}


### pg ps

Show PostgreSQL current connections. Queries `pg_stat_activity` view.

```bash
pig pg ps                         # Show client connections
pig pg ps -a                      # Show all connections (including system)
pig pg ps -u admin                # Filter by user
pig pg ps -d mydb                 # Filter by database
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--all` | `-a` | Show all connections (including system) |
| `--user` | `-u` | Filter by user |
| `--database` | `-d` | Filter by database |
{.full-width}


### pg kill

Terminate PostgreSQL connections. **Default is dry-run mode**, requires `-x` to execute.

```bash
pig pg kill                       # Show connections to be terminated (dry-run)
pig pg kill --pid 12345           # Show that PID only (dry-run)
pig pg kill -x                    # Actually terminate connections
pig pg kill --pid 12345 -x        # Terminate specific PID
pig pg kill -u admin -x           # Terminate user's connections
pig pg kill -d mydb -x            # Terminate database connections
pig pg kill -s idle -x            # Terminate idle connections
pig pg kill --cancel -x           # Cancel queries instead of terminating
pig pg kill -w 5 -x               # Repeat every 5 seconds
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--execute` | `-x` | Actually execute (default is dry-run) |
| `--pid` | | Terminate specific PID |
| `--user` | `-u` | Filter by user |
| `--database` | `-d` | Filter by database |
| `--state` | `-s` | Filter by state (idle/active/idle in transaction) |
| `--query` | `-q` | Filter by query pattern |
| `--all` | `-a` | Include replication connections |
| `--cancel` | `-c` | Cancel queries instead of terminating |
| `--watch` | `-w` | Repeat every N seconds |
| `--plan` | | Preview primitive plan without executing |
{.full-width}

**Safety:** `--pid` without `--execute/-x` only queries `pg_stat_activity`; it does not call `pg_cancel_backend` or `pg_terminate_backend`. `--state` and `--query` parameters are validated to accept only simple alphanumeric patterns, preventing SQL injection.


## Database Maintenance Commands

### pg vacuum

Vacuum database tables by executing maintenance SQL through `psql`.

```bash
pig pg vacuum                     # Vacuum current database
pig pg vacuum mydb                # Vacuum specific database
pig pg vacuum -a                  # Vacuum all databases
pig pg vacuum mydb -t mytable     # Vacuum specific table
pig pg vacuum mydb -n myschema    # Vacuum tables in schema
pig pg vacuum mydb --full         # VACUUM FULL (requires exclusive lock)
pig pg vacuum mydb --full -y      # Skip VACUUM FULL confirmation prompt
pig pg vacuum mydb --full --plan  # Preview VACUUM FULL impact
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--all` | `-a` | Process all databases |
| `--schema` | `-n` | Specify schema |
| `--table` | `-t` | Specify table |
| `--verbose` | `-V` | Verbose output |
| `--full` | `-F` | VACUUM FULL (requires exclusive lock) |
| `--yes` | `-y` | Skip VACUUM FULL confirmation prompt |
| `--plan` | | Preview vacuum plan without executing |
{.full-width}

**Safety:** `VACUUM FULL` rewrites relations and requires exclusive locks, so it requires confirmation in text mode and `--yes` in JSON/YAML mode. When `--all` processes multiple databases, Pig attempts every database and returns a partial-failure error if any database fails.

**Security:** `--schema` and `--table` parameters are validated for proper PostgreSQL identifier format.


### pg analyze

Analyze database tables to update statistics.

```bash
pig pg analyze                    # Analyze current database
pig pg analyze mydb               # Analyze specific database
pig pg analyze -a                 # Analyze all databases
pig pg analyze mydb -t mytable    # Analyze specific table
```

**Options:** Same as `pg vacuum` (without `--full`).


### pg freeze

Freeze vacuum database to prevent transaction ID wraparound.

```bash
pig pg freeze                     # Freeze current database
pig pg freeze mydb                # Freeze specific database
pig pg freeze -a                  # Freeze all databases
```

**Options:** Same as `pg analyze`.


### pg repack

Online table repacking. Requires `pg_repack` extension.

```bash
pig pg repack mydb                # Repack all tables in database
pig pg repack -a                  # Repack all databases
pig pg repack mydb -t mytable     # Repack specific table
pig pg repack mydb -n myschema    # Repack tables in schema
pig pg repack mydb -j 4           # Use 4 parallel jobs
pig pg repack mydb --plan         # Show tables to be repacked
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--all` | `-a` | Process all databases |
| `--schema` | `-n` | Specify schema |
| `--table` | `-t` | Specify table |
| `--verbose` | `-V` | Verbose output |
| `--jobs` | `-j` | Number of parallel jobs (default 1) |
| `--plan` | `-N` | Show tables to be repacked |
{.full-width}


## Log Commands

Log commands view PostgreSQL CSV log files. Default log directory is `/pg/log/postgres`, can be changed via `--log-dir`. The default `pg log` action shows the latest CSV log snapshot; use `pg log -f` or `pg log tail` for real-time output. Use `-o json` to convert CSV log rows to JSONL records; `yaml` and `json-pretty` are not supported for log snapshots.

**Log Command Global Options:**

| Option | Description |
|:---|:---|
| `--log-dir` | Log directory path (default: `/pg/log/postgres`) |
| `-n, --lines` | Positive number of lines to show (default: `50`) |
| `-f, --follow` | Follow latest log output |
{.full-width}


### pg log

Show the latest PostgreSQL log snapshot, or follow it with `-f`.

```bash
pig pg log                        # Show latest 50 lines
pig pg log -n 100                 # Show latest 100 lines
pig pg log -f                     # Follow latest log
```

**Permission Handling:** If current user lacks permission to read log directory, command automatically retries with `sudo`.


### pg log list

List log files in log directory.

```bash
pig pg log list                              # List logs in default directory
pig pg log list --log-dir /var/log/postgres  # List logs in specified directory
```


### pg log tail

Real-time log viewing (like `tail -f`). Default views latest CSV log file.

```bash
pig pg log tail                   # View latest log
pig pg log tail postgresql.csv    # View specific log file
pig pg log tail -n 100            # Show last 100 lines then follow
pig pg log tail --log-dir /var/log/postgres  # Use custom directory
```

**Options:**

| Option | Short | Default | Description |
|:---|:---|:---|:---|
| `--lines` | `-n` | 50 | Positive number of lines to show |
{.full-width}


### pg log show

Show log file content.

```bash
pig pg log show                   # Output latest log
pig pg log show -n 100            # Output last 100 lines
pig pg log show postgresql.csv    # Output specific log file
```

**Options:**

| Option | Short | Default | Description |
|:---|:---|:---|:---|
| `--lines` | `-n` | 50 | Positive number of lines to show |
{.full-width}


### pg log less

Open log file with less. Defaults to end of file (`+G`).

```bash
pig pg log less                   # Open latest log with less
pig pg log less postgresql.csv    # Open specific log file
```


### pg log grep

Search log files.

```bash
pig pg log grep ERROR             # Search for ERROR lines
pig pg log grep --ignore-case error  # Case insensitive
pig pg log grep -C 3 ERROR        # Show 3 lines context
pig pg log grep ERROR pg.csv      # Search specific log file
```

**Options:**

| Option | Short | Description |
|:---|:---|:---|
| `--ignore-case` |  | Case insensitive |
| `--context` | `-C` | Show N lines of context |
{.full-width}


## pg svc Subcommand

`pg svc` provides systemctl-based PostgreSQL service management:

```bash
pig pg svc start                 # Start postgres service
pig pg svc stop                  # Stop postgres service
pig pg svc restart               # Restart postgres service
pig pg svc reload                # Reload postgres service
pig pg svc status                # Show service status
```

**Alias Reference:**

| Command | Alias |
|:--------|:------|
| `pg svc start` | `boot, up` |
| `pg svc stop` | `halt, dn, down` |
| `pg svc restart` | `reboot, rt` |
| `pg svc reload` | `rl, hup` |
| `pg svc status` | `st, stat` |
{.full-width}


## Design Notes

**Relationship with Native Tools:**

`pig pg` is not a simple wrapper of PostgreSQL native tools, but a higher-level abstraction for common operations:

- Service control commands (init/start/stop/restart/reload/promote) call `pg_ctl` or `systemctl`
- `status` command shows process and related service status beyond `pg_ctl status`
- Connection management commands (psql/ps/kill) call `psql`
- Maintenance commands (vacuum/analyze/freeze) execute SQL through `psql`
- repack command calls `pg_repack`
- Log commands call system tools like `tail`, `less`, `grep`

For full native tool functionality, call the respective commands directly.

**Security Considerations:**

- `--state`, `--query`, `--schema`, `--table` parameters are validated to prevent SQL injection
- `pg kill` defaults to dry-run mode to prevent accidents
- Log commands auto-retry with sudo when permissions insufficient

**Platform Support:**

This command is designed for Linux systems, some features depend on `systemctl` and `journalctl`.

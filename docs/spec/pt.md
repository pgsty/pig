---
title: "pig patroni"
description: "Run patronictl transparently with Pigsty service and config helpers"
weight: 170
icon: fas fa-infinity
module: [PIG]
categories: [Reference]
---

# `pig pt` Design Contract

Status: **normative; implemented**

`pig patroni` (alias `pig pt`) is a transparent launcher for the installed
`patronictl` binary. Pig does not copy Patronictl's command tree, flags,
argument validation, confirmation policy, output model, or version-specific
behavior.

Pig adds only these local workflows:

1. deterministic config-file selection for every Patronictl-backed invocation;
2. `pt set` as a small PostgreSQL/Patroni configuration sugar;
3. `pt start`, `pt stop`, and the `pt service` / `pt svc` family;
4. `pt status` for combined local and cluster status;
5. `pt log` for local Patroni logs.

Everything else is passed to Patronictl without maintaining an allowlist of
upstream commands or subcommand flags.

## Boundary and non-goals

The cluster-control path has one implementation and one authority:

```text
pig pt config selection and local dispatch
        |
        v
installed patronictl + one selected config
        |
        +-- Patroni REST API
        +-- configured DCS
        +-- PostgreSQL (query)
```

Pig does not:

- embed `go-patroni`, a DCS client, another REST control engine, or `etcdctl`;
- gate behavior on the installed Patronictl version;
- maintain a Patronictl command inventory or per-command option structs;
- parse or normalize forwarded native-command output into command-specific
  DTOs;
- add confirmation, `--force`, preflight checks, retries, or post-state reads to
  forwarded commands;
- validate Patroni dynamic-configuration relationships;
- retain Pig aliases for upstream commands such as `ls`, `so`, or `fo`;
- register a special `pt config` command.

## Command grammar

```text
pig pt [PIG_PT_OPTIONS] <native-command> [native-arguments...]
pig pt set <key=value>... [--yes] [--plan]
pig pt start|stop
pig pt service|svc <start|stop|restart|reload|status>
pig pt status
pig pt log ...
pig pt -- <native-command> [native-arguments...]
```

The first command token determines dispatch:

- `set`, `start`/`up`, `stop`/`dn`, `service`/`svc`, `status`/`st`, and
  `log`/`l` select Pig's local implementation;
- every other token selects transparent Patronictl forwarding;
- a leading `--` after `pt` disables local-name recognition and provides a
  collision escape hatch, for example `pig pt -- set ...`;
- `pig pt` with no command prints Pig's `pt` help and does not run Patronictl;
- `pig pt --help` prints Pig help;
- native help uses `pig pt <native-command> --help`; Patronictl root help uses
  `pig pt -- --help`.

`pt` is a runnable Cobra parent with `cobra.ArbitraryArgs`. Its flag set uses
non-interspersed parsing. Pig/PT options before the first native command token
are consumed; the native command and every subsequent token are retained
verbatim and in order:

```bash
pig pt list pg-test --watch 5 --format json
pig pt query pg-test -c 'select now()' -d postgres -U dbuser
```

This design must not use Cobra/pflag's unknown-flag allowlist. That mode drops
unknown flags and, in some forms, their following values.

Pig models the small Patronictl root-option surface listed below because those
options precede the command token. Native subcommand options are never modeled.
If a future Patronictl release adds an unknown root option, callers can pass the
entire native form through the collision escape hatch:

```bash
pig pt -- --future-root-option VALUE native-command ...
```

Shell completion covers Pig's local commands and wrapper options only. Pig does
not copy Patronictl's dynamic completion inventory.

## Pig/PT options

| Option | Meaning |
|:--|:--|
| `-c`, `--config-file PATH` | Explicit Patronictl configuration file |
| `--dbsu USER` | OS user used to execute Patronictl |
| `-d URL`, `--dcs-url URL`, `--dcs URL` | Patronictl root DCS override |
| `-k`, `--insecure` | Patronictl root TLS-verification override |

These options must precede the native command. This prevents Pig from stealing
same-named native subcommand flags:

```bash
pig pt -c /path/patroni.yml list   # wrapper/root config option
pig pt query -c 'select 1'         # native query SQL option
```

Normal Pig root options also belong before the native command. Once the native
command token is encountered, remaining tokens belong exclusively to
Patronictl.

## Config-file resolution

Every Patronictl invocation receives exactly one Pig-selected root `-c PATH`.
The precedence is normative:

1. explicit `-c/--config-file` supplied before the command;
2. non-empty `PATRONICTL_CONFIG_FILE`;
3. `/etc/patroni/patroni.yml`, when it exists and is readable;
4. `/infra/conf/patronictl.yml`, when it exists and is readable;
5. `/etc/patroni/patroni.yml` without claiming it exists, so Patronictl's error
   names the conventional target.

Explicit and environment paths are authoritative. Pig does not silently
replace them when they are missing or unreadable. Relative explicit and
environment paths are converted to absolute paths before changing OS user.

The two conventional candidates are checked for actual readability as the
resolved DBSU, because that is the identity that consumes the selected file.
When Pig already runs as DBSU it can check directly. Otherwise one fixed,
read-only DBSU probe checks `/etc` first and `/infra` second; Pig must not infer
access from Unix mode bits. Failure to start the DBSU probe is an execution
error, not evidence that both files are unreadable.

Resolution is lazy. Pure systemd operations such as `pt svc start` do not
resolve Patronictl, switch to DBSU, or probe either config file. Native
forwarding, `set`, `status`, and config-aware log operations resolve only when
they consume the selected config.

| State | Selected path |
|:--|:--|
| explicit path present | explicit path |
| no explicit, environment non-empty | environment path |
| both conventional files readable | `/etc/patroni/patroni.yml` |
| only `/etc` readable | `/etc/patroni/patroni.yml` |
| only `/infra` readable | `/infra/conf/patronictl.yml` |
| neither readable | `/etc/patroni/patroni.yml` fallback |

The resulting logical invocation is:

```text
patronictl -c SELECTED_PATH [--dcs-url URL] [--insecure] <native argv...>
```

## Transparent execution

For a forwarded native command Pig must:

- resolve `patronictl` from `PATH` before changing OS user;
- execute as explicit `--dbsu`, then `$PIG_DBSU`, then `postgres`;
- preserve native argv spelling and order after the command token;
- directly inherit stdin, stdout, stderr, and terminal file descriptors;
- preserve native prompts, editor behavior, streaming output, and exit status,
  including Click usage exit status `2`;
- avoid output capture, duplicate error rendering, command hints, full-argv
  logging, success-text inspection, and retries;
- avoid reading cluster or DCS state before or after execution.

The forwarding path therefore uses a dedicated DBSU execution primitive. It
must not reuse a helper that inserts capture buffers or changes terminal
identity. Pig may prepend only the selected config and explicit wrapper-level
DCS/TLS options.

If `patronictl` cannot be resolved, Pig returns `CodePtNotFound`. Once the
binary starts, Patronictl owns its diagnostics and exit status.

### Output modes

Transparent forwarding supports Pig's `text` output mode only. A Pig root
`-o json`, `-o yaml`, or `-o json-pretty` placed before the native command is
rejected before execution, with guidance to use Patronictl's native output
option for that command.

Pig's early output-mode scan must stop at the native command token. Therefore:

```bash
pig -o json pt list             # rejected: Pig structured output requested
pig pt -o json list             # rejected: Pig structured output requested
pig pt list --format json       # forwarded unchanged
pig pt list -o json             # forwarded unchanged; Patronictl validates it
```

Local `set`, service, status, and log workflows retain their documented Pig
output behavior. The native forwarding path never creates a generic Pig
command envelope.

## `pig pt set`

`set` edits the cluster selected by the resolved configuration:

```text
pig pt set KEY=VALUE [KEY=VALUE ...] [--yes] [--plan]
```

It accepts one or more complete `key=value` tokens. The key is the non-empty,
trimmed substring before the first `=`. The remaining value is preserved,
including additional `=` characters. Shell quoting remains the caller's
responsibility.

### Key classification

The following scalar Patroni dynamic keys become repeated native `--set`
arguments:

```text
loop_wait
ttl
retry_timeout
primary_race_backoff
maximum_lag_on_failover
maximum_lag_on_syncnode
max_timelines_history
primary_start_timeout
primary_stop_timeout
synchronous_mode
synchronous_mode_strict
synchronous_node_count
failsafe_mode
check_timeline
member_slots_ttl
```

This small list covers Pigsty's RTO/RPO and synchronous-replication controls,
plus Patroni's documented top-level scalar controls. `pause` is deliberately
excluded because `patronictl pause` and `resume` own that workflow.

Every other key is treated as a PostgreSQL parameter and becomes a native
`--pg` argument. This includes custom GUCs containing dots, such as
`timescaledb.telemetry_level`.

To prevent known structural Patroni paths from being silently written as
PostgreSQL GUCs, `set` rejects dotted keys beginning with these namespaces:

```text
postgresql.
standby_cluster.
slots.
ignore_slots.
```

The error points to native `pig pt edit-config --set`. Pig does not attempt to
support structural, list, map, version-specific, Citus-group, or non-default
cluster Patroni configuration through this sugar.

Classification uses the exact trimmed key and never rewrites the caller's
token.

### Translation and execution

All pairs are translated into one native `edit-config` call, preserving input
order:

```bash
pig pt set ttl=60 max_connections=200 synchronous_mode=on

# logical native invocation
patronictl -c SELECTED edit-config \
  --set ttl=60 \
  --pg max_connections=200 \
  --set synchronous_mode=on
```

This produces one Patronictl diff, confirmation, concurrency check, and DCS
update. Pig does not pre-read the DCS or reimplement relationships such as:

```text
loop_wait + 2 * retry_timeout <= ttl
```

Patronictl parses values as YAML. Consequently both `KEY=null` and `KEY=`
remove a key; Pig passes either spelling unchanged.

`--yes/-y` appends native `--force`. Without it Patronictl displays the diff
and owns confirmation. `--plan` runs no Patronictl mutation and renders the
selected config plus the translated, shell-quoted native argv. If `--yes` is
also present it has no effect on the plan.

In structured Pig output, execution requires `--yes` because hidden prompts
are forbidden. The result uses the existing generic local-command envelope;
`--plan` remains side-effect-free.

After Patronictl returns success, Pig analyzes only keys routed through `--pg`.
Patronictl also returns success when an interactive change is declined or the
configuration is already unchanged, so Pig phrases all follow-up guidance
conditionally. Known postmaster-context parameters retain the current restart
warning and first direct the operator to inspect members. Because native
`patronictl restart` requires an explicit cluster, Pig describes the follow-up
syntax but does not emit it as an incomplete replayable command:

```text
pig pt list
pig pt restart CLUSTER --pending
```

Advanced configuration remains available transparently:

```bash
pig pt edit-config --set postgresql.use_pg_rewind=true
pig pt edit-config --set standby_cluster.host=10.0.0.1
pig pt edit-config pg-other --group 1 --set ttl=60
```

### Migration from `pt config`

`pt config` is removed immediately and has no hidden compatibility alias:

```text
pt config pg K=V   -> pt set K=V
pt config set K=V  -> pt set K=V for one of the Patroni scalar keys
pt config show     -> pt show-config
pt config edit     -> pt edit-config
```

An unrecognized `pig pt config ...` invocation is forwarded to Patronictl, so
native behavior remains authoritative if Patronictl ever adds that command.

## Local service workflow

`pt service` is the canonical local namespace and `pt svc` is its alias:

```bash
pig pt svc start
pig pt svc stop
pig pt svc restart
pig pt svc reload
pig pt svc status
```

Hidden top-level `pt start` and `pt stop` shortcuts call the same local
implementation. Top-level `pt restart` is never a daemon shortcut; it is
forwarded to `patronictl restart`.

The existing local aliases remain: `start`/`up`, `stop`/`dn`, and service-child
aliases `up`, `dn`, `rs`, `rl`, and `st`. Service helpers operate the local
`patroni` systemd unit and retain their existing structured local-command
results.

## Local status and logs

`pt status` retains the existing combined local workflow: systemd service
state, Patroni processes, and cluster members from Patronictl. Its local alias
is `pt st`.

`pt log` retains the v1.5.1-style local log workflow:

```bash
pig pt log -n 100
pig pt log -f
pig pt log show -o json
pig pt log tail -n 100
pig pt log grep -n 500 ERROR
```

The log directory comes from `log.dir` in the selected config and falls back
to `/pg/log/patroni`. Follow/tail and grep remain terminal streams; structured
output supports bounded snapshots only. Service and log helpers are
Linux/systemd workflows, not Patronictl emulation.

The existing local log aliases remain: top-level `log`/`l`, `show`/`cat`/`c`/`s`,
`tail`/`t`/`f`/`follow`, and `grep`/`g`/`search`.

## Debug logging and sensitive data

At debug level Pig records config-selection decisions to stderr or the selected
log sink:

```text
patronictl config selected: source=explicit path=/path/patroni.yml
patronictl config candidate skipped: path=/etc/patroni/patroni.yml reason=not-readable
patronictl config selected: source=infra path=/infra/conf/patronictl.yml
patronictl config fallback: path=/etc/patroni/patroni.yml reason=no-readable-candidate
```

Normal output does not announce the injected config. Logs and hints must not
contain full native argv, SQL, DCS URLs, or `set` values. `set --plan` is the
explicit exception: it displays caller-supplied configuration values and must
warn that plans may contain sensitive data.

## Compatibility consequences

This contract intentionally changes the current implementation:

- newly installed Patronictl commands and subcommand flags work without a Pig
  release;
- native help, validation, confirmation, output, and errors are authoritative;
- command-specific DTOs and safety gates for forwarded Patronictl commands,
  Pig aliases, automatic target/scope selection, and rewritten flags are
  removed;
- Pig structured output is unavailable for forwarded native commands;
- scripts must use native Patronictl syntax, output options, and exit behavior;
- `pt config` is replaced by `set`, `show-config`, and `edit-config`;
- config selection adds a readable `/infra/conf/patronictl.yml` fallback while
  preserving `/etc/patroni/patroni.yml` priority;
- local `start`, `stop`, `service/svc`, `status`, and `log` remain stable.

## Acceptance tests

Implementation is acceptable only when tests prove:

1. unknown native commands and native subcommand flags reach the runner
   unchanged;
2. native argument order, spaces, quotes, and additional `=` characters survive
   both direct and `su -c` execution;
3. native `query -c SQL` is not consumed as the wrapper config option;
4. wrapper `-c/--config-file` works before the native command;
5. a future unknown native root option works through the leading `--` escape;
6. `--` bypasses every local-name and alias collision, including `pt -- set`
   and `pt -- st`;
7. `pt` with no args, Pig help, native subcommand help, and native root help
   follow this contract;
8. config resolution covers explicit, environment, both-readable, etc-only,
   infra-only, and neither-readable cases in the normative order;
9. conventional readability is evaluated as DBSU, and probe-launch failure is
   not treated as an unreadable candidate;
10. relative explicit/environment paths become absolute before DBSU execution;
11. debug logs describe selection without logging native argv or values;
12. native stdin/stdout/stderr, TTY behavior, and exit codes `1` and `2` are
    preserved without duplicate diagnostics;
13. Pig non-text `-o` before a native command is rejected, while output-looking
    flags after the command are forwarded unchanged;
14. `set` rejects zero pairs, empty keys, and malformed tokens before execution;
15. every scalar Patroni key maps to `--set`, while ordinary and dotted custom
    GUCs map to `--pg`;
16. `postgresql.use_pg_rewind`, `standby_cluster.host`, `slots.*`, and
    `ignore_slots.*` are rejected with native `edit-config --set` guidance;
17. mixed keys produce one ordered `edit-config` invocation;
18. only `--yes` adds `--force`; default text execution leaves confirmation to
    Patronictl;
19. `--plan` does not mutate and renders the same translated logical argv;
20. `null`, empty values, and values containing `=` are passed unchanged;
21. restart hints analyze only PG-routed keys, require an explicit `CLUSTER`,
    and never emit the invalid `pig pt restart --pending` command;
22. `start`, `stop`, `service/svc`, `status`, and `log` select local code, while
    top-level `restart` forwards to Patronictl;
23. `pt config` is not registered locally and therefore follows passthrough
    behavior.

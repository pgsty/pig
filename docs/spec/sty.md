---
title: "pig sty"
description: "Normative design contract for Pigsty installation and IaC management"
weight: 150
icon: fas fa-server
module: [PIG]
categories: [Reference]
---

# `pig sty` Design Contract

Status: **normative; native controller bootstrap, Inventory/CMDB migration, and native Grafana management implemented**

Contract version: **1.8**

Last updated: **2026-08-14**

This document is the design contract for the `pig sty` installation and
controller command family. Sty consumes the lossless Inventory API, but the
Inventory model, selectors, validation, Check, redaction, and CMDB contracts
are owned exclusively by [`inventory.md`](inventory.md).

The top-level [`pig inventory` contract](inventory.md) is the authority for the
public Inventory command tree, the thin adapter to Pigsty's existing CMDB, and
migration from older Sty paths. A planned command is not a promise that the
command already exists; its migration gate must pass before it is advertised
as stable.

The former [implementation goal prompt](sty-implementation-goal.md) records the
historical P0–P2 execution objective and is archival; its greenfield CMDB phase
is explicitly superseded. New root-command work MUST follow the delivery plan
and gates in
[`inventory.md`](inventory.md#11-delivery-plan-and-explicit-non-goals).

Normative words such as **MUST**, **MUST NOT**, **SHOULD**, and **MAY** have their
usual RFC 2119 meaning.


## 1. Product boundary and priority

`pig sty` manages the local Pigsty installation and a small number of directly
related controller-side services. Configure and Grafana management consume the
active Inventory, while generic Inventory operations and Check belong to the
root `pig inventory|inv` domain. Sty is not a generic replacement for Ansible,
Grafana, PostgreSQL, Terraform, or Vagrant.

Implementation priority is fixed as follows:

1. **P0 — Inventory first.** The lossless Inventory, root command, Validate,
   and Check gates passed.
2. **P1 — Grafana dashboards.** The native HTTP implementation remains under
   `sty`.
3. **P2 — CMDB adapter.** The former revision-store direction was withdrawn;
   the implemented surface is a thin adapter to existing `cmdb.sql` and the
   Inventory scripts.
4. **P3 — Vagrant and Terraform.** Thin adapters only, after explicit
   reprioritization or completion of the earlier gates.

The following are outside this roadmap:

- application lifecycle management;
- application Docker/Compose management, because applications may be remote;
- an unspecialized `pig sty docker` command until its target semantics receive
  a separate design;
- migrating node-side Ansible helpers or application entrypoints merely to
  reduce shell line count.


## 2. Implementation status

| Surface | Status at contract v1.8 |
|:---|:---|
| native `sty boot` | Implemented |
| Lossless `inventory.Document`, ordered semantic model, selectors, editor, mutations, diagnostics | Implemented baseline |
| `sty conf/configure` through Inventory API, with `--raw` fallback | Implemented |
| staged `sty init`, fail-closed Pro EULA, atomic release download | Implemented |
| `sty deploy` invocation-consent immediate execution | Implemented |
| `sty edit|e [selector]` | Removed after migration to root Inventory |
| `sty validate|val|v` | Removed after migration to root Inventory |
| root `inventory|inv` | Implemented through the v1.3 P0-P2 contract |
| `sty check|ck` | Removed after migration to root Inventory |
| root `inventory check|ck` | Implemented |
| `sty grafana|gf info/list/boot/load/init/dump/clean/lang/style` | Implemented |
| root `inventory cmdb` thin adapter | Implemented with Check, plans, confirmation, dump, and source guards |
| existing Pigsty `cmdb.sql` and Inventory scripts | Current CMDB authority |
| former `pigsty_cmdb` revision-store surface | Removed |
| `sty vagrant`, `sty terraform` | Deferred P3 |
| `sty doctor` | Name reserved; implementation deferred |


## 3. Command taxonomy and compatibility

### 3.1 Sty stable surface

Sty retains installation and directly related controller-service commands.
Inventory edit, validate, and Check have no Sty compatibility aliases:

```text
pig sty
  init | i
  boot | b | bootstrap
  conf | c | configure
  deploy | d | de

  list | l | ls                       # canonical release listing
  get | g | download                  # canonical release download

  grafana | gf
    info | list | boot
    init [path] | load [path] | dump [path] | clean [path]
    lang <language-code>
    style <dark|light|auto>

  doctor | dc                         # reserved, not implemented in P0-P2
  vagrant | vg                        # deferred P3
  terraform | tf                      # deferred P3
```

The released `sty list/l/ls` and `sty get/g/download` spellings are the sole
release-management surface. They preserve their 1.x command identifiers and
structured payloads; Sty has no nested `release` namespace.

### 3.2 Root Inventory migration promise

The canonical Inventory spelling is now outside Sty:

```text
pig inventory | inv status
pig inventory | inv list | ls [selector] [--depth N]
pig inventory | inv show [selector]
pig inventory | inv edit | e [selector] [--from FILE|-]
pig inventory | inv validate | v [file] [--strict] [--ansible] [--timeout DURATION]
pig inventory | inv check | ck [--profile PROFILE] [--sudo] [--port PORT] [--user USER]
pig inventory | inv diff <other-file>
pig inventory | inv cmdb
  check [file] [-d DATABASE]
  init [-d DATABASE] [--plan] [--yes|-y]
  load [file] [-d DATABASE] [--plan] [--yes|-y] [--strict]
  dump [file] [-d DATABASE] [--force|-f]
  enable [-d DATABASE] [--plan]
  disable [--plan]
```

Pig v1.6.0 released `sty edit/validate`. After their canonical root paths
shipped, both were deliberately retired; `sty check|ck` was likewise removed
after `inventory check|ck` shipped. The unreleased `sty inventory` and
`sty cmdb` namespaces are also absent rather than becoming permanent aliases.
The exact mapping and compatibility decision are defined by
[`inventory.md`](inventory.md#9-compatibility-and-release-contract).

`sty list/l/ls` MUST NEVER be repurposed to mean Inventory list. It remains the
canonical release listing, while `pig inv ls` provides the short Inventory
spelling without changing an existing command's meaning.

### 3.2.1 Native controller bootstrap

`pig sty boot` is a thin native coordinator. It requires only the platform
package manager (`dnf`, EL7 `yum`, or `apt-get`) and reuses Pig's existing
repository configuration, package installation, extension-alias resolution,
sudo, and Result-envelope implementations. Go code supplies HTTP(S) download
and restricted `tar.gz` extraction because a minimal host may lack
`curl`, `wget`, `tar`, and `gzip`. Archive extraction MUST stream through Go's
`compress/gzip` and `archive/tar`; Bootstrap MUST NOT probe, install, repair, or
invoke host `tar`/`gzip`, including on Debian-family systems.

Source handling has three reported modes: `offline` uses a selected package or
a committed `/www/pigsty/repo_complete`; `online` adds the existing `node` and
`pigsty` repository modules; and `existing` reports the compatibility fallback
when a `--keep` repository cache refresh fails but the configured host
repositories can still satisfy installation. The `pigsty` module MUST contain
both `pigsty-infra` (including SOW) and `pigsty-pgsql`. Matching the legacy
bootstrap contract, existing repository definitions are backed up and replaced
by default before the complete bootstrap set is added. Only explicit `--keep`
preserves them. Offline package transactions MUST be restricted to
`pigsty-local`. A committed local repository is preserved and reused; if a
package is also selected, the result MUST warn that it was not unpacked and
MUST NOT report it as the package used.

Replacement-style repository configuration is atomic at the active definition
boundary. Online region detection happens before active definitions are moved.
When an online module cannot be written or its package-manager cache refresh
fails, Pig MUST remove definitions created by that failed attempt and restore
every path recorded in `BackupInfo`. Offline extraction MUST commit only the
repository tree: it MUST NOT write `local.repo`/`local.list` until existing
definitions have been backed up. Failure to write that local definition or to
complete its strictly local cache/package transaction likewise restores the
recorded definitions. Shared repository Results expose `rollback_info`;
Bootstrap summarizes it as `repository_rollback=restored|failed`. Additive
`--keep` behavior does not remove or roll back pre-existing definitions.

`repo_complete` establishes only that a repository tree was committed. It does
not prove that repository metadata and package dependencies are consumable.
Bootstrap MUST therefore run its controller package transaction even when the
controller already appears ready. In offline mode both cache refresh and package
installation MUST remain restricted to `pigsty-local`; failure to resolve or
install Ansible and its dependencies from that scope is a Bootstrap failure.
The generated `/etc/yum.repos.d/local.repo` MUST set `module_hotfixes=1` on
EL8 and EL9 so DNF modular filtering cannot hide packages from the offline
repository; EL7 and EL10 omit that setting.

Online bootstrap installs Pigsty's embedded signing key and renders the two
Pigsty repositories with signature verification enabled, so APT must not depend
on `apt-key`, curl, or wget and must not emit a missing Pigsty-key warning.

Controller bootstrap MUST use the narrow package set required by the legacy
`check_utils` responsibility, plus SOW, the platform OpenSSH client, Python
requests, jmespath, and cryptography. It MUST NOT expand the broader
`node-bootstrap` alias or install node tuning, performance, kernel,
kernel-header, kernel-image, or ZFS packages. In particular, Ubuntu bootstrap
MUST NOT request `linux-tools-generic`, and no platform may request `tar` or
`gzip` as a Bootstrap dependency. Final verification MUST execute
`ansible-playbook` and probe `yaml`, `jinja2`, `jmespath`, `requests`, and either
`cryptography` or `OpenSSL` using the Python interpreter that actually launches
Ansible, rather than assuming the system `python3` is the same runtime. On EL8,
the explicit Jinja2, requests, jmespath, and cryptography packages therefore use
the `python3.12-*` namespace required by the packaged Ansible interpreter.
On Debian 12 and 13 it MUST also install the `locales` package, enable
`en_US.UTF-8 UTF-8` in `/etc/locale.gen`, run `locale-gen en_US.UTF-8`, set
`LANG` and `LC_ALL` through `update-locale`, and verify that the generated
locale is listed by `locale -a`. This repair does not apply to Ubuntu or EL.

After hard controller verification succeeds, Bootstrap MUST test key-based SSH
to the invoking administrator at `127.0.0.1`. Failure is non-fatal: Bootstrap
warns, authorizes an existing standard user key when available, or generates an
RSA key when none exists, and retries once. A failed repair remains a structured
warning and MUST NOT turn an otherwise successful Bootstrap into failure.

After controller and localhost-SSH checks, Bootstrap MUST inspect the invoking
administrator's default `~/pigsty` path. When Bootstrap was entered through
sudo, this means the account identified by `SUDO_UID`, not root's home. An
existing path is preserved without a network probe. If the path is absent and a
Pigsty source repository is reachable, Bootstrap runs the current executable's
`pig sty init` as that administrator so the resulting tree has user ownership.
An unreachable source leaves Bootstrap successful with a warning and a
`pig sty init` next action. Once automatic initialization starts, any failure is
a Bootstrap failure.

The conventional implicit `/tmp/pkg.tgz` is accepted only when it is a
non-group/world-writable regular file owned by root or the sudo caller.
Explicit `--path` accepts a local file or downloads an HTTP(S) URL before the
single sudo re-exec, including compact `-pVALUE` shorthand. Native output uses
`pig.sty.boot/v2`; the Boot command is the sole implementation and has no legacy
raw-script mode or dependency on a Pigsty `bootstrap` script.

### 3.3 Reserved names

`doctor` is reserved but not implemented by this contract. Instance-wide
Grafana deletion is available only through the explicit
`pig sty grafana clean --all` mode.

### 3.4 Lifecycle side effects and transactions

Bare `pig sty`, release listing, help, and all other Sty invocations MUST read
an existing Pig config when present but MUST NOT create `~/.pig` or
`~/.pig/config.yml` merely through command initialization. Native `sty conf` is
the only Sty path that performs controller CPU/OS discovery; raw Configure owns
its script probes, and other Sty commands skip them.

`sty get` downloads into a private sibling candidate. Pig flushes and closes
the candidate, verifies the published checksum when available, validates the
complete gzip/tar stream in every case,
then atomically renames it over the final file and syncs the parent directory.
An existing file is trusted without checksum metadata only when it is already a
complete archive. A failed or interrupted candidate MUST NOT truncate, delete,
or replace the existing target.

`sty init` validates the target and resolves Pigsty Pro EULA consent before
network discovery, download, target creation, or staging. Non-interactive Pro
installation fails closed unless `--accept-eula` is present. Installation
constructs the complete final tree in a private sibling directory. With
`--force`, it first clones the existing tree so user-created files,
`pigsty.yml`, and PKI material retain overlay semantics. A successful candidate
is committed by rename; a failed extraction leaves the target unchanged, and a
failed replacement attempts rollback to the prior directory.

`pig sty deploy` is the deliberate exception to prompt-based destructive
gating. Invoking the command is the consent boundary: after preflight it starts
Ansible immediately, without a Pig prompt or `--yes`. Its ANCS declaration MUST
remain `risk=high`, `confirm=none`, and `consent=invocation`; the warning and
help text MUST state this execution model.


## 4. Layering and code ownership

Repository layout MUST follow `AGENTS.md`:

- `cmd/sty.go` contains only Cobra commands, aliases, annotations, flags,
  argument validation, and option mapping for Sty. The command package stays
  flat.
- `cmd/inventory.go` owns the canonical root Inventory command glue; root
  registration remains in `cmd/root.go`.
- `cli/sty` owns Configure, installation/release lifecycle, and Grafana use
  cases plus their stable command DTOs.
- `cli/inventory` owns root Inventory, Validate, Check, and CMDB use cases plus
  their stable command DTOs. `cmd/inventory.go` MUST NOT import `cli/sty`.
- the top-level `inventory` package owns static Inventory parsing, the lossless
  document, semantic values, selectors, mutations, and validation primitives.
- focused internal adapters MAY own Grafana HTTP, Ansible subprocess, CMDB SQL,
  or external executable integration.

Do not build a universal environment-driver framework. Share only contracts
that have at least two concrete consumers. `Finding` and output/plan helpers are
valid shared concepts; Check probes remain private to Check until another real
consumer exists.


## 5. Inventory integration summary (non-normative)

This section only explains assumptions used by Sty Configure and Grafana.
[`inventory.md`](inventory.md) is the sole normative Inventory authority and
wins if any summary here drifts.

### 5.1 Two-layer model

Static YAML uses two deliberately separate layers:

- **`Document`** is the only lossless write source. It retains original bytes,
  node spans, comments, scalar spelling, padding, indentation, flow/block style,
  line endings, and document boundaries.
- **`Inventory` / `Value`** is an ordered semantic view of declared groups,
  hosts, children, vars, and values. Mapping entries use ordered slices rather
  than unordered Go maps.

No supported write path marshals the entire semantic model or YAML tree back to
the file. Existing scalar updates replace only their source lexeme; insertion,
deletion, rename, uncomment, and fragment replacement use minimal source
patches.

Unknown tags, anchors, aliases, and vault values MUST round-trip unchanged.
Direct edits that cannot be made losslessly MUST fail rather than normalize the
document.

### 5.2 Declaration scope, not effective vars

P0 models source declarations. It MUST NOT pretend to implement:

- Ansible multi-group precedence or `ansible_group_priority`;
- role defaults or external `group_vars/host_vars` directories;
- vault decryption;
- dynamic or directory Inventory;
- a final effective-vars resolver.

Rules MAY read declarations across scopes for precedence-free structural checks
such as uniqueness, duplicate membership, union-based references, or port
collisions. Rules MUST NOT compute override outcomes or report an effective
value as absent when inheritance may provide it. A future effective-vars
resolver is a separate design and must demonstrate parity with Ansible.

### 5.3 Inventory API direction

The shared package supplies typed paths and ordered access. Use cases SHOULD be
expressed through narrow helpers rather than raw text replacement, including:

- global vars: `all.vars`;
- group vars: `all.children.<group>.vars`;
- host vars: `all.children.<group>.hosts.<host>`;
- nested mapping/sequence values;
- set/upsert/delete/rename with optional leading comments;
- exact placeholder replacement across keys, active scalar lexemes, and comment
  tokens when explicitly requested by a template workflow;
- atomic mutation sessions with redacted change sets.

Comments are presentation data owned by `Document`, not semantic values. An API
that accepts a comment MUST define whether it is a leading standalone comment
or an inline comment and MUST preserve surrounding user comments.

### 5.4 Future effective-vars resolver

The lossless declaration model remains independent from any future Ansible
resolver. Effective values MUST be produced by a separate, read-only layer so
that resolving inheritance never rewrites or normalizes the source `Document`.

The resolver is delivered in stages:

1. an Ansible-backed oracle records `ansible-config` and `ansible-inventory`
   results for a pinned `ansible-core` version;
2. a native Inventory resolver reproduces source ordering, `all`/parent/child/
   host flattening, same-level alphabetical order, `ansible_group_priority`,
   multiple Inventory source order, the configured hash behavior, and the
   standard `host_group_vars` loading rules;
3. a role-context resolver overlays the selected role defaults and explicit
   play/role/task/extra-variable inputs required by one concrete use case;
4. differential fixtures compare every native result and its value provenance
   with the pinned Ansible oracle.

There is deliberately no context-free "final vars" map that silently includes
role defaults: role defaults and play/task variables are execution-context
inputs, not Inventory declarations. Each resolved value records its winning
source and overridden candidates. Vaults, arbitrary vars plugins, dynamic
Inventory plugins, Jinja execution, or unsupported Ansible configuration MUST
fall back to the Ansible adapter or fail explicitly; they MUST NOT be
approximated. Datasource projection remains out of scope until the relevant
host- and role-context parity corpus passes.

### 5.5 `pig inventory list` and `pig inventory show`

`pig inventory list [selector]` is safe for automation. It emits ordered paths,
object kinds, group/host topology, variable keys, and value kinds; it does not
emit arbitrary variable values. JSON/YAML output uses a versioned DTO.

For a static source, `pig inventory show [selector]` is a `cat`-equivalent view:

- no selector prints the original complete document;
- a selector prints the exact/dedented YAML fragment defined by fragment-edit
  semantics;
- output is verbatim and may contain passwords or other secrets;
- the command is text-only and MUST reject JSON/YAML structured mode with a
  parameter-class error;
- the body MUST NOT be copied to logs, diagnostics, or a structured Result.

A managed CMDB source has no original presentation bytes, so root `list` and
`show` do not pretend to support it. Canonical generated export is the explicit
`pig inventory cmdb dump` operation. A future `show --redact` MAY be additive,
but redacted output MUST NOT be called lossless.


## 6. Selector examples (non-normative)

The normative grammar and compatibility fixtures live in
[`inventory.md`](inventory.md#4-static-inventory-commands).

The existing selector grammar is stable:

- empty selector or `/` selects the whole document;
- `vars` means `/all/vars`;
- a leading name such as `pg-meta` means `/all/children/pg-meta`;
- under a group, `vars` and `hosts` are reserved container shorthands;
- other group segments omit `children`, so `parent.child.vars` resolves nested
  groups;
- `[0]` addresses a sequence index;
- `["quoted.key"]` addresses a key containing dots, brackets, IP punctuation,
  quotes, or other ambiguous characters;
- `/all/children/...` is the canonical JSON-Pointer-style spelling, including
  `~0` and `~1` unescaping.

Examples:

```text
vars.infra_portal
pg-meta
pg-meta.vars.pg_databases
pg-meta.vars.pg_databases[0]
pg-meta.hosts["10.10.10.10"]
/all/children/pg-meta/hosts/10.10.10.10
```

Ambiguous shorthand MUST fail; callers use the canonical pointer when a literal
key collides with shorthand resolution. Stable selector failures are:

- `SELECTOR_SYNTAX`;
- `SELECTOR_NOT_FOUND`;
- `SELECTOR_TYPE`.

IP-key, dotted-key, quoted-key, nested-group, index, and pointer escape fixtures
are normative compatibility tests.


## 7. Source integration summary (non-normative)

Normative source precedence and safety are defined by
[`inventory.md`](inventory.md#3-source-contract).

`-H/--home` resolves Pigsty home first. `-i/--inventory` always names a concrete
static YAML source for one invocation; it may live outside Pigsty home and never
persists a CMDB source switch.

When `-i` is absent, the target resolution order is:

1. `PIGSTY_INVENTORY`;
2. `inventory` in `~/.pig/config.yml`;
3. the exact Inventory directive deliberately parsed from
   `<resolved-pigsty-home>/ansible.cfg`;
4. `<resolved-pigsty-home>/pigsty.yml`.

The `ansible.cfg` Inventory directive is the authoritative persistent Ansible
source. Pig reads it deliberately; it never infers CMDB mode from database
reachability or incidental files. Unsupported dynamic/directory sources fail
with an explicit diagnostic instead of silently falling back. The value-free
source descriptor, exact relative-path bases, and local-only `inventory status`
contract are frozen in `inventory.md`.

Lossless and declaration-semantic operations currently require one static
`Document`. The known CMDB adapter is limited to the explicit `inventory cmdb`
commands. Database-generated YAML never claims lossless source presentation.
No generic Snapshot/source backend is part of the current contract.


## 8. Mutation integration summary (non-normative)

Normative mutation and file safety are defined by
[`inventory.md`](inventory.md#5-static-write-and-file-safety-contract).

For a static file, revision means the exact original bytes, represented by a
content hash or an equivalent byte comparison captured at read time.

Every edit MUST:

1. parse and validate the original source;
2. stage non-overlapping source changes transactionally;
3. reparse and validate the candidate;
4. re-read the destination immediately before commit and reject a revision
   mismatch;
5. write a sibling temporary file, flush it, rename atomically, and sync the
   parent directory when supported.

On conflict or validation failure, the destination remains untouched and no
partial transaction is exposed.

Additional requirements:

- symlink Inventory destinations are refused;
- editor directories are mode `0700`; fragments and candidates are `0600`;
- a new committed Inventory defaults to `0600`; an existing Inventory keeps any
  stricter owner-only mode, while Pig removes group/other access and never
  loosens permissions;
- a mode change produces one human notice;
- replacement ownership follows the writing process and this is documented;
- a failed interactive edit retains the private recovery fragment, prints its
  path, and leaves deletion to the user; it is never uploaded or logged;
- change sets contain identifiers and operation kinds, never old/new secret
  values.

Scope and consent are distinct. Flags such as `--prune` or `--reset` select
scope but never imply confirmation. Commands whose contract declares a Pig
confirmation gate require an interactive prompt or explicit `--yes`; a non-TTY
invocation without `--yes` fails with the confirmation-required status class.
The explicit `sty deploy` invocation is separately defined as immediate consent
in section 3.4 and MUST NOT synthesize such a gate.


## 9. Output, status, and redaction

### 9.1 Structured output

Pig IaC DTOs reuse the repository-wide Result envelope with `api: 1`. Every new
structured data payload MUST also carry a stable `kind`, for example:

```text
pig.sty.init/v1
pig.sty.boot/v2
pig.sty.configure/v1
pig.sty.configure.raw/v1
pig.sty.deploy/v1
pig.sty.release.list/v1
pig.sty.release.get/v1
pig.inventory.check/v1
pig.sty.grafana.info/v1
pig.sty.grafana.list/v1
pig.sty.grafana.boot/v1
pig.sty.grafana.load/v1
pig.sty.grafana.init/v1
pig.sty.grafana.dump/v1
pig.sty.grafana.clean/v1
pig.sty.grafana.lang/v1
pig.sty.grafana.style/v1
```

The envelope API versions envelope shape. Root Inventory commands use
`pig.inventory.*` kinds defined by `inventory.md`; the retired
`pig.sty.inventory.validate/v1` kind is not emitted by the current command
tree. Breaking DTO changes require a new payload kind/version and, where the
envelope itself changes, a new envelope API version.

### 9.2 Status and shell exit contract

Sty uses the existing MMCCNN status-code scheme and global shell mapping:

| Result category | Shell exit |
|:---|---:|
| success | 0 |
| operation/internal | 1 |
| parameter/usage | 2 |
| permission | 3 |
| dependency | 4 |
| network | 5 |
| resource | 6 |
| confirmation required | 7 |
| configuration/validation | 8 |
| state/conflict | 9 |

Composite Check output contains one deduplicated finding set. Command setup or
argument errors fail immediately. For completed probes, the selected failure is
deterministic: highest finding severity, then fixed stage order
`validate → local → ansible → network → ssh → privilege`, then stable diagnostic
code order. Its MMCCNN code determines the shell exit.

`--strict` changes the command failure threshold. It MUST NOT rewrite a stored
warning into an error diagnostic.

### 9.3 Redaction matrix

| Operation | Human stdout/editor | Structured output | Logs/errors |
|:---|:---|:---|:---|
| `inventory list` | topology/keys/kinds only | same safe metadata | no arbitrary values |
| `inventory show` | verbatim, secret-bearing | refused | body never copied |
| interactive `inventory edit` | selected private fragment | refused | identifiers only |
| `inventory edit --from` | no fragment on stdout | identifiers/change kinds | no source bytes |
| `validate`, `check` | redacted diagnostics | redacted diagnostics | redacted diagnostics |
| native `configure` | identifiers and output path | identifiers/change kinds | no generated password values |
| `configure --raw -g` | legacy output including generated passwords | captured legacy output including generated passwords | command metadata only; captured payload is secret-bearing |
| Grafana | UID/folder/action/status, no credentials | same | sanitized HTTP errors |
| CMDB | operation, row/object counts, output path | same safe metadata | no variable, SQL, or DSN payloads |

Redaction uses a central, tested secret classifier for known password, secret,
token, API key, private-key, and credential fields. Safer surfaces avoid emitting
arbitrary values at all. External adapters discard stdout that may contain
Inventory secrets and sanitize stderr before creating diagnostics.

Raw Configure is an intentional exception, not a redaction bug. `--raw -g`
MUST display every random password emitted by the legacy script. Its structured
`captured_output` and text stdout are explicitly secret-bearing; callers MUST
protect, avoid logging, and dispose of that output accordingly. The payload
kind and `secret_bearing_output: true` metadata make this exception machine
detectable. Native Configure continues to expose generated secret identifiers,
never their values.


## 10. Validate historical summary (non-normative)

The current command contract is
[`inventory.md`](inventory.md#44-validatev-file).

`pig inventory validate|v` is the sole public validation path.
`pig sty validate|val|v` is intentionally not registered.

`validate` is deterministic source-configuration analysis. Its default engine
is native Go. `--ansible` is an optional compatibility adapter; absence of
Ansible is reported explicitly and does not change native rule semantics.

Validation layers are:

1. YAML document and structural safety;
2. Ansible static YAML Inventory structure;
3. Pigsty declaration and identity rules;
4. complete-Inventory advisory rules.

Pigsty rules include declared global identity, group/host shape, module entity
identity, sequence type/range/uniqueness, `pg_role`, cluster naming, explicitly
declared users, databases, services, HBA, VIP, and other checks covered by the
pinned legacy validator.

Parity is falsifiable:

- P0.2 pins the reviewed legacy `bin/validate` source beside test fixtures;
- representative inventories run through both validators;
- every intentional mismatch is recorded in a divergence registry with stable
  rule ID, legacy result, native result, and rationale;
- known legacy defects are fixed and registered, not copied silently.

Vault/tagged values remain opaque. A rule needing plaintext emits a stable
`UNVERIFIABLE` informational diagnostic rather than a false error.


## 11. Inventory Check historical summary (non-normative)

The current command contract is
[`inventory.md`](inventory.md#45-checkck).

The public command and normative probe behavior are defined by
[`inventory.md` section 4.5](inventory.md#45-checkck). `pig sty check|ck` is
intentionally not registered. The deterministic failure selection and
redaction rules in sections 9.2 and 9.3 of this document remain shared output
contracts for the root command.

`doctor` remains deferred. If implemented later, it collects post-deployment
diagnostics and a redacted support bundle; it does not become a second Check
engine and receives no repair authority from this contract.


## 12. Grafana dashboard contract (P1)

Dashboard management uses a small native Grafana HTTP adapter. It MUST NOT shell
out to `files/grafana/grafana.py` and MUST NOT manage data sources. The public
surface is deliberately limited to:

```text
pig sty grafana info
pig sty grafana list
pig sty grafana boot
pig sty grafana load  [PATH]
pig sty grafana init  [PATH]
pig sty grafana dump  [PATH]
pig sty grafana dump  -d|--dir DIR
pig sty grafana clean [PATH] [--yes] [--dry-run]
pig sty grafana clean --all [--yes] [--dry-run]
pig sty grafana lang  <LANGUAGE_CODE>
pig sty grafana style <dark|light|auto>
```

`gf` is the command alias for `grafana`. `list` is a read-only remote inventory:
it has no positional path, lists every dashboard visible in the active
organization, and orders results by folder UID, title, then dashboard UID. Its
structured payload includes endpoint, organization, count, UID, title, folder
identity/title, and Grafana URL.

`info` performs the public health request before any authenticated request. It
reports endpoint, database health, Grafana version/commit, authentication
status, active organization, dashboard/folder counts, and organization/user
preferences when those authenticated reads are available. No configured
credential is a successful health-only result with authentication status
`not_configured`. A configured credential rejected by Grafana preserves the
health data but returns the permission-class failure code with status
`unauthorized` or `forbidden`. An unhealthy database or unreachable endpoint is
never reported as success.

When an endpoint is selected explicitly by flag or environment, `info` checks
its public health before loading Inventory credentials. A missing or malformed
Inventory remains a configuration failure, but the structured result preserves
the observed endpoint health and reports authentication status `error`.

The experimental `pig sty dashboard|ds` command, manifest, tombstones, and its
incompatible `check/diff` behaviors are removed rather than retained as
aliases.

### 12.1 Path and scope

Without `PATH`, Pig resolves `<PigstyHome>/files/grafana`. Pigsty home follows
the existing global `-H`, `PIGSTY_HOME`/configuration, and valid `~/pigsty`
resolution. Failure to resolve home is an error; Grafana commands MUST NOT fall
back to the current working directory.

For a complete root dump, `-d/--dir DIR` replaces that default output root and
may use any safe directory name; it does not need to be named `grafana`. The
directory may already exist or may be created when its parent exists. `--dir`
and positional `PATH` are mutually exclusive.

An explicit path uses the resolved root when it is contained by that root.
Otherwise the nearest ancestor named exactly `grafana` becomes the root. The
only valid relative shapes are:

```text
.                              complete corpus
<folder>                       one direct Grafana folder
<uid>.json                     one General dashboard
<folder>/<uid>.json            one dashboard in a folder
```

Deeper paths, unsafe UIDs, filename/JSON UID mismatches, duplicate UIDs,
oversized JSON, and symlink scopes fail before network mutation. Directory
scans are one level deep, select non-hidden `*.json`, and execute in bytewise
name order. A directory name is the stable `folderUid`; its uppercase form is
only the display title.

Dashboard files may use either the legacy dashboard document (`uid` at the
root) or Grafana's native `dashboard.grafana.app/v2` `Dashboard` resource
(`metadata.name` as the UID). The filename MUST match the identity field for
its format. Schema-v2 resources are loaded through Grafana's resource API so
tab layouts and section variables never pass through the lossy legacy
`/api/dashboards/db` conversion. Pig strips server-managed resource metadata
and status, applies the directory folder UID as `grafana.app/folder`, and
rejects unsupported dashboard resource versions before network mutation.
Dump preserves schema v2 when the existing destination file is schema v2; a
new dump destination retains the legacy format for backward compatibility.

### 12.2 Load, boot, and init composition

`load` validates the complete selected local scope before contacting Grafana.
It upserts folders and dashboards with overwrite enabled and never prunes live
resources. Root JSON is placed in General; each direct subdirectory maps to the
equal `folderUid`. It has no authority to rename organizations, star dashboards,
or change preferences, including when the selected root contains `pigsty.json`.

`boot` has no local path. Before mutation it verifies that dashboard UID
`pigsty` exists in the active organization. It idempotently renames the current
organization to `Pigsty`, stars that dashboard for the authenticated identity,
and PATCHes only `homeDashboardUID: pigsty` into the organization and current
user preferences. It MUST NOT set or reset language, theme, timezone, week
start, or any unrelated preference.

`init` is the root-only composition `load` then `boot`. An explicit `PATH` must
resolve to a complete grafana root; folder and single-file callers use `load`.
Init performs all local validation before its first network mutation, executes
the complete load phase, and runs boot only when load has zero failed or skipped
objects. It never renames or changes preferences before dashboard loading. Its
structured result retains kind `pig.sty.grafana.init/v1`, identifies load and
boot phases, and reports a boot failure after a successful load as partial
initialization rather than silent success.

### 12.3 Language and style preferences

`lang <LANGUAGE_CODE>` PATCHes only the `language` field for both the active
organization and current user. Codes are matched case-insensitively against the
versioned language set supported by Pigsty's target Grafana version and are
canonicalized before the first request. Convenience aliases include `en` to
`en-US`, `zh`/`zh-CN` to `zh-Hans`, and `zh-TW` to `zh-Hant`. Unsupported codes
fail locally; Pig MUST NOT store arbitrary strings that Grafana would accept but
cannot render.

`style` accepts exactly `dark`, `light`, or `auto`. The first two map directly
to Grafana themes and `auto` maps to Grafana's `system` value. It PATCHes only
the `theme` field for both the active organization and current user. The empty
Grafana theme means inheritance/default and MUST NOT be used to represent the
explicit CLI value `auto`.

### 12.4 Clean ownership and confirmation

Normal clean ownership is the UID set declared by the selected local JSON:

- JSON scope deletes one UID;
- folder scope deletes UIDs in that directory;
- root scope deletes UIDs in the complete local corpus;
- folders are deleted only after a refreshed remote search proves them empty.

Remote dashboards with other UIDs remain untouched even when they share a
selected folder. `--all` is the sole instance-wide reset, is mutually exclusive
with `PATH`, and deletes all dashboards before folders.

Every clean first returns a deterministic endpoint/organization/object plan.
`--dry-run` succeeds without mutation. Text mode without `--yes` prints the
plan and prompts with default No; structured or non-interactive callers must
provide `--yes`. Interactive execution pins the plan digest and fails if live
state changes before application. A confirmation-required result MUST NOT
synthesize a replay command: callers add `--yes` to their original invocation
so its path, endpoint, and credential context cannot be dropped or rewritten.

### 12.5 Dump direction and local-write safety

`dump` treats Grafana as the export source: file scope exports one UID, folder
scope exports every dashboard with that `folderUid`, and root scope exports the
complete organization. Empty `folderUid` maps to root and non-empty
`folderUid` maps directly to a first-level directory; folder title is never
used as path identity.

Dump never deletes local files. Local-only JSON remains in place and is
reported. Unsafe remote UIDs, target collisions, folder movement during the
operation, and path escapes fail before writes. Exported bytes are canonical:
object keys are sorted, containers up to 400 bytes and nesting depth 10 are
inlined without padding, and larger containers use deterministic two-space
layout. Only a byte-identical canonical file is `unchanged`; a semantically
equal file with non-canonical formatting is rewritten once. Repeating a dump
against unchanged remote state therefore produces byte-identical files and no
second-write churn. Changed files use a same-directory temporary file plus
rename. Atomicity is per file, not a multi-file transaction.

Export normalization sets `id` to null and `version` to 1, records the Pigsty
author/license fields, and clears template-variable current selections. Dump
does not expose or modify datasource credentials because datasources are out of
scope.

### 12.6 Connection and API compatibility

Endpoint resolution is `--endpoint`, `GRAFANA_ENDPOINT`,
`all.vars.grafana_endpoint`, then the controller-facing Pigsty default
`http://i.pigsty/ui`. It MUST NOT infer a controller endpoint from
`grafana_port` or loopback. Username resolution is `--username`,
`GRAFANA_USERNAME`, `all.vars.grafana_admin_username`, then `admin`. Password
resolution is explicit `--password`, owner-only `--password-file`,
`GRAFANA_PASSWORD`, then `all.vars.grafana_admin_password`; no default password
is embedded. `--password` is the emergency escape hatch and is mutually
exclusive with `--password-file`. Help MUST warn that it can be exposed through
argv and shell history. A complete explicit endpoint, username, and password or
password file bypasses Inventory parsing so a missing or malformed Inventory
cannot block recovery. Logs, errors, next actions, structured output, and test
diagnostics never contain credentials or response bodies.

Preference updates use PATCH and `homeDashboardUID`; they MUST NOT use PUT or
the deprecated numeric `homeDashboardId`, because omitted PUT fields reset
unrelated preferences. The health request is sent without authentication even
when credentials were configured. All other API requests use the resolved
credential.

The adapter stays in `internal/grafana`, uses the bounded standard-library HTTP
client, refuses redirects, and encapsulates all legacy `/api` paths. No Grafana
SDK is added. The narrow business interface permits a future `/apis` adapter
without changing command semantics.


## 13. CMDB historical summary (non-normative)

The current CMDB contract is
[`inventory.md`](inventory.md#7-cmdb-safety-and-commands).

Pigsty CMDB is an existing product surface, not a new database design. Its fact
sources are the adjacent Pigsty checkout's:

- `files/cmdb.sql`;
- `bin/inventory_load`;
- `bin/inventory_cmdb`;
- `bin/inventory_conf`.

The canonical public namespace is `pig inventory cmdb`. The unreleased
`pig sty cmdb` namespace is removed before release. The corrected verbs are:

- `check`: verify schema, projections, managed source state, and optional
  value-free drift;
- `init`: apply the exact version-matched `cmdb.sql` baseline to the target
  database;
- `load`: replace the existing Inventory declaration tables using the mapping
  and OS-default behavior of `inventory_load`;
- `dump`: query `pigsty.raw_config` and generate canonical YAML;
- `enable`: activate an `inventory.sh` querying `pigsty.inventory`;
- `disable`: set the Ansible Inventory back to `pigsty.yml`.

The baseline owns the existing `pigsty` and `pglog` schemas. CMDB storage uses
`global_var`, `group`, `group_var`, `host`, and `host_var`; it stores JSONB
declarations and cannot preserve YAML comments or source style.

The withdrawn `pigsty_cmdb` revision-store implementation, migrations,
Snapshot history, CAS, rollback, three-way merge, backup bundle, and source
state machine are not part of the corrected contract. The supersession record
is [`sty-cmdb-rfc.md`](sty-cmdb-rfc.md); exact command behavior is defined by
[`inventory.md`](inventory.md#7-cmdb-safety-and-commands).


## 14. Vagrant and Terraform contract (P3)

P3 commands are thin adapters around the native executables and state engines.
They may expose status/plan/up/down/destroy/output/ssh-config operations, but:

- Pig MUST NOT reimplement provider or state semantics;
- native exit status and useful stderr remain observable after sanitization;
- destroy requires a plan and confirmation;
- SSH configuration writes require an explicit destination, backup, and
  confirmation;
- hard-coded demo passwords are not migrated into Go defaults.

No P3 code lands merely because a Make target exists.


## 15. Script and Makefile migration

Migration policy is decided per released surface. Inventory edit, validate, and
Check now have root-only command paths; the unreleased experimental
`sty dashboard` surface was replaced directly by `sty grafana`:

| Existing surface | Target command | Earliest phase |
|:---|:---|:---|
| `configure` / configure-related Make target | `pig sty conf` | already available |
| `make validate`, `bin/validate` | `pig inventory validate` | root migration |
| `dashboard-init` | future `pig sty grafana init` | deferred Pigsty call-site migration |
| `dashboard-dump` | future `pig sty grafana dump` | deferred Pigsty call-site migration |
| `dashboard-clean` | future `pig sty grafana clean --all --yes` | deferred Pigsty call-site migration |
| `files/cmdb.sql` | `pig inventory cmdb init` | corrected CMDB adapter |
| `bin/inventory_load` | `pig inventory cmdb load` | corrected CMDB adapter |
| `bin/inventory_cmdb` | `pig inventory cmdb enable` | corrected CMDB adapter |
| `bin/inventory_conf` | `pig inventory cmdb disable` | corrected CMDB adapter |
| query `pigsty.raw_config` | `pig inventory cmdb dump` | corrected CMDB adapter |
| Vagrant/Terraform Make targets | thin P3 commands | P3 only |

Legacy retention is decided per released surface. Configure keeps its explicit
`--raw` rollback path during native stabilization. Boot is deliberately
Native-only: there is no Boot wrapper or deprecation window for the Pigsty
`bootstrap` script.


## 16. Phase gates and acceptance

### G0 — Design contract

This document freezes taxonomy, aliases, selectors, output/exit behavior,
redaction, source resolution, mutation safety, priorities, and non-goals.

The original lossless P0, Validate, and Check acceptance criteria remain
regression requirements. The former revision-store result is superseded and is
not a passed product gate. Root Inventory P0–P2 and the CMDB safety closure are
implemented under the executable gates in
[`inventory.md`](inventory.md#12-acceptance-gates).

### P0.1 — Inventory completion

Deliver list/show services and DTOs, value-free source status, comment-aware
editing, revision and mode notices, and source-path
behavior.

Acceptance:

- parse/no-op is byte-for-byte identical over the portable fixture set and the
  adjacent Pigsty `conf/**/*.yml` corpus;
- scalar/source-patch tests prove untouched bytes remain identical;
- selector tests cover dotted/IP keys, indexes, nested groups, and pointers;
- conflict tests write nothing and retain a private recovery fragment;
- symlink refusal, 0600 mode, tighten-notice, and structured-show refusal pass;
- read-only source discovery writes nothing and executes no source;
- canonical commands call one use case without dead Sty delegates.

### P0.2 — Validate completion

Deliver declaration-scope rule parity, pinned legacy fixtures, the divergence
registry, vault opacity, and optional Ansible compatibility.

Acceptance:

- every reference-corpus mismatch is fixed or registered;
- rule IDs and diagnostic paths/ranges are stable;
- warnings and `--strict` threshold behavior are tested;
- canonical hyphenated group names do not create strict-mode noise;
- Kafka and Mongo/Ferret identity fixtures cover missing, invalid, and duplicate
  declarations;
- vault/tag fixtures produce `UNVERIFIABLE`, not false failures;
- current Pigsty config corpus parses and validates to the documented manifest
  and diagnostic baseline.

### P0.3 — Check

Deliver deterministic local Check and explicit opt-in profiles.

Acceptance:

- default tests prove no network I/O;
- allowlists and per-probe timeouts are enforced;
- findings deduplicate deterministically;
- mixed-result status selection follows section 9.2;
- disposable targets cover network/SSH behavior without persistent mutation.

### P1 — Grafana dashboards

Acceptance includes exact scope classification, symlink/path-escape rejection,
filename/UID validation, pure-load authority, load-before-boot ordering,
boot suppression after any load failure, unauthenticated health, missing versus
rejected credentials, exact preference PATCH bodies, language alias/allowlist
validation, `auto` to `system`, emergency-password Inventory bypass/redaction,
local-ownership clean, explicit all-instance clean, folder-empty protection,
pinned interactive plans, per-object partial-failure reporting, and an HTTP
fake. Live validation may exercise info, load, boot, init, preferences, and clean
independently from dump so a test run cannot overwrite the repository corpus.

### P2 — CMDB

P2 uses the corrected, deliberately small scope. PostgreSQL tests must prove
exact `cmdb.sql` application, table parity with `inventory_load`, transaction
rollback, non-mutating value-free plans, confirmation gates, canonical and
overwrite-safe `raw_config` dump, dynamic Inventory query parity, CMDB check,
and guarded enable/disable behavior. No revision/history/merge system is an
acceptance requirement.

### P3 — Vagrant/Terraform

Only native-CLI passthrough, plan rendering, exit preservation, and destructive
confirmation are accepted. P3 requires explicit reprioritization.

Every phase runs proportional package tests plus `go test ./...`, `go vet ./...`,
and `git diff --check`. Inventory and concurrent adapters additionally run the
relevant race tests.


## 17. Adversarial review record

The contract was reviewed in two rounds in one Claude Code session:

- Claude Code: `2.1.207`
- model: `claude-fable-5` (`fable` alias), effort `max`
- session: `39e24edc-08d9-46ce-a599-54aec12f8dab`
- Llama 3 70B was not exposed by the installed Claude Code, so the available
  Fable model was used explicitly.

Round 1 rejected the draft contract while accepting its architecture. Its
required corrections covered secret-bearing `show`, stable selector/output
contracts, static revision semantics, validation divergence tracking, Check
probe boundaries, the former Dashboard ownership/dump limitations, and CMDB provenance.

Round 2 retested those findings against repository evidence. It withdrew the
proposal to compute effective vars, accepted declaration-scope validation,
accepted the explicit 1.x-to-2.0 `list` migration, added the former Dashboard removed-UID
tombstones and structured payload kinds, and returned the verdict
**CONVERGED**.

The owner decision of 2026-07-18 to promote Inventory to the root command
supersedes that review's `sty list` repurposing conclusion. A second owner
correction on the same date also rejects its greenfield CMDB premise: the
existing `cmdb.sql` and Inventory scripts are authoritative. The lossless
model, selector, and validation conclusions remain; the Dashboard and
revision-store CMDB conclusion does not. `sty list` keeps release semantics
until removal.

The owner decision of 2026-07-18 replaces that experimental Dashboard surface
with the path-scoped `pig sty grafana` contract in section 12. A separate
adversarial review first rejected ambiguous path, dump, and clean semantics,
then converged after local-declaration clean, explicit `clean --all`, stable
`folderUid` path identity, and remote-as-source dump rules were fixed. The exact
requested historical Claude 3.5 Sonnet snapshot was unavailable; the installed
Claude Code `sonnet` alias reported `claude-sonnet-5` for the replacement review.

The v1.4 `info`/`boot`/`load`/preferences extension received a further
adversarial review. It converged on anonymous health before authentication,
root-only `init = load + boot`, boot suppression after any incomplete load,
current-organization rename through `/api/org`, UID-based home preference
PATCHes, fixed language aliases, and `auto` to `system`. Claude Opus was not
exposed in the active agent runtime, so the strongest available
`gpt-5.6-sol` adversarial reviewer was used and that substitution is recorded
rather than represented as a Claude result.

The native `sty boot` revision received a read-only Claude Code Fable 5 review
at `effort=max` on 2026-08-14. It found no blocker and judged the simplified
coordinator free of the discarded provider/journal/transaction framework. Its
two P2 findings (selected-package reporting with an existing committed repo and
root `repo boot` idempotency) and four P3 hardening/synchronization findings
were resolved before acceptance.


## 18. Current stable workflow

The currently implemented installation path remains:

```bash
pig sty init [--accept-eula]     # install Pigsty to ~/pigsty
pig sty boot [--path PKG|URL]    # native controller bootstrap
pig sty conf                     # native Inventory-aware configure
pig sty conf --raw               # legacy configure fallback
pig inventory edit [selector]    # edit full Inventory or one fragment
pig inventory validate [file]    # native baseline validation
pig inventory check              # local readiness; remote profiles are opt-in
pig sty deploy                   # immediately run deploy.yml/install.yml
pig sty list                     # list available Pigsty releases
pig sty get [version]            # canonical atomic release download
pig sty gf info                  # health and authentication status
pig sty gf load [path]           # pure dashboard/folder loading
pig sty gf init [root]           # complete load followed by boot
```

`sty configure` remains an alias of `sty conf`. Generic templates may apply
explicit PostgreSQL version/region/proxy/locale/password options through the
Inventory mutation API; version-pinned templates retain their pinned kernel
semantics. `--raw` remains only as the explicit Configure rollback path during
native stabilization.

The canonical Inventory workflow is defined by [`inventory.md`](inventory.md).
New automation should use `pig inventory|inv`; Sty exposes no edit, validate,
or Check command.

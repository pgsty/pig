# AGENTS

## Cobra Command Layout

- Keep `cmd` flat: one top-level command belongs in one top-level Go file under `cmd/`.
- Do not create per-command subdirectories under `cmd` for normal CLI commands. The intended shape is:
  - `cmd/pg.go` for `pig pg` / `pig postgres`
  - `cmd/pb.go` for `pig pb` / `pig pgbackrest`
  - `cmd/pt.go` for `pig pt` / `pig patroni`
  - `cmd/pe.go` for `pig pe` / `pig pg_exporter`
  - `cmd/sty.go`, `cmd/do.go`, `cmd/repo.go`, etc. for their matching top-level commands
- A command may have a matching top-level test file, for example `cmd/pg_test.go` for `cmd/pg.go`.
- Complex commands such as `pg`, `pb`, and `pt` should still stay in one command file unless the user explicitly asks for a different layout.
- `cmd/root.go` owns root command setup, global flags, and top-level command registration.
- `cmd/utils.go` owns shared command-layer helpers used by multiple command files.
- Keep `cmd` files as clean Cobra entry points: define command names, aliases, annotations, flags, argument validation, and option mapping there.
- Put concrete business logic in the relevant `cli/*`, `internal/*`, or other implementation package instead of growing it inside `cmd`.
- Avoid duplicated command glue. Reuse shared helpers such as annotation builders, structured output handling, plan rendering, log-output validation, and legacy structured wrappers.

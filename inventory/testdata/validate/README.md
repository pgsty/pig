# Validate parity fixtures

`legacy_validate.py` is the reviewed Pigsty `bin/validate` snapshot from
2026-02-04. Its SHA-256 is
`bb54927a945d11e0c60ca114579157bebbc6e1bbd4e36294ea77eea9b2a405d6`.
Parity tests execute this pinned file, never a mutable adjacent checkout.

`parity.json` records representative legacy/native outcomes and requires every
intentional outcome mismatch to reference an entry in `divergences.json`.
Fixtures contain placeholders only; validator output must not contain arbitrary
Inventory values.

`corpus-baseline.json` records the adjacent Pigsty config corpus observed on
2026-07-17. The 49 logical Inventory paths comprise 48 regular YAML files and
the `conf/app/supa.yml` symlink. Native validation has zero errors; warnings are
recorded by stable rule ID. The corpus check is enabled with `PIGSTY_CONF_DIR`
and never resolves Ansible effective variables.

# Pigsty 4.6 configure fixtures

These unmodified templates come from `pgsty/pigsty` commit
`8046eb4cbd65b2eef7c9e11b4e7ba4f401bec5bd`. Their upstream license and copyright
headers are retained. `conf/build/oss.yml` starts with `region: china`, while
`conf/slim.yml` starts with `region: default`.

Reproduce each fixture from that checkout with:

```bash
git show 8046eb4cbd65b2eef7c9e11b4e7ba4f401bec5bd:conf/build/oss.yml
git show 8046eb4cbd65b2eef7c9e11b4e7ba4f401bec5bd:conf/slim.yml
```

The same revision's `configure` script applies every non-empty resolved region,
including `default`, over the template's `all.vars.region`. Tests copy these
files into a temporary Pigsty directory and exercise the native pipeline without
external commands, network probes, or an installed Pigsty tree.

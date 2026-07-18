# `pig inventory` implementation goal prompt

Execution status: **completed through M3 on 2026-07-18**. This prompt remains
as the auditable migration brief; the normative current state is
[`docs/spec/inventory.md`](inventory.md).

This prompt implements the normative contract in
[`docs/spec/inventory.md`](inventory.md). It does not override that contract.

```text
请在 /Users/vonng/pgsty/pig 仓库中实现顶层 `pig inventory|inv` 命令，
严格以以下契约为准：

/Users/vonng/pgsty/pig/docs/spec/inventory.md

开始修改前完整阅读：

- /Users/vonng/pgsty/pig/AGENTS.md
- /Users/vonng/pgsty/pig/docs/spec/inventory.md
- /Users/vonng/pgsty/pig/docs/spec/sty.md
- /Users/vonng/pgsty/pig/inventory
- /Users/vonng/pgsty/pig/cli/sty
- /Users/vonng/pgsty/pigsty/files/cmdb.sql
- /Users/vonng/pgsty/pigsty/bin/inventory_load
- /Users/vonng/pgsty/pigsty/bin/inventory_cmdb
- /Users/vonng/pgsty/pigsty/bin/inventory_conf

关键纠偏：Pigsty CMDB 已经存在，不得重新设计数据库模型。

- `files/cmdb.sql` 是 schema、视图、函数与默认参数目录的事实源；
- `inventory_load` 是 all.vars、group、host、group vars、host vars 以及
  OS-specific defaults/package_map 的导入语义事实源；
- `pigsty.raw_config` 是声明式配置导出的事实源；
- `pigsty.inventory` 是 Ansible dynamic inventory JSON 的事实源；
- `inventory_cmdb` 与 `inventory_conf` 是启用/停用语义事实源。

严禁新增或延续以下设计：

- `pigsty_cmdb` schema；
- 独立 migration ledger；
- Snapshot revision/history/CAS/rollback；
- three-way merge、backup bundle、runtime/source-switch state record；
- 为了 CMDB relocation 建立 universal backend framework。

按以下 gate 执行：

M1 — Root static Inventory

- 新增 flat `cmd/inventory.go`，在 `cmd/root.go` 的 pigsty group 注册
  `inventory|inv`；不得创建 cmd 子目录。
- 暴露 `list|ls`、`show`、`edit|e`、`validate|v`。
- 复用现有 Inventory/Edit/Validate use case，不复制业务逻辑。
- 只保留已发布的 `sty edit/validate` delegate；未发布的 `sty inventory`
  namespace 在发布前移除。
- `sty list/l/ls` 永远保持 Pigsty release list 语义。

M2 — Thin CMDB adapter

- `cmdb init`：对目标数据库执行 resolved Pigsty home 中版本匹配的完整
  `files/cmdb.sql`；不得翻译成 Go migration。若 pigsty/pglog 已存在，要求
  interactive confirmation 或 `--yes`。
- `cmdb load`：用共享 Inventory parser 做 representability 校验；在单事务中
  按 inventory_load 的映射 truncate/upsert 五张声明表；事务失败全部回滚；随后
  按脚本语义 best-effort 更新 OS defaults 与 package_map。
- `cmdb dump`：查询 `pigsty.raw_config`，解码为共享语义 Value，生成 canonical
  compact YAML。输出只保证 CMDB 可表达范围内的语义等价，不声称保留注释、
  顺序、padding、quote/tag/anchor。
- `cmdb enable`：验证 `SELECT text FROM pigsty.inventory`，生成 0755
  inventory.sh，并把 ansible.cfg 的受支持 inventory directive 改为 inventory.sh。
- `cmdb disable`：把该 directive 改回 pigsty.yml，不删表、不删 inventory.sh。
- 连接顺序：显式 `-d/--database`、METADB_URL、service=meta；DSN/password 不得
  进入 Result/log。

M3 — Retire abandoned implementation

- 只有 M2 parity 完整通过后，才移除 internal/cmdb 与 cli/sty/cmdb_* 中基于
  pigsty_cmdb revision store 的实现和测试。
- 现有 Pigsty shell/python 脚本在 parity gate 通过前保持为 rollback path。
- 未发布的 `sty cmdb` namespace 在发布前移除；不得借兼容路径继续写入
  pigsty_cmdb。

必须提供的验证：

- root alias/help 与 install|i 无冲突；
- 现有 lossless Inventory corpus、edit、validate 测试不回归；
- disposable PostgreSQL 中执行 exact cmdb.sql 后对象验证通过；
- 多个代表性 conf 与 inventory_load 的五张表内容一致；
- 注入导入失败时五张表保持原事务前状态；
- OS defaults/package_map 与脚本一致；
- raw_config dump 重新 parse 后语义一致；
- dynamic inventory JSON 与 pigsty.inventory 查询一致；
- enable/disable 最终 ansible.cfg 语义与现有脚本一致；
- corrected command 不创建 pigsty_cmdb schema；
- structured output 不含 Inventory value、SQL body、DSN 或密码；
- go test ./...、相关 race tests、go vet ./...、git diff --check 通过。

保留用户现有工作区改动。按 M1、M2、M3 功能分区提交，不把无关文件夹带进
提交；每个 gate 完成时明确报告实际验证与未验证条件。
```

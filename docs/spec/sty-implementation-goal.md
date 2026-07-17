# `pig sty` implementation goal prompt

The following prompt is intended to be copied into a fresh Codex/Claude task or
used as the objective of a persistent goal. The normative contract remains
[`docs/spec/sty.md`](sty.md); this prompt does not override it.

```text
请在 /Users/vonng/pgsty/pig 仓库中创建并持续执行一个“Pigsty sty IaC
控制面落地”目标。

唯一规范性设计依据是：
/Users/vonng/pgsty/pig/docs/spec/sty.md

开始任何修改前，完整阅读该契约、仓库 AGENTS.md、当前 cmd/sty.go、
cli/sty、inventory、internal/output，以及相关测试。先核对现状，不要把契约中
“已实现基线”和“计划中”混为一谈，也不要重写已经通过测试的 lossless
Inventory 核心。

总体目标：

1. 最高优先级完成 Inventory，然后依次完成 Validate 与 Check。
2. P0 全部通过后，优先完成安全的 Dashboard 原生实现。
3. CMDB 先交付独立 RFC；只有 RFC、破坏性测试设计和 owner review 通过后才写
   mutation 代码。
4. Vagrant/Terraform 仅保留低优先级设计与薄适配器边界，除非 owner 明确重新
   排优先级，否则不实现。
5. APP、APP Docker/Compose 以及未定义语义的 sty docker 全部不做。

必须遵守的工程边界：

- cmd 保持扁平；所有 sty Cobra wiring 留在 cmd/sty.go，业务逻辑进入
  cli/sty、inventory 或聚焦的 internal adapter。
- 保留并复用现有 Result api:1、MMCCNN 状态码、全局退出码、Plan、结构化输出和
  redaction 机制，不另造一套框架。
- 静态 YAML 写回只能由 lossless Document/source patch 完成；禁止整棵 AST 或
  Go struct marshal 回写。
- 不自行模拟 Ansible effective vars、group priority、vault 解密、dynamic
  inventory、group_vars/host_vars 或 role defaults。
- 不执行 legacy 的 files/cmdb.sql、bin/inventory_load、bin/inventory_cmdb、
  bin/inventory_conf；Dashboard 不 shell out 到 grafana.py。
- 保留用户工作区的无关改动；尤其不要 reset、覆盖或顺手整理不属于本阶段的
  go.mod/go.sum、生成物或其他文件。
- 只做实现当前 gate 所需的最小抽象，避免 universal driver、过度接口化和清理
  churn。

严格按以下 gate 顺序执行。前一个 gate 没有通过，不得开始后一个 gate 的代码。

G0 — 契约核对

- 将 docs/spec/sty.md 视为冻结契约。
- 若实现现状与契约存在冲突，先给出证据和最小修订建议，不要默默改变语义。
- 核对 sty 子命令 alias 无冲突，并固定 1.x/2.0 list 迁移测试。

P0.1 — Inventory completion

- 增加 canonical `pig sty inventory|inv` namespace：
  `list [selector]`、`show [selector]`、`edit [selector]`。
- `sty edit|e` 与 `inventory edit` 共用一个 use case；不得复制实现。
- `inventory list` 只输出有序 topology/metadata、key 和 value kind，不泄露任意
  value；结构化 DTO 带 `kind: pig.sty.inventory.list/v1`。
- `inventory show` 是 verbatim、可能包含秘密的 cat-equivalent，只支持 text；
  JSON/YAML 模式必须返回 parameter-class failure，正文不得进入 Result/log。
- 补齐全局变量、组变量、主机变量的 typed API：set/upsert/delete/rename、可选
  leading/inline comment，以及精确 placeholder replacement；配置逻辑不得回退到
  sed/regex 流水线。
- 把 revision/CAS、symlink refusal、0600、mode tighten notice、owner 语义、
  recovery fragment 生命周期变成测试约束。
- 增加 `sty release list/get` canonical path；1.x `sty list` 仍列 release，并只在
  text stderr 发 deprecation；stdout 和 structured payload 保持兼容。

P0.1 验收：

- portable fixture 与相邻 ~/pigsty/conf/**/*.yml corpus 的 parse/no-op
  byte-for-byte 一致；
- scalar/insert/delete/rename/comment/flow/block/CRLF/UTF-8/anchor/tag/vault 的
  source-patch golden 全绿；
- selector 覆盖 IP/dotted quoted key、index、nested group、RFC6901 pointer；
- 并发编辑冲突不写目标、保留 0600 recovery fragment；
- canonical/delegate 命令等价测试通过；
- 对 inventory 相关 package 运行 race test；
- go test ./...、go vet ./...、git diff --check 通过。

P0.2 — Validate completion

- `inventory validate` 为 canonical，`sty validate|val|v` 为同一 use case 的
  delegate。
- 固定当前 ~/pigsty/bin/validate 的 reviewed snapshot 与 fixture corpus。
- 完成 YAML、Ansible static inventory、Pigsty declaration 三层规则，覆盖显式
  global/group/host identity、seq、pg_role、cluster、users、databases、services、
  HBA、VIP 等 legacy 范围。
- 校验声明可以做不涉及 precedence 的跨 scope 结构检查，但不得计算 effective
  overrides，也不得因可能被继承的字段在当前 scope 缺失而报 false error。
- 每个 intentional parity difference 必须进入 divergence registry，包含稳定
  rule ID、legacy result、native result、rationale。
- tagged/vault plaintext 不可读时输出稳定 UNVERIFIABLE info，而非 false error。
- `--ansible` 仅作为有界 optional adapter：候选文件 0600，丢弃可能含密码的
  stdout，stderr 脱敏。

P0.2 验收：

- 所有 legacy/reference fixture mismatch 均已修复或登记；
- diagnostic code/path/range/severity 稳定；
- `--strict` 只改变失败阈值，不改 warning severity；
- 当前 Pigsty conf corpus 与 ansible-inventory 基线结果有记录；
- go test ./...、go vet ./...、git diff --check 通过。

P0.3 — Check

- 实现 `pig sty check|ck`，只读、永不 auto-fix。
- 默认 local profile：解析/校验 active Snapshot，检查 Pigsty home/layout、必需
  文件和本地工具；测试必须证明默认不访问网络。
- 明确 opt-in profile：ansible、network、ssh；`--sudo` 单独 opt-in。
- 所有外部命令必须在 allowlist 中，有单 probe timeout，stdout/stderr 按契约
  丢弃或脱敏。
- 返回单一、去重 Finding 集；失败选择遵守 severity → stage → stable code 的
  确定顺序和已有 MMCCNN exit mapping。
- 文档明确远程 probe 只是 observational，会产生认证日志/告警/锁定计数副作用。

P0.3 验收：

- no-network default、allowlist、timeout、dedup、mixed-category exit fixture 全绿；
- disposable target 测试不产生持久配置修改；
- go test ./...、go vet ./...、git diff --check 通过。

P1 — Dashboard

- 实现 `pig sty dashboard|ds list|check|diff|init|dump|clean` 的原生 Grafana
  HTTP adapter。
- 以 versioned manifest 的 active UID 集定义 ownership；folder 只定义 placement。
- manifest 维护 removed-UID tombstone lineage，UID 永不复用；只有 `init --prune`
  可以在 plan + confirmation/--yes 后删除 tombstoned live UID。
- copied dashboard（新 UID）不可删除；folder 只有在无 unmanaged resource 时才删。
- 初期禁止 broad deletion of unmanaged resources。
- `init` idempotent upsert，不先 global clean。
- `dump` 不是 datasource backup；secureJsonData 永不 dump，init 从 inventory/env/
  protected input 重新注入且不得用空值覆盖 live secret。
- diff 使用 versioned volatile-field normalization fixture；provisioned/read-only
  object 必须有 per-object result，不得 silent partial success。

P1 验收：

- HTTP fake 覆盖 tombstone prune、copy immunity、folder rule、partial failure；
- dump secret absence 与 normalization fixture 全绿；
- 使用 disposable Grafana container 做兼容测试；
- go test ./...、go vet ./...、git diff --check 通过。

P2 — CMDB RFC gate

- 先新增独立 RFC，明确 versioned migrations、Snapshot/revision、source provenance、
  three-way merge、rollback、split-brain notice、ansible.cfg narrow patch 和容器化
  destructive tests。
- RFC 必须明确 plain dump 只有 canonical generated style；`dump --merge` 必须有
  recorded base，base mismatch 必须 conflict。
- 在 owner 明确批准 RFC 前停止 CMDB mutation 实现；不得把旧 destructive script
  包一层当成 native 实现。
- 获批后才实现 init/load/dump/enable/disable/check，并用 PostgreSQL container
  验证 migration idempotence、transaction rollback、prune/reset confirmation、
  revision conflict、three-way base mismatch、fail-closed ansible.cfg shape。

P3 — deferred

- 不主动实现 Vagrant/Terraform。若 owner 重新排序，只允许 native CLI 的薄适配，
  destroy/SSH config write 必须 plan + confirmation + backup；不得重写 state/provider。

交付方式：

- 每个 gate 开始时报告当前证据和最小文件范围；执行中保持简短状态更新。
- 每个 gate 完成时列出实际验证命令、通过结果、未验证的外部条件和剩余风险。
- 不用“形式相似”替代语义等价；对 legacy 差异建立 fixture/divergence evidence。
- 按功能 gate 分区提交，提交中不得夹带用户的无关工作区改动；只有当前 gate 的
  acceptance 全部通过后才能标记完成。
```

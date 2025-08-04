---
title: PIG CLI
description: PostgreSQL 与扩展生态包管理器
icon: PiggyBank
full: true
cascade:
  type: docs
breadcrumbs: false
comments: false
---

—— **Postgres Install Genius，PostgreSQL 生态中缺失的扩展包管理器**

{{< cards >}}
{{< card link="/intro"   title="Introduction" subtitle="为什么我们需要一个PG专用包管理器？" icon="sparkles" >}}
{{< card link="/start"   title="Get Started"  subtitle="快速上手与样例"  icon="play" >}}
{{< card link="/install" title="Installation" subtitle="下载、安装、更新 pig" icon="save" >}}
{{< /cards >}}

## 快速上手

使用以下命令即可[快速上手](/zh/pig/start) PIG 包管理器：

```bash tab="default"
curl -fsSL https://repo.pigsty.io/pig | bash
```
```bash tab="mirror"
curl -fsSL https://repo.pigsty.cc/pig | bash
```

安装完成后，即可使用。例如，若需安装 [**`pg_duckdb`**](/zh/e/pg_duckdb/) 扩展：

```bash
$ pig repo add pigsty pgdg -u  # 添加 pgdg 与 pigsty 源，并更新仓库缓存
$ pig ext install pg17         # 安装 PostgreSQL 17 内核（原生 PGDG 包）
$ pig ext install pg_duckdb    # 安装 pg_duckdb 扩展（针对当前 pg17）
```

--------

<Cards>
    <Card icon={<Zap className="text-green-500"/>} title='快速上手' href={"/zh/pig/start"}>
        PIG CLI 快速上手
    </Card>
    <Card icon={<Download className="text-blue-600"/>} title='安装' href={"/zh/pig/install"}>
        安装 PostgreSQL 扩展
    </Card>
    <Card icon={<HelpCircle className="text-purple-600"/>} title='特性' href={"/zh/pig/why"}>
        为什么需要 PostgreSQL 包管理器？
    </Card>
    <Card icon={<Rocket className="text-orange-500"/>} title='发布说明' href={"/zh/pig/release"}>
        PIG 命令行工具发布说明
    </Card>
</Cards>

--------

## 命令

你可以执行 `pig help <command>` 获取子命令的详细帮助。

<Cards>
    <Card icon={<Terminal className="text-slate-600"/>} title='pig' href={"/zh/pig/pig"}>
        主命令行接口
    </Card>
    <Card icon={<Package className="text-indigo-600"/>} title='pig repo' href={"/zh/pig/repo"}>
        管理 Linux 发行版的软件仓库
    </Card>
    <Card icon={<Puzzle className="text-emerald-600"/>} title='pig ext' href={"/zh/pig/ext"}>
        安装、移除、搜索、扫描 PostgreSQL 扩展
    </Card>
    <Card icon={<Hammer className="text-amber-600"/>} title='pig build' href={"/zh/pig/build"}>
        配置扩展构建环境并编译扩展
    </Card>
</Cards>

--------

## 源代码

`pig` 命令行工具由 [Vonng](https://vonng.com/en/)（冯若航 rh@vonng.com）开发，并以 [Apache 2.0](https://github.com/pgsty/pig/?tab=Apache-2.0-1-ov-file#readme) 许可证开源。

更多信息请参见 [pigsty](https://pgsty.com) 项目，可一键高效交付所有扩展：

- https://github.com/pgsty/pig
- https://github.com/pgsty/pigsty

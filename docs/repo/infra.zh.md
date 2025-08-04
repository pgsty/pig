---
title: INFRA 仓库
description: 可观测性/PostgreSQL工具软件仓库，Linux发行版大版本无关的软件包
icon: Landmark
---


import { File, Folder, Files } from 'fumadocs-ui/components/files';
import { Badge } from "@/components/ui/badge";
import { KeyRound, FileTerminal } from "lucide-react";

The [`pigsty-infra`](https://github.com/pgsty/infra-pkg) repo contains packages that are generic to any PostgreSQL version and Linux major version,
including prometheus & grafana stack, admin tools for postgres, and many utils written in go.

This repo is maintained by the [Pigsty](https://pigsty.io), you can find all build specs on [https://github.com/pgsty/infra-pkg](https://github.com/pgsty/infra-pkg).
Prebuilt RPM / DEB packages for RHEL / Debian / Ubuntu distros available for `x86_64` and `aarch64` arch.

| Linux  | Package | x86_64 | aarch64 |
|:------:|:-------:|:------:|:-------:|
|   EL   |  `rpm`  |   ✓    |    ✓    |
| Debian |  `deb`  |   ✓    |    ✓    |

--------

## 快速上手

You can add the `pigsty-infra` repo with the [`pig`](/cmd/repo/) CLI tool, it will automatically choose from `apt/yum/dnf`.

```bash tab="default"
curl https://repo.pigsty.io/pig | bash  # download and install the pig CLI tool
pig repo add infra                      # add pigsty-infra repo file to you system
pig repo update                         # update local repo cache with apt / dnf
```
```bash tab="mirror"
curl https://repo.pigsty.cc/pig | bash  # install pig from mirror
pig repo add infra                      # add pigsty-infra repo file to you system
pig repo update                         # update local repo cache with apt / dnf
```
```bash tab="hint"
# you can manage infra repo with these commands:
pig repo add infra -u       # add repo file, and update cache
pig repo add infra -ru      # remove all existing repo, add repo and make cache
pig repo set infra          # = pigsty repo add infra -ru

pig repo add all            # add infra, node, pgsql repo to your system
pig repo set all            # remove existing repo, add above repos and update cache
```

### APT 仓库

On **Debian / Ubuntu** compatible Linux distros, you can add the [GPG Key](/repo/gpg) and APT repo file manually with:

```bash tab="default"
# Add Pigsty's GPG public key to your system keychain to verify package signatures
curl -fsSL https://repo.pigsty.io/key | sudo gpg --dearmor -o /etc/apt/keyrings/pigsty.gpg

# Get Debian distribution codename (distro_codename=jammy, focal, bullseye, bookworm)
# and write the corresponding upstream repository address to the APT List file
distro_codename=$(lsb_release -cs)
sudo tee /etc/apt/sources.list.d/pigsty-infra.list > /dev/null <<EOF
deb [signed-by=/etc/apt/keyrings/pigsty.gpg] https://repo.pigsty.io/apt/infra generic main
EOF

# Refresh APT repository cache
sudo apt update
```
```bash tab="mirror"
# Add Pigsty's GPG public key to your system keychain to verify package signatures
curl -fsSL https://repo.pigsty.cc/key | sudo gpg --dearmor -o /etc/apt/keyrings/pigsty.gpg

# Get Debian distribution codename (distro_codename=jammy, focal, bullseye, bookworm)
# and write the corresponding upstream repository address to the APT List file
distro_codename=$(lsb_release -cs)
sudo tee /etc/apt/sources.list.d/pigsty-infra.list > /dev/null <<EOF
deb [signed-by=/etc/apt/keyrings/pigsty.gpg] https://repo.pigsty.cc/apt/infra generic main
EOF

# Refresh APT repository cache
sudo apt update
```

### YUM 仓库

On **RHEL** compatible Linux distros, you can add the [GPG Key](/repo/gpg) and APT repo file manually with:

```bash tab="default"
# Add Pigsty's GPG public key to your system keychain to verify package signatures
curl -fsSL https://repo.pigsty.io/key | sudo tee /etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty >/dev/null

# Add Pigsty Repo definition files to /etc/yum.repos.d/ directory
sudo tee /etc/yum.repos.d/pigsty-infra.repo > /dev/null <<-'EOF'
[pigsty-infra]
name=Pigsty Infra for $basearch
baseurl=https://repo.pigsty.io/yum/infra/$basearch
skip_if_unavailable = 1
enabled = 1
priority = 1
gpgcheck = 1
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty
module_hotfixes=1
EOF

# Refresh YUM/DNF repository cache
sudo yum makecache;
```
```bash tab="mirror"
# Add Pigsty's GPG public key to your system keychain to verify package signatures
curl -fsSL https://repo.pigsty.cc/key | sudo tee /etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty >/dev/null

# Add Pigsty Repo definition files to /etc/yum.repos.d/ directory
sudo tee /etc/yum.repos.d/pigsty-infra.repo > /dev/null <<-'EOF'
[pigsty-infra]
name=Pigsty Infra for $basearch
baseurl=https://repo.pigsty.cc/yum/infra/$basearch
skip_if_unavailable = 1
enabled = 1
priority = 1
gpgcheck = 1
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty
module_hotfixes=1
EOF

# Refresh YUM/DNF repository cache
sudo yum makecache;
```





--------

## 内容

### Prometheus 技术栈

|                                    Name                                     | Version | License | Comment |
|:---------------------------------------------------------------------------:|:-------:|:-------:|:--------|
|    [VictoriaMetrics](https://github.com/VictoriaMetrics/VictoriaMetrics)    | 1.120.0 |         |         |
| [VictoriaLogs](https://github.com/VictoriaMetrics/VictoriaMetrics/releases) | 1.24.0  |         |         |
|           [prometheus](https://github.com/prometheus/prometheus)            |  3.4.2  |         |         |
|          [pushgateway](https://github.com/prometheus/pushgateway)           | 1.11.1  |         |         |
|         [alertmanager](https://github.com/prometheus/alertmanager)          | 0.28.1  |         |         |
|    [blackbox_exporter](https://github.com/prometheus/blackbox_exporter)     | 0.27.0  |         |         |
|             [pg_exporter](https://github.com/Vonng/pg_exporter)             |  1.0.0  |         |         |
|    [pgbackrest_exporter](https://github.com/woblerr/pgbackrest_exporter)    | 0.20.0  |         |         |
|        [node_exporter](https://github.com/prometheus/node_exporter)         |  1.9.1  |         |         |
|     [keepalived_exporter](https://github.com/mehdy/keepalived-exporter)     |  1.7.0  |         |         |
|   [nginx_exporter](https://github.com/nginxinc/nginx-prometheus-exporter)   |  1.4.2  |         |         |
|    [zfs_exporter](https://github.com/waitingsong/zfs_exporter/releases/)    |  3.8.1  |         |         |
|      [mysqld_exporter](https://github.com/prometheus/mysqld_exporter)       | 0.17.2  |         |         |
|        [redis_exporter](https://github.com/oliver006/redis_exporter)        | 1.74.0  |         |         |
|        [kafka_exporter](https://github.com/danielqsj/kafka_exporter)        |  1.9.0  |         |         |
|       [mongodb_exporter](https://github.com/percona/mongodb_exporter)       | 0.44.0  |         |         |
|                  [mtail](https://github.com/google/mtail)                   |  3.0.8  |         |         |

### Grafana 技术栈

|                                                 Name                                                  | Version | License | Comment                |
|:-----------------------------------------------------------------------------------------------------:|:-------:|:-------:|:-----------------------|
|                            [grafana](https://github.com/grafana/grafana/)                             | 12.0.2  |         | Visualization Platform |
|                                [loki](https://github.com/grafana/loki)                                |  3.1.1  |         | The logging platform   |
|                    [promtail](https://github.com/grafana/loki/releases/tag/v3.0.0)                    |  3.0.0  |         | Obsolete               |
|                       [vector](https://github.com/vectordotdev/vector/releases)                       | 0.48.0  |         |                        |
|            [grafana-infinity-ds](https://github.com/grafana/grafana-infinity-datasource/)             |  3.3.0  |         |                        |
|    [grafana-victorialogs-ds](https://github.com/VictoriaMetrics/victorialogs-datasource/releases/)    | 0.18.1  |         |                        |
| [grafana-victoriametrics-ds](https://github.com/VictoriaMetrics/victoriametrics-datasource/releases/) | 0.16.0  |         |                        |
|        [grafana-plugins](https://github.com/pgsty/infra-pkg/tree/main/noarch/grafana-plugins)         | 12.0.0  |         |                        |

### 数据库组件

PostgreSQL related tools, DBMS, and other utils

|                           Name                            |    Version     | License | Comment                   |
|:---------------------------------------------------------:|:--------------:|:-------:|:--------------------------|
|          [etcd](https://github.com/etcd-io/etcd)          |     3.6.1      |         | Fault Tolerant DCS        |
|          [minio](https://github.com/minio/minio)          | 20250613113347 |         | FOSS S3 Server            |
|            [mcli](https://github.com/minio/mc)            | 20250521015954 |         | FOSS S3 Client            |
|        [kafka](https://kafka.apache.org/downloads)        |     4.0.0      |         | Message Queue             |
|        [duckdb](https://github.com/duckdb/duckdb)         |     1.3.1      |         | Embedded OLAP             |
|     [ferretdb](https://github.com/FerretDB/FerretDB)      |     2.3.1      |         | MongoDB over PG           |
| [tigerbeetle](https://github.com/tigerbeetle/tigerbeetle) |    0.16.48     |         | Financial OLTP            |
|     [IvorySQL](https://github.com/IvorySQL/IvorySQL)      |      4.5       |         | Oracle Compatible PG 17.5 |

### 数据库工具

Pig the package manager, PostgreSQL tools, DBMS, and other utils

|                                Name                                 | Version |  License   |        Comment         |
|:-------------------------------------------------------------------:|:-------:|:----------:|:----------------------:|
|                 [pig](https://github.com/pgsty/pig)                 |  0.5.0  | Apache-2.0 | The pg package manager |
|  [vip-manager](https://github.com/cybertec-postgresql/vip-manager)  |  4.0.0  |            |                        |
| [pg_timetable](https://github.com/cybertec-postgresql/pg_timetable) | 5.13.0  |            |                        |
|  [pev2](https://github.com/pgsty/infra-pkg/tree/main/noarch/pev2)   | 1.15.0  |            |                        |
|             [sealos](https://github.com/labring/sealos)             |  5.0.1  |            |                        |
|        [rclone](https://github.com/rclone/rclone/releases/)         | 1.70.2  |            |                        |
|             [restic](https://github.com/restic/restic)              | 0.18.0  |            |                        |
|           [juicefs](https://github.com/juicedata/juicefs)           |  1.2.3  |            |                        |
|            [dblab](https://github.com/danvergara/dblab)             | 0.33.0  |            |                        |
|            [v2ray](https://github.com/v2fly/v2ray-core)             | 5.28.0  |            |                        |



------

## 变更日志

### 2025-07-04

| Name                       | Old            | New            | Comment |
|:---------------------------|:---------------|:---------------|:--------|
| prometheus                 | 3.4.1          | 3.4.2          | -       |
| grafana                    | 12.0.1         | 12.0.2         | -       |
| vector                     | 0.47.0         | 0.48.0         | -       |
| rclone                     | 1.69.0         | 1.70.2         | -       |
| vip-manager                | 3.0.0          | 4.0.0          | -       |
| blackbox_exporter          | 0.26.0         | 0.27.0         | -       |
| redis_exporter             | 1.72.1         | 1.74.0         | -       |
| duckdb                     | 1.3.0          | 1.3.1          | -       |
| etcd                       | 3.6.0          | 3.6.1          | -       |
| ferretdb                   | 2.2.0          | 2.3.1          | -       |
| dblab                      | 0.32.0         | 0.33.0         | -       |
| tigerbettle                | 0.16.41        | 0.16.48        | -       |
| grafana-victorialogs-ds    | 0.16.3         | 0.18.1         | -       |
| grafana-victoriametrics-ds | 0.15.1         | 0.16.0         | -       |
| grafana-inifinity-ds       | 3.2.1          | 3.3.0          | -       |
| victorialogs               | 1.22.2         | 1.24.0         | -       |
| victoriametrics            | 1.117.1        | 1.120.0        | -       |

### 2025-06-01

| Name                       | Old            | New            | Comment |
|:---------------------------|:---------------|:---------------|:--------|
| grafana                    | -              | 12.0.1         | -       |
| prometheus                 | -              | 3.4.1          | -       |
| keepalived_exporter        | -              | 1.7.0          | -       |
| redis_exporter             | -              | 1.73.0         | -       |
| victoriametrics            | -              | 1.118.0        | -       |
| victorialogs               | -              | 1.23.1         | -       |
| tigerbeetle                | -              | 0.16.42        | -       |
| grafana-victorialogs-ds    | -              | 0.17.0         | -       |
| grafana-infinity-ds        | -              | 3.2.2          | -       |

### 2025-05-22

| Name                       | Old            | New            | Comment |
|:---------------------------|:---------------|:---------------|:--------|
| dblab                      | -              | 0.32.0         | -       |
| prometheus                 | -              | 3.4.0          | -       |
| duckdb                     | -              | 1.3.0          | -       |
| etcd                       | -              | 3.6.0          | -       |
| pg_exporter                | -              | 1.0.0          | -       |
| ferretdb                   | -              | 2.2.0          | -       |
| rclone                     | -              | 1.69.3         | -       |
| minio                      | -              | 20250422221226 | -       |
| mcli                       | -              | 20250416181326 | -       |
| nginx_exporter             | -              | 1.4.2          | -       |
| keepalived_exporter        | -              | 1.6.2          | -       |
| pgbackrest_exporter        | -              | 0.20.0         | -       |
| redis_exporter             | -              | 1.27.1         | -       |
| victoriametrics            | -              | 1.117.1        | -       |
| victorialogs               | -              | 1.22.2         | -       |
| pg_timetable               | -              | 5.13.0         | -       |
| tigerbeetle                | -              | 0.16.41        | -       |
| pev2                       | -              | 1.15.0         | -       |
| grafana                    | -              | 12.0.0         | -       |
| grafana-victorialogs-ds    | -              | 0.16.3         | -       |
| grafana-victoriametrics-ds | -              | 0.15.1         | -       |
| grafana-infinity-ds        | -              | 3.2.1          | -       |
| grafana_plugins            | -              | 12.0.0         | -       |

### 2025-04-23

| Name                       | Old            | New            | Comment |
|:---------------------------|:---------------|:---------------|:--------|
| mtail                      | -              | 3.0.8          | new     |
| pig                        | -              | 0.4.0          | -       |
| pg_exporter                | -              | 0.9.0          | -       |
| prometheus                 | -              | 3.3.0          | -       |
| pushgateway                | -              | 1.11.1         | -       |
| keepalived_exporter        | -              | 1.6.0          | -       |
| redis_exporter             | -              | 1.70.0         | -       |
| victoriametrics            | -              | 1.115.0        | -       |
| victoria_logs              | -              | 1.20.0         | -       |
| duckdb                     | -              | 1.2.2          | -       |
| pg_timetable               | -              | 5.12.0         | -       |
| vector                     | -              | 0.46.1         | -       |
| minio                      | -              | 20250422221226 | -       |
| mcli                       | -              | 20250416181326 | -       |

### 2025-04-05

| Name                       | Old            | New            | Comment |
|:---------------------------|:---------------|:---------------|:--------|
| pig                        | -              | 0.3.4          | -       |
| etcd                       | -              | 3.5.21         | -       |
| restic                     | -              | 0.18.0         | -       |
| ferretdb                   | -              | 2.1.0          | -       |
| tigerbeetle                | -              | 0.16.34        | -       |
| pg_exporter                | -              | 0.8.1          | -       |
| node_exporter              | -              | 1.9.1          | -       |
| grafana                    | -              | 11.6.0         | -       |
| zfs_exporter               | -              | 3.8.1          | -       |
| mongodb_exporter           | -              | 0.44.0         | -       |
| victoriametrics            | -              | 1.114.0        | -       |
| minio                      | -              | 20250403145628 | -       |
| mcli                       | -              | 20250403170756 | -       |

### 2025-03-23

| Name                       | Old            | New            | Comment |
|:---------------------------|:---------------|:---------------|:--------|
| etcd                       | -              | 3.5.20         | -       |
| pgbackrest_exporter        | -              | 0.19.0         | rebuild |
| victorialogs               | -              | 1.17.0         | -       |
| vslogcli                   | -              | 1.17.0         | -       |

### 2025-03-17

| Name                       | Old            | New            | Comment |
|:---------------------------|:---------------|:---------------|:--------|
| kafka                      | -              | 4.0.0          | -       |
| Prometheus                 | -              | 3.2.1          | -       |
| AlertManager               | -              | 0.28.1         | -       |
| blackbox_exporter          | -              | 0.26.0         | -       |
| node_exporter              | -              | 1.9.0          | -       |
| mysqld_exporter            | -              | 0.17.2         | -       |
| kafka_exporter             | -              | 1.9.0          | -       |
| redis_exporter             | -              | 1.69.0         | -       |
| DuckDB                     | -              | 1.2.1          | -       |
| etcd                       | -              | 3.5.19         | -       |
| FerretDB                   | -              | 2.0.0          | -       |
| tigerbeetle                | -              | 0.16.31        | -       |
| vector                     | -              | 0.45.0         | -       |
| VictoriaMetrics            | -              | 1.114.0        | -       |
| VictoriaLogs               | -              | 1.16.0         | -       |
| rclone                     | -              | 1.69.1         | -       |
| pev2                       | -              | 1.14.0         | -       |
| grafana-victorialogs-ds    | -              | 0.16.0         | -       |
| grafana-victoriametrics-ds | -              | 0.14.0         | -       |
| grafana-infinity-ds        | -              | 3.0.0          | -       |
| timescaledb-event-streamer | -              | 0.12.0         | new     |
| restic                     | -              | 0.17.3         | new     |
| juicefs                    | -              | 1.2.3          | new     |

### 2025-02-12

| Name                       | Old            | New            | Comment |
|:---------------------------|:---------------|:---------------|:--------|
| pushgateway                | 1.10.0         | 1.11.0         | -       |
| alertmanager               | 0.27.0         | 0.28.0         | -       |
| nginx_exporter             | 1.4.0          | 1.4.1          | -       |
| pgbackrest_exporter        | 0.18.0         | 0.19.0         | -       |
| redis_exporter             | 1.66.0         | 1.67.0         | -       |
| mongodb_exporter           | 0.43.0         | 0.43.1         | -       |
| VictoriaMetrics            | 1.107.0        | 1.111.0        | -       |
| VictoriaLogs               | v1.3.2         | 1.9.1          | -       |
| DuckDB                     | 1.1.3          | 1.2.0          | -       |
| Etcd                       | 3.5.17         | 3.5.18         | -       |
| pg_timetable               | 5.10.0         | 5.11.0         | -       |
| FerretDB                   | 1.24.0         | 2.0.0          | -       |
| tigerbeetle                | 0.16.13        | 0.16.27        | -       |
| grafana                    | 11.4.0         | 11.5.1         | -       |
| vector                     | 0.43.1         | 0.44.0         | -       |
| minio                      | 20241218131544 | 20250207232109 | -       |
| mcli                       | 20241121172154 | 20250208191421 | -       |
| rclone                     | 1.68.2         | 1.69.0         | -       |

### 2024-11-19

| Name                       | Old            | New            | Comment |
|:---------------------------|:---------------|:---------------|:--------|
| Prometheus                 | 2.54.0         | 3.0.0          | -       |
| VictoriaMetrics            | 1.102.1        | 1.106.1        | -       |
| VictoriaLogs               | v0.28.0        | 1.0.0          | -       |
| MySQL Exporter             | 0.15.1         | 0.16.0         | -       |
| Redis Exporter             | 1.62.0         | 1.66.0         | -       |
| MongoDB Exporter           | 0.41.2         | 0.42.0         | -       |
| Keepalived Exporter        | 1.3.3          | 1.4.0          | -       |
| DuckDB                     | 1.1.2          | 1.1.3          | -       |
| etcd                       | 3.5.16         | 3.5.17         | -       |
| tigerbeetle                | 16.8           | 0.16.13        | -       |
| grafana                    | -              | 11.3.0         | -       |
| vector                     | -              | 0.42.0         | -       |

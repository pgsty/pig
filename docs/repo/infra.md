---
title: INFRA Repo
description: Packages that are generic to any PostgreSQL version and Linux major version.
icon: Landmark
---

The [`pigsty-infra`](https://github.com/pgsty/infra-pkg) repo contains packages that are generic to any PostgreSQL version and Linux major version,
including prometheus & grafana stack, admin tools for postgres, and many utils written in go.

This repo is maintained by the [Pigsty](https://doc.pgsty.com), you can find all build specs on [https://github.com/pgsty/infra-pkg](https://github.com/pgsty/infra-pkg).
Prebuilt RPM / DEB packages for RHEL / Debian / Ubuntu distros available for `x86_64` and `aarch64` arch.

| Linux  | Package | x86_64 | aarch64 |
|:------:|:-------:|:------:|:-------:|
|   EL   |  `rpm`  |   ✓    |    ✓    |
| Debian |  `deb`  |   ✓    |    ✓    |

--------

## Quick Start

You can add the `pigsty-infra` repo with the [`pig`](/cmd/repo) CLI tool, it will automatically choose from `apt/yum/dnf`.

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



--------

## Manual Setup

You can also use this repo directly without the `pig` CLI tool, by add them to your linux os repo list manually:

### APT Repo

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

### YUM Repo

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

## Content

### Prometheus Stack

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

### Grafana Stack

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

### Databases

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

### DB Utils

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



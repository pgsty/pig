---
title: Repository
description: The infrastructure to deliver PostgreSQL Extensions
icon: Warehouse
weight: 700
breadcrumbs: false
---


Pigsty has a repository that provides 200+ extra PostgreSQL extensions on 10 mainstream [Linux Distros](/intro#linux-compatibility).
It is designed to work together with the official PostgreSQL Global Development Group ([PGDG](https://www.postgresql.org/download/linux/)) repo.
Together, they can provide up to [423 PostgreSQL Extensions](https://ext.pgsty.com/list) out-of-the-box.


## Get Started

You can enable the pigsty infra & pgsql repo with the [pig](/pig/) CLI tool, or add them manually to your system:

```bash tab="pig"
curl https://repo.pigsty.io/pig | bash      # download and install the pig CLI tool
pig repo add all -u                         # add linux, pgdg, pigsty repo and update cache
```
```bash tab="apt"
# Add Pigsty's GPG public key to your system keychain to verify package signatures
curl -fsSL https://repo.pigsty.io/key | sudo gpg --dearmor -o /etc/apt/keyrings/pigsty.gpg

# Get Debian distribution codename (distro_codename=jammy, focal, bullseye, bookworm), and write the corresponding upstream repository address to the APT List file
distro_codename=$(lsb_release -cs)
sudo tee /etc/apt/sources.list.d/pigsty-io.list > /dev/null <<EOF
deb [signed-by=/etc/apt/keyrings/pigsty.gpg] https://repo.pigsty.io/apt/infra generic main
deb [signed-by=/etc/apt/keyrings/pigsty.gpg] https://repo.pigsty.io/apt/pgsql/${distro_codename} ${distro_codename} main
EOF

# Refresh APT repository cache
sudo apt update
```
```bash tab="yum"
# Add Pigsty's GPG public key to your system keychain to verify package signatures
curl -fsSL https://repo.pigsty.io/key | sudo tee /etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty >/dev/null

# Add Pigsty Repo definition files to /etc/yum.repos.d/ directory, including two repositories
sudo tee /etc/yum.repos.d/pigsty-io.repo > /dev/null <<-'EOF'
[pigsty-infra]
name=Pigsty Infra for $basearch
baseurl=https://repo.pigsty.io/yum/infra/$basearch
skip_if_unavailable = 1
enabled = 1
priority = 1
gpgcheck = 1
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty
module_hotfixes=1

[pigsty-pgsql]
name=Pigsty PGSQL For el$releasever.$basearch
baseurl=https://repo.pigsty.io/yum/pgsql/el$releasever.$basearch
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

All the RPM / DEB packages are signed with [GPG Key](/repo/gpg) fingerprint (`B9BD8B20`) in Pigsty repository.


---------

## Compatibility

Pigsty has two major repos: [INFRA](/repo/infra/) and [PGSQL](/repo/pgsql/),
provide DEB / RPM for `x86_64` and `aarch64` architecture.

The [`infra`](/repo/infra) repo contains packages that are generic to any PostgreSQL version and Linux major version,
including prometheus & grafana stack, admin tools for postgres, and many utils written in go.

| Linux  | Package | x86_64 | aarch64 |
|:------:|:-------:|:------:|:-------:|
|   EL   |  `rpm`  |   ✓    |    ✓    |
| Debian |  `deb`  |   ✓    |    ✓    |

The [`pgsql`](/repo/pgsql) repo contains packages that are ad hoc to specific PostgreSQL Major Versions.
(Often ad hoc to a specific Linux distro major version, too) Including extensions, and some kernel forks.

|  OS / Arch   | OS  |                                                                                               x86_64                                                                                                |                                                                                               aarch64                                                                                               |
|:------------:|:---:|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------:|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------:|
|     EL8      | el8 | <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge> | <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge> |
|     EL9      | el9 | <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge> | <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge> |
|  Debian 12   | d12 | <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge> | <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge> |
| Ubuntu 22.04 | u22 | <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge> | <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge> |
| Ubuntu 24.04 | u24 | <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge> | <Badge variant="blue-subtle">17</Badge><Badge variant="blue-subtle">16</Badge><Badge variant="blue-subtle">15</Badge><Badge variant="blue-subtle">14</Badge><Badge variant="blue-subtle">13</Badge> |

The file hierarchy of the repository may look like this:

<Files>
    <Folder name="https://repo.pigsty.io" defaultOpen>
        <Folder name="apt" defaultOpen>
            <Folder name="infra">
                <Folder name="amd64"></Folder>
                <Folder name="arm64"></Folder>
            </Folder>
            <Folder name="pgsql">
                <Folder name="x86_64"></Folder>
                <Folder name="aarch64"></Folder>
            </Folder>
        </Folder>
        <Folder name="yum" defaultOpen>
            <Folder name="infra">
                <Folder name="x86_64"></Folder>
                <Folder name="aarch64"></Folder>
            </Folder>
            <Folder name="pgsql">
                <Folder name="x86_64"></Folder>
                <Folder name="aarch64"></Folder>
            </Folder>
        </Folder>
        <a href={"https://repo.pigsty.io/pig"}><File name="pig" icon={<FileTerminal className="text-orange-500" />} /></a>
        <a href={"https://repo.pigsty.io/key"}><File name="key" icon={<KeyRound className="text-blue-500" />} /></a>
    </Folder>
</Files>


------

## Source

Building specs of these repos and packages are open-sourced on GitHub:

- https://github.com/pgsty/rpm
- https://github.com/pgsty/deb
- https://github.com/pgsty/infra-pkg

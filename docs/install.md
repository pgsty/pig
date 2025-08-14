---
title: Installation
description: How to download and install the Pigsty CLI tool `pig`
icon: Download
weight: 300
breadcrumbs: false
---


## Script

The simplest way to install `pig` is to run the following installation script:

{{< tabs items="Default,Mirror" >}}

{{< tab >}}
```bash tab="Global"
curl -fsSL https://repo.pigsty.io/pig | bash     # via Cloudflare
```
{{< /tab >}}

{{< tab >}}
```bash tab="Mirror"
curl -fsSL https://repo.pigsty.cc/pig | bash     # via China Mirror
```
{{< /tab >}}

{{< /tabs >}}

It downloads the latest `pig` RPM / DEB from the pigsty [repo](/repo/) and install via `rpm` or `dpkg`.


## Release

You can also download `pig` package (`RPM`/`DEB`/ Tarball) directly from the [Latest GitHub Release Page](https://github.com/pgsty/pig/releases/latest):

{{< filetree/container >}}
{{< filetree/file name="latest" >}}
{{< filetree/folder name="v0.6.0" state="open" >}}
{{< filetree/file name="https://repo.pigsty.io/pkg/pig/v0.6.0/pig_0.6.0-1_amd64.deb" >}}
{{< filetree/file name="https://repo.pigsty.io/pkg/pig/v0.6.0/pig_0.6.0-1_arm64.deb" >}}
{{< filetree/file name="https://repo.pigsty.io/pkg/pig/v0.6.0/pig-0.6.1-1.aarch64.rpm" >}}
{{< filetree/file name="https://repo.pigsty.io/pkg/pig/v0.6.0/pig-0.6.1-1.x86_64.rpm" >}}
{{< filetree/file name="https://repo.pigsty.io/pkg/pig/v0.6.0/pig-v0.6.0.linux-amd64.tar.gz" >}}
{{< filetree/file name="https://repo.pigsty.io/pkg/pig/v0.6.0/pig-v0.6.0.linux-arm64.tar.gz" >}}

{{< /filetree/folder >}}
{{< filetree/folder name="v0.5.0" state="closed" >}}{{< /filetree/folder >}}
{{< filetree/folder name="......" state="closed" >}}{{< /filetree/folder >}}
{{< /filetree/container >}}



## Repository

The pig package is also available in the [`pigsty-infra`](/repo/infra) repo,
You can add the repo to your system, and install it with OS package manager:

For EL distribution such as  RHEL，RockyLinux，CentOS，Alma Linux，OracleLinux,...:

### YUM

```bash tab="yum"
sudo tee /etc/yum.repos.d/pigsty-infra.repo > /dev/null <<-'EOF'
[pigsty-infra]
name=Pigsty Infra for $basearch
baseurl=https://repo.pigsty.io/yum/infra/$basearch
enabled = 1
gpgcheck = 0
module_hotfixes=1
EOF

sudo yum makecache;
sudo yum install -y pig
```

### APT

For Debian, Ubuntu and compatible Linux Distributions:

```bash tab="apt"
sudo tee /etc/apt/sources.list.d/pigsty-infra.list > /dev/null <<EOF
deb [trusted=yes] https://repo.pigsty.io/apt/infra generic main
EOF

sudo apt update;
sudo apt install -y pig
```


## Upgrade

Once installed, you can self-update `pig` itself to the latest version with:

```bash
pig update   # upgrade pig itself to the latest available version
```


## Build

You can also build `pig` from source code. It's a go program, hosted on [github.com/pgsty/pig](https://github.com/pgsty/pig)

```bash
git clone https://github.com/pgsty/pig.git; cd pig
go get -u; go build
```


## Uninstall

```bash
apt remove -y pig     # Debian / Ubuntu ...
yum remove -y pig     # RHEL / CentOS / RockyLinux ...
rm -rf /usr/bin/pig   # If installed via raw binary, just remove it
```
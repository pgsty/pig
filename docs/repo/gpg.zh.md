---
title: GPG 密钥
description: 导入 Pigsty 仓库使用的 GPG 公钥以验证软件包签名完整性
icon: KeyRound
---

你可以通过检查 GPG 签名来验证从 Pigsty 仓库下载的软件包的完整性，本文会介绍如何导入用于签名软件包的 GPG 密钥。
你也可以生成自己的 GPG 密钥，构建自己的仓库，并用它来签发你自己的软件包。


## 摘要

Pigsty 仓库中所有的 RPM / DEB 软件包都使用 GPG 密钥（指纹：`B9BD8B20`）进行签名，以确保软件包的完整性和来源的可信度。

完整摘要： `9592A7BC7A682E7333376E09E7935D8DB9BD8B20` Ruohang Feng (Pigsty) [rh@vonng.com](mailto:rh@vonng.com)

```
pub   rsa4096 2024-07-16 [SC]
      9592A7BC7A682E7333376E09E7935D8DB9BD8B20
uid           [ultimate] Ruohang Feng (Pigsty) <rh@vonng.com>
sub   rsa4096 2024-07-16 [E]
```

你可以在这里找到 Pigsty 使用的 GPG 公钥： https://repo.pigsty.io/key or https://repo.pigsty.cc/key




## 导入

在 RHEL 兼容的 Linux 发行版上，你可以使用以下命令导入此密钥：

```bash tab="default"
curl -fsSL https://repo.pigsty.io/key | sudo tee /etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty >/dev/null
```
```bash tab="mirror"
curl -fsSL https://repo.pigsty.cc/key | sudo tee /etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty >/dev/null
```

在 Debian / Ubuntu 兼容的 Linux 发行版上，你可以使用以下命令导入此密钥：

```bash tab="default"
curl -fsSL https://repo.pigsty.io/key | sudo gpg --dearmor -o /etc/apt/keyrings/pigsty.gpg
```
```bash tab="mirror"
curl -fsSL https://repo.pigsty.cc/key | sudo gpg --dearmor -o /etc/apt/keyrings/pigsty.gpg
```



## 公钥

Pigsty 仓库使用的 GPG 公钥内容如下：

```bash
-----BEGIN PGP PUBLIC KEY BLOCK-----

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
/c7w
=4rRi
-----END PGP PUBLIC KEY BLOCK-----
```


## 使用

如果你想要分发自己的仓库并使用自己的 GPG 密钥进行签名，以下是简单介绍：

### 安装 GPG 软件包

```bash tab="brew"
brew install gnupg pinentry-mac
```
```bash tab="apt"
sudo apt install gnupg2 pinentry-curses
```
```bash tab="dnf"
sudo dnf install gnupg2 pinentry-curses
```

### 生成 GPG 密钥

你可以使用以下命令生成一个新的 GPG 密钥对：

```bash
gpg --full-generate-key
```

### 导入 GPG 私钥

如果你有一个 GPG 私钥文件（例如 `mykey.secret`），你可以使用以下命令导入它：

```bash
gpg --import mykey.secret
```

### 列出 GPG 密钥

你可以列出已经导入的 GPG 公钥和私钥：

```bash
$ gpg --list-key
[keyboxd]
---------
pub   rsa4096 2024-07-16 [SC]
      9592A7BC7A682E7333376E09E7935D8DB9BD8B20
uid           [ unknown] Ruohang Feng (Pigsty) <rh@vonng.com>
sub   rsa4096 2024-07-16 [E]

$ gpg --list-secret-key
[keyboxd]
---------
sec   rsa4096 2024-07-16 [SC]
      9592A7BC7A682E7333376E09E7935D8DB9BD8B20
uid           [ unknown] Ruohang Feng (Pigsty) <rh@vonng.com>
ssb   rsa4096 2024-07-16 [E]
```

### 签名 RPM 包

如果你想要用 GPG 私钥签名你的 RPM 包，可以在 `~/.rpmmacros` 文件中指定 GPG 密钥：

```bash
%_signature   gpg
%_gpg_path    ~/.gnupg
%_gpg_name    B9BD8B20
%_gpg_digest_algo  sha256
```

这里的 `%_gpg_name` 是你的 GPG 密钥的指纹，请将 Pigsty 密钥指纹 `B9BD8B20` 替换为你自己的密钥指纹。

```bash
rpm --addsign yourpackage.rpm
```



### 签名 DEB 包

如果你想要用 GPG 私钥签名你的 DEB 包，可以在 `reprepro` 配置文件中指定 GPG 密钥：

```yaml
Origin: Pigsty
Label: Pigsty INFRA
Codename: generic
Architectures: amd64 arm64
Components: main
Description: pigsty apt repository for infra components
SignWith: 9592A7BC7A682E7333376E09E7935D8DB9BD8B20
```

同理，将这里的 `SignWith` 替换为你自己的 GPG 密钥指纹即可，`reprepro` 会自动使用它进行签名。
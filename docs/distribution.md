# 分发与第三方安装渠道

目标：用户在各平台用一行命令安装 Agent（无需 GUI）。

## 一览

| 平台 | 推荐安装方式 | 备选 |
|------|----------------|------|
| macOS | `brew install foqerhk/tap/runeverything` | `install.sh` |
| Linux（自动识别） | `curl …/install-linux.sh \| bash` | 见下表 |
| Debian / Ubuntu | `install-apt.sh` / `apt install` | |
| Fedora / RHEL / Rocky | `install-rpm.sh`（dnf/yum） | |
| openSUSE | `install-rpm.sh`（zypper） | |
| Arch / Manjaro | `install-arch.sh` / AUR PKGBUILD | |
| Alpine | `install-apk.sh` / APKBUILD | |
| Windows | PowerShell 一键 `install.ps1` | Scoop / winget / Chocolatey |

## macOS — Homebrew

```bash
brew install foqerhk/tap/runeverything
runeverything pair
brew services start runeverything
```

## Linux — 一键自动识别

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-linux.sh | bash
```

会按 `/etc/os-release` 分流到 apt / dnf|yum|zypper / pacman / apk。

### Debian / Ubuntu（apt）

**推荐：一条命令搞定（自动加源 + 安装）**

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-apt.sh | sudo bash
```

之后即可直接：

```bash
sudo apt update
sudo apt install runeverything
sudo apt upgrade runeverything
```

> 官方源里没有本包，第一次不能只跑裸的 `apt install runeverything`。  
> 一键脚本会写入 `/etc/apt/sources.list.d/runeverything.list`，之后就和装其它软件一样。

若只要手动加源：

```bash
echo 'deb [trusted=yes arch=amd64,arm64] https://foqerhk.github.io/runeverything/apt stable main' \
  | sudo tee /etc/apt/sources.list.d/runeverything.list
sudo apt update && sudo apt install runeverything
```

### Fedora / RHEL / CentOS / Rocky / Alma（dnf / yum）

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-rpm.sh | sudo bash
```

或手动：

```bash
sudo dnf install https://github.com/foqerhk/runeverything/releases/download/v0.1.0/runeverything-0.1.0-1.x86_64.rpm
```

### openSUSE（zypper）

同上 `install-rpm.sh`（自动选 zypper），或：

```bash
sudo zypper install ./runeverything-0.1.0-1.x86_64.rpm
```

### Arch Linux / Manjaro（pacman）

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-arch.sh | sudo bash
```

AUR 风格 PKGBUILD：[`packaging/aur/PKGBUILD`](../packaging/aur/PKGBUILD)

```bash
cd packaging/aur && makepkg -si
```

### Alpine（apk）

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-apk.sh | sudo bash
```

APKBUILD：[`packaging/alpine/APKBUILD`](../packaging/alpine/APKBUILD)

### 通用 tarball（任意发行版）

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.sh | sh
```

## Windows

### PowerShell 一键（推荐，无需预先装包管理器）

```powershell
irm https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.ps1 | iex
```

### Scoop

```powershell
scoop bucket add runeverything https://github.com/foqerhk/scoop-runeverything
scoop install runeverything
```

### winget（仓库内清单）

```powershell
irm https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-winget.ps1 | iex
```

### Chocolatey

```powershell
choco install runeverything -y --source "https://github.com/foqerhk/runeverything/releases/download/v0.1.0/"
```

nupkg 由 `scripts/sync-chocolatey.sh` 生成并随 Release 发布。

## 发版流程

```bash
./scripts/build-release.sh 0.1.0          # 含 tar/zip/.deb/.rpm + apt repo
./scripts/sync-homebrew-formula.sh 0.1.0
./scripts/sync-scoop-manifest.sh 0.1.0
./scripts/sync-winget-manifest.sh 0.1.0
./scripts/sync-chocolatey.sh 0.1.0
./scripts/sync-aur-pkgbuild.sh 0.1.0
./scripts/sync-alpine-apkbuild.sh 0.1.0
gh release create v0.1.0 dist/v0.1.0/* --title "v0.1.0"
# 推送 homebrew-tap / scoop-runeverything / gh-pages(apt)
```

## 资源命名

```
runeverything_darwin_{amd64,arm64}.tar.gz
runeverything_linux_{amd64,arm64}.tar.gz
runeverything_windows_{amd64,arm64}.zip
runeverything_<ver>_{amd64,arm64}.deb
runeverything-<ver>-1.{x86_64,aarch64}.rpm
runeverything.<ver>.nupkg
SHA256SUMS
```

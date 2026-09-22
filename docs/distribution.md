# 分发与第三方安装渠道

目标：用户在各平台用一行命令安装 Agent（无需 GUI）。

## 一览

| 平台 | 推荐安装方式 | 备选 |
|------|----------------|------|
| macOS | `brew install foqerhk/tap/runeverything` | `install.sh` |
| Linux（自动识别） | `curl …/install-linux.sh \| bash` | 见下表 |
| Debian / Ubuntu | `ppa:foqerhk/runeverything` / `install-apt.sh` | GitHub Pages APT |
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

**推荐：Launchpad PPA**

```bash
sudo add-apt-repository ppa:foqerhk/runeverything
sudo apt update
sudo apt install runeverything
```

PPA：https://launchpad.net/~foqerhk/+archive/ubuntu/runeverything  
上传新版本：`python3 scripts/upload-ppa.py`（需 `~/.runeverything-gpg`）

**或一条命令**（优先 PPA，失败则回退 GitHub Pages / Release .deb）：

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-apt.sh | sudo bash
```

备选：手动加 GitHub Pages 源

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

## 公益中继（P2P）

- 官方种子：按地区只拉本区 getnode `/seeds.json`（国内 `.cn`，国外 `.net`；见 [regions.md](regions.md)）；国外可另同步 GitHub Pages。
- 节点协议：`GET/POST /v1/peers`；志愿者默认 gossip，关闭：`RE_SHARE_RELAY=0`。
- 家庭 Agent：seeds → crawl → `/healthz` ping → 选最低延迟。
- **免责**：志愿者中继只提供尽力连通，不提供保密保证。

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

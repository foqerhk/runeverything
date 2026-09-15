# 分发与第三方安装渠道

目标：用户在各平台用一行命令安装 Agent（无需 GUI）。

## 一览

| 平台 | 推荐安装方式 | 备选 |
|------|----------------|------|
| macOS | `brew install foqerhk/tap/runeverything` | `curl …/install.sh \| sh` |
| Linux (Debian/Ubuntu) | `curl …/install-apt.sh \| sudo bash` 或 `apt install runeverything` | 通用 `install.sh` |
| Linux (其他) | `curl …/install.sh \| sh` | Arch：`packaging/aur/PKGBUILD` |
| Windows | Scoop / `install.ps1` | winget（manifest 待合入官方仓库） |

## macOS — Homebrew

```bash
brew install foqerhk/tap/runeverything
runeverything pair
brew services start runeverything
```

仓库：`foqerhk/runeverything`（主项目）+ `foqerhk/homebrew-tap`。

## Linux — Debian / Ubuntu（apt）

一键安装（下载 `.deb` 并用 apt 安装）：

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-apt.sh | sudo bash
```

添加本项目 APT 源（GitHub Pages）：

```bash
echo 'deb [trusted=yes] https://foqerhk.github.io/runeverything/apt stable main' \
  | sudo tee /etc/apt/sources.list.d/runeverything.list
sudo apt update
sudo apt install runeverything
```

包内含二进制与 user systemd unit：`systemctl --user enable --now runeverything`。

发版时由 `scripts/build-deb.sh` + `scripts/build-apt-repo.sh` 生成
`runeverything_<ver>_amd64.deb` / `arm64.deb` 与 `dist/apt/` 仓库树。

## Linux — 通用安装脚本

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.sh | sh
```

Linux 上若已装 Homebrew：

```bash
brew install foqerhk/tap/runeverything
```

### Arch Linux — AUR 风格 PKGBUILD

见 [`packaging/aur/PKGBUILD`](../packaging/aur/PKGBUILD)。可自行提交到 AUR，或：

```bash
cd packaging/aur
makepkg -si
```

## Windows — Scoop（推荐）

```powershell
scoop bucket add runeverything https://github.com/foqerhk/scoop-runeverything
scoop install runeverything
runeverything pair
```

Bucket 源码：[`packaging/scoop`](../packaging/scoop) → 仓库 `foqerhk/scoop-runeverything`。

### Windows — 安装脚本

```powershell
irm https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.ps1 | iex
```

### Windows — winget

Manifest 已放在 [`packaging/winget`](../packaging/winget)。需向 [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs) 提 PR；合并后：

```powershell
winget install Foqerhk.RunEverything
```

在官方合入前请用 Scoop 或 `install.ps1`。

## 发版流程（一次更新全渠道）

```bash
# 1. 交叉编译
./scripts/build-release.sh 0.1.0

# 2. 同步各渠道校验和
./scripts/sync-homebrew-formula.sh 0.1.0
./scripts/sync-scoop-manifest.sh 0.1.0
./scripts/sync-winget-manifest.sh 0.1.0
./scripts/sync-aur-pkgbuild.sh 0.1.0

# 3. 打 tag + GitHub Release（上传 dist/v0.1.0/*）
git tag v0.1.0 && git push origin v0.1.0
gh release create v0.1.0 dist/v0.1.0/* --title "v0.1.0" --generate-notes

# 4. 推送 tap / scoop bucket 仓库中的公式更新
```

## 资源命名

```
runeverything_darwin_arm64.tar.gz
runeverything_darwin_amd64.tar.gz
runeverything_linux_arm64.tar.gz
runeverything_linux_amd64.tar.gz
runeverything_windows_amd64.zip
runeverything_windows_arm64.zip
SHA256SUMS
```

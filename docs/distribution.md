# 分发与第三方安装渠道

目标：用户在各平台用一行命令安装 Agent（无需 GUI）。

## macOS — Homebrew（推荐）

用户侧：

```bash
brew install foqerhk/tap/runeverything
runeverything pair
# 可选常驻
brew services start runeverything
```

仓库侧准备：

| 仓库 | 用途 |
|------|------|
| `foqerhk/runeverything` | 主项目 + GitHub Releases 二进制 |
| `foqerhk/homebrew-tap` | Homebrew Tap（内容来自本仓库 `packaging/homebrew-tap/`） |

发布流程：

```bash
# 1. 主仓库打 tag 并推送
git tag v0.1.0 && git push origin v0.1.0

# 2. 交叉编译产物
./scripts/build-release.sh 0.1.0

# 3. 把 dist/v0.1.0/* 上传到 GitHub Release（可用 gh）
gh release create v0.1.0 dist/v0.1.0/* --title "v0.1.0" --notes "Initial release"

# 4. 把 sha256 写回 Formula
./scripts/sync-homebrew-formula.sh 0.1.0

# 5. 将 packaging/homebrew-tap 推到独立仓库 foqerhk/homebrew-tap
#    （首次：在 GitHub 新建 homebrew-tap，再 push Formula）
```

首次在未上架 Release 前，可用源码安装试通：

```bash
brew install --HEAD foqerhk/tap/runeverything
```

> Homebrew Core（无 tap 的 `brew install runeverything`）需要一定 star/知名度，早期请先用 **自定义 Tap**。

## Linux — 脚本 / Homebrew on Linux

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.sh | sh
```

若用户已装 Linuxbrew / Homebrew：

```bash
brew install foqerhk/tap/runeverything
```

## Windows — 脚本（后续可加 Scoop / winget）

```powershell
irm https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.ps1 | iex
```

后续可选：

- **Scoop** bucket：类似 Homebrew tap，放 `packaging/scoop/runeverything.json`
- **winget**：提交到 `microsoft/winget-pkgs`

## 资源命名约定

`scripts/build-release.sh` 产出：

```
runeverything_darwin_arm64.tar.gz
runeverything_darwin_amd64.tar.gz
runeverything_linux_arm64.tar.gz
runeverything_linux_amd64.tar.gz
runeverything_windows_amd64.zip
runeverything_windows_arm64.zip
SHA256SUMS
```

与 `scripts/install.sh`、Homebrew Formula 一致。

## GitHub 组织名

当前文档与 Formula 默认使用 `foqerhk/runeverything`。若你的 GitHub 用户名/组织不同，全局替换后再发布。

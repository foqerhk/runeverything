# RunEverything

开源、免费的跨平台远程 Agent：装在 Mac / Linux / Windows 上，用手机 App（如 KoKo）扫码连接，远程与 coding agent 交互。

- **开源、免费**，无 GUI
- 一行命令安装，终端打印**二维码**，扫码即连
- 通过 Relay 穿透 NAT（家用电脑不必开端口）

```
KoKo (手机)  ←── WSS ──→  Relay（中继）  ←── WSS ──→  RunEverything Agent（本机）
                                                      └─ PTY / agent CLI
```

---

## 安装

### 选你的系统

| 系统 | 推荐方式 |
|------|----------|
| **macOS** | Homebrew |
| **Linux** | 自动识别脚本（apt / dnf / zypper / pacman / apk） |
| **Windows** | Scoop 或 PowerShell 脚本 |

### macOS

```bash
brew install foqerhk/tap/runeverything
```

安装后配对并（可选）开机自启：

```bash
runeverything pair
brew services start runeverything
```

### Linux（推荐：自动识别发行版）

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-linux.sh | bash
```

脚本会按系统自动走 apt / dnf|yum|zypper / Arch / Alpine。

#### 或按发行版手动安装

**Debian / Ubuntu（apt）**

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-apt.sh | sudo bash
```

长期使用可加 APT 源：

```bash
echo 'deb [trusted=yes] https://foqerhk.github.io/runeverything/apt stable main' \
  | sudo tee /etc/apt/sources.list.d/runeverything.list
sudo apt update
sudo apt install runeverything
```

**Fedora / RHEL / CentOS / Rocky / Alma（dnf / yum）**

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-rpm.sh | sudo bash
```

**openSUSE（zypper）**

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-rpm.sh | sudo bash
```

**Arch / Manjaro**

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-arch.sh | sudo bash
```

或用仓库内 PKGBUILD：`cd packaging/aur && makepkg -si`

**Alpine**

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-apk.sh | sudo bash
```

**任意 Linux（通用脚本，不走包管理器）**

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.sh | sh
```

### Windows

**Scoop（推荐）**

```powershell
scoop bucket add runeverything https://github.com/foqerhk/scoop-runeverything
scoop install runeverything
```

**PowerShell 一键脚本**

```powershell
irm https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.ps1 | iex
```

---

## 使用

### 1. 配置中继（必做）

家用电脑在 NAT 后面时，需要一个可达的 Relay。先设环境变量或改配置文件：

```bash
export RE_RELAY=wss://你的中继地址/ws
# 可选：写进配置
# ~/.runeverything/config.json 里的 relay_url / public_relay
```

本地自测可以起仓库自带的 Relay：

```bash
# 另开一个终端
go run ./cmd/relay -listen :8787
# Agent 侧
export RE_RELAY=ws://127.0.0.1:8787/ws
```

### 2. 启动 Agent 并扫码

```bash
runeverything          # 或：runeverything run
```

终端会打印二维码和 JSON / `koko://pair?...` 深链。用 KoKo（或兼容客户端）扫码完成配对。

重新显示二维码：

```bash
runeverything pair
```

查看本机设备信息：

```bash
runeverything status
```

### 3. 常驻后台（可选）

| 系统 | 方式 |
|------|------|
| macOS | `brew services start runeverything` |
| Linux（systemd） | `systemctl --user enable --now runeverything` |
| Windows | 安装脚本会注册登录计划任务；或手动运行 `runeverything run -no-qr` |

### 常用命令

| 命令 | 说明 |
|------|------|
| `runeverything` / `runeverything run` | 连接 Relay，常驻并提供远程会话 |
| `runeverything pair` | 刷新配对令牌并打印二维码 |
| `runeverything status` | 显示 device_id、Relay、配置路径 |
| `runeverything version` | 打印版本号 |

### 配置说明

默认目录：`~/.runeverything/`（可用 `RE_HOME` 覆盖）

| 环境变量 | 说明 | 默认 |
|----------|------|------|
| `RE_RELAY` | Agent 连接的 Relay WebSocket 地址 | `ws://127.0.0.1:8787/ws` |
| `RE_HOME` | 身份与配置目录 | `~/.runeverything` |

`config.json` 示例：

```json
{
  "relay_url": "wss://your-relay.example/ws",
  "public_relay": "wss://your-relay.example/ws"
}
```

`public_relay` 会写进二维码，供手机端连接（可与 Agent 内网 `relay_url` 不同）。

---

## 开发者本地联调

需要 Go 1.24+。

```bash
go build -o bin/relay ./cmd/relay
go build -o bin/runeverything ./cmd/agent
go build -o bin/retest ./cmd/retest

# 终端 1
./bin/relay -listen :8787

# 终端 2
./bin/runeverything -relay ws://127.0.0.1:8787/ws

# 终端 3：用二维码 JSON 里的 device_id / pairing_token
./bin/retest -device DEVICE -token TOKEN
```

发版与各渠道打包细节见 [docs/distribution.md](docs/distribution.md)。

## 文档

- [架构说明](docs/architecture.md)
- [协议规范](docs/protocol.md)
- [KoKo 对接](docs/koko-integration.md)
- [分发渠道（brew / apt / rpm / Scoop…）](docs/distribution.md)

## 许可证

MIT

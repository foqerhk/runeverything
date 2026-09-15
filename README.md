# RunEverything

开源、免费的跨平台远程 Agent 服务，为 **Intent Computing** 提供桌面端 / 服务器端通信能力。

同类产品有蒲公英远程桌面等；RunEverything 的差异是：

- **开源、免费**
- **面向 Intent Computing**：手机 App（如 KoKo）远程连接本机，与 coding agent 交互
- **无 GUI**：一行命令安装，终端打印二维码，扫码即连

## 架构一览

```
KoKo (手机)  ←── WSS ──→  Relay（中继）  ←── WSS ──→  RunEverything Agent（本机）
                                                      └─ PTY / agent CLI
```

Agent 主动出站连 Relay，穿透 NAT；手机从不直连家用网络。

## 快速开始（开发）

需要 Go 1.24+（macOS 26+ 上请勿使用过旧工具链，否则二进制可能缺 `LC_UUID`）。

```bash
# 构建
go build -o bin/relay ./cmd/relay
go build -o bin/runeverything ./cmd/agent
go build -o bin/retest ./cmd/retest

# 终端 1：启动本地 Relay
./bin/relay -listen :8787

# 终端 2：启动 Agent（打印配对二维码）
./bin/runeverything -relay ws://127.0.0.1:8787/ws

# 重新显示配对二维码
./bin/runeverything pair

# 查看状态
./bin/runeverything status

# 终端 3：用配对 JSON 里的 device_id / pairing_token 做联调
./bin/retest -device DEVICE -token TOKEN
```

## 一行安装（发布后）

### macOS（Homebrew）

```bash
brew install foqerhk/tap/runeverything
runeverything pair
brew services start runeverything
```

### Linux

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.sh | sh
```

### Windows（Scoop）

```powershell
scoop bucket add runeverything https://github.com/foqerhk/scoop-runeverything
scoop install runeverything
```

### Windows（脚本）

```powershell
irm https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.ps1 | iex
```

各渠道发版与校验和同步见 [docs/distribution.md](docs/distribution.md)。
## 命令

| 命令 | 说明 |
|------|------|
| `runeverything` / `runeverything run` | 连接 Relay 并保持常驻 |
| `runeverything pair` | 刷新并打印配对二维码 |
| `runeverything status` | 显示设备 ID、Relay、配置路径 |

## 配置

默认目录：`~/.runeverything/`

| 环境变量 | 说明 | 默认 |
|----------|------|------|
| `RE_RELAY` | Relay WebSocket URL | `ws://127.0.0.1:8787/ws` |
| `RE_HOME` | 配置与身份目录 | `~/.runeverything` |

## 文档

- [架构说明](docs/architecture.md)
- [协议规范](docs/protocol.md)
- [KoKo 对接](docs/koko-integration.md)
- [分发 / Homebrew 上架](docs/distribution.md)

## 许可证

MIT

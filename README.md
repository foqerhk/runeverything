# RunEverything

开源、免费的跨平台远程 Agent：装在 Mac / Linux / Windows 上，用手机 App（如 KoKo）扫码连接，远程与 coding agent 交互。

- **开源、免费**；Windows 提供安装包 + 托盘，Mac / Linux 命令行
- 一行命令或下载 Setup 安装，扫码即连
- 通过 Relay 穿透 NAT（家用电脑不必开端口）

```
KoKo (手机)  ←── wss 信令 + UDP 画面/键鼠 ──→  Relay  ←── 同 ──→  Agent（本机桌面）
                                                      └─ PTY / agent CLI
```

---

## 安装

### 选你的系统

| 系统 | 推荐方式 |
|------|----------|
| **Windows** | 下载 `RunEverythingSetup_amd64.exe` 安装（托盘 + 开机自启） |
| **macOS** | Homebrew |
| **Linux** | 自动识别脚本（apt / dnf / zypper / pacman / apk） |

### Windows（推荐：安装包）

1. 打开 [Releases](https://github.com/foqerhk/runeverything/releases/latest)
2. 按电脑架构下载安装包：
   - **`RunEverythingSetup_amd64.exe`** — 常见 64 位 PC（推荐）
   - **`RunEverythingSetup_arm64.exe`** — Windows on Arm
   - **`RunEverythingSetup_386.exe`** — 32 位 Windows
3. 双击安装（装到 `%LOCALAPPDATA%\RunEverything`，无需管理员）
4. 右下角托盘图标 → **Show pairing QR**，用 KoKo 扫码

系统要求：**Windows 10 / 11**（及对应 Server）。当前 Go 工具链自 1.21 起**不再支持 Windows 7 / 8**。

也可继续用 PowerShell / Scoop / winget / Chocolatey（见下文）。

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

推荐 Launchpad PPA：

```bash
sudo add-apt-repository ppa:foqerhk/runeverything
sudo apt update
sudo apt install runeverything
```

或一条命令（优先 PPA，失败则回退 GitHub Pages / Release .deb）：

```bash
curl -fsSL https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-apt.sh | sudo bash
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

Windows 上任选一种即可（推荐从上到下）：

#### 1）PowerShell 一键安装（备选）

以普通用户打开 PowerShell：

```powershell
irm https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.ps1 | iex
```

脚本会：下载发布包 → 装到 `%LOCALAPPDATA%\RunEverything\bin` → 加入用户 PATH → 注册登录自启（`runeverything tray`）→ 可再配对。

可选：先指定中继再装：

```powershell
$env:RE_RELAY = "wss://your-relay.example/re2"
irm https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install.ps1 | iex
```

#### 2）Scoop

```powershell
scoop bucket add runeverything https://github.com/foqerhk/scoop-runeverything
scoop install runeverything
runeverything pair
```

#### 3）winget（用仓库内清单，无需等官方源收录）

需已安装 [App Installer / winget](https://aka.ms/getwinget)：

```powershell
irm https://raw.githubusercontent.com/foqerhk/runeverything/main/scripts/install-winget.ps1 | iex
```

或在克隆仓库后：

```powershell
winget install --manifest .\packaging\winget\Foqerhk.RunEverything\0.1.0
```

#### 4）Chocolatey

```powershell
choco install runeverything -y --source "https://github.com/foqerhk/runeverything/releases/download/v0.1.0/"
```

（需本机已安装 Chocolatey；nupkg 随 Release 发布。）

---

## 使用

### 1. 中继（BTC 风格 P2P 发现）

**家庭 / NAT（Mac 等）**：默认从 GitHub 官方 **seeds.json** 起步，向种子节点拉取 `/v1/peers`，递归发现更多公益节点，再对候选做 health ping，**选延迟最低**的作为转发。也可手动指定：

```bash
export RE_RELAY=wss://你的中继地址/re2
# config.json 可设 "relay_manual": true 锁定，不再自动换节点
```

**公网 Linux**：跑 Relay 时默认把自己 gossip 进网络（可被家庭用户发现）。这是**带宽公益**，可随时关闭：

```bash
export RE_SHARE_RELAY=0
go run ./cmd/relay -listen :8787 -share=false
```

**推荐（官方子域 + 证书，手机可直连 wss）**：志愿者无需自备域名；开 80/443，设官方 Registrar 地址后自动领取子域并申请证书：

```bash
export RE_AUTO_DOMAIN=1
export RE_REGISTRAR_URL=https://registrar.example.com
# 无需手工 join token：开启分享时自动 enroll
go run ./cmd/relay -auto-domain
```

**RE2.1（信令 wss + 自研 UDP 数据面；Noise E2E；默认远程桌面，可选 PTY）**：

```bash
# 终端 1
go run ./cmd/relay -listen :8787 -public ws://127.0.0.1:8787/re2 -allow-ws
# 终端 2
RE_RELAY=ws://127.0.0.1:8787/re2 go run ./cmd/agent
# 终端 3：用二维码里的 device/token/noise_pub
go run ./cmd/retest -relay ws://127.0.0.1:8787/re2 -device … -token … -noise-pub …
```

规范见 [docs/protocol-v2.md](docs/protocol-v2.md)、KoKo 交接 [docs/koko-re2-handoff.md](docs/koko-re2-handoff.md)。

说明见 [docs/official-domain.md](docs/official-domain.md)（官网闭源 Registrar + 本仓库公开客户端）。

#### 公益中继说明（必读）

- 官方只在 GitHub 维护**初始种子列表**（`docs/seeds.json` → Pages `/seeds.json`）；其余节点靠 P2P 互相交换地址发现。
- 志愿者贡献的是**连通性 / 带宽**；中继**不能**解密画面/键鼠/PTY（仅见密文）。
- 谁能操作你的电脑，仍只取决于谁扫了你的配对码。
- 种子 URL：`RE_SEEDS_URL`（默认 Pages + raw GitHub）；或 `RE_SEEDS=wss://a/re2,wss://b/re2` 直接指定。

本地自测：

```bash
# 终端 1 — 种子/公益节点
go run ./cmd/relay -listen :8787 -public ws://127.0.0.1:8787/re2 -allow-ws -share=false
# 终端 2 — Agent（手动指定本地，跳过公网发现）
export RE_RELAY=ws://127.0.0.1:8787/re2
go run ./cmd/agent
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

### 3. 常驻后台（家庭桌面 24h 在线）

家庭电脑要当远端，需要**进程常驻 + 尽量不睡眠**。锁屏一般不断网；真正会掉线的是睡眠/休眠/断网。

| 系统 | 方式 |
|------|------|
| macOS | `brew services start runeverything` 或 install.sh 的 launchd |
| Linux 桌面 | `systemctl --user enable --now runeverything`（安装脚本会尝试 `loginctl enable-linger`） |
| Windows | 安装脚本注册登录计划任务 |

Agent 运行时会自动防闲置睡眠（macOS `caffeinate`、Windows Away Mode、Linux `systemd-inhibit`）。可用 `RE_KEEP_AWAKE=0` 关闭。

建议：

- 插着电源；笔记本合盖可能仍睡眠（macOS 需合盖模式 + 外接电源/显示器）
- 系统设置里把「自动睡眠」调长或关掉；允许锁屏，不必一直亮屏
- 路由器勿踢长连接；公司网/访客 Wi‑Fi 可能限制常驻

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
| `RE_RELAY` | Agent 连接的 Relay（设置后不再 P2P 发现） | seeds → crawl → 最低 ping |
| `RE_SHARE_RELAY` | 公网 Relay 是否把自己 gossip 出去 | `1`（`0` 关闭） |
| `RE_AUTO_DOMAIN` | Relay 自动领取官方子域 + ACME wss | 关闭 |
| `RE_REGISTRAR_URL` | 官方 Registrar（claim / 滥用上报） | 默认按地区：`.cn` / `.net` getnode |
| `RE_JOIN_TOKEN` | 可选；有则跳过自动 enroll | 空（自动领取） |
| `RE_REPORT` | 家庭端失败是否上报 registrar | `1`（`0` 关） |
| `RE_REGION` | 强制区域 `cn` / `intl` | 自动检测 |
| `RE_SEEDS_URL` | 官方种子列表 JSON | 默认按地区 getnode `/seeds.json`（国内不跨境） |
| `RE_SEEDS` | 逗号分隔种子 URL（覆盖文件） | 空 |
| `RE_KEEP_AWAKE` | Agent 运行时阻止闲置睡眠 | `1`（`0` 关闭） |
| `RE_HOME` | 身份与配置目录 | `~/.runeverything` |

`config.json` 示例：

```json
{
  "relay_url": "wss://your-relay.example/re2",
  "public_relay": "wss://your-relay.example/re2",
  "relay_manual": true,
  "share_relay": false
}
```

`public_relay` 会写进二维码，供手机端连接（可与 Agent 内网 `relay_url` 不同）。

---

## 开发者本地联调

需要 Go 1.25+。

```bash
go build -o bin/relay ./cmd/relay
go build -o bin/runeverything ./cmd/agent
go build -o bin/retest ./cmd/retest

# 终端 1
./bin/relay -listen :8787

# 终端 2
./bin/runeverything -relay ws://127.0.0.1:8787/re2

# 终端 3：用二维码 JSON 里的 device_id / pairing_token / noise_pub
./bin/retest -relay ws://127.0.0.1:8787/re2 -device DEVICE -token TOKEN -noise-pub NOISE_PUB
```

发版与各渠道打包细节见 [docs/distribution.md](docs/distribution.md)。

## 文档

- [架构说明](docs/architecture.md)
- [协议规范（RE2）](docs/protocol-v2.md)
- [KoKo 对齐交接](docs/koko-re2-handoff.md)
- [分发渠道（brew / apt / rpm / Scoop…）](docs/distribution.md)

## 许可证

MIT

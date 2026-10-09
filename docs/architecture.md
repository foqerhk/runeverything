# Architecture

## Role in Intent Computing

- **KoKo** (mobile): remote control + (optional) developer PTY to a home machine.
- **RunEverything**: always-on Agent + volunteer Relay (NAT traversal).

## Components

```
┌─────────┐  wss /re2 (signaling)  ┌───────┐  wss /re2  ┌─────────┐
│  KoKo   │ ─────────────────────► │ Relay │ ◄───────── │  Agent  │
│         │  UDP REUDP (media)     │       │  UDP       │         │
└─────────┘ ─────────────────────► └───────┘ ◄───────── └────┬────┘
                                                              │
                                                    screen + input (+ PTY)
```

1. **Agent** (`cmd/agent`)
   - wss REGISTER；UDP ASSOC；Noise；默认 Desktop（`internal/desktop`），PTY / AI 会话
   - QR **v=3** 含 `noise_pub` + `udp` + `lan[]`（同一局域网时手机直连，不经中继转发媒体）
   - 存活检测：桌面 15s 无心跳即关闭；`RE_SESSION_IDLE`（默认 2m）无流量释放 Noise 会话，PTY 保留
   - `ide-mirror`：把 Cursor IDE 对话镜像进 PTY，手机端原生渲染（OSC 7788/7789/7790，见 protocol-v2）

2. **Relay** (`cmd/relay`)
   - `/re2` WebSocket 信令；同端口（或 `-udp`）REUDP 转发
   - 不解密 DATA 载荷
   - 每台电脑同时只允许一个控制端：另一台设备 BIND 时回 `controller_busy`，用户确认后 `force` 接管
   - 状态全在内存（配对令牌、session_ticket、控制端）：重启后手机需重新扫码

3. **Install scripts** — launchd / systemd / Scheduled Task

## Trust model

Whoever scans a valid pairing QR can control the desktop (and open PTY). Pairing tokens expire after 10 minutes; until then the same QR can be redeemed again (each redeem gets its own session ticket), and publishing a new QR invalidates the previous token. Optional hardening: `RE_PAIR_CONFIRM=1` (confirm on the host) and `RE_ACCESS_PASSWORD` (session password).

Only one phone controls a computer at a time. A second phone sees who is in control and must confirm before taking over; the displaced phone is told and stops reconnecting.

## Persistence

| Path | Purpose |
|------|---------|
| `~/.runeverything/identity.json` | device_id + secret |
| `~/.runeverything/config.json` | relay URLs |
| `~/.runeverything/last_pairing.json` | last QR |
| `~/.runeverything/noise_static.json` | Noise long-term key |

## Local development

```bash
go run ./cmd/relay -listen :8787 -public ws://127.0.0.1:8787/re2 \
  -public-udp 127.0.0.1:8787 -allow-ws -share=false

RE_DESKTOP_FAKE=1 go run ./cmd/agent -relay ws://127.0.0.1:8787/re2

go run ./cmd/retest -desktop -relay ws://127.0.0.1:8787/re2 -udp 127.0.0.1:8787 \
  -device … -token … -noise-pub …
```

## Production notes

- 区域节点与部署：见 [regions.md](./regions.md)。
- 官方域名 / ACME：见 [official-domain.md](./official-domain.md)。**UDP 不能走 Cloudflare 代理**（灰云仅影响 HTTP(S)；节点域保持 DNS-only）。
- 协议：[protocol-v2.md](./protocol-v2.md)；KoKo：[koko-re2-handoff.md](./koko-re2-handoff.md)

## Volunteer relay directory (P2P)

1. Seeds：按地区只拉本区 getnode `/seeds.json`（国内 `.cn`，国外 `.net`；详见 [regions.md](./regions.md)）；国外可另加 GitHub 镜像
2. `/v1/peers` gossip；Peer 可带 `udp`
3. Agent 选最低 ping 的 wss；`udp` 来自 REGISTER_OK / QR

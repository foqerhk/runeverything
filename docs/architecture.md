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
   - wss REGISTER；UDP ASSOC；Noise；默认 Desktop（`internal/desktop`），可选 PTY
   - QR **v=3** 含 `noise_pub` + `udp`

2. **Relay** (`cmd/relay`)
   - `/re2` WebSocket 信令；同端口（或 `-udp`）REUDP 转发
   - 不解密 DATA 载荷

3. **Install scripts** — launchd / systemd / Scheduled Task

## Trust model

Whoever scans a valid pairing QR can control the desktop (and open PTY). Pairing tokens expire (~10 min), one-shot.

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

- 官方域名 / ACME：见 [official-domain.md](./official-domain.md)。**UDP 不能走 Cloudflare 代理**（灰云仅影响 HTTP(S)；节点域保持 DNS-only）。
- 协议：[protocol-v2.md](./protocol-v2.md)；KoKo：[koko-re2-handoff.md](./koko-re2-handoff.md)

## Volunteer relay directory (P2P)

1. Seeds：优先 `https://getnode.intentcomputing.cn/seeds.json`，回退 GitHub Pages / `docs/seeds.json`（`url` + 可选 `udp`）
2. `/v1/peers` gossip；Peer 可带 `udp`
3. Agent 选最低 ping 的 wss；`udp` 来自 REGISTER_OK / QR

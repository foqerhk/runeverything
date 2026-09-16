# Architecture

## Role in Intent Computing

- **KoKo** (mobile): Intent Computing client — talk to coding agent on a remote machine.
- **RunEverything** (this repo): always-on agent on Mac / Linux / Windows + optional Relay for NAT traversal.

KoKo Phase 1 uses SSH directly. RunEverything adds a **scan-to-pair** path with a custom protocol so home PCs behind NAT work without port forwarding.

## Components

```
┌─────────────┐         WSS          ┌─────────────┐         WSS          ┌──────────────────┐
│  KoKo App   │ ───────────────────► │    Relay    │ ◄─────────────────── │ Agent (this host)│
│  (client)   │                      │  cmd/relay  │   outbound tunnel    │   cmd/agent      │
└─────────────┘                      └─────────────┘                      └────────┬─────────┘
                                                                                   │
                                                                                   ▼
                                                                            PTY / tmux / agent CLI
```

1. **Agent** (`cmd/agent`, binary name `runeverything`)
   - Stores identity in `~/.runeverything/identity.json`
   - Outbound WebSocket to Relay (`?role=agent`)
   - Prints pairing QR (JSON + `koko://pair?...`)
   - Opens local PTY sessions on request

2. **Relay** (`cmd/relay`)
   - Accepts agent and client WebSockets
   - Holds pairing tokens and session tokens (MVP: in-memory)
   - Routes control + binary PTY frames by `device_id` / `session_id`

3. **Install scripts**
   - `scripts/install.sh` — macOS launchd / Linux systemd --user
   - `scripts/install.ps1` — Windows scheduled task

## Trust model (MVP)

Whoever scans a valid pairing QR can obtain a `session_token` and open PTY sessions on that device. There is no multi-user account system yet. Pairing tokens expire (~10 minutes) and are one-shot.

## Persistence

| Path | Purpose |
|------|---------|
| `~/.runeverything/identity.json` | `device_id` + `device_secret` |
| `~/.runeverything/config.json` | relay URLs |
| `~/.runeverything/last_pairing.json` | last QR payload (debug) |
| `~/.runeverything/agent.log` | launchd/systemd logs (when installed) |

## Local development

```bash
# Terminal 1 — public/seed relay (enable -allow-ws for local ws://)
go run ./cmd/relay -listen :8787 -public ws://127.0.0.1:8787/ws -allow-ws

# Terminal 2 — another volunteer (gossips with seed via RE_SEEDS)
RE_SEEDS=ws://127.0.0.1:8787/ws \
  go run ./cmd/relay -listen :8788 -public ws://127.0.0.1:8788/ws -allow-ws

# Terminal 3 — home agent discovers via seeds then picks lowest ping
RE_SEEDS=ws://127.0.0.1:8787/ws \
  go run ./cmd/agent
```

## Production notes

- Put Relay behind TLS (e.g. Caddy/Nginx) and advertise `wss://...` as `public_relay`.
- Agent may use an internal `relay_url` while QR embeds `public_relay`.
- Replace in-memory auth store before multi-instance Relay deployment.

## Volunteer relay directory (P2P)

Bitcoin-style discovery:

1. Official **seed list** on GitHub: `docs/seeds.json` (Pages `/seeds.json`).
2. Each relay exposes `GET|POST /v1/peers` and gossips addr lists with known peers.
3. Home agents: load seeds → crawl `/v1/peers` → probe `/healthz` → **lowest ping** wins.
4. Opt out of advertising: `RE_SHARE_RELAY=0` / `-share=false`.
5. Volunteer relays are **connectivity only**, not a confidentiality boundary.

`cmd/directory` remains an optional centralized registry for ops; the default path is P2P.

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
# Terminal 1
go run ./cmd/relay -listen :8787

# Terminal 2
go run ./cmd/agent -relay ws://127.0.0.1:8787/ws

# Terminal 3 — after reading device_id + pairing_token from QR JSON
go run ./cmd/retest -relay ws://127.0.0.1:8787/ws -device DEVICE -token TOKEN
```

## Production notes

- Put Relay behind TLS (e.g. Caddy/Nginx) and advertise `wss://...` as `public_relay`.
- Agent may use an internal `relay_url` while QR embeds `public_relay`.
- Replace in-memory auth store before multi-instance Relay deployment.

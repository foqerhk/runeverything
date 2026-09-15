# KoKo Integration Guide

This document is the contract for wiring **KoKo** (Intent Computing mobile app) to **RunEverything**.

SSH support in KoKo can remain; RunEverything is the scan-to-pair / NAT-friendly path.

## Pairing flow

1. User installs Agent on the machine (`install.sh` / `install.ps1` or `go run ./cmd/agent`).
2. Agent prints a QR whose content is JSON:

```json
{
  "v": 1,
  "relay": "wss://relay.example/ws",
  "device_id": "...",
  "pairing_token": "...",
  "name": "MacBook",
  "expires_at": 1710000000
}
```

Deep link form (same fields): `koko://pair?v=1&relay=...&device_id=...&pairing_token=...&name=...&expires_at=...`

3. KoKo scans QR / opens deep link, opens WebSocket:

`{relay}?role=client`

4. Send `pair_redeem` → receive `pair_ack` with `session_token`.
5. Persist `{ device_id, session_token, relay, name }` like a “host” entry (alongside SSH hosts).
6. Later connects: `client_hello` with stored token → `session_open` → binary PTY I/O.

See [protocol.md](protocol.md) for full frame definitions.

## Suggested KoKo data model

Extend host/session model (names illustrative):

| Field | Notes |
|-------|--------|
| `transport` | `ssh` \| `runeverything` |
| `relayURL` | from QR `relay` |
| `deviceID` | from QR |
| `sessionToken` | from `pair_ack` |
| `displayName` | QR `name` |
| `projectDirs[]` | same as SSH hosts — passed as `cwd` on `session_open` |

## Opening a coding agent session

When user enters a project:

```json
{
  "type": "session_open",
  "data": {
    "session_id": "<unique ≤16 chars>",
    "cwd": "/Users/you/proj",
    "cmd": ["agent"],
    "cols": 80,
    "rows": 24,
    "use_tmux": true,
    "tmux_name": "koko-<project-slug>"
  }
}
```

- Prefer `use_tmux: true` so disconnect/re-attach matches current KoKo SSH + tmux behavior.
- If `agent` is not on PATH, open shell (`cmd` omitted) and let the user start it, or detect and show guidance (same UX as SSH path).
- On resize (keyboard / rotation), send `resize` with new cols/rows.
- stdin: binary frames `dir=1`; stdout: `dir=2` (see protocol).

## Terminal UX parity

Reuse SwiftTerm (or Android equivalent) against the PTY byte stream — no SSH library required on this path.

Still handle:

- agent login URL detection in scrollback (existing KoKo feature)
- Shortcut bar (Esc, Ctrl, arrows, …) → write corresponding bytes to stdin frames
- Scrollback retained on device; remote tmux keeps process state

## Error handling

| Code / situation | App behavior |
|------------------|--------------|
| `pair_failed` | QR expired — ask user to run `runeverything pair` |
| `offline` | Agent not connected to relay — show retry |
| `auth_failed` | Clear stored token — re-pair |
| Relay TLS / network errors | Same as failed SSH connect messaging |

## Out of scope for RunEverything (for now)

- Changing KoKo source in this repository
- Full remote desktop framebuffer
- Cloud account / multi-tenant auth

## Smoke test without KoKo

```bash
go run ./cmd/relay -listen :8787
go run ./cmd/agent -relay ws://127.0.0.1:8787/ws
# copy device_id + pairing_token from printed JSON
go run ./cmd/retest -device ... -token ...
```

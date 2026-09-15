# RunEverything Protocol v1

Transport: WebSocket (ws/wss). Control plane = **text** JSON frames. PTY data = **binary** frames.

Query parameter on connect:

| role | meaning |
|------|---------|
| `agent` | Desktop/server RunEverything process |
| `client` | KoKo / retest / other client |

Example: `ws://relay.example/ws?role=agent`

## Envelope (text)

```json
{
  "type": "register",
  "id": "optional-correlation-id",
  "data": { }
}
```

## Agent lifecycle

### `register` (agent → relay)

```json
{
  "type": "register",
  "data": {
    "device_id": "...",
    "device_secret": "...",
    "name": "hostname",
    "os": "darwin",
    "arch": "amd64"
  }
}
```

### `register_ok` (relay → agent)

```json
{ "type": "register_ok", "data": { "device_id": "..." } }
```

### `pair_offer` (agent → relay)

Publishes a short-lived pairing token (TTL ~10 minutes). Previous tokens for the same device are invalidated.

```json
{
  "type": "pair_offer",
  "data": {
    "device_id": "...",
    "pairing_token": "...",
    "name": "hostname",
    "expires_at": 1710000000,
    "relay": "wss://relay.example/ws"
  }
}
```

## Client pairing

### `pair_redeem` (client → relay)

```json
{
  "type": "pair_redeem",
  "data": {
    "device_id": "...",
    "pairing_token": "..."
  }
}
```

### `pair_ack` (relay → client)

```json
{
  "type": "pair_ack",
  "data": {
    "device_id": "...",
    "session_token": "...",
    "name": "hostname",
    "relay": "wss://relay.example/ws"
  }
}
```

`session_token` is long-lived (MVP: in-memory until relay restart). Pairing token is one-shot.

### `client_hello` / `client_ok`

```json
{
  "type": "client_hello",
  "data": { "device_id": "...", "session_token": "..." }
}
```

```json
{
  "type": "client_ok",
  "data": { "device_id": "...", "online": true }
}
```

## Sessions (PTY)

### `session_open` (client → relay → agent)

```json
{
  "type": "session_open",
  "data": {
    "session_id": "16charsmaxxxxxx",
    "cwd": "/path/to/project",
    "cmd": ["agent"],
    "cols": 80,
    "rows": 24,
    "use_tmux": true,
    "tmux_name": "koko-proj"
  }
}
```

- `session_id`: max 16 bytes (UTF-8), used in binary frames.
- `cmd` empty → login shell (`$SHELL` or bash / PowerShell on Windows).
- `use_tmux: true` → `tmux new-session -A -s <name>` when tmux exists.

### `session_ready` / `session_close` / `resize`

Standard control messages with `session_id`. Resize:

```json
{
  "type": "resize",
  "data": { "session_id": "...", "cols": 120, "rows": 40 }
}
```

## Binary PTY frames

| offset | field |
|--------|--------|
| 0 | direction: `1` = stdin (client→agent), `2` = stdout (agent→client) |
| 1..16 | `session_id` UTF-8, zero-padded |
| 17.. | raw bytes |

Relay multiplexes by `device_id` + `session_id` route table.

## QR / pairing payload

JSON (also printable as `koko://pair?...` deep link):

```json
{
  "v": 1,
  "relay": "wss://relay.example/ws",
  "device_id": "...",
  "pairing_token": "...",
  "name": "hostname",
  "expires_at": 1710000000
}
```

## Errors

```json
{
  "type": "error",
  "id": "...",
  "data": { "code": "offline", "message": "device agent offline" }
}
```

Common codes: `bad_role`, `bad_json`, `auth_failed`, `pair_failed`, `not_authed`, `offline`, `session_open_failed`.

## Keepalive

Either side may send `{ "type": "ping" }`; peer replies `{ "type": "pong" }`. WebSocket ping/pong may also be used by the stack.

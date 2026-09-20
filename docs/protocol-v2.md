# Protocol RE2 / RE2.1（生产）

版本：**3**（配对 QR）。应用帧仍为 RE2；**数据面为自研 UDP（REUDP）**。

KoKo 对齐：→ [koko-re2-handoff.md](./koko-re2-handoff.md)

## 目标

- **自研应用层协议** + **Noise E2E**（中继不可读）
- **信令**：`wss://…/re2`（ATS / 证书 / 配对）
- **数据**：自研 UDP **REUDP**（画面 / 键鼠 / 可选 PTY）——不用 QUIC/WebRTC

## 双平面

| 平面 | 载体 | 职责 |
|------|------|------|
| 信令 | `wss://host/re2` | REGISTER / PAIR / BIND，下发 `udp` |
| 数据 | UDP `host:port` | ASSOC → Noise → OPEN_DESKTOP / VIDEO / INPUT |

QR `v:3` 必填字段增加 `udp`（`host:port`）。Cloudflare 灰云**不能**代理 UDP；节点域保持 DNS-only。

## 连接顺序

1. Client / Agent 分别连 `wss://…/re2?role=…`
2. Agent `REGISTER` → `REGISTER_OK{udp}`；Client `PAIR` → `BIND` → `BIND_OK{udp}`
3. 两端 UDP `ASSOC`（agent 用 device_secret，client 用 session_ticket）
4. Client 发起 Noise_XXpsk3（与 RE2 相同 PSK/prologue），握手消息走 **REUDP DATA 可靠通道**
5. 默认 `OPEN_DESKTOP`；开发者仍可 `OPEN_SESSION`（PTY）

## REUDP 外层包

```
magic u16 = 0x5255 ("RU")
version u8 = 1
ptype u8
flags u8          // RELIABLE | FIN | LATEST
rsv u8
route_hash u32    // FNV-1a(device_id)
seq u32
ack u32
payload_len u16   // ≤ 1200
payload
```

| ptype | 含义 |
|-------|------|
| ASSOC / ASSOC_OK | 绑定 UDP 五元组到已有 wss 会话 |
| DATA | 不透明（Noise / AEAD 密文） |
| ACK | 可靠通道确认 |
| PING/PONG | 保活 |

可靠性：控制/键鼠按钮/Noise = 可靠；鼠标移动 = Latest；视频分片 = 不可靠。拥塞：AIMD + pacing（`internal/reudp`）。

## 隧道内消息（加密后）

保留 PTY 类型 `0x01–0x07`，并增加：

| msg | 含义 |
|-----|------|
| `0x20` OPEN_DESKTOP | max_width/height, fps, codec=h264 |
| `0x21` DESKTOP_READY | width/height/codec |
| `0x22` DESKTOP_CLOSE | |
| `0x23` VIDEO | session + frame_id + flags + part/parts + Annex-B |
| `0x24` INPUT_MOUSE | 归一化 x,y + buttons |
| `0x25` INPUT_KEY | |
| `0x26` INPUT_TOUCH | 映射鼠标 |
| `0x30` AUDIO | 预留 |
| `0x31` CLIPBOARD | 预留 |

VIDEO 线协议：**H.264 Annex-B**（禁止把 MJPEG 写进长期协议）。

## Noise

与 RE2.0 相同：`Noise_XXpsk3_25519_ChaChaPoly_SHA256`，prologue `runeverything-re2-v2`，PSK=`SHA256("re2-psk-v1|"+token)`。

## 中继

- wss：原样转发 NOISE/TUNNEL
- UDP：ASSOC 校验后按 route 转发 DATA/ACK；ASSOC 前限速防放大

## 能力清单（公益 · 无账号）

- 多显示器 / 选屏、光标分离、剪贴板、文件传输、STATS→ABR、关键帧请求
- 中继 `/v1/capacity` + `scripts/probe-bandwidth.sh` 实测限流；家庭端 **延迟+繁忙** 选路
- 会话空闲超时、审计日志、打洞信令、互踢；KoKo 见 handoff（无登录）

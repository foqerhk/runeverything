# Protocol RE2 / RE2.1（生产）

版本：**3**（配对 QR）。应用帧仍为 RE2；**数据面为自研 UDP（REUDP）**。
常量以代码为准：外层/内层类型见 `internal/re2/frame.go`，控制载荷见 `internal/re2/ctrl.go`，REUDP 见 `internal/reudp`。

KoKo 对齐：→ [koko-re2-handoff.md](./koko-re2-handoff.md)

## 目标

- **自研应用层协议** + **Noise E2E**（中继不可读）
- **信令**：`wss://…/re2`（ATS / 证书 / 配对 / BIND）
- **数据**：自研 UDP **REUDP**（画面 / 键鼠 / PTY）——不用 QUIC/WebRTC；UDP 不通时退回 WSS 隧道

## 双平面

| 平面 | 载体 | 职责 |
|------|------|------|
| 信令 | `wss://host/re2` | REGISTER / PAIR / BIND，下发 `udp`；UDP 不通时也承载 NOISE / TUNNEL |
| 数据 | UDP `host:port`（经中继）或局域网直连 | ASSOC → Noise → OPEN_DESKTOP / VIDEO / INPUT |

QR `v:3` 字段：`relay`、`device_id`、`pairing_token`、`name`、`expires_at`、`noise_pub`、`udp`（`host:port`）、`lan[]`（Agent 局域网 `ip:port` 候选）。Cloudflare 灰云**不能**代理 UDP；节点域保持 DNS-only。

## 连接顺序

1. Client / Agent 分别连 `wss://…/re2`
2. Agent `REGISTER` → `REGISTER_OK{udp}`，再 `PAIR_OFFER{pairing_token, expires_at}`
3. Client 首次扫码：`PAIR_REDEEM` → `PAIR_ACK{session_ticket}`；之后重连只用 ticket，不再兑换二维码
4. Client `BIND{device_id, session_ticket, client_id, client_name, force, channel}` → `BIND_OK{udp, channel}`（见下文「控制端独占」「数据通道」）
5. 同一局域网时 Client 先发 `REHP1|token` 探测 `lan[]`，成功则数据直接发往 Agent（UDP·LAN）；否则两端 UDP `ASSOC` 到中继（Agent 用 device_secret，Client 用 session_ticket）
6. Client 发起 Noise_XXpsk3，握手消息走 **REUDP DATA 可靠通道**；UDP 失败时改走 WSS 的 NOISE / TUNNEL
7. 默认 `OPEN_DESKTOP`；AI 会话用 `OPEN_SESSION`（PTY）

## 外层帧（wss）

| 类型 | 值 | 说明 |
|------|----|------|
| REGISTER / REGISTER_OK | `0x01` / `0x02` | Agent 上线 |
| PAIR_OFFER | `0x03` | Agent 发布配对令牌 |
| PAIR_REDEEM / PAIR_ACK | `0x04` / `0x05` | Client 兑换令牌，拿到 session_ticket |
| BIND / BIND_OK | `0x06` / `0x07` | Client 用 ticket 绑定设备 |
| NOISE / TUNNEL | `0x10` / `0x11` | 不透明，中继原样转发 |
| PING / PONG | `0x1E` / `0x1F` | 保活 |
| ERROR | `0x20` | `{code, message, peer?, since?}` |

### 中继错误码

| code | 场景 | Client 处理 |
|------|------|-------------|
| `expired` | 配对令牌过期 | 提示重新扫码 |
| `pair_failed` | 令牌未知、已被新令牌替换或 device 不匹配 | 让 Agent 刷新二维码 |
| `auth_failed` | ticket 无效（中继重启会清空内存里的 ticket） | 连续失败视为中继重启，需重新扫码 |
| `offline` | Agent 不在线 | 提示唤醒电脑 |
| `relay_full` | 中继达到 `max_sessions` | 换节点或稍后再试 |
| `controller_busy` | 另一台设备正在控制（`peer` = 对方名称，`since` = 开始时间） | 询问用户是否接管，确认后 `force: true` 重新 BIND |
| `superseded` | 本机被另一台设备强制接管 | 断开并停止自动重连 |
| `peer_gone` | 对端断开（发给另一端，`route` 标明是哪个通道） | Agent 释放该通道的会话；Client 走重连 |
| `controller_changed` | 仅发给 Agent：控制端被强制替换 | Agent 释放旧控制端的桌面会话和数据通道 |
| `channel_unsupported` | Agent 没声明支持数据通道（旧 Agent），或通道名未知 | 退回旧方式（见「数据通道」） |

## 控制端独占（踢人）

同一台电脑同时只允许一台手机控制，按手机算，不按通道算。BIND 携带：

- `client_id`：每次安装固定的随机 ID（KoKo 存在 Keychain）
- `client_name`：设备名，用于对方的提示文案
- `force`：用户确认接管时为 `true`
- `channel`：空 = 桌面通道，`data` = 数据通道

中继规则（`cmd/relay/re2.go` `bindRE2Client`）：

1. 没有在线控制端，或 `client_id` 相同（本机重连），或任一方没带 `client_id`（旧客户端）：只替换**同一通道**的旧连接；同一台手机的桌面通道和数据通道可以同时在线。
2. 另一台手机占着任一通道且未带 `force`：回 `controller_busy`，不影响对方。
3. 带 `force`：给旧控制端的**所有通道**发 `superseded` 再关闭，给 Agent 发 `controller_changed`，然后回 `BIND_OK`。不管新手机连的是画面还是会话，旧手机的画面和会话一起断。

Client 自动重连时遇到 `controller_busy` 必须停止，不得自动 `force`；遇到忙碌也不能改试备用中继绕过。

## 数据通道

AI 会话列表、AI 终端（PTY）、IDE 镜像走单独的数据通道，与远程画面互不影响：画面开着、断开、走 UDP 还是 WSS，都不影响数据通道，反之亦然。

- Agent `REGISTER` 带 `channels: true` 声明支持；Client `BIND{channel: "data"}`，中继回 `BIND_OK{channel: "data"}` 表示已分通道。
- 数据通道的 NOISE / TUNNEL 帧 `route` 为 `<device_id>#data`，桌面通道仍是 `<device_id>`。中继按 BIND 的通道决定转发路由，Client 不能改写。
- 数据通道只走 WSS，有自己的 Noise 会话（同一组配对 PSK），不参与桌面的空闲超时和 UDP 切换。Agent 只在该通道上处理 PING、OPEN_SESSION / PTY / RESIZE / SESSION_CLOSE、AGENT_CHAT_*；在数据通道打开的 PTY 输出回数据通道，桌面断开不会关闭它们（AI 进程本身在 screen 里）。
- 中继每台手机只留一个数据通道连接：同一台手机再 BIND 数据通道会替换上一个，所以 KoKo 每台电脑只维护一条共用的数据隧道（`DesktopSessionHub.dataTunnel`）。
- 兼容：中继回 `channel_unsupported`（旧 Agent），或 `BIND_OK` 里没有 `channel`（旧中继）时，KoKo 按旧方式走：画面在线时复用桌面隧道，否则用不带 `channel` 的 BIND。

## 存活检测

- Client 在所有在线阶段都发低频心跳（桌面打开时随输入/统计帧，空闲时约 10s 一次 PING）。
- Agent `livenessLoop` 每 2s 检查一次：
  - 桌面已打开且 **15s** 没有收到 Client 的任何认证流量 → 关闭远程桌面（停止采集）。
  - **`RE_SESSION_IDLE`（默认 2m）** 没有流量 → 释放 Noise 会话；PTY / AI 会话继续运行，手机回来后重新握手即可。

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

可靠性：控制 / 键鼠按钮 / Noise = 可靠；鼠标移动 = Latest；视频分片 = 不可靠（`video_plane=v1`：REUDP DATA 前缀 `V1` + 显式 nonce ChaCha20-Poly1305，与有序 Noise 隧道分离）。旧客户端不带 `video_plane` 时 Agent 回退到 Noise 可靠视频。拥塞：AIMD + pacing。

每次 Noise 握手成功后两端可靠序号从 0 重新开始；Agent 只重置发送侧，接收侧在握手前已重置（可能已收到 Client 的 OPEN）。

局域网直连：`REHP1|token` / `REHP1|pong` 探测（`ProbePreferDirect`），成功后 DATA/ACK 直接发往对端地址；监听用 `udp4`。

## 隧道内消息（加密后）

| 类型 | 值 | 说明 |
|------|----|------|
| OPEN_SESSION / SESSION_READY / SESSION_CLOSE | `0x01`–`0x03` | PTY 会话（可指定 `cmd`、`cwd`、tmux） |
| RESIZE / PTY_DATA | `0x04` / `0x05` | |
| PING / PONG | `0x06` / `0x07` | 应用层心跳 |
| APP_ERROR | `0x0F` | `{code, message}` |
| OPEN_DESKTOP | `0x20` | max_width/height, fps, codec=h264, `password`；`display_id` 在 Darwin 为 CGDirectDisplayID |
| DESKTOP_READY | `0x21` | width/height/codec；`displays[]` 可含 `virtual` / `fb_width` / `fb_height` |
| DESKTOP_CLOSE | `0x22` | `reason` |
| VIDEO | `0x23` | session + frame_id + flags + part/parts + Annex-B |
| INPUT_MOUSE / INPUT_KEY / INPUT_TOUCH | `0x24`–`0x26` | 归一化坐标；KEY 含 IME `text` |
| AUDIO / CLIPBOARD | `0x30` / `0x31` | Opus（`RE_AUDIO_PCM=1` 退回 pcm16）；剪贴板含 `image/png` |
| CURSOR / DISPLAYS | `0x32` / `0x33` | 光标层；屏幕列表 / 选屏 |
| STATS / KEYFRAME_REQ / VIDEO_NACK | `0x34`–`0x36` | ABR 反馈、请求 IDR、补发最新 IDR 的缺片 |
| FILE_OFFER / CHUNK / ACK / PULL / LIST | `0x40`–`0x44` | 文件传输与目录浏览（限 `RE_XFER_ROOT`） |
| INPUT_MODE | `0x45` | 相对鼠标 / 锁定键 |
| HOLE_PUNCH / PAIR_CONFIRM / AUDIT | `0x50`–`0x52` | 打洞信令、本机确认（`RE_PAIR_CONFIRM`）、审计 |
| WOL | `0x60` | 网络唤醒 |
| CAMERA_* | `0x70`–`0x73` | 主机摄像头 → 手机 |
| PHONECAM_* | `0x74`–`0x77` | 手机 → Agent 虚拟摄像头 |
| USB_* | `0x80`–`0x83` | USB 转发 |
| PRINTER_* | `0x90`–`0x92` | 远程打印 |
| AGENT_CHAT_LIST / DETAIL | `0xA0` / `0xA1` | AI 会话列表（Cursor / Claude / Codex / Gemini） |

VIDEO 线协议：**H.264 Annex-B**（禁止把 MJPEG 写进长期协议）。

## IDE 对话镜像（PTY 内带内协议）

`runeverything ide-mirror cursor <composerId>`（或 `--new <folder> cursor`）在 PTY 里镜像 Cursor IDE 的对话，手机通过 OSC 序列交换结构化数据（`internal/idemirror`）：

| OSC | 方向 | 内容 |
|-----|------|------|
| `ESC ] 7788 ; <base64 JSON> BEL` | Agent → 手机 | 状态：模式、模型、是否忙、`chat > 0` 表示支持原生对话 |
| `ESC ] 7789 ; <base64 JSON> BEL` | 手机 → Agent | 操作：`send`、`answer`、`resync`、`mode`、`model`、`cancel`、`keepAll`、`undoAll`、`refresh` |
| `ESC ] 7790 ; <base64 JSON> BEL` | Agent → 手机 | 对话帧 `{reset, messages[]}`，消息按 `idx` 增量更新；角色为 user / assistant / tool / question |

手机作答（`answer`）会作为一条普通消息发给 Cursor，格式为每题 `题目\n→ 选项; 选项`。镜像默认回放最近 150 条。

## Noise

`Noise_XXpsk3_25519_ChaChaPoly_SHA256`，prologue `runeverything-re2-v2`，PSK=`SHA256("re2-psk-v1|"+token)`。
PSK 只参与 msg3，因此 Agent 用同一个临时密钥同时尝试多个候选 PSK（当前令牌、已配对过的令牌、近期令牌），二维码刷新后老手机仍能重连。

## 中继

- wss：原样转发 NOISE / TUNNEL（按通道路由）；按「控制端独占」规则处理 BIND
- UDP：ASSOC 校验后按 route 转发 DATA/ACK；ASSOC 前限速防放大
- 配对令牌在过期前可重复兑换，每次兑换都发新的 session_ticket；Agent 发布新令牌会作废该设备的旧令牌
- session_ticket 只存在中继内存里：**中继重启后已配对手机需要重新扫码**
- 容量：`RE_BW_MBPS_UP` / `RE_PER_USER_KBPS` 推算 `max_sessions`，或 `RE_MAX_SESSIONS` 直接指定

## 能力清单（公益 · 无账号）

- 多显示器 / 选屏、光标分离、剪贴板、文件传输、STATS→ABR、关键帧请求
- 中继 `/v1/capacity` + `scripts/probe-bandwidth.sh` 实测限流；家庭端 **延迟+繁忙** 选路
- 心跳与空闲释放、审计日志、局域网直连 / 打洞、控制端独占（确认后接管）；KoKo 见 handoff（无登录）

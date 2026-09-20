# KoKo ↔ RunEverything 产品能力交接（公益远控 · 无账号）

> **产品定位**：志愿中继 + 家庭 Agent；**不对齐商业账号/会员/付费墙**。  
> 功能对齐远控体验（画面/键鼠/文件等），分发与鉴权靠**扫码配对**，不靠用户注册。

桌面实现仓库：`RunEverything`。本文供 KoKo Cursor 对齐。

---

## 不做

- 账号体系、会员、云端设备列表登录
- 付费节点 / 强制中心化身份

## 要做（客户端）

1. QR **`v:3`**：`relay` + **`udp`** + `noise_pub` + token…
2. wss 信令 PAIR/BIND → REUDP ASSOC → Noise → **OPEN_DESKTOP**
3. **H.264 硬解**播放 VIDEO 分片；丢包发 `STATS` / `KEYFRAME_REQ`
4. **光标层**单独画 `CURSOR`（勿依赖画面里的指针）
5. **多显示器**：处理 `DESKTOP_READY.displays` / `DISPLAYS` select
6. **剪贴板** `CLIPBOARD` 双向
7. **触摸手势 → INPUT_TOUCH/MOUSE**；虚拟键盘 → INPUT_KEY（含 `text` IME）
8. **文件** FILE_OFFER / FILE_CHUNK / FILE_ACK
9. **音频** AUDIO（opus/pcm16）可选
10. **打洞** HOLE_PUNCH（offer/candidate）；失败保持中继
11. 后台保活；断线自动 BIND+Noise 重连（同 pairing_token 生命周期内）
12. 配对过期 / `relay_full` / Agent 离线错误提示（引导换节点或重扫）

## 中继与选路（公益）

- 家庭 Agent **按延迟 + busy（繁忙度）**选节点（`/healthz` + `/v1/capacity`）
- 志愿节点跑 `scripts/probe-bandwidth.sh` 实测上行，设置 `RE_MAX_SESSIONS`
- 满员返回 `relay_full`，KoKo 应换节点或提示稍后

## 安全（无账号）

- 扫码即授权；可选 Agent 侧 `RE_PAIR_CONFIRM=1` 本机确认
- 可选会话口令：`RE_ACCESS_PASSWORD` + OPEN_DESKTOP.`password`
- 会话空闲 `RE_SESSION_IDLE`（默认 30m）断开
- Agent 写 `~/.runeverything/audit.log`
- 第二客户端默认**顶掉**前一个（互踢）
- 文件落盘 `~/.runeverything/xfer/`（按原始文件名，已净化路径）
- Agent 多屏：真枚举 + 选屏采集；硬编优先（Win MF / macOS VT / Linux VAAPI）；ABR 跟 STATS
- 采集优先 ffmpeg（Win gdigrab，`RE_CAPTURE=dda` 可试 ddagrab；macOS avfoundation；Linux x11grab）
- 音频默认 Opus（`RE_AUDIO_PCM=1` 可退回 pcm16）；常驻播放
- `FILE_PULL`：客户端拉取 Agent 侧文件（限 `RE_XFER_ROOT` / xfer 目录）
- `privacy_blank`：本机黑屏（Win 排除采集；macOS sharingType none）
- 剪贴板支持 `image/png`（DataB64）
- **采集一步到位**：Win 原生 DXGI Desktop Duplication → ffmpeg gdigrab → GDI；macOS 常驻 SCStream → ffmpeg → 单帧 SCK
- **WOL** `MsgWakeOnLAN`；**摄像头** `CAMERA_*`（ffmpeg MJPEG/H264）；**USB** 列表+usbip 绑定；**打印机** 列表+作业投递
- **FILE_LIST** 目录浏览；文件拉取支持 `resume_from` + progress
- **INPUT_MODE** 相对鼠标/游戏模式；横向滚轮；本机原生确认弹窗
- 打洞回传本机 `candidates` 列表

### 新增内层类型（加密后）

| 类型 | 用途 |
|------|------|
| CURSOR 0x32 | 光标位置 |
| DISPLAYS 0x33 | 列表/选屏 |
| STATS 0x34 | RTT/丢包 → ABR |
| KEYFRAME_REQ 0x35 | 要关键帧 |
| FILE_* 0x40–42 | 文件上传 |
| FILE_PULL 0x43 | 客户端拉取 Agent 文件 |
| FILE_LIST 0x44 | 目录列表 |
| INPUT_MODE 0x45 | 相对鼠标/锁定键 |
| HOLE_PUNCH 0x50 | P2P |
| WOL 0x60 | 网络唤醒 |
| CAMERA_* 0x70–73 | 摄像头通道 |
| USB_* 0x80–83 | USB 转发 |
| PRINTER_* 0x90–92 | 远程打印 |
| CLIPBOARD 0x31 | 剪贴板 |
| AUDIO 0x30 | 音频 |

## 给写 KoKo 的一句话

> 实现公益远控客户端：无登录；扫码 v3；wss+REUDP+Noise；硬解 H.264；光标层、多屏、剪贴板、文件、STATS/ABR、打洞回退中继；满员与离线有清晰 UX。

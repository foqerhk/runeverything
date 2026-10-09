# 远控桌面测试用例

> 覆盖 KoKo（iOS 客户端）↔ RunEverything Agent（macOS/Windows/Linux 主机）远程桌面全链路。  
> 自动化入口：`-RE2E2E` / `-RE2E2EQuality` / `-RE2E2EMenu` / `-RE2E2EForceWSS` / `-RE2E2EForceUDP` 等（见 `RE2E2EAutoConnect.swift`、`ios/scripts/re2-device-e2e-*.sh`）。  
> 用例编号：`RD-<域>-<序号>`。优先级：P0 阻断发版 / P1 核心体验 / P2 增强 / P3 边缘。

---

## 0. 环境与前置

| 项 | 要求 |
|----|------|
| Agent | 个人号签名 Team `U5SLTWD6AH`；`RE_NOTARIZE=0 ./scripts/install-macos-app.sh` 安装后启动 `tray`（勿直接 `go build -o /Applications/...`） |
| TCC | macOS：屏幕录制 + 辅助功能；麦克风/摄像头按功能需要 |
| KoKo | Xcode 27.1 beta；真机（推荐 iPhone Air / 14 Pro Max） |
| 网络 | 同 LAN（PreferDirect）+ 可出网中继；可选仅中继 / 仅 WSS |
| 配对 | Agent 产出 `~/.runeverything/last_pairing.json`（QR v3） |
| 虚拟屏 | 高分辨率：`RE_VDISPLAY=8k` / `16k` 启动 tray |

**通用通过标准（画面类）**

- `phase == streaming`，`frameImage != nil`
- 持续解码：`frameEpoch` 在观测窗内递增（LAN 正常 ≥ 约 1fps；弱网允许更低但非冻死）
- `DESKTOP_READY` 的 `width×height` 与近期解码 `pic` 在 slack 内一致（约 max(48, dim/10)）
- 路径标签与强制路径一致：`UDP·LAN` / `UDP·Relay` / `WSS·Relay`

---

## 1. 配对与会话建立

### RD-PAIR-01 扫码配对成功（P0）
- **前置**：Agent tray 已注册中继，屏幕有 QR / pair JSON 可用  
- **步骤**：KoKo 扫码或注入 `re2-pair.json` → PAIR/BIND → Noise  
- **期望**：设备出现在主机列表；无账号登录；`pairing_token` 生命周期内可重连  

### RD-PAIR-02 配对过期 / 无效 token（P1）
- **步骤**：使用过期或篡改 token 连接  
- **期望**：明确错误提示；引导重扫；不卡死 UI  

### RD-PAIR-03 Agent 离线（P1）
- **步骤**：停掉 tray 后客户端连接  
- **期望**：离线提示；可重试；不误报「桌面打开超时」掩盖离线  

### RD-PAIR-04 本机确认 `RE_PAIR_CONFIRM=1`（P2）
- **步骤**：开启后客户端 OPEN；主机弹确认，点允许 / 拒绝 / 超时  
- **期望**：允许→桌面打开；拒绝/超时→客户端可见失败文案  

### RD-PAIR-05 会话口令 `RE_ACCESS_PASSWORD`（P2）
- **步骤**：设口令后分别用正确/错误/空口令 OPEN  
- **期望**：正确进入；错误拒绝且可重试  

### RD-PAIR-06 第二控制端确认接管（P1）
- **步骤**：设备 A 已远控；设备 B 连同一台电脑（E2E：B 用 `-RE2E2EStored`，再加 `-RE2E2EForce`）  
- **期望**：B 未确认时收到 `controller_busy`，提示「A 正在控制这台电脑」，A 不受影响；B 确认后接管，A 收到 `superseded`、提示「已在另一台设备上连接」且不自动重连；Agent 日志有 `controller changed`；无双端同时写键鼠  
- **补充**：A 自己重连（同一 `client_id`）不应触发提示  

### RD-PAIR-07 心跳与空闲释放（P2）
- **步骤**：建立桌面后让手机停止发送（杀掉 App 或断网）；另测 `RE_SESSION_IDLE` 调短后的空闲  
- **期望**：约 15s 后 Agent 关闭远程桌面（日志 `no heartbeat from phone`）；`RE_SESSION_IDLE`（默认 2m）后释放 Noise 会话，PTY / AI 会话仍在；手机回来可正常重新握手；audit 有记录  

---

## 2. 传输路径

### RD-PATH-01 UDP·LAN PreferDirect（P0）
- **前置**：手机与 Mac 同网段；pair 含 `lan`  
- **步骤**：默认连接（不 force WSS）  
- **期望**：`path` 含 `UDP·LAN`；`video_plane=1` 时视频走 VideoMedia；画面可播  

### RD-PATH-02 WSS·Relay 强制（P0）
- **步骤**：`-RE2E2EForceWSS` / `forceWSS`  
- **期望**：全程 `WSS·Relay`；有画面；encode 软顶 ≤1280×720  

### RD-PATH-03 UDP·Relay（跨网 / stripLAN）（P0）
- **步骤**：异网或 `-RE2E2ENoLAN` + forceUDP  
- **期望**：`UDP·Relay`（非误标 LAN）；可播；弱网可降清晰度  

### RD-PATH-04 LAN 失败回退中继（P1）
- **步骤**：同网但阻断 UDP 直连（防火墙/隔离）  
- **期望**：回退 Relay/WSS；HUD 路径更新；不断开死循环  

### RD-PATH-05 打洞 HOLE_PUNCH（P2）
- **步骤**：跨 NAT 场景观察 offer/candidate  
- **期望**：成功则直连；失败保持中继；过滤虚拟网卡候选  

### RD-PATH-06 中继满员 `relay_full`（P2）
- **步骤**：节点满员时注册/连接  
- **期望**：客户端提示换节点或稍后；不无限重试打爆 UI  

### RD-PATH-07 路径切换不丢会话语义（P1）
- **步骤**：LAN ↔ Relay 抖动（可拔 LAN）  
- **期望**：可恢复 streaming；质量/privacy 状态合理；不长期 `handshaking` 假死  

---

## 3. 首帧与画面管线

### RD-VID-01 首开桌面出画（P0）
- **步骤**：OPEN_DESKTOP → DESKTOP_READY → 首个 IDR  
- **期望**：≤ 约 8–15s（含 TCC/确认）出画；超时文案「桌面未能及时打开」仅在真超时出现  

### RD-VID-02 READY 尺寸与解码尺寸一致（P0）
- **步骤**：记录 READY `w×h` 与首帧 `pic`  
- **期望**：slack 内一致；禁止 READY 标清而码流长期停在其它档（历史回归：READY 950 / encode 672）  

### RD-VID-03 关键帧请求（P0）
- **步骤**：菜单/API `requestKeyframe`；或模拟丢包触发 `KEYFRAME_REQ`  
- **期望**：Agent 打 IDR；画面恢复；不 IDR 风暴导致组装永久失败  

### RD-VID-04 冻屏检测与恢复（P0）
- **步骤**：弱网或人为 stall；观察 STATS `stall` / want_keyframe  
- **期望**：保留最后一帧（不黑屏闪烁）；可恢复；导航栏可出现弱网提示  

### RD-VID-05 H.264 / HEVC 编解码（P1）
- **步骤**：小分辨率 H.264；≥4K/5K 路径 HEVC（若主机支持）  
- **期望**：无 -12909；HEVC 回退 H.264 时仍可播；codec 与 READY 一致  

### RD-VID-06 多 part IDR 组装（P0）
- **步骤**：大 IDR（数十～上百 part）在 LAN / WSS  
- **期望**：组装完成并解码；WSS 下 keyframe 间隔足够，不被 1Hz 重询撕碎  

### RD-VID-07 video_plane vs Noise 视频（P0）
- **步骤**：LAN PreferDirect 开 video_plane；切质量时保持 plane=1  
- **期望**：不因质量切换剥掉 video_plane 导致 MAC fail / peer_gone  

### RD-VID-08 Soft decode 兜底（P3）
- **步骤**：`-RE2SoftDecode` / 强制软解  
- **期望**：可出画（可更慢）；生产默认仍 HW-first  

---

## 4. 画质档位与 ABR

档位（相对主机 native）：**超清 1.0 / 高清 0.75 / 标清 0.55 / 流畅 0.35**（`DesktopVideoQuality`）。

### RD-Q-01 菜单切 流畅（P0）
- **步骤**：streaming 下选「流畅」  
- **期望**：新 OPEN/soft reopen；READY≈0.35×native（受 path 上限）；`pic` 跟上；导航含「流畅」；切后持续解码  

### RD-Q-02 菜单切 标清 / 高清 / 超清（P0）
- **步骤**：依次切换  
- **期望**：每档 `painted` + `continued`；UI `videoQuality` 与 nav title 一致；不发明中间分辨率  

### RD-Q-03 同档重复点击不折腾（P1）
- **步骤**：已在标清且正在出画时再点标清  
- **期望**：跳过无意义 CLOSE/OPEN；可拉 keyframe；不撕 VT  

### RD-Q-04 快速连点多档（P0）
- **步骤**：流畅→高清→标清 连续点（菜单 rapid quality）  
- **期望**：debounce 后最终档正确；不冻屏；WSS 不卡死组装  

### RD-Q-05 Soft reopen 不 peer_gone（P0）
- **步骤**：PreferDirect LAN 上反复改画质  
- **期望**：同 session 热更；不因 mid-stream `closeDesktop` 导致直连 peer_gone  

### RD-Q-06 Soft reopen 后画面尺寸跟随（P0）
- **步骤**：1056 档切到 672 档  
- **期望**：READY 672 后 `pic` 变为 ≈672；禁止 desk=672 而 pic 长期 1056（当前联调焦点）  

### RD-Q-07 ABR 弱网降档（P1）
- **步骤**：限速/丢包；观察 STATS → Agent ABR  
- **期望**：向流畅/降码率；OPEN 后 hold（约 15s）内不立刻砍掉用户刚选的档  

### RD-Q-08 ABR 不突破 OPEN 上限（P1）
- **步骤**：选流畅后网络变好  
- **期望**：不涨到超过当前 OPEN max；用户再选手动更高档才上去  

### RD-Q-09 WSS 软顶 1280×720（P0）
- **步骤**：forceWSS 选超清  
- **期望**：实际 encode ≤1280×720；客户端 expect 按此 clamp；仍可播  

### RD-Q-10 几何就绪后一次 native reopen（P1）
- **步骤**：首开 provisional 尺寸，displays 到达后  
- **期望**：仅一次按 native×档位 reopen；无多级 climb 抖闪  

### RD-Q-11 自动化质量矩阵（P0）
- **步骤**：`-RE2E2EQuality`：seed 标清 → 流畅 → 高清 → 超清 → 标清  
- **期望**：`qualityOK=true`；每档 painted/continued；高清/超清在 LAN 上达到商业清晰度门槛（≥720p 类）  

---

## 5. 显示器与高分辨率

### RD-DISP-01 多屏列表（P0）
- **步骤**：双屏主机连接；刷新 displays  
- **期望**：列表非空；主屏/副屏尺寸正确；含 virtual 标记（若有）  

### RD-DISP-02 切换显示器（P0）
- **步骤**：选副屏 OPEN  
- **期望**：READY `display_id` 确认；画面为该屏内容；光标坐标在该屏逻辑内  

### RD-DISP-03 主屏 display_id=0 omitempty（P1）
- **步骤**：选主屏（id 0）  
- **期望**：客户端正确处理缺省字段；confirmedDisplayID=0  

### RD-DISP-04 真机 ≥5K 副屏（P1）
- **步骤**：`-RE2E2E5K` 且存在 ≥5K 屏  
- **期望**：能选中并以高分辨率协商；有出画（允许低 fps）  

### RD-DISP-05 虚拟屏 8K（P1）
- **前置**：`RE_VDISPLAY=8k`  
- **步骤**：`-RE2E2E8K`  
- **期望**：列表出现 virtual FB；可 OPEN；HEVC/降 fps 策略生效  

### RD-DISP-06 虚拟屏 16K（P2）
- **前置**：`RE_VDISPLAY=16k`；主机有 ffmpeg/libx265  
- **步骤**：`-RE2E2E16K`  
- **期望**：不因 VT 8192 硬顶直接失败；有帧或明确错误；不拖死 Agent  

---

## 6. 输入：键鼠触控

### RD-IN-01 绝对触控点击（P0）
- **步骤**：点桌面控件  
- **期望**：主机对应位置点击；光标层更新  

### RD-IN-02 拖拽 / 移动（P0）
- **步骤**：拖移窗口或选区  
- **期望**：连续 move；无严重丢点；重连后 move 不长期静音  

### RD-IN-03 滚轮纵向 / 横向（P1）
- **步骤**：双指滚、横向滚  
- **期望**：主机滚动方向正确  

### RD-IN-04 相对鼠标 / 游戏模式（P1）
- **步骤**：切换 INPUT_MODE game on/off  
- **期望**：模式标志正确；游戏场景可相对移动；切回 trackpad 正常  

### RD-IN-05 虚拟键盘与 IME text（P1）
- **步骤**：输入英文/中文到主机文本框  
- **期望**：`INPUT_KEY` / text 到达；中文 IME 不乱码（按平台能力）  

### RD-IN-06 快捷键组合（P1）
- **步骤**：Cmd+C/V、Alt+Tab 等（macOS/Win 分别测）  
- **期望**：修饰键状态正确；不粘键  

### RD-IN-07 Space 三指手势（P0）
- **步骤**：三指左右滑（`gesture=space`）  
- **期望**：主机 Space/桌面切换有响应；**视频不中断**（历史回归）  

### RD-IN-08 手势中重连（P2）
- **步骤**：拖拽过程中短断网恢复  
- **期望**：`remotePointerSuspended` 解除；可继续操作  

---

## 7. 光标

### RD-CUR-01 独立光标层（P0）
- **步骤**：移动主机鼠标  
- **期望**：手机光标 overlay 跟随；不依赖画面里的主机指针像素  

### RD-CUR-02 隐藏主机光标（P1）
- **步骤**：OPEN `hide_cursor=true`  
- **期望**：采集画面无主机光标；overlay 仍可用  

### RD-CUR-03 光标可见性心跳（P2）
- **步骤**：静止数秒  
- **期望**：不误判光标死亡；心跳间隔合理不抢带宽  

---

## 8. 更多菜单联合能力

对应 `-RE2E2EMenu` joint smoke。

### RD-MENU-01 请求关键帧（P0）
- **期望**：`keyframe` 后仍持续出画  

### RD-MENU-02 剪贴板推送文本（P0）
- **步骤**：手机剪贴板写入 → `pushLocalClipboard`  
- **期望**：主机剪贴板变为该文本  

### RD-MENU-03 剪贴板主机→手机文本（P1）
- **步骤**：主机复制文本  
- **期望**：手机收到 CLIPBOARD  

### RD-MENU-04 剪贴板图片 PNG（P2）
- **步骤**：任一侧复制小图  
- **期望**：对端可粘贴/预览；超大图有限制或失败提示  

### RD-MENU-05 显示器刷新（P0）
- **期望**：`displays == true` 且 count≥1  

### RD-MENU-06 远程文件列表（P0）
- **步骤**：`FILE_LIST` 根/xfer 目录  
- **期望**：无 error；空目录也可接受；路径净化无 `..` 逃逸  

### RD-MENU-07 文件上传手机→主机（P1）
- **步骤**：选文件 FILE_OFFER/CHUNK  
- **期望**：落盘 `~/.runeverything/xfer/`；进度可达 100%  

### RD-MENU-08 文件拉取主机→手机（P1）
- **步骤**：FILE_PULL；含 `resume_from`  
- **期望**：完整文件；断点续传正确  

### RD-MENU-09 音频静音开关（P1）
- **步骤**：mute / unmute  
- **期望**：本地标志正确；主机有声时 unmute 可听见（需 mic 权限）  

### RD-MENU-10 隐私黑屏 on/off（P0）
- **步骤**：privacy blank 开→关  
- **期望**：开：主机屏幕隐私/黑；手机仍有码流或占位且能再出画；关：恢复；`privacyOn/Off` 均 painted  

### RD-MENU-11 菜单联合收尾（P0）
- **期望**：`menuOK`；`finalPaint`；相位 streaming  

---

## 9. 音频

### RD-AUD-01 主机音频下发（P1）
- **步骤**：主机播放音乐；手机 unmute  
- **期望**：可听到；延迟可接受；Opus 默认  

### RD-AUD-02 无麦克风权限（P2）
- **步骤**：拒绝主机麦克风 TCC  
- **期望**：READY `mic_authorized=false`；自动 mute；UI 控件禁用合理  

### RD-AUD-03 PCM 回退 `RE_AUDIO_PCM=1`（P3）
- **期望**：仍可播或明确失败  

---

## 10. 摄像头

### RD-CAM-01 主机摄像头预览（P1）
- **步骤**：开启 camera → 手机预览  
- **期望**：有预览帧；关停止  

### RD-CAM-02 无摄像头权限（P2）
- **期望**：`camera_authorized=false`；不能误开；错误码友好  

### RD-CAM-03 手机当摄像头 PhoneCam（P1）
- **前置**：主机虚拟摄像头（AkVirtualCamera / KoKo Phone Camera）已装且同签名 Team  
- **步骤**：手机开启 PhoneCam  
- **期望**：主机视频会议可选到该摄像头；有画面；不堵住桌面 UDP 读循环  

### RD-CAM-04 PhoneCam 与桌面并存（P1）
- **期望**：桌面帧率不明显饿死；busy 时丢 PhoneCam 帧而非卡死 Agent  

---

## 11. 隐私、安全与权限

### RD-SEC-01 屏幕录制权限缺失（P0）
- **步骤**：撤销 Screen Recording 后 OPEN  
- **期望**：Agent 拒绝并提示；客户端可见；重装签名后需重新授权  

### RD-SEC-02 辅助功能缺失（P1）
- **期望**：可看画面但键鼠注入失败/提示；不静默无操作  

### RD-SEC-03 签名 Team 与 TCC 一致（P0）
- **步骤**：错误用公司证 `BGC93C2SX5` 安装  
- **期望**：录屏/辅助功能对不上（负向用例）；文档要求个人号  

### RD-SEC-04 Audit 日志（P2）
- **步骤**：open/deny/文件传输  
- **期望**：`~/.runeverything/audit.log` 有对应事件  

### RD-SEC-05 文件路径穿越（P1）
- **步骤**：FILE_LIST/PULL 尝试 `../`  
- **期望**：拒绝；不出沙箱  

---

## 12. 断线、重连与生命周期

### RD-LIFE-01 中继 peer_gone 后恢复（P0）
- **步骤**：中继断开或 Agent 短暂重启  
- **期望**：客户端重 BIND+Noise；或明确失败可手动重连；不永久 handshaking  

### RD-LIFE-02 App 进后台再回前台（P1）
- **步骤**：远控中切后台 30s+ 回前台  
- **期望**：保活或快速恢复出画；自动 keyframe  

### RD-LIFE-03 Agent tray 崩溃拉起（P1）
- **步骤**：kill Agent 再开 tray  
- **期望**：需重新配对或同 token 重连策略符合产品；TCC 仍有效（同签名）  

### RD-LIFE-04 质量/隐私切换中 keepalive（P0）
- **步骤**：切换过程抓 ping  
- **期望**：openingDesktop 期间仍 ping；不因 idle 被踢  

### RD-LIFE-05 WSS Noise 恢复（P1）
- **步骤**：WSS 解密连续失败触发 recover  
- **期望**：新 session OPEN；恢复出画；不与质量 OPEN 死锁  

---

## 13. UI / UX

### RD-UI-01 导航状态行（P1）
- **期望**：`路径 · 画质 · kb/s`；弱网有提示  

### RD-UI-02 连接中/等待视频文案（P1）
- **期望**：opening / waiting for video / connected 状态可区分  

### RD-UI-03 更多菜单可达（P1）
- **期望**：画质、显示器、文件、隐私、音频、摄像头入口可用  

### RD-UI-04 错误可恢复（P1）
- **期望**：失败有按钮重试/重扫；不白屏  

---

## 14. 平台矩阵

| 主机 | 最低集 | 备注 |
|------|--------|------|
| macOS（个人签名） | P0 全路径 + 画质 + 菜单 | 主开发机 |
| Windows | 首开、键鼠、 DXGI 采集、隐私 | 可用 RDP smoke 技能远程测 |
| Linux | 首开、x11grab、键鼠 | P2 |

| 客户端 | 最低集 |
|--------|--------|
| iPhone Air / 14 Pro Max 真机 | P0 |
| 模拟器 | 仅协议/非硬解相关；不替代真机画质 |

---

## 15. 自动化映射

| 用例簇 | 启动参数 / 请求 JSON | 结果字段 |
|--------|----------------------|----------|
| 冒烟出画 | `-RE2E2E` | `ok`, `painted`, `path*` |
| LAN | 默认或 `forceUDP` | `path` ⊃ UDP·LAN |
| WSS | `-RE2E2EForceWSS` | `path` ⊃ WSS |
| 画质矩阵 | `-RE2E2EQuality` | `qualityOK`, `qualityMatrix[]` |
| 更多菜单 | `-RE2E2EMenu` | `menuOK`, `menu.*` |
| 5K/8K/16K | `-RE2E2E5K` / `8K` / `16K` | hi-res display 选中与出画 |
| 联合 | Quality+Menu+路径强制 | 上述字段均要绿 |

**建议每次发版门禁（真机 Air）**

1. LAN：`-RE2E2E -RE2E2EQuality -RE2E2EMenu -RE2E2EForceUDP`  
2. WSS：`-RE2E2E -RE2E2EQuality -RE2E2EMenu -RE2E2EForceWSS`  
3. （可选）`RE_VDISPLAY=8k` + `-RE2E2E8K`

结果拉取：`Documents/re2-e2e-result.json`。

---

## 16. 已知高风险回归清单（必测）

| 风险 | 相关用例 | 现象 |
|------|----------|------|
| Soft reopen 后 pic 卡在旧分辨率 | RD-Q-06 | desk=672 pic=1056 |
| 质量切换 peer_gone（LAN） | RD-Q-05 | PreferDirect 断开 |
| WSS 连切冻屏 | RD-Q-04 | 组装被 IDR 风暴撕掉 |
| ABR 裁掉标清只剩流畅 | RD-VID-02 / RD-Q-08 | READY 与码流不符 |
| video_plane 被质量切换剥掉 | RD-VID-07 | kb/s→0 / MAC fail |
| Space 手势后无画面 | RD-IN-07 | gesture 后 freeze |
| 错误签名导致 TCC 失效 | RD-SEC-03 | 无法采集/注入 |
| 直接覆盖 App 二进制未 codesign | 环境前置 | 同 TCC 失效 |
| Encrypt 前未做包长检查 | 文件/聊天/视频分片 | nonce 烧毁、会话失步 |

---

## 17. 用例执行记录模板

```text
日期:
主机: macOS … / Agent build …
客户端: iPhone … / KoKo build …
路径: LAN | WSS | Relay
用例 ID:
结果: Pass / Fail / Blocked
证据: result.json 路径、关键日志摘录、截图
备注:
```

---

## 18. 维护说明

- 产品能力变更时同步改本文件与 `docs/koko-re2-handoff.md`。  
- 自动化已覆盖的在第 15 节标注；纯手工 P2/P3 可进发版抽测表。  
- 远控桌面「绿」的定义：**第 15 节门禁两条路径 `ok=true` 且 `qualityOK`/`menuOK` 按启用项为 true**。

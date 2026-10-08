# Agent 本机联调（macOS 托盘）

给 App / KoKo 侧改 RunEverything Agent 后，如何签名并正确更新本机托盘版本。

> **联调标准（写死）**：`RE_NOTARIZE=0` → `install-macos-app.sh`。协议调试可显式启动
> `…/RunEverything tray >>/tmp/re-agent.log`；但从 Cursor Agent/Shell 启动的子进程会被 macOS
> TCC 归责给 `Cursor.app`，不能用于辅助功能/远程输入验收。交付或验收手势前必须通过
> LaunchServices 重启（`open -a RunEverything --args tray`）。  
> 不要用日常公证、不要裸 `cp` 进 `.app`。对外分发才 `RE_NOTARIZE=1`。

## 路径与身份

| 项 | 值 |
| --- | --- |
| Agent 源码 | `/Users/liuwei/Downloads/RunEverything` |
| 安装目标 | `/Applications/RunEverything.app`（**不要**用 Homebrew 的 `runeverything`，易双开/跑旧进程） |
| Bundle ID | `com.foqerhk.runeverything` |
| Team ID | `U5SLTWD6AH`（个人号 wei liu / `ruier09@qq.com`） |
| 签名身份 | `Developer ID Application: wei liu (U5SLTWD6AH)` |
| Go | `~/.local/go/bin/go`（1.26+），`CGO_ENABLED=1` |
| Entitlements | `packaging/macos/runeverything.entitlements` |
| Info.plist 模板 | `packaging/macos/Info.plist`（安装时覆盖进 `.app`） |

> **不要用**三鹰公司证书 `Developer ID Application: Chongqing Sanying… (BGC93C2SX5)` 签 RunEverything / 虚拟摄像头。脚本默认已切到个人号；可用 `RE_SIGN_IDENTITY` / `RE_DEVELOPMENT_TEAM` 覆盖。

## 签名与公证（用什么）

| 步骤 | 用什么 | 说明 |
| --- | --- | --- |
| **codesign** | 钥匙串里的个人 **Developer ID Application** | `Developer ID Application: wei liu (U5SLTWD6AH)`。CSR → 网页创建证书（Account Holder）→ 双击 `.cer` 导入；API Key **不能**创建 Developer ID 证书。 |
| **公证 notarytool** | KoKo `.env` 的 **ASC API Key**（个人号） | `ASC_KEY_ID` / `ASC_ISSUER_ID` / `ASC_PRIVATE_KEY_PATH`（如 `AuthKey_DKV6CD8CUQ.p8`）。`scripts/sign-macos.sh` / `install-macos-app.sh` 在 `RE_NOTARIZE=1` 时调用。公证不需要 codesign 私钥，但 Team 须与签名证书一致（均为 `U5SLTWD6AH`）。 |
| **脚本入口** | `scripts/sign-macos.sh`、`scripts/install-macos-app.sh` | 默认身份见上表；**日常联调用 `RE_NOTARIZE=0`**（与多次联调一致）。 |

Camera Extension（手机摄像头 / AkVirtualCameraCX）额外材料：

| 项 | 值 |
| --- | --- |
| Host Bundle ID | `com.foqerhk.runeverything.vcamCX` |
| Extension Bundle ID | `com.foqerhk.runeverything.vcamCX.Extension` |
| 签名身份 | 同上个人 Developer ID |
| Entitlements | `packaging/akvirtualcamera/AkVirtualCameraCX.entitlements`（含 `system-extension.install`）、`…Extension.entitlements` |
| 描述文件 | ASC 个人号下 `MAC_APP_DIRECT`（Developer ID），需含 `SYSTEM_EXTENSION_INSTALL`；打包时用 `RE_VCAMCX_PROFILE_APP` / `RE_VCAMCX_PROFILE_EXT` 嵌入 |
| 打包脚本 | `scripts/package-akvirtualcamera-macos.sh`（Team 默认 `U5SLTWD6AH`） |

已知卡点：带 `system-extension.install` 的 CX 在本机可能被 amfid 拒启（`No matching profile found`）。Developer ID 分发 Camera Extension 通常还需向 Apple 申请 [System Extension 权限](https://developer.apple.com/contact/request/system-extension/)。整包 `RunEverything.app` 在 **`RE_NOTARIZE=1`** 时会自动去掉未签名 / Development 签的 `akvirtualcamera` 嵌套代码（否则 notary Invalid）。

## 改完代码：编译 → 安装签名 → 杀旧进程 → 重启

缺一不可。只 `go build` 到 `/tmp` **不会**更新托盘。

```bash
export PATH="$HOME/.local/go/bin:$PATH"
export GOTOOLCHAIN=auto
export RE_NOTARIZE=0   # 日常联调标准：签名+entitlements，不公证

cd /Users/liuwei/Downloads/RunEverything
CGO_ENABLED=1 go build -ldflags "-X main.version=0.2.3-dev" -o /tmp/runeverything ./cmd/agent

./scripts/install-macos-app.sh /tmp/runeverything
# 拷贝到 /Applications/RunEverything.app + codesign（含 entitlements）

pkill -f '/Applications/RunEverything.app/Contents/MacOS/RunEverything' || true
sleep 1

# 协议调试启动（带 tray + 文件日志）：
/Applications/RunEverything.app/Contents/MacOS/RunEverything tray >>/tmp/re-agent.log 2>&1 &

# 如果上面的命令由 Cursor Agent / Cursor IDE Shell 执行：
# TCC 会把 responsible process 记成 Cursor.app。协议调试结束后，必须这样重启，
# 再验收远程鼠标、键盘和手势：
pkill -f '/Applications/RunEverything.app/Contents/MacOS/RunEverything' || true
sleep 1
open -a RunEverything --args tray
```

`install-macos-app.sh` 内部会调用 `scripts/sign-macos.sh`。日常联调保持 `RE_NOTARIZE=0`；未公证可能多弹一次权限，属正常。

### Cursor 启动导致“已勾辅助功能但手势无效”

macOS TCC 不只看请求进程，还看 `responsible process`。从 Cursor Agent/Shell 直接执行
`/Applications/RunEverything.app/Contents/MacOS/RunEverything tray` 时，可能出现：

- 系统设置中 `RunEverything` 已开启“辅助功能”；
- 签名、Team、Bundle ID、entitlements 全部正确；
- 远端鼠标/手势消息已到 Agent，但 Mac 不执行输入；
- TCC 日志把 `responsible` 记为 `Cursor.app`。

此时反复删除、重新添加 RunEverything 权限无效，因为授权对象与当前责任链不一致。处理：

```bash
pkill -f '/Applications/RunEverything.app/Contents/MacOS/RunEverything' || true
sleep 1
open -a RunEverything --args tray
```

确认新进程由 LaunchServices/launchd 启动：

```bash
PID="$(pgrep -f '/Applications/RunEverything.app/Contents/MacOS/RunEverything tray' | head -1)"
ps -p "$PID" -o pid=,ppid=,command=
# 期望 PPID=1；不是 Cursor 进程的子进程
```

需要铁证时查 TCC：

```bash
log show --last 5m --info --debug \
  --predicate '(subsystem == "com.apple.TCC" OR process == "tccd") AND eventMessage CONTAINS[c] "com.foqerhk.runeverything"' \
  | rg 'kTCCServiceAccessibility|responsible=|ReqResult'
```

期望看到 `responsible=com.foqerhk.runeverything` 和
`ReqResult(Auth Right: Allowed (System Set))`。

注意：不要从 Cursor Shell 运行 `RunEverything __re_perm_check` 来判断最终权限；这个检查进程本身也会
继承 Cursor 的 TCC responsibility，可能返回 `10`（屏幕录制有、辅助功能无），即使 LaunchServices
启动的真实托盘进程已经获准。

**禁止**：

- `go build -o /Applications/RunEverything.app/Contents/MacOS/RunEverything`
- `cp /tmp/runeverything /Applications/…/RunEverything`（跳过 entitlements）
- 在 `Contents/MacOS/` 里留 `*.bak*`

## 验证是否真是新版本

```bash
pgrep -lf 'RunEverything.app/Contents/MacOS/RunEverything tray'
stat -f '%Sm %N' /Applications/RunEverything.app/Contents/MacOS/RunEverything
ps -p "$(pgrep -f 'RunEverything.app/Contents/MacOS/RunEverything tray' | head -1)" -o lstart=,etime=

# 签名身份必须是个人号
codesign -dv --verbose=2 /Applications/RunEverything.app 2>&1 | egrep 'Authority|TeamIdentifier'
```

进程启动时间应 **≥** 二进制 mtime。只 build、不 install、或未杀掉旧 tray = 仍在跑旧码。

装完可贴一句：PID + `stat` mtime，避免「以为更新了其实旧进程」。

## 签名注意

- 必须用本机已有的个人 **Developer ID**（Team `U5SLTWD6AH`），不要 ad-hoc，也不要用三鹰 `BGC93C2SX5`，否则屏幕录制 / 辅助功能 TCC 会对不上。
- 勿删 entitlements 里的 `device.audio-input` / `device.camera`；`Info.plist` 需保留麦克风 / 摄像头用途说明。
- 需要公证对外分发时：`RE_NOTARIZE=1 ./scripts/install-macos-app.sh …`，并配置 KoKo `.env` 里个人号的 `ASC_*`。

## 虚拟显示（`RE_VDISPLAY`）

无头 / 高分虚拟桌面由独立 helper `Contents/MacOS/re-vdisplay` 持有私有 `CGVirtualDisplay` 对象（进程退出即销毁）。

| 环境变量 | 含义 |
| --- | --- |
| `RE_VDISPLAY=5k\|8k\|16k` | Agent 启动时自动创建虚拟屏 |
| `RE_VDISPLAY=off` / 未设置 | 不创建 |
| `RE_VDISPLAY_BIN=/path` | 开发时指定 helper 路径（默认同 bundle） |

Profiles（第一期）：

| mode | framebuffer | logical mode | 编码出画 |
| --- | --- | --- | --- |
| `5k` | 5120×2880 | 2560×1440 HiDPI | ≤5K（Ultra 原画） |
| `8k` | 7680×4320 | 3840×2160 HiDPI | ≤8K 满血 |
| `16k` | 15360×8640 | 7680×4320 HiDPI | ≤16K 满血出画（FB 像素 OPEN；**VT 硬顶 8192**，>`8192` 走本机 `ffmpeg`/`libx265`，约数 fps；需 `/usr/local/bin/ffmpeg` 或 Homebrew） |

`DisplayInfo.id` / `OPEN_DESKTOP.display_id` 在 Darwin 上为 **`CGDirectDisplayID`**（不是列表下标）。旧客户端若仍传 `0`/`1`，SCK 会回退按下标匹配。

构建 helper + 冒烟：

```bash
./scripts/build-re-vdisplay.sh /tmp/re-vdisplay
./scripts/vdisplay-smoke.sh 8k
# 安装时会自动编进 .app（或传入第二个参数）：
./scripts/install-macos-app.sh /tmp/runeverything /tmp/re-vdisplay
RE_VDISPLAY=8k /Applications/RunEverything.app/Contents/MacOS/RunEverything tray
```

## 联调常见坑

1. **改完必须重装并重启托盘**；扫码前确认进程已是新二进制。
2. UDP 端口每次重启会变 → **必须重新扫码**；旧 QR 的 `lan: ip:port` 会失效。
3. LAN REHP1 监听用 **`udp4`**（`internal/reudp`）。不要改回裸 `"udp"` dual-stack——在 Rosetta 下会出现「端口在听但不回 `REHP1|pong`」。
4. 远程桌面默认 **不采麦**（避免橙色麦标挤掉托盘图标）。需要时：`RE_AUDIO=1`。
5. 崩溃报告：`~/Library/Logs/DiagnosticReports/RunEverything-*.ips`  
   运行日志：`/tmp/re-agent.log`（需按上面方式带重定向启动）。
6. 本机防火墙若开启，确认允许 `/Applications/RunEverything.app` 传入连接。
7. 虚拟屏：确认 `.app` 内有 `Contents/MacOS/re-vdisplay`；私有 API 在某些 macOS 上不可用时 Agent 只打日志，不影响真实屏。
8. 二维码里的 `relay` 应是公网 `wss://…`（`public_relay`）；`lan` 仅同 Wi‑Fi 直连。蜂窝扫到 `ws://192.168.x.x` 会卡在「正在配对」。
9. **Cursor 启动与 TCC**：Cursor Agent/Shell 直启托盘只用于抓 `/tmp/re-agent.log`；远程输入验收前必须用 LaunchServices 重启，并确认 TCC 的 responsible identity 是 RunEverything。

## 与 KoKo / 模拟器

- KoKo iOS 编译用 **Xcode 27.1 beta**：  
  `DEVELOPER_DIR=/Applications/Xcode-27.1-beta.app/Contents/Developer`
- 模拟器联调：改 Mac 上 Agent；App 改 KoKo。协议 / 行为变更两边对齐。
- 相关协议说明见 `docs/koko-re2.md`、`docs/koko-re2-handoff.md`、`docs/protocol-v2.md`。

## 协作建议

- 可为联调修改 Agent（tray / reudp / desktop / re2 等）。
- 非琐碎改动请短说明：文件 + 原因；装完确认 PID / mtime。

## Git 提交：禁止 AI 痕迹（硬性）

推 GitHub 时以**真人**提交，统一身份：

| 项 | 值 |
| --- | --- |
| Name | `foqerhk` |
| Email | `foqerhk@gmail.com` |

**禁止**出现在 commit message / trailer / author 里：

- `Co-authored-by: Cursor <cursoragent@cursor.com>`
- 任何 `Cursor` / `cursoragent` / `Generated by` / `Made with Cursor` / AI 机器人署名

单次提交示例（不要改全局 git config）：

```bash
export GIT_AUTHOR_NAME=foqerhk GIT_AUTHOR_EMAIL=foqerhk@gmail.com
export GIT_COMMITTER_NAME=foqerhk GIT_COMMITTER_EMAIL=foqerhk@gmail.com
git commit --author='foqerhk <foqerhk@gmail.com>' -m "$(cat <<'EOF'
Your human-sounding message here.

EOF
)"
```

提交前：`git log -1 --format='%an <%ae>%n%b'` 确认无 Cursor。更完整说明见本机 runbook `~/Downloads/KOKO_RE_LOCAL_DEVICE_AGENT_SERVER.md`。

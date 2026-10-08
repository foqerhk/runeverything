package i18n

// Simplified Chinese catalog (also used for Traditional Chinese system locales).
var zh = map[string]string{
	"log.prefix": "runeverything: ",

	"usage": `RunEverything Agent — Intent Computing 远程端点（仅 RE2）

用法:
  runeverything           默认：状态栏/托盘常驻（菜单栏图标；显示二维码 / 复制链接 / 开机自启 / 退出）
  runeverything tray      同上（兼容别名）
  runeverything run       无界面连接中继 /re2，打印配对二维码，提供 PTY 会话
  runeverything qr        显示当前配对二维码（读 last_pairing.json，不打断已在跑的 agent）
  runeverything pair      向中继刷新配对令牌并打印二维码（另开连接，可能挤掉正在跑的 agent）
  runeverything status    显示设备身份与配置
  runeverything config    查看/设置公网 IP 检测与地区接口（详见：runeverything config help）
  runeverything version   打印版本号

参数 (run):
  -relay URL              中继 WebSocket 地址（默认自动发现公益中继，或 RE_RELAY）
  -public URL             写入二维码的客户端地址（默认与 -relay 相同；路径强制 /re2）
  -no-qr                  启动时不打印二维码（仍会注册配对令牌）

中继选择:
  处于 NAT 后（家庭/办公）：从官方种子发现公益中继（延迟最低）。
  拥有直连公网 IP（非 NAT）：使用本机中继；二维码广告本机 — 不经中间网关。
  可用 -relay / RE_RELAY / relay_manual 锁定。关闭分享：RE_SHARE_RELAY=0。
`,

	"config.usage": `用法:
  runeverything config show
  runeverything config set-ip-echo --cn URL,URL --intl URL,URL
  runeverything config set-geo-url URL
  runeverything config reset-endpoints

接口优先级：环境变量 > config.json > 内置默认
  RE_IP_ECHO_CN / ip_echo_cn     国内优先的出口 IP 检测 URL（纯文本 IP）
  RE_IP_ECHO_INTL / ip_echo_intl 国外优先的出口 IP 检测 URL
  RE_GEO_URL / geo_url           国家码 JSON API（{"status":"success","countryCode":"CN"}）
  RE_REGION=cn|intl              跳过地理 API，强制区域
  RE_LANG=zh|en                  强制界面/日志语言（zh-* 均用简体中文）

内置出口 IP 检测默认:
  国内: %s
  国外: %s
  地区: %s
`,

	"config.unknown":                   "未知 config 子命令 %q\n\n",
	"config.need_cn_or_intl":           "请至少指定 --cn 或 --intl",
	"config.empty_cn":                  "--cn 列表为空",
	"config.empty_intl":                "--intl 列表为空",
	"config.geo_usage":                 "用法: runeverything config set-geo-url URL",
	"config.saved_ip_echo":             "已将出口 IP 检测接口写入 config.json",
	"config.saved_geo":                 "已将 geo_url 写入 config.json",
	"config.reset_ok":                  "已清除 config.json 中的 ip_echo_* 与 geo_url（恢复内置默认）",
	"config.endpoints_effective":       "当前生效接口:",
	"config.overrides":                 "config.json 覆盖项:",
	"config.default":                   "（默认）",
	"config.region_forced":             "\nRE_REGION=%s（已强制区域；不使用地理 API）\n",

	"status.home":         "目录:         %s\n",
	"status.device_id":    "设备 ID:      %s\n",
	"status.name":         "名称:         %s\n",
	"status.relay":        "中继:         %s\n",
	"status.public_relay": "公开中继:     %s\n",
	"status.protocol":     "协议:         RE2 (v%d)\n",
	"status.relay_manual": "手动锁定中继: %v\n",
	"status.share_relay":  "分享中继:     %v\n",
	"status.platform":     "平台:         %s/%s\n",
	"status.lang":         "语言:         %s\n",
	"status.nat_no":       "NAT:          否（直连公网 IP %s）\n",
	"status.nat_yes":      "NAT:          是（使用公益中继网关）\n",

	"log.direct_public":   "直连公网 IP %s（非 NAT）；跳过公益中继发现",
	"log.behind_nat":      "处于 NAT 后；正在发现公益中继网关",
	"log.selected_relay":  "已选择中继 %s（延迟最低）",
	"log.relay_offline":   "警告: 无法连接中继（%v）；仍打印离线二维码",
	"log.shutting_down":   "正在退出",
	"log.disconnected":    "已断开: %v；%s 后重连",
	"log.session_closed":  "会话已关闭 %s（%s）",
	"log.keepalive":       "保活: 正在阻止闲置睡眠（设 RE_KEEP_AWAKE=0 可关闭）",
	"log.keepalive_fail":   "保活: %v（机器仍可能睡眠）",
	"log.re2_noise_udp":    "re2 Noise 会话已在 UDP 上建立 device=%s",

	"log.re2_registered":       "re2 已注册为 %s（%s），经 %s",
	"log.re2_pair_offer":       "re2 配对要约: %v",
	"log.re2_noise_ok":         "re2 Noise 会话已建立 device=%s",
	"log.re2_handshake_fail":   "re2 握手失败: %v",
	"log.re2_handshake_init":   "re2 握手初始化: %v",
	"log.re2_decrypt_fail":     "re2 解密失败: %v",
	"log.re2_inner":            "re2 内层消息: %v",
	"log.re2_relay_error":      "re2 中继错误: %s %s",
	"log.re2_unknown_frame":    "re2 未知帧类型=%s",
	"log.re2_unknown_inner":    "re2 未知内层类型=%d",
	"log.reudp_assoc_warn":     "reudp 关联警告: %v（继续使用 wss）",
	"log.reudp_associated":     "reudp 已关联 device=%s hint=%s",
	"log.video_keyframe":       "视频关键帧 id=%d parts=%d annexB=%d（可靠）",
	"log.reudp_recv":           "reudp 接收: %v",
	"log.session_idle":         "会话空闲超时（%s）",
	"log.session_read":         "会话 %s 读取: %v",
	"log.p2p_selected":         "p2p: 已选择 %s（健康 %d/%d）",
	"log.p2p_keep_sticky":      "p2p: 保留粘性中继 %s（%v）",
	"log.p2p_discover_failed":  "p2p: 发现失败（%v）；继续使用 %s",
	"log.perm_ok":              "本机权限已就绪（屏幕录制 + 辅助功能）",
	"log.perm_screen_need":     "缺少「屏幕录制」权限：系统设置 → 隐私与安全性 → 屏幕录制 → 勾选 runeverything（改完后请重启本程序）",
	"log.perm_ax_need":         "缺少「辅助功能」权限：系统设置 → 隐私与安全性 → 辅助功能 → 勾选 runeverything（用于远程键鼠）",
	"log.perm_mic_hint":        "若需远程声音：系统设置 → 隐私与安全性 → 麦克风 → 允许 runeverything / Terminal（可选）",
	"log.perm_camera_hint":     "若使用摄像头外设：系统设置 → 隐私与安全性 → 摄像头 → 允许 runeverything（可选）",

	"status.perm_screen": "屏幕录制:   %s\n",
	"status.perm_ax":     "辅助功能:   %s\n",
	"status.perm_yes":    "已授权",
	"status.perm_no":     "未授权（远程桌面不可用）",
	"status.perm_n_a":    "不适用",

	"err.perm_screen": "屏幕录制未授权：请在系统设置中勾选 runeverything 后重启 Agent",
	"err.perm_ax":     "辅助功能未授权：远程键鼠可能无效，请在系统设置中勾选 runeverything",

	"pair.title":     "=== RunEverything 配对 ===",
	"pair.device":    "设备: %s（%s）",
	"pair.relay":     "中继: %s",
	"pair.udp":       "UDP:  %s",
	"pair.lan":       "LAN:  %s",
	"pair.proto":     "协议: v%d",
	"pair.noise":     "Noise: %s…",
	"pair.expires":   "过期: %s",
	"pair.json":      "JSON 载荷:",
	"pair.deeplink":  "深链:",

	"qr.ok":         "当前配对码（%s，剩余 %s）:",
	"qr.missing":    "没有配对文件 %s。请先启动 runeverything run，或执行 runeverything pair。",
	"qr.incomplete": "配对文件不完整: %s",
	"qr.expired":    "当前配对码已过期（%s）。请重启 runeverything run 生成新码，或执行 runeverything pair。",

	"desktop.confirm":         "允许远程桌面会话？",
	"desktop.confirm_title":   "RunEverything",
	"desktop.btn_allow":       "允许",
	"desktop.btn_deny":        "拒绝",
	"desktop.timeout_denied":  "（超时 — 已拒绝）",
	"desktop.stdin_hint":      " [y/N]: ",

	"tray.tooltip":            "RunEverything Agent",
	"tray.tooltip_running":    "RunEverything — 运行中",
	"tray.tooltip_failed":     "RunEverything（启动失败）",
	"tray.tooltip_reconnecting": "RunEverything — 重连中…",
	"tray.show_qr":            "显示配对二维码",
	"tray.show_qr_tip":        "打开二维码窗口（过期自动刷新）",
	"tray.copy_link":          "复制配对链接",
	"tray.copy_link_tip":      "复制深链到剪贴板",
	"tray.open_folder":        "打开数据目录",
	"tray.open_folder_tip":    "打开 ~/.runeverything",
	"tray.autostart":          "登录时启动",
	"tray.autostart_tip":      "开机/登录后自动打开托盘 Agent",
	"tray.autostart_on":       "已开启登录自启",
	"tray.autostart_off":      "已关闭登录自启",
	"tray.autostart_fail":     "设置自启失败: %s",
	"tray.start_windows":      "开机自启",
	"tray.start_windows_tip":  "登录计划任务",
	"tray.quit":               "退出",
	"tray.quit_tip":           "停止 Agent",
	"tray.not_running":        "Agent 未运行",
	"tray.pair_failed":        "配对失败: %s",
	"tray.qr_opened":          "已打开二维码；配对链接已复制",
	"tray.link_copied":        "配对链接已复制",
	"tray.need_desktop":       "托盘需要图形桌面会话（请在已登录的桌面环境运行 runeverything tray）",
	"tray.unsupported":        "当前系统不支持托盘模式；请使用: runeverything run",
	"tray.perms_linux_hint":   "Linux 桌面权限因发行版而异；请确保会话已登录且允许远程/屏幕共享相关能力。",
	"tray.only_windows":       "托盘模式仅支持 Windows；请使用: runeverything run",
	"singleton.busy":          "已有 RunEverything Agent 在运行（请从菜单栏/托盘退出，或稍候再试）",
	"tray.controlled":         "当前桌面被远程控制中…",
	"tray.tooltip_controlled": "RunEverything — 当前桌面被远程控制中",
	"perm.window_title":       "权限查看",
	"perm.window_sub":         "请逐项了解说明；点「去设置」会弹出系统授权确认（确认框里可再进系统设置）。屏幕录制勾选后，点「完成并重新打开」即可生效。",
	"perm.auto_refresh":       "从系统对话框回到本窗口后会刷新状态；仅屏幕录制通常需要重新打开本程序。",
	"perm.btn_settings":       "去设置",
	"perm.btn_restart":        "重启生效",
	"perm.btn_done":           "完成",
	"perm.btn_done_restart":   "完成并重新打开 RunEverything",
	"perm.status_ok":          "已授权",
	"perm.status_need":        "未授权",
	"perm.status_restart":     "已授权，需重启",
	"perm.status_optional":    "可选",
	"perm.badge_required":     "必需",
	"perm.badge_optional":     "可选",
	"perm.restart_desc":       "系统已勾选，重新打开后才会在当前进程生效。",
	"perm.screen_title":       "屏幕录制",
	"perm.screen_desc":        "用于采集桌面画面，供手机端远程观看。",
	"perm.ax_title":           "辅助功能",
	"perm.ax_desc":            "用于远程键盘、鼠标控制。",
	"perm.mic_title":          "麦克风",
	"perm.mic_desc":           "需要分享麦克风声音时再开启。",
	"perm.camera_title":       "摄像头",
	"perm.camera_desc":        "需要分享摄像头外设时再开启。",
	"perm.phonecam_title":     "用手机当摄像头",
	"perm.phonecam_desc":      "可选：把手机画面变成电脑上的虚拟摄像头（Zoom/Meet/浏览器）。macOS 新系统必须用摄像头扩展：点「去设置」输管理员密码 → 在 AkVirtualCameraCX 窗口点 Install extension → 系统设置里允许 → 远程桌面开启「用手机当摄像头」→ 网页摄像头列表选「KoKo Phone Camera」（不要选 MacBook 摄像头）。",
	"perm.phonecam_desc_on":   "系统已枚举到 %s。请在远程桌面开启「用手机当摄像头」，并在网页/会议软件里选择该摄像头。",
	"perm.phonecam_status_off": "未开启",
	"perm.phonecam_status_on":  "已可用",
	"perm.win_screen_title":   "屏幕采集",
	"perm.win_screen_desc":    "用于采集桌面画面，供手机端远程观看（Windows 一般无需额外系统开关）。",
	"perm.win_input_title":    "键鼠控制",
	"perm.win_input_desc":     "用于远程键盘、鼠标控制（Windows 一般无需额外系统开关）。",
	"perm.linux_title":        "桌面会话",
	"perm.linux_desc":         "Linux 权限因发行版而异；请保持已登录的图形桌面会话。",

	"tray.check_perms":     "查看使用权限…",
	"tray.check_perms_tip": "屏幕录制 / 辅助功能 / 用手机当摄像头等",
	"tray.status":          "状态…",
	"tray.status_tip":      "查看设备与连接状态",
	"tray.config":          "设置…",
	"tray.config_tip":      "中继与分享等配置",
	"tray.help":            "帮助…",
	"tray.help_tip":        "命令与用法说明",
	"tray.about":           "关于 / 版本…",
	"tray.about_tip":       "版本号与检查更新",

	"ui.status_title":          "状态",
	"ui.help_title":            "帮助",
	"ui.about_title":           "关于 RunEverything",
	"ui.config_title":          "设置",
	"ui.config_relay":          "中继地址 (relay)",
	"ui.config_public":         "公开中继 (public)",
	"ui.config_manual":         "锁定中继（不自动发现）",
	"ui.config_manual_hint":    "关闭（推荐）：每次启动自动发现延迟更低的公益中继。开启：一直使用上方填的中继地址，不再换节点。",
	"ui.config_share":          "分享本机为公益中继候选",
	"ui.config_share_hint":     "开启后，若本机对外提供中继能力，可被目录发现并分享给其他用户（需有公网可达地址）。不影响你自己连别人的中继。",
	"ui.config_save":           "保存",
	"ui.config_cancel":         "取消",
	"ui.config_cli_hint":       "完整接口配置仍可用：runeverything config",
	"ui.qr_title":              "配对二维码",
	"ui.qr_expires_in":         "剩余 %@（过期后自动刷新）",
	"ui.qr_refreshing":         "正在刷新二维码…",
	"ui.qr_refresh":            "立即刷新",
	"ui.version_line":          "版本: %s\n",
	"ui.app_name":              "RunEverything",
	"ui.github_url":            "https://github.com/foqerhk/runeverything",
	"ui.contact_email":         "hello_intentcomputing@gmail.com",
	"ui.check_update":          "检查更新",
	"ui.update_latest":         "已是最新版本（%s）",
	"ui.update_available":      "发现新版本 %s（当前 %s），正在打开下载页…",
	"ui.update_none":           "未找到可用版本信息",
	"ui.update_fail":           "检查更新失败：%s",
	"log.perm_open_panel_hint": "缺少必需权限：请从菜单栏打开「查看使用权限」按说明授权",

	"status.lan":            "局域网 IP:  %s\n",
	"status.lan_port":       "局域网端口: %s\n",
	"status.perm_mic":       "麦克风:     %s\n",
	"status.perm_camera":    "摄像头:     %s\n",
	"status.control_yes":    "控制连接:   是（有设备正在控制本机）\n",
	"status.control_no":     "控制连接:   否\n",
	"status.control_desktop": "  远程桌面: 会话 %s\n",
	"status.control_pty":    "  终端会话: %d 个\n",
	"status.control_peer":   "  对端地址: %s\n",
	"status.control_since":  "  开始时间: %s\n",
}

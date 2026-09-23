package i18n

// Simplified Chinese catalog (also used for Traditional Chinese system locales).
var zh = map[string]string{
	"log.prefix": "runeverything: ",

	"usage": `RunEverything Agent — Intent Computing 远程端点（仅 RE2）

用法:
  runeverything [run]     连接中继 /re2，打印配对二维码，提供 PTY 会话
  runeverything tray      Windows：托盘常驻（二维码 / 开机自启 / 退出）
  runeverything pair      刷新配对令牌并打印二维码
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
	"log.reudp_recv":           "reudp 接收: %v",
	"log.session_idle":         "会话空闲超时（%s）",
	"log.session_read":         "会话 %s 读取: %v",
	"log.p2p_selected":         "p2p: 已选择 %s（健康 %d/%d）",
	"log.p2p_keep_sticky":      "p2p: 保留粘性中继 %s（%v）",
	"log.p2p_discover_failed":  "p2p: 发现失败（%v）；继续使用 %s",

	"pair.title":     "=== RunEverything 配对 ===",
	"pair.device":    "设备: %s（%s）",
	"pair.relay":     "中继: %s",
	"pair.udp":       "UDP:  %s",
	"pair.proto":     "协议: v%d",
	"pair.noise":     "Noise: %s…",
	"pair.expires":   "过期: %s",
	"pair.json":      "JSON 载荷:",
	"pair.deeplink":  "深链:",

	"desktop.confirm":         "允许远程桌面会话？",
	"desktop.confirm_title":   "RunEverything",
	"desktop.btn_allow":       "允许",
	"desktop.btn_deny":        "拒绝",
	"desktop.timeout_denied":  "（超时 — 已拒绝）",
	"desktop.stdin_hint":      " [y/N]: ",

	"tray.tooltip":            "RunEverything Agent",
	"tray.tooltip_running":    "RunEverything — 运行中",
	"tray.tooltip_failed":     "RunEverything（启动失败）",
	"tray.show_qr":            "显示配对二维码",
	"tray.show_qr_tip":        "打开二维码图片",
	"tray.copy_link":          "复制配对链接",
	"tray.copy_link_tip":      "复制深链到剪贴板",
	"tray.open_folder":        "打开数据目录",
	"tray.open_folder_tip":    "打开 ~/.runeverything",
	"tray.start_windows":      "开机自启",
	"tray.start_windows_tip":  "登录计划任务",
	"tray.quit":               "退出",
	"tray.quit_tip":           "停止 Agent",
	"tray.not_running":        "Agent 未运行",
	"tray.pair_failed":        "配对失败: %s",
	"tray.qr_opened":          "已打开二维码；配对链接已复制",
	"tray.link_copied":        "配对链接已复制",
	"tray.only_windows":       "托盘模式仅支持 Windows；请使用: runeverything run",
}

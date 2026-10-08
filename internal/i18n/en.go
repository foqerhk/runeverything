package i18n

// English message catalog (keys are stable IDs).
var en = map[string]string{
	"log.prefix": "runeverything: ",

	"usage": `RunEverything Agent — Intent Computing remote endpoint (RE2 only)

Usage:
  runeverything           Default: menu-bar / system tray agent (QR / copy link / autostart / quit)
  runeverything tray      Same as default (compat alias)
  runeverything run       Headless: connect to relay /re2, print pairing QR, serve PTY sessions
  runeverything qr        Show current pairing QR (reads last_pairing.json; safe while agent is up)
  runeverything pair      Mint a new pairing token on the relay and print QR (extra connection; may kick a running agent)
  runeverything status    Show device identity and config
  runeverything config    Show/set public-IP echo + geo endpoints (see: runeverything config help)
  runeverything version   Print version

Flags (run):
  -relay URL              Relay WebSocket URL (default: auto-discover volunteer relay, or RE_RELAY)
  -public URL             URL embedded in QR for clients (defaults to -relay; path forced to /re2)
  -no-qr                  Do not print QR on start (still registers pairing token)

Relay selection:
  Behind NAT (home/office): discover volunteer relays via official seeds (lowest ping).
  Direct public IP (not behind NAT): use local relay; QR advertises this host — no middle gateway.
  Pin with -relay / RE_RELAY / relay_manual. Opt out of sharing with RE_SHARE_RELAY=0.
`,

	"config.usage": `Usage:
  runeverything config show
  runeverything config set-ip-echo --cn URL,URL --intl URL,URL
  runeverything config set-geo-url URL
  runeverything config reset-endpoints

Endpoints (priority: env > config.json > built-in defaults):
  RE_IP_ECHO_CN / ip_echo_cn     mainland public-IP echo URLs (plain text body)
  RE_IP_ECHO_INTL / ip_echo_intl international public-IP echo URLs
  RE_GEO_URL / geo_url           country JSON API ({"status":"success","countryCode":"CN"})
  RE_REGION=cn|intl              skip geo lookup and force region
  RE_LANG=zh|en                  force UI/log language (zh-* → Simplified Chinese)

Built-in IP echo defaults:
  CN:   %s
  Intl: %s
  Geo:  %s
`,

	"config.unknown":                   "unknown config subcommand %q\n\n",
	"config.need_cn_or_intl":           "set at least one of --cn or --intl",
	"config.empty_cn":                  "empty --cn list",
	"config.empty_intl":                "empty --intl list",
	"config.geo_usage":                 "usage: runeverything config set-geo-url URL",
	"config.saved_ip_echo":             "saved ip echo endpoints to config.json",
	"config.saved_geo":                 "saved geo_url to config.json",
	"config.reset_ok":                  "cleared ip_echo_* and geo_url from config.json (built-in defaults active)",
	"config.endpoints_effective":       "endpoints (effective):",
	"config.overrides":                 "config.json overrides:",
	"config.default":                   "(default)",
	"config.region_forced":             "\nRE_REGION=%s (forces region; geo unused)\n",

	"status.home":         "home:         %s\n",
	"status.device_id":    "device_id:    %s\n",
	"status.name":         "name:         %s\n",
	"status.relay":        "relay:        %s\n",
	"status.public_relay": "public_relay: %s\n",
	"status.protocol":     "protocol:     RE2 (v%d)\n",
	"status.relay_manual": "relay_manual: %v\n",
	"status.share_relay":  "share_relay:  %v\n",
	"status.platform":     "platform:     %s/%s\n",
	"status.lang":         "lang:         %s\n",
	"status.nat_no":       "nat:          no (direct public IP %s)\n",
	"status.nat_yes":      "nat:          yes (use volunteer relay gateway)\n",

	"log.direct_public":   "direct public IP %s (not behind NAT); skipping volunteer relay discovery",
	"log.behind_nat":      "behind NAT; discovering volunteer relay gateway",
	"log.selected_relay":  "selected relay %s (lowest ping)",
	"log.relay_offline":   "warning: could not reach relay (%v); printing offline QR anyway",
	"log.shutting_down":   "shutting down",
	"log.disconnected":    "disconnected: %v; reconnecting in %s",
	"log.session_closed":  "session closed %s (%s)",
	"log.keepalive":       "keepalive: preventing idle sleep (set RE_KEEP_AWAKE=0 to disable)",
	"log.keepalive_fail":   "keepalive: %v (machine may still sleep)",
	"log.re2_noise_udp":    "re2 noise session established over UDP device=%s",

	"log.re2_registered":       "re2 registered as %s (%s) via %s",
	"log.re2_pair_offer":       "re2 pair offer: %v",
	"log.re2_noise_ok":         "re2 noise session established device=%s",
	"log.re2_handshake_fail":   "re2 handshake failed: %v",
	"log.re2_handshake_init":   "re2 handshake init: %v",
	"log.re2_decrypt_fail":     "re2 decrypt failed: %v",
	"log.re2_inner":            "re2 inner: %v",
	"log.re2_relay_error":      "re2 relay error: %s %s",
	"log.re2_unknown_frame":    "re2 unknown frame type=%s",
	"log.re2_unknown_inner":    "re2 unknown inner type=%d",
	"log.reudp_assoc_warn":     "reudp assoc warning: %v (continuing on wss)",
	"log.reudp_associated":     "reudp associated device=%s hint=%s",
	"log.video_keyframe":       "video keyframe id=%d parts=%d annexB=%d (reliable)",
	"log.reudp_recv":           "reudp recv: %v",
	"log.session_idle":         "session idle timeout (%s)",
	"log.session_read":         "session %s read: %v",
	"log.p2p_selected":         "p2p: selected %s (%d/%d healthy)",
	"log.p2p_keep_sticky":      "p2p: keep sticky relay %s (%v)",
	"log.p2p_discover_failed":  "p2p: discover failed (%v); keeping %s",
	"log.perm_ok":              "host permissions OK (Screen Recording + Accessibility)",
	"log.perm_screen_need":     "Screen Recording permission missing: System Settings → Privacy & Security → Screen Recording → enable runeverything (then restart this agent)",
	"log.perm_ax_need":         "Accessibility permission missing: System Settings → Privacy & Security → Accessibility → enable runeverything (remote keyboard/mouse)",
	"log.perm_mic_hint":        "For remote audio: System Settings → Privacy & Security → Microphone → allow runeverything / Terminal (optional)",
	"log.perm_camera_hint":     "For camera peripherals: System Settings → Privacy & Security → Camera → allow runeverything (optional)",

	"status.perm_screen": "screen_recording: %s\n",
	"status.perm_ax":     "accessibility:   %s\n",
	"status.perm_yes":    "granted",
	"status.perm_no":     "denied (remote desktop will fail)",
	"status.perm_n_a":    "n/a",

	"err.perm_screen": "Screen Recording not granted — enable runeverything in System Settings, then restart the Agent",
	"err.perm_ax":     "Accessibility not granted — remote input may fail; enable runeverything in System Settings",

	"pair.title":     "=== RunEverything Pairing ===",
	"pair.device":    "Device: %s (%s)",
	"pair.relay":     "Relay:  %s",
	"pair.udp":       "UDP:    %s",
	"pair.lan":       "LAN:    %s",
	"pair.proto":     "Proto:  v%d",
	"pair.noise":     "Noise:  %s…",
	"pair.expires":   "Expires: %s",
	"pair.json":      "JSON payload:",
	"pair.deeplink":  "Deep link:",

	"qr.ok":         "Current pairing QR (%s, %s left):",
	"qr.missing":    "No pairing file at %s. Start runeverything run first, or run runeverything pair.",
	"qr.incomplete": "Incomplete pairing file: %s",
	"qr.expired":    "Current pairing QR expired (%s). Restart runeverything run or run runeverything pair.",

	"desktop.confirm":         "Allow remote desktop session?",
	"desktop.confirm_title":   "RunEverything",
	"desktop.btn_allow":       "Allow",
	"desktop.btn_deny":        "Deny",
	"desktop.timeout_denied":  "(timeout — denied)",
	"desktop.stdin_hint":      " [y/N]: ",

	"tray.tooltip":              "RunEverything Agent",
	"tray.tooltip_running":      "RunEverything — running",
	"tray.tooltip_failed":       "RunEverything (failed to start)",
	"tray.tooltip_reconnecting": "RunEverything — reconnecting…",
	"tray.show_qr":              "Show pairing QR",
	"tray.show_qr_tip":          "Open QR window (auto-refreshes when expired)",
	"tray.copy_link":            "Copy pair link",
	"tray.copy_link_tip":        "Copy deep link to clipboard",
	"tray.open_folder":          "Open data folder",
	"tray.open_folder_tip":      "Open ~/.runeverything",
	"tray.check_perms":          "View permissions…",
	"tray.check_perms_tip":      "Screen Recording / Accessibility / Phone as Webcam",
	"tray.status":               "Status…",
	"tray.status_tip":           "Device and connection status",
	"tray.config":               "Settings…",
	"tray.config_tip":           "Relay and sharing options",
	"tray.help":                 "Help…",
	"tray.help_tip":             "Commands and usage",
	"tray.about":                "About / Version…",
	"tray.about_tip":            "Version and check for updates",
	"tray.autostart":            "Start at login",
	"tray.autostart_tip":        "Launch tray agent when you log in",
	"tray.autostart_on":         "Autostart enabled",
	"tray.autostart_off":        "Autostart disabled",
	"tray.autostart_fail":       "Autostart failed: %s",
	"tray.start_windows":        "Start with Windows",
	"tray.start_windows_tip":    "Logon scheduled task",
	"tray.quit":                 "Quit",
	"tray.quit_tip":             "Stop agent",
	"tray.not_running":          "Agent is not running",
	"tray.pair_failed":          "Pairing failed: %s",
	"tray.qr_opened":            "QR opened; pair link copied",
	"tray.link_copied":          "Pair link copied",
	"tray.need_desktop":         "Tray needs a graphical desktop session (run: runeverything tray)",
	"tray.unsupported":          "Tray mode is not supported on this OS; use: runeverything run",
	"tray.perms_linux_hint":     "Linux desktop permissions vary by distro; ensure a logged-in graphical session.",
	"tray.only_windows":         "tray mode is only available on Windows; use: runeverything run",
	"singleton.busy":            "Another RunEverything agent is already running (quit it from the menu bar / tray, or wait a moment)",
	"tray.controlled":           "Desktop is being remotely controlled…",
	"tray.tooltip_controlled":   "RunEverything — desktop is being remotely controlled",
	"perm.window_title":         "Permissions",
	"perm.window_sub":           "Read each item, then tap Grant to show the system permission prompt. After Screen Recording, tap Done & relaunch.",
	"perm.auto_refresh":         "Status updates when you return to this window. Only Screen Recording typically needs a relaunch.",
	"perm.btn_settings":         "Grant…",
	"perm.btn_restart":          "Restart to apply",
	"perm.btn_done":             "Done",
	"perm.btn_done_restart":     "Done & relaunch RunEverything",
	"perm.status_ok":            "Granted",
	"perm.status_need":          "Not granted",
	"perm.status_restart":       "Granted — restart needed",
	"perm.status_optional":      "Optional",
	"perm.badge_required":       "Required",
	"perm.badge_optional":       "Optional",
	"perm.restart_desc":         "Already checked in System Settings; relaunch this process for it to take effect.",
	"perm.screen_title":         "Screen Recording",
	"perm.screen_desc":          "Captures the desktop for remote viewing.",
	"perm.ax_title":             "Accessibility",
	"perm.ax_desc":              "Enables remote keyboard and mouse control.",
	"perm.mic_title":            "Microphone",
	"perm.mic_desc":             "Needed if you want to share microphone audio remotely.",
	"perm.camera_title":         "Camera",
	"perm.camera_desc":          "Needed if you share a camera peripheral.",
	"perm.phonecam_title":       "Use Phone as Webcam",
	"perm.phonecam_desc":        "Optional: expose the phone feed as a virtual webcam (Zoom/Meet/browser). Tap Grant to install the driver (admin password), then Install extension in the helper app and allow the Camera Extension in System Settings. In the website camera picker, choose “KoKo Phone Camera”, not the built-in Mac camera.",
	"perm.phonecam_desc_on":     "System lists %s. Enable “Use Phone as Webcam” from remote desktop, then select that camera in the site/meeting app.",
	"perm.phonecam_status_off":  "Off",
	"perm.phonecam_status_on":   "Available",
	"perm.win_screen_title":     "Screen capture",
	"perm.win_screen_desc":      "Captures the desktop for remote viewing (usually no extra Windows toggle).",
	"perm.win_input_title":      "Keyboard & mouse",
	"perm.win_input_desc":       "Enables remote keyboard and mouse control (usually no extra Windows toggle).",
	"perm.linux_title":          "Desktop session",
	"perm.linux_desc":           "Linux permissions vary by distro; keep a logged-in graphical session.",

	"ui.status_title":          "Status",
	"ui.help_title":            "Help",
	"ui.about_title":           "About RunEverything",
	"ui.config_title":          "Settings",
	"ui.config_relay":          "Relay URL",
	"ui.config_public":         "Public relay URL",
	"ui.config_manual":         "Pin relay (disable auto-discover)",
	"ui.config_manual_hint":    "Off (recommended): each launch auto-picks a lower-latency public relay. On: always use the relay URL above.",
	"ui.config_share":          "Offer this host as a public relay candidate",
	"ui.config_share_hint":     "When on, if this machine can serve as a relay it may be discovered by others (needs a reachable public address). Does not change how you connect outbound.",
	"ui.config_save":           "Save",
	"ui.config_cancel":         "Cancel",
	"ui.config_cli_hint":       "Full endpoint config: runeverything config",
	"ui.qr_title":              "Pairing QR",
	"ui.qr_expires_in":         "%@ left (auto-refreshes when expired)",
	"ui.qr_refreshing":         "Refreshing QR…",
	"ui.qr_refresh":            "Refresh now",
	"ui.version_line":          "Version: %s\n",
	"ui.app_name":              "RunEverything",
	"ui.github_url":            "https://github.com/foqerhk/runeverything",
	"ui.contact_email":         "hello_intentcomputing@gmail.com",
	"ui.check_update":          "Check for updates",
	"ui.update_latest":         "Already up to date (%s)",
	"ui.update_available":      "New version %s available (current %s). Opening download…",
	"ui.update_none":           "No release info found",
	"ui.update_fail":           "Update check failed: %s",
	"log.perm_open_panel_hint": "Required permissions missing: open View permissions from the menu bar",

	"status.lan":             "LAN IP:          %s\n",
	"status.lan_port":        "LAN port:        %s\n",
	"status.perm_mic":        "microphone:      %s\n",
	"status.perm_camera":     "camera:          %s\n",
	"status.control_yes":     "controlled:      yes\n",
	"status.control_no":      "controlled:      no\n",
	"status.control_desktop": "  desktop sid:   %s\n",
	"status.control_pty":     "  pty sessions:  %d\n",
	"status.control_peer":    "  peer addr:     %s\n",
	"status.control_since":   "  since:         %s\n",
}

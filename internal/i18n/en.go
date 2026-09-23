package i18n

// English message catalog (keys are stable IDs).
var en = map[string]string{
	"log.prefix": "runeverything: ",

	"usage": `RunEverything Agent — Intent Computing remote endpoint (RE2 only)

Usage:
  runeverything [run]     Connect to relay /re2, print pairing QR, serve PTY sessions
  runeverything tray      Windows: system tray agent (QR / autostart / quit)
  runeverything pair      Refresh pairing token and print QR (requires running agent OR local-only offer)
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
	"log.reudp_recv":           "reudp recv: %v",
	"log.session_idle":         "session idle timeout (%s)",
	"log.session_read":         "session %s read: %v",
	"log.p2p_selected":         "p2p: selected %s (%d/%d healthy)",
	"log.p2p_keep_sticky":      "p2p: keep sticky relay %s (%v)",
	"log.p2p_discover_failed":  "p2p: discover failed (%v); keeping %s",

	"pair.title":     "=== RunEverything Pairing ===",
	"pair.device":    "Device: %s (%s)",
	"pair.relay":     "Relay:  %s",
	"pair.udp":       "UDP:    %s",
	"pair.proto":     "Proto:  v%d",
	"pair.noise":     "Noise:  %s…",
	"pair.expires":   "Expires: %s",
	"pair.json":      "JSON payload:",
	"pair.deeplink":  "Deep link:",

	"desktop.confirm":         "Allow remote desktop session?",
	"desktop.confirm_title":   "RunEverything",
	"desktop.btn_allow":       "Allow",
	"desktop.btn_deny":        "Deny",
	"desktop.timeout_denied":  "(timeout — denied)",
	"desktop.stdin_hint":      " [y/N]: ",

	"tray.tooltip":            "RunEverything Agent",
	"tray.tooltip_running":    "RunEverything — running",
	"tray.tooltip_failed":     "RunEverything (failed to start)",
	"tray.show_qr":            "Show pairing QR",
	"tray.show_qr_tip":        "Open QR code image",
	"tray.copy_link":          "Copy pair link",
	"tray.copy_link_tip":      "Copy deep link to clipboard",
	"tray.open_folder":        "Open data folder",
	"tray.open_folder_tip":    "Open ~/.runeverything",
	"tray.start_windows":      "Start with Windows",
	"tray.start_windows_tip":  "Logon scheduled task",
	"tray.quit":               "Quit",
	"tray.quit_tip":           "Stop agent",
	"tray.not_running":        "Agent is not running",
	"tray.pair_failed":        "Pairing failed: %s",
	"tray.qr_opened":          "QR opened; pair link copied",
	"tray.link_copied":        "Pair link copied",
	"tray.only_windows":       "tray mode is only available on Windows; use: runeverything run",
}

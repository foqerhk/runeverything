package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/identity"
	"github.com/foqerhk/runeverything/internal/netutil"
	"github.com/foqerhk/runeverything/internal/p2p"
	"github.com/foqerhk/runeverything/internal/protocol"
)

func statusReportText() string {
	id, err := identity.LoadOrCreate()
	if err != nil {
		return err.Error()
	}
	cfg, err := identity.LoadConfig()
	if err != nil {
		return err.Error()
	}
	applyNetworkPrefs(cfg)
	applyRE2Paths(cfg)
	home, _ := identity.HomeDir()
	var b strings.Builder
	b.WriteString(i18n.T("status.home", home))
	b.WriteString(i18n.T("status.device_id", id.DeviceID))
	b.WriteString(i18n.T("status.name", id.Name))
	b.WriteString(i18n.T("status.relay", cfg.RelayURL))
	b.WriteString(i18n.T("status.public_relay", cfg.PublicRelay))
	b.WriteString(i18n.T("status.protocol", protocol.VersionRE2))
	b.WriteString(i18n.T("status.relay_manual", cfg.RelayManual))
	b.WriteString(i18n.T("status.share_relay", p2p.SharingEnabled(cfg.ShareRelay)))
	osName, arch := identity.PlatformInfo()
	b.WriteString(i18n.T("status.platform", osName, arch))
	b.WriteString(i18n.T("status.lang", i18n.Active()))
	b.WriteString(i18n.T("ui.version_line", version))

	if lans := netutil.LocalLANIPv4s(); len(lans) > 0 {
		parts := make([]string, 0, len(lans))
		for _, ip := range lans {
			parts = append(parts, ip.String())
		}
		b.WriteString(i18n.T("status.lan", strings.Join(parts, ", ")))
	} else {
		b.WriteString(i18n.T("status.lan", i18n.T("status.perm_n_a")))
	}
	lanPort := 0
	if a := getTrayAgent(); a != nil {
		a.mu.Lock()
		if a.udpEP != nil {
			lanPort = a.udpEP.ListenPort()
		}
		a.mu.Unlock()
	}
	if lanPort <= 0 {
		lanPort = identity.PreferredUDPListenPort()
	}
	if lanPort > 0 {
		b.WriteString(i18n.T("status.lan_port", fmt.Sprintf("%d", lanPort)))
	} else {
		b.WriteString(i18n.T("status.lan_port", i18n.T("status.perm_n_a")))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if ip, ok := netutil.DirectPublicIP(ctx); ok {
		b.WriteString(i18n.T("status.nat_no", ip))
	} else {
		b.WriteString(i18n.T("status.nat_yes"))
	}

	yes, no := i18n.T("status.perm_yes"), i18n.T("status.perm_no")
	if runtime.GOOS == "darwin" {
		p := desktop.CheckHostPermissions()
		writePerm := func(key string, ok bool) {
			if ok {
				b.WriteString(i18n.T(key, yes))
			} else {
				b.WriteString(i18n.T(key, no))
			}
		}
		writePerm("status.perm_screen", p.ScreenRecording)
		writePerm("status.perm_ax", p.Accessibility)
		writePerm("status.perm_mic", p.Microphone)
		writePerm("status.perm_camera", p.Camera)
	}

	cs := ControlStatus{}
	if a := getTrayAgent(); a != nil {
		cs = a.controlStatus()
	}
	if cs.DesktopActive || cs.PTYSessions > 0 {
		b.WriteString(i18n.T("status.control_yes"))
		if cs.DesktopActive {
			b.WriteString(i18n.T("status.control_desktop", cs.DesktopSID))
		}
		if cs.PTYSessions > 0 {
			b.WriteString(i18n.T("status.control_pty", cs.PTYSessions))
		}
		if cs.PeerAddr != "" {
			b.WriteString(i18n.T("status.control_peer", cs.PeerAddr))
		}
		if !cs.Since.IsZero() {
			b.WriteString(i18n.T("status.control_since", cs.Since.Local().Format("15:04:05")))
		}
	} else {
		b.WriteString(i18n.T("status.control_no"))
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

func helpReportText() string {
	return strings.TrimSpace(i18n.T("usage")) + "\n"
}

func configFormDefaults() (relay, pub string, manual, share bool) {
	cfg, err := identity.LoadConfig()
	if err != nil {
		return "", "", false, true
	}
	applyNetworkPrefs(cfg)
	applyRE2Paths(cfg)
	return cfg.RelayURL, cfg.PublicRelay, cfg.RelayManual, p2p.SharingEnabled(cfg.ShareRelay)
}

func saveConfigFromUI(relay, pub string, manual, share bool) error {
	cfg, err := identity.LoadConfig()
	if err != nil {
		return err
	}
	cfg.RelayURL = strings.TrimSpace(relay)
	cfg.PublicRelay = strings.TrimSpace(pub)
	cfg.RelayManual = manual
	cfg.ShareRelay = &share
	applyRE2Paths(cfg)
	return identity.SaveConfig(cfg)
}

func aboutVersionText() string {
	return i18n.T("ui.version_line", version)
}

func fmtErr(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}

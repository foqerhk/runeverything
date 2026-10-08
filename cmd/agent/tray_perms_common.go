//go:build darwin || linux || windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/identity"
)

const permsUIMarker = "perms_ui_shown_v2"

type permRow struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Desc     string `json:"desc"`
	Badge    string `json:"badge"`
	Required bool   `json:"required"`
	Granted  bool   `json:"granted"`
	Optional bool   `json:"optional"`
	Status   string `json:"status"`
	Action   string `json:"action"` // "ok" | "settings" | "optional" | "restart"
}

func permsUIMarkerPath() string {
	home, err := identity.HomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, permsUIMarker)
}

func permsUIAlreadyShown() bool {
	path := permsUIMarkerPath()
	if path == "" {
		return true
	}
	_, err := os.Stat(path)
	return err == nil
}

func markPermsUIShown() {
	path := permsUIMarkerPath()
	if path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o600)
}

func collectPermRows() []permRow {
	switch runtime.GOOS {
	case "darwin":
		return collectPermRowsDarwin()
	case "windows":
		return collectPermRowsWindows()
	default:
		return []permRow{{
			ID: "linux", Title: i18n.T("perm.linux_title"), Desc: i18n.T("perm.linux_desc"),
			Badge: i18n.T("perm.badge_optional"), Required: false, Optional: true, Granted: true,
			Status: i18n.T("perm.status_optional"), Action: "optional",
		}}
	}
}

func collectPermRowsDarwin() []permRow {
	p := desktop.CheckHostPermissionsEffective()
	rows := []permRow{
		{
			ID: "screen", Title: i18n.T("perm.screen_title"), Desc: i18n.T("perm.screen_desc"),
			Badge: i18n.T("perm.badge_required"), Required: true, Granted: p.ScreenRecording,
		},
		{
			ID: "accessibility", Title: i18n.T("perm.ax_title"), Desc: i18n.T("perm.ax_desc"),
			Badge: i18n.T("perm.badge_required"), Required: true, Granted: p.Accessibility,
		},
		{
			ID: "microphone", Title: i18n.T("perm.mic_title"), Desc: i18n.T("perm.mic_desc"),
			Badge: i18n.T("perm.badge_optional"), Required: false, Optional: true, Granted: p.Microphone,
		},
		{
			ID: "camera", Title: i18n.T("perm.camera_title"), Desc: i18n.T("perm.camera_desc"),
			Badge: i18n.T("perm.badge_optional"), Required: false, Optional: true, Granted: p.Camera,
		},
		phoneCamPermRow(),
	}
	for i := range rows {
		if rows[i].ID == "phonecam" {
			continue // already finalized
		}
		needsRestart := rows[i].ID == "screen" && p.ScreenNeedsRestart
		switch {
		case rows[i].Granted:
			rows[i].Status = i18n.T("perm.status_ok")
			rows[i].Action = "ok"
		case needsRestart:
			rows[i].Status = i18n.T("perm.status_restart")
			rows[i].Action = "restart"
			rows[i].Desc = i18n.T("perm.restart_desc")
		case rows[i].Optional:
			rows[i].Status = ""
			rows[i].Action = "settings"
		default:
			rows[i].Status = i18n.T("perm.status_need")
			rows[i].Action = "settings"
		}
	}
	return rows
}

func collectPermRowsWindows() []permRow {
	// Windows has no TCC sheet like macOS; surface the optional phone-webcam driver
	// plus informational rows so the same “权限查看” entry point exists.
	rows := []permRow{
		{
			ID: "screen", Title: i18n.T("perm.win_screen_title"), Desc: i18n.T("perm.win_screen_desc"),
			Badge: i18n.T("perm.badge_required"), Required: true, Granted: true,
			Status: i18n.T("perm.status_ok"), Action: "ok",
		},
		{
			ID: "input", Title: i18n.T("perm.win_input_title"), Desc: i18n.T("perm.win_input_desc"),
			Badge: i18n.T("perm.badge_required"), Required: true, Granted: true,
			Status: i18n.T("perm.status_ok"), Action: "ok",
		},
		{
			ID: "microphone", Title: i18n.T("perm.mic_title"), Desc: i18n.T("perm.mic_desc"),
			Badge: i18n.T("perm.badge_optional"), Required: false, Optional: true, Granted: true,
			Status: i18n.T("perm.status_optional"), Action: "optional",
		},
		{
			ID: "camera", Title: i18n.T("perm.camera_title"), Desc: i18n.T("perm.camera_desc"),
			Badge: i18n.T("perm.badge_optional"), Required: false, Optional: true, Granted: true,
			Status: i18n.T("perm.status_optional"), Action: "optional",
		},
		phoneCamPermRow(),
	}
	return rows
}

func phoneCamPermRow() permRow {
	st := desktop.PhoneCamDriver()
	row := permRow{
		ID:       "phonecam",
		Title:    i18n.T("perm.phonecam_title"),
		Desc:     i18n.T("perm.phonecam_desc"),
		Badge:    i18n.T("perm.badge_optional"),
		Required: false,
		Optional: true,
		Granted:  st.Installed,
	}
	if st.Installed {
		row.Status = i18n.T("perm.phonecam_status_on")
		row.Action = "ok"
		if st.Name != "" {
			row.Desc = i18n.T("perm.phonecam_desc_on", st.Name)
		}
	} else {
		// Default off — optional install via 去设置.
		row.Status = i18n.T("perm.phonecam_status_off")
		row.Action = "settings"
	}
	return row
}

func permsRowsJSON() string {
	b, err := json.Marshal(collectPermRows())
	if err != nil {
		return "[]"
	}
	return string(b)
}

func trayMaybeShowPermissionsOnFirstRun() {
	if runtime.GOOS != "darwin" {
		return
	}
	p := desktop.CheckHostPermissions()
	if p.ScreenRecording && p.Accessibility {
		return
	}
	if permsUIAlreadyShown() {
		return
	}
	time.AfterFunc(800*time.Millisecond, trayShowPermissionsPanel)
}

func trayCheckPermissions() { trayShowPermissionsPanel() }

func openPhoneCamPermSettings() {
	_ = desktop.OpenPhoneCamDriverInstall()
}

//go:build darwin || linux

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

const permsUIMarker = "perms_ui_shown_v1"

type permRow struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Desc     string `json:"desc"`
	Required bool   `json:"required"`
	Granted  bool   `json:"granted"`
	Optional bool   `json:"optional"`
	Status   string `json:"status"`
	Action   string `json:"action"` // "ok" | "settings" | "optional"
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
	if runtime.GOOS != "darwin" {
		return []permRow{{
			ID: "linux", Title: i18n.T("perm.linux_title"), Desc: i18n.T("perm.linux_desc"),
			Required: false, Optional: true, Granted: true,
			Status: i18n.T("perm.status_optional"), Action: "optional",
		}}
	}
	p := desktop.CheckHostPermissions()
	rows := []permRow{
		{
			ID: "screen", Title: i18n.T("perm.screen_title"), Desc: i18n.T("perm.screen_desc"),
			Required: true, Granted: p.ScreenRecording,
		},
		{
			ID: "accessibility", Title: i18n.T("perm.ax_title"), Desc: i18n.T("perm.ax_desc"),
			Required: true, Granted: p.Accessibility,
		},
		{
			ID: "microphone", Title: i18n.T("perm.mic_title"), Desc: i18n.T("perm.mic_desc"),
			Required: false, Optional: true,
		},
		{
			ID: "camera", Title: i18n.T("perm.camera_title"), Desc: i18n.T("perm.camera_desc"),
			Required: false, Optional: true,
		},
	}
	for i := range rows {
		switch {
		case rows[i].Granted:
			rows[i].Status = i18n.T("perm.status_ok")
			rows[i].Action = "ok"
		case rows[i].Optional:
			rows[i].Status = i18n.T("perm.status_optional")
			rows[i].Action = "settings"
		default:
			rows[i].Status = i18n.T("perm.status_need")
			rows[i].Action = "settings"
		}
	}
	return rows
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
	if permsUIAlreadyShown() {
		p := desktop.CheckHostPermissions()
		if p.ScreenRecording && p.Accessibility {
			return
		}
	}
	time.AfterFunc(800*time.Millisecond, trayShowPermissionsPanel)
}

func trayCheckPermissions() { trayShowPermissionsPanel() }

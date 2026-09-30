//go:build darwin || linux

package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/identity"
)

const permsUIMarker = "perms_ui_shown_v1"

var (
	permsUIMu   sync.Mutex
	permsUIURL  string
	permsUIOnce sync.Once
)

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

func ensurePermsUIServer() (string, error) {
	var startErr error
	permsUIOnce.Do(func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			startErr = err
			return
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(permsUIHTML()))
		})
		mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rows": collectPermRows(),
			})
		})
		mux.HandleFunc("/api/open", func(w http.ResponseWriter, r *http.Request) {
			id := r.URL.Query().Get("id")
			switch id {
			case "screen":
				_ = desktop.RequestScreenRecording()
				_ = desktop.OpenPrivacySettings("screen")
			case "accessibility":
				_ = desktop.RequestAccessibility()
				_ = desktop.OpenPrivacySettings("accessibility")
			case "microphone", "camera":
				_ = desktop.OpenPrivacySettings(id)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		})
		go func() { _ = http.Serve(ln, mux) }()
		permsUIMu.Lock()
		permsUIURL = fmt.Sprintf("http://%s/", ln.Addr().String())
		permsUIMu.Unlock()
	})
	if startErr != nil {
		return "", startErr
	}
	permsUIMu.Lock()
	defer permsUIMu.Unlock()
	if permsUIURL == "" {
		return "", fmt.Errorf("permissions UI failed to start")
	}
	return permsUIURL, nil
}

func trayShowPermissionsPanel() {
	url, err := ensurePermsUIServer()
	if err != nil {
		trayAlert(i18n.T("desktop.confirm_title"), err.Error())
		return
	}
	markPermsUIShown()
	_ = trayOpenPath(url)
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

func permsUIHTML() string {
	title := html.EscapeString(i18n.T("perm.window_title"))
	subtitle := html.EscapeString(i18n.T("perm.window_sub"))
	btnDone := html.EscapeString(i18n.T("perm.btn_done"))
	hint := html.EscapeString(i18n.T("perm.auto_refresh"))
	btnSettingsJSON, _ := json.Marshal(i18n.T("perm.btn_settings"))
	return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>` + title + `</title>
<style>
  :root {
    --bg: #f4f7f7;
    --card: #ffffff;
    --ink: #163033;
    --muted: #5a6e70;
    --line: #d7e3e3;
    --ok: #1a9a6c;
    --need: #c45c26;
    --accent: #107882;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", "PingFang SC", sans-serif;
    background: radial-gradient(1200px 500px at 10% -10%, #d9f3f1 0%, var(--bg) 55%);
    color: var(--ink); min-height: 100vh;
  }
  main { max-width: 640px; margin: 0 auto; padding: 36px 20px 48px; }
  h1 { font-size: 24px; font-weight: 700; margin: 0 0 8px; letter-spacing: -0.02em; }
  .sub { color: var(--muted); font-size: 14px; line-height: 1.5; margin-bottom: 22px; }
  .hint { font-size: 12px; color: var(--muted); margin: 14px 0 0; }
  .row {
    display: grid; grid-template-columns: 1fr auto; gap: 12px 16px; align-items: center;
    background: var(--card); border: 1px solid var(--line); border-radius: 14px;
    padding: 16px 18px; margin-bottom: 12px;
    box-shadow: 0 8px 24px rgba(16, 48, 51, 0.04);
  }
  .title { font-size: 15px; font-weight: 600; margin: 0 0 4px; }
  .desc { font-size: 13px; color: var(--muted); line-height: 1.45; margin: 0; }
  .right { display: flex; flex-direction: column; align-items: flex-end; gap: 8px; min-width: 120px; }
  .status { font-size: 13px; font-weight: 600; display: inline-flex; align-items: center; gap: 6px; }
  .status.ok { color: var(--ok); }
  .status.need { color: var(--need); }
  .status.optional { color: var(--muted); }
  .check {
    width: 18px; height: 18px; border-radius: 50%; background: var(--ok); color: #fff;
    display: inline-flex; align-items: center; justify-content: center; font-size: 12px;
  }
  button.settings {
    appearance: none; border: 0; cursor: pointer;
    background: linear-gradient(135deg, var(--accent), #0e9a90);
    color: #fff; font-size: 13px; font-weight: 600;
    padding: 8px 14px; border-radius: 999px;
  }
  button.settings:hover { filter: brightness(1.05); }
  .footer { display: flex; justify-content: flex-end; margin-top: 18px; }
  button.done {
    appearance: none; border: 1px solid var(--line); background: #fff; color: var(--ink);
    font-size: 13px; font-weight: 600; padding: 10px 16px; border-radius: 10px; cursor: pointer;
  }
</style>
</head>
<body>
<main>
  <h1>` + title + `</h1>
  <p class="sub">` + subtitle + `</p>
  <div id="list"></div>
  <p class="hint">` + hint + `</p>
  <div class="footer"><button class="done" onclick="window.close()">` + btnDone + `</button></div>
</main>
<script>
const btnSettingsLabel = ` + string(btnSettingsJSON) + `;
async function load() {
  const res = await fetch('/api/status');
  const data = await res.json();
  const list = document.getElementById('list');
  list.innerHTML = '';
  for (const row of data.rows) {
    const el = document.createElement('div');
    el.className = 'row';
    let statusClass = 'optional';
    let statusHTML = escapeHtml(row.status);
    if (row.action === 'ok') {
      statusClass = 'ok';
      statusHTML = '<span class="check">✓</span>' + escapeHtml(row.status);
    } else if (row.action === 'settings' && row.required) {
      statusClass = 'need';
    }
    let action = '';
    if (row.action === 'settings') {
      action = '<button class="settings" data-id="' + escapeAttr(row.id) + '">' + escapeHtml(btnSettingsLabel) + '</button>';
    }
    el.innerHTML = '<div><p class="title"></p><p class="desc"></p></div><div class="right"><div class="status ' + statusClass + '">' + statusHTML + '</div>' + action + '</div>';
    el.querySelector('.title').textContent = row.title;
    el.querySelector('.desc').textContent = row.desc;
    const btn = el.querySelector('button.settings');
    if (btn) btn.addEventListener('click', () => openSettings(btn.getAttribute('data-id')));
    list.appendChild(el);
  }
}
function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
}
function escapeAttr(s) { return escapeHtml(s); }
async function openSettings(id) {
  await fetch('/api/open?id=' + encodeURIComponent(id));
  setTimeout(load, 400);
}
load();
setInterval(load, 1500);
</script>
</body>
</html>`
}

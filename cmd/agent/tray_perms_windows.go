//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/foqerhk/runeverything/internal/i18n"
)

func trayShowPermissionsPanel() {
	markPermsUIShown()
	rows := collectPermRows()

	dir, err := os.MkdirTemp("", "re-perms-*")
	if err != nil {
		trayAlert(i18n.T("perm.window_title"), fallbackPermsText(rows))
		return
	}
	defer os.RemoveAll(dir)

	jsonPath := filepath.Join(dir, "rows.json")
	actionPath := filepath.Join(dir, "action.txt")
	psPath := filepath.Join(dir, "panel.ps1")

	if err := writePermsJSON(jsonPath, rows); err != nil {
		trayAlert(i18n.T("perm.window_title"), fallbackPermsText(rows))
		return
	}
	if err := os.WriteFile(psPath, []byte(windowsPermsPanelPS1()), 0o600); err != nil {
		trayAlert(i18n.T("perm.window_title"), fallbackPermsText(rows))
		return
	}

	title := i18n.T("perm.window_title")
	for {
		cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", psPath,
			"-JsonPath", jsonPath,
			"-ActionPath", actionPath,
			"-Title", title,
			"-Subtitle", i18n.T("perm.window_sub"),
			"-BtnSettings", i18n.T("perm.btn_settings"),
			"-BtnDone", i18n.T("perm.btn_done"),
			"-Hint", i18n.T("perm.auto_refresh"),
		)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		err := cmd.Run()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				trayAlert(title, fallbackPermsText(rows))
				return
			}
		}
		if code != 2 {
			return
		}
		b, readErr := os.ReadFile(actionPath)
		_ = os.Remove(actionPath)
		if readErr != nil {
			return
		}
		switch strings.TrimSpace(string(b)) {
		case "phonecam":
			openPhoneCamPermSettings()
		}
		rows = collectPermRows()
		if err := writePermsJSON(jsonPath, rows); err != nil {
			return
		}
	}
}

func writePermsJSON(path string, rows []permRow) error {
	b, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func fallbackPermsText(rows []permRow) string {
	var b strings.Builder
	b.WriteString(i18n.T("perm.window_sub"))
	b.WriteString("\n\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "• %s — %s\n  %s\n\n", r.Title, r.Status, r.Desc)
	}
	return b.String()
}

func windowsPermsPanelPS1() string {
	return `
param(
  [string]$JsonPath,
  [string]$ActionPath,
  [string]$Title,
  [string]$Subtitle,
  [string]$BtnSettings,
  [string]$BtnDone,
  [string]$Hint
)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()

$script:ActionPath = $ActionPath
$rows = Get-Content -Raw -Encoding UTF8 $JsonPath | ConvertFrom-Json
$form = New-Object System.Windows.Forms.Form
$script:PermsForm = $form
$form.Text = $Title
$form.Size = New-Object System.Drawing.Size(620, 540)
$form.StartPosition = 'CenterScreen'
$form.MinimizeBox = $false
$form.MaximizeBox = $false
$form.FormBorderStyle = 'FixedDialog'
$form.TopMost = $true

$sub = New-Object System.Windows.Forms.Label
$sub.Text = $Subtitle
$sub.AutoSize = $false
$sub.Location = New-Object System.Drawing.Point(16, 12)
$sub.Size = New-Object System.Drawing.Size(572, 52)
$sub.ForeColor = [System.Drawing.Color]::FromArgb(90,90,90)
$form.Controls.Add($sub)

$panel = New-Object System.Windows.Forms.Panel
$panel.Location = New-Object System.Drawing.Point(16, 72)
$panel.Size = New-Object System.Drawing.Size(572, 350)
$panel.AutoScroll = $true
$form.Controls.Add($panel)

$y = 0
foreach ($r in $rows) {
  $card = New-Object System.Windows.Forms.Panel
  $card.Location = New-Object System.Drawing.Point(0, $y)
  $card.Size = New-Object System.Drawing.Size(548, 86)
  $card.BackColor = [System.Drawing.Color]::FromArgb(245,245,247)

  $t = New-Object System.Windows.Forms.Label
  $t.Text = [string]$r.title
  $t.Font = New-Object System.Drawing.Font('Microsoft YaHei UI', 10, [System.Drawing.FontStyle]::Bold)
  $t.Location = New-Object System.Drawing.Point(12, 8)
  $t.AutoSize = $true
  $card.Controls.Add($t)

  $badge = New-Object System.Windows.Forms.Label
  $badge.Text = [string]$r.badge
  $badge.Font = New-Object System.Drawing.Font('Microsoft YaHei UI', 8.5, [System.Drawing.FontStyle]::Bold)
  $badge.Location = New-Object System.Drawing.Point(12, 32)
  $badge.AutoSize = $true
  if ($r.required) { $badge.ForeColor = [System.Drawing.Color]::FromArgb(210,80,20) }
  else { $badge.ForeColor = [System.Drawing.Color]::FromArgb(70,120,190) }
  $card.Controls.Add($badge)

  $d = New-Object System.Windows.Forms.Label
  $d.Text = [string]$r.desc
  $d.Font = New-Object System.Drawing.Font('Microsoft YaHei UI', 8.5)
  $d.ForeColor = [System.Drawing.Color]::FromArgb(100,100,100)
  $d.Location = New-Object System.Drawing.Point(70, 32)
  $d.Size = New-Object System.Drawing.Size(340, 46)
  $card.Controls.Add($d)

  $st = New-Object System.Windows.Forms.Label
  $st.Text = [string]$r.status
  $st.Font = New-Object System.Drawing.Font('Microsoft YaHei UI', 8.5, [System.Drawing.FontStyle]::Bold)
  $st.AutoSize = $true
  $st.Location = New-Object System.Drawing.Point(420, 10)
  if ([string]$r.action -eq 'ok') {
    $st.Text = ([char]0x2713).ToString() + ' ' + [string]$r.status
    $st.ForeColor = [System.Drawing.Color]::FromArgb(25,140,100)
  } else {
    $st.ForeColor = [System.Drawing.Color]::FromArgb(120,120,120)
  }
  $card.Controls.Add($st)

  if ([string]$r.action -eq 'settings') {
    $btn = New-Object System.Windows.Forms.Button
    $btn.Text = $BtnSettings
    $btn.Size = New-Object System.Drawing.Size(100, 28)
    $btn.Location = New-Object System.Drawing.Point(430, 40)
    $rowId = [string]$r.id
    $btn.Tag = $rowId
    $btn.Add_Click({
      $id = [string]$this.Tag
      Set-Content -Path $script:ActionPath -Value $id -Encoding ASCII
      $script:PermsForm.DialogResult = [System.Windows.Forms.DialogResult]::Retry
      $script:PermsForm.Close()
    })
    $card.Controls.Add($btn)
  }

  $panel.Controls.Add($card)
  $y += 94
}

$hintLabel = New-Object System.Windows.Forms.Label
$hintLabel.Text = $Hint
$hintLabel.ForeColor = [System.Drawing.Color]::FromArgb(140,140,140)
$hintLabel.Location = New-Object System.Drawing.Point(16, 432)
$hintLabel.Size = New-Object System.Drawing.Size(450, 40)
$form.Controls.Add($hintLabel)

$done = New-Object System.Windows.Forms.Button
$done.Text = $BtnDone
$done.Size = New-Object System.Drawing.Size(90, 30)
$done.Location = New-Object System.Drawing.Point(498, 436)
$done.DialogResult = [System.Windows.Forms.DialogResult]::OK
$form.Controls.Add($done)
$form.AcceptButton = $done

[void]$form.ShowDialog()
if ($form.DialogResult -eq [System.Windows.Forms.DialogResult]::Retry) { exit 2 }
exit 0
`
}

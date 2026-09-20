//go:build windows

package winutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const TaskName = "RunEverythingAgent"

// InstallDir is %LOCALAPPDATA%\RunEverything\bin
func InstallDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}
	return filepath.Join(base, "RunEverything", "bin")
}

func AgentPath() string {
	return filepath.Join(InstallDir(), "runeverything.exe")
}

func StartMenuShortcutPath() string {
	programs := os.Getenv("APPDATA")
	if programs == "" {
		programs = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
	}
	return filepath.Join(programs, "Microsoft", "Windows", "Start Menu", "Programs", "RunEverything.lnk")
}

// EnsureInstallDir creates the install directory.
func EnsureInstallDir() error {
	return os.MkdirAll(InstallDir(), 0o755)
}

// AddUserPath prepends dir to the user PATH if missing.
func AddUserPath(dir string) error {
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf(`$d='%s'; $p=[Environment]::GetEnvironmentVariable('Path','User'); if (-not $p) { $p='' }; if ($p -notlike ('*'+$d+'*')) { [Environment]::SetEnvironmentVariable('Path', ($d+';'+$p), 'User') }`,
			escapePS(dir)))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

// CreateStartMenuShortcut points to the agent tray command.
func CreateStartMenuShortcut(exePath string) error {
	lnk := StartMenuShortcutPath()
	_ = os.MkdirAll(filepath.Dir(lnk), 0o755)
	ps := fmt.Sprintf(
		`$s=(New-Object -ComObject WScript.Shell).CreateShortcut('%s'); $s.TargetPath='%s'; $s.Arguments='tray'; $s.WorkingDirectory='%s'; $s.Description='RunEverything Agent'; $s.Save()`,
		escapePS(lnk), escapePS(exePath), escapePS(filepath.Dir(exePath)),
	)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

// RegisterLogonTask starts the agent tray at user logon.
func RegisterLogonTask(exePath string) error {
	tr := fmt.Sprintf(`"%s" tray`, exePath)
	cmd := exec.Command("schtasks", "/Create",
		"/TN", TaskName,
		"/TR", tr,
		"/SC", "ONLOGON",
		"/RL", "LIMITED",
		"/F",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func UnregisterLogonTask() error {
	cmd := exec.Command("schtasks", "/Delete", "/TN", TaskName, "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()
	return nil
}

func LogonTaskExists() bool {
	cmd := exec.Command("schtasks", "/Query", "/TN", TaskName)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run() == nil
}

func OpenFile(path string) error {
	cmd := exec.Command("cmd", "/C", "start", "", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

func OpenFolder(path string) error {
	return OpenFile(path)
}

func SetClipboardText(text string) error {
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf("Set-Clipboard -Value '%s'", escapePS(text)))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

func NotifyBalloon(title, body string) {
	ps := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
$n=New-Object System.Windows.Forms.NotifyIcon
$n.Icon=[System.Drawing.SystemIcons]::Information
$n.Visible=$true
$n.ShowBalloonTip(5000,'%s','%s',[System.Windows.Forms.ToolTipIcon]::Info)
Start-Sleep -Seconds 6
$n.Dispose()
`, escapePS(title), escapePS(body))
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
}

func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// FreeConsole detaches from a parent console (for tray mode).
func FreeConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("FreeConsole")
	_, _, _ = proc.Call()
}

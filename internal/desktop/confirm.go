package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/foqerhk/runeverything/internal/i18n"
)

// ConfirmLocal shows a native yes/no dialog when possible; falls back to stderr prompt.
func ConfirmLocal(prompt string, wait time.Duration) bool {
	if wait <= 0 {
		wait = 60 * time.Second
	}
	switch runtime.GOOS {
	case "darwin":
		if ok, err := confirmDarwin(prompt, wait); err == nil {
			return ok
		}
	case "windows":
		if ok, err := confirmWindows(prompt, wait); err == nil {
			return ok
		}
	case "linux":
		if ok, err := confirmLinux(prompt, wait); err == nil {
			return ok
		}
	}
	return confirmStdin(prompt, wait)
}

func confirmStdin(prompt string, wait time.Duration) bool {
	fmt.Fprint(os.Stderr, prompt)
	fmt.Fprint(os.Stderr, i18n.T("desktop.stdin_hint"))
	ch := make(chan string, 1)
	go func() {
		var line string
		_, _ = fmt.Scanln(&line)
		ch <- line
	}()
	select {
	case line := <-ch:
		return line == "y" || line == "Y" || line == "yes" || line == "是"
	case <-time.After(wait):
		fmt.Fprintln(os.Stderr, i18n.T("desktop.timeout_denied"))
		return false
	}
}

func confirmDarwin(prompt string, wait time.Duration) (bool, error) {
	deny := i18n.T("desktop.btn_deny")
	allow := i18n.T("desktop.btn_allow")
	title := i18n.T("desktop.confirm_title")
	script := fmt.Sprintf(`display dialog %q buttons {%q,%q} default button %q cancel button %q with title %q giving up after %d`,
		prompt, deny, allow, deny, deny, title, int(wait.Seconds()))
	cmd := exec.Command("osascript", "-e", script)
	err := cmd.Run()
	return err == nil, nil
}

func confirmLinux(prompt string, wait time.Duration) (bool, error) {
	title := i18n.T("desktop.confirm_title")
	if _, err := exec.LookPath("zenity"); err == nil {
		cmd := exec.Command("zenity", "--question", "--title="+title, "--text="+prompt, fmt.Sprintf("--timeout=%d", int(wait.Seconds())))
		err := cmd.Run()
		return err == nil, nil
	}
	if _, err := exec.LookPath("kdialog"); err == nil {
		cmd := exec.Command("kdialog", "--yesno", prompt, "--title", title)
		err := cmd.Run()
		return err == nil, nil
	}
	return false, fmt.Errorf("no dialog tool")
}

func confirmWindows(prompt string, wait time.Duration) (bool, error) {
	title := i18n.T("desktop.confirm_title")
	ps := fmt.Sprintf(`Add-Type -AssemblyName PresentationFramework; $r=[System.Windows.MessageBox]::Show('%s','%s','YesNo','Question'); if($r -eq 'Yes'){exit 0}else{exit 1}`,
		strings.ReplaceAll(prompt, "'", "''"),
		strings.ReplaceAll(title, "'", "''"))
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		return err == nil, nil
	case <-time.After(wait):
		_ = cmd.Process.Kill()
		return false, fmt.Errorf("timeout")
	}
}

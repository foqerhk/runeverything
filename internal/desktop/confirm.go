package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
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
	ch := make(chan string, 1)
	go func() {
		var line string
		_, _ = fmt.Scanln(&line)
		ch <- line
	}()
	select {
	case line := <-ch:
		return line == "y" || line == "Y" || line == "yes"
	case <-time.After(wait):
		fmt.Fprintln(os.Stderr, "(timeout — denied)")
		return false
	}
}

func confirmDarwin(prompt string, wait time.Duration) (bool, error) {
	script := fmt.Sprintf(`display dialog %q buttons {"Deny","Allow"} default button "Deny" cancel button "Deny" with title "RunEverything" giving up after %d`,
		prompt, int(wait.Seconds()))
	cmd := exec.Command("osascript", "-e", script)
	err := cmd.Run()
	return err == nil, nil
}

func confirmLinux(prompt string, wait time.Duration) (bool, error) {
	if _, err := exec.LookPath("zenity"); err == nil {
		cmd := exec.Command("zenity", "--question", "--title=RunEverything", "--text="+prompt, fmt.Sprintf("--timeout=%d", int(wait.Seconds())))
		err := cmd.Run()
		return err == nil, nil
	}
	if _, err := exec.LookPath("kdialog"); err == nil {
		cmd := exec.Command("kdialog", "--yesno", prompt)
		err := cmd.Run()
		return err == nil, nil
	}
	return false, fmt.Errorf("no dialog tool")
}

func confirmWindows(prompt string, wait time.Duration) (bool, error) {
	ps := fmt.Sprintf(`Add-Type -AssemblyName PresentationFramework; $r=[System.Windows.MessageBox]::Show('%s','RunEverything','YesNo','Question'); if($r -eq 'Yes'){exit 0}else{exit 1}`,
		strings.ReplaceAll(prompt, "'", "''"))
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		return err == nil, nil
	case <-time.After(wait):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return false, nil
	}
}

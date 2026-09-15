package ptyx

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"

	"github.com/creack/pty"
)

// Session wraps a local PTY process.
type Session struct {
	ID   string
	Cmd  *exec.Cmd
	File *os.File

	mu     sync.Mutex
	closed bool
}

func DefaultShell() []string {
	if runtime.GOOS == "windows" {
		return []string{"powershell.exe"}
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	return []string{shell}
}

func whichTmux() string {
	path, err := exec.LookPath("tmux")
	if err != nil {
		return ""
	}
	return path
}

// Start launches a PTY session.
func Start(id string, cwd string, cmdArgs []string, cols, rows int, useTmux bool, tmuxName string) (*Session, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	var cmd *exec.Cmd
	if useTmux {
		tmux := whichTmux()
		if tmux == "" {
			return nil, fmt.Errorf("tmux not found")
		}
		name := tmuxName
		if name == "" {
			name = "re-" + id
		}
		// Attach or create named session.
		cmd = exec.Command(tmux, "new-session", "-A", "-s", name)
	} else {
		args := cmdArgs
		if len(args) == 0 {
			args = DefaultShell()
		}
		cmd = exec.Command(args[0], args[1:]...)
	}
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Env = os.Environ()

	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	return &Session{ID: id, Cmd: cmd, File: f}, nil
}

func (s *Session) Resize(cols, rows int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.File == nil {
		return io.ErrClosedPipe
	}
	return pty.Setsize(s.File, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (s *Session) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.File == nil {
		return 0, io.ErrClosedPipe
	}
	return s.File.Write(p)
}

func (s *Session) Read(p []byte) (int, error) {
	return s.File.Read(p)
}

func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.File != nil {
		_ = s.File.Close()
	}
	if s.Cmd != nil && s.Cmd.Process != nil {
		_ = s.Cmd.Process.Kill()
		_, _ = s.Cmd.Process.Wait()
	}
	return nil
}

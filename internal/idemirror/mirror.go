// Package idemirror mirrors a GUI IDE agent chat (Cursor IDE composer) into a terminal:
// conversation output is streamed from the IDE's local store, and each typed line is
// delivered into the IDE window through the KoKo IDE Bridge extension plus a paste.
//
// KoKo also exchanges structured data with the mirror in-band: the mirror writes chat
// state as OSC 7788 (base64 JSON) and reads phone actions as OSC 7789 on stdin.
package idemirror

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// desk is the host-side UI automation used to drive the IDE window.
type desk interface {
	Trusted() bool
	Raise(pid int, titleHint string) error
	Frontmost() int
	PasteAndSubmit(text string)
	Paste(text string)
	PressButton(pid int, title string) bool
}

// Options configures one mirror run.
type Options struct {
	ComposerID string
	// NewInFolder creates a new Cursor IDE chat in this project folder instead of opening ComposerID.
	NewInFolder string
	StateDB     string
	Zh          bool
	History     int // entries replayed on start
}

const (
	oscStateTag  = "\x1b]7788;"
	oscActionTag = "\x1b]7789;"
)

type job struct {
	line   string
	action *action
}

type mirror struct {
	opt   Options
	home  string
	store *cursorStore
	meta  composerMeta
	desk  desk
	out   io.Writer

	modes, models []option

	mu        sync.Mutex
	input     []rune
	status    string
	rend      renderer
	lastIdx   int
	lastState string
}

// Run blocks until the user exits (Ctrl-D) or stdin closes.
func Run(opt Options) error {
	if opt.History <= 0 {
		opt.History = 40
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	m := &mirror{opt: opt, home: home, desk: newDesk(), out: os.Stdout, lastIdx: -1}

	restore := rawMode()
	defer restore()

	if opt.NewInFolder != "" {
		id, err := m.createComposer(opt.NewInFolder)
		if err != nil {
			m.emit(ansiErr + m.t("新建 Cursor 会话失败：", "Could not create a Cursor chat: ") + err.Error() + ansiReset + "\r\n")
			return err
		}
		m.opt.ComposerID = id
	}
	m.store, err = newCursorStore(opt.StateDB, m.opt.ComposerID)
	if err != nil {
		return err
	}
	m.meta, err = m.store.meta()
	if err != nil {
		m.emit(ansiErr + m.t("找不到这个 Cursor 会话：", "Cursor conversation not found: ") + err.Error() + ansiReset + "\r\n")
		return err
	}
	m.modes, m.models, _ = m.store.options()

	m.banner()
	m.rend.next = max(m.meta.Count-opt.History, 0)
	m.poll()
	m.publishState()
	go m.ensureBridge()

	jobs := make(chan job, 16)
	go m.worker(jobs)
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(800 * time.Millisecond)
		defer t.Stop()
		for tick := 1; ; tick++ {
			select {
			case <-done:
				return
			case <-t.C:
				m.poll()
				if tick%3 == 0 {
					m.refreshMeta()
				}
			}
		}
	}()
	defer close(done)
	return m.readInput(os.Stdin, jobs)
}

func (m *mirror) t(zh, en string) string {
	if m.opt.Zh {
		return zh
	}
	return en
}

func (m *mirror) banner() {
	name := m.meta.Name
	if name == "" {
		name = m.t("新会话", "New chat")
	}
	var sb strings.Builder
	// Each (re)attach replays recent history, so start from a clean screen and scrollback.
	sb.WriteString("\x1b[H\x1b[2J\x1b[3J")
	sb.WriteString("\x1b[1mCursor IDE · " + name + ansiReset + "\r\n")
	if m.meta.Cwd != "" {
		sb.WriteString(ansiDim + m.meta.Cwd + ansiReset + "\r\n")
	}
	sb.WriteString(ansiDim + m.t(
		"输入消息后回车，会发送到电脑上的 Cursor；Ctrl-C 停止生成，Ctrl-D 退出。",
		"Type a message and press Return to send it to Cursor on the computer. Ctrl-C stops generation, Ctrl-D exits.",
	) + ansiReset + "\r\n")
	m.emit(sb.String())
}

// emit prints output above the input line and redraws the prompt.
func (m *mirror) emit(s string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emitLocked(s)
}

func (m *mirror) emitLocked(s string) {
	_, _ = io.WriteString(m.out, "\r\x1b[2K"+s+m.promptLocked())
}

func (m *mirror) promptLocked() string {
	p := ansiOK + "› " + ansiReset + string(m.input)
	if m.status != "" {
		p += "  " + ansiDim + m.status + ansiReset
	}
	return p
}

func (m *mirror) setStatus(s string) {
	m.mu.Lock()
	m.status = s
	m.emitLocked("")
	m.mu.Unlock()
}

func (m *mirror) note(s string) { m.emit(ansiDim + "  " + s + ansiReset + "\r\n") }
func (m *mirror) fail(err error) {
	m.emit(ansiErr + "  ✗ " + err.Error() + ansiReset + "\r\n")
}

func (m *mirror) poll() {
	m.mu.Lock()
	from := m.rend.next
	m.mu.Unlock()
	rows, err := m.store.bubbles(from)
	if err != nil || len(rows) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastIdx = rows[len(rows)-1].Idx
	if s := m.rend.feed(rows, time.Now()); s != "" {
		m.emitLocked(s)
	}
}

func (m *mirror) refreshMeta() {
	meta, err := m.store.meta()
	if err != nil {
		return
	}
	m.mu.Lock()
	m.meta = meta
	m.mu.Unlock()
	m.publishState()
}

func (m *mirror) readInput(r io.Reader, jobs chan<- job) error {
	buf := make([]byte, 4096)
	var pending []byte
	pasting := false
	for {
		n, err := r.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
		scan:
			for len(pending) > 0 {
				c := pending[0]
				switch {
				case c == 0x1b:
					seq, ok := escapeSeq(pending)
					if !ok {
						break scan
					}
					pending = pending[len(seq):]
					switch {
					case seq == "\x1b[200~":
						pasting = true
					case seq == "\x1b[201~":
						pasting = false
					case strings.HasPrefix(seq, oscActionTag):
						if a, err := decodeAction(seq); err == nil {
							jobs <- job{action: &a}
						}
					}
					continue
				case c == '\r' || c == '\n':
					pending = pending[1:]
					if pasting {
						m.edit(func(in []rune) []rune { return append(in, '\n') })
						continue
					}
					m.mu.Lock()
					line := strings.TrimSpace(string(m.input))
					m.input = m.input[:0]
					m.emitLocked("")
					m.mu.Unlock()
					if line != "" {
						jobs <- job{line: line}
					}
					continue
				case c == 0x7f || c == 0x08:
					pending = pending[1:]
					m.edit(func(in []rune) []rune {
						if len(in) > 0 {
							return in[:len(in)-1]
						}
						return in
					})
					continue
				case c == 0x15: // Ctrl-U
					pending = pending[1:]
					m.edit(func([]rune) []rune { return nil })
					continue
				case c == 0x03: // Ctrl-C
					pending = pending[1:]
					m.mu.Lock()
					hadInput := len(m.input) > 0
					m.input = m.input[:0]
					m.emitLocked("")
					m.mu.Unlock()
					if !hadInput {
						jobs <- job{action: &action{Op: "cancel"}}
					}
					continue
				case c == 0x04: // Ctrl-D
					m.emit(m.t("已退出 Cursor IDE 会话镜像。", "Left the Cursor IDE chat mirror.") + "\r\n")
					return nil
				case c < 0x20:
					pending = pending[1:]
					continue
				}
				if !utf8.FullRune(pending) {
					break scan
				}
				rn, size := utf8.DecodeRune(pending)
				pending = pending[size:]
				m.edit(func(in []rune) []rune { return append(in, rn) })
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// escapeSeq returns the complete escape sequence (CSI, SS3 or OSC) at the start of b.
func escapeSeq(b []byte) (string, bool) {
	if len(b) < 2 {
		return "", false
	}
	switch b[1] {
	case '[', 'O':
		for i := 2; i < len(b); i++ {
			if b[i] >= 0x40 && b[i] <= 0x7e {
				return string(b[:i+1]), true
			}
		}
		return "", false
	case ']':
		for i := 2; i < len(b); i++ {
			if b[i] == 0x07 {
				return string(b[:i+1]), true
			}
			if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
				return string(b[:i+2]), true
			}
		}
		return "", false
	}
	return string(b[:2]), true
}

func (m *mirror) edit(f func([]rune) []rune) {
	m.mu.Lock()
	m.input = f(m.input)
	m.emitLocked("")
	m.mu.Unlock()
}

// action is one structured request from KoKo (mode/model switch, review decisions).
type action struct {
	Op   string `json:"op"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

func decodeAction(seq string) (action, error) {
	body := strings.TrimPrefix(seq, oscActionTag)
	body = strings.TrimSuffix(strings.TrimSuffix(body, "\x07"), "\x1b\\")
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return action{}, err
	}
	var a action
	if err := json.Unmarshal(raw, &a); err != nil {
		return action{}, err
	}
	if a.Op == "" {
		return action{}, errors.New("empty op")
	}
	return a, nil
}

// rawMode disables line buffering and echo; the mirror draws its own input line.
func rawMode() func() {
	set := func(args ...string) {
		cmd := exec.Command("stty", args...)
		cmd.Stdin = os.Stdin
		_ = cmd.Run()
	}
	set("raw", "-echo")
	return func() { set("sane") }
}

package idemirror

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// worker runs typed lines and KoKo actions one at a time; each may drive the IDE window.
func (m *mirror) worker(jobs <-chan job) {
	for j := range jobs {
		var err error
		switch {
		case j.action != nil:
			err = m.runAction(*j.action)
		default:
			m.setStatus(m.t("发送中…", "sending…"))
			err = m.deliver(j.line)
		}
		m.setStatus("")
		if err != nil {
			m.fail(err)
		}
		m.refreshMeta()
	}
}

func (m *mirror) runAction(a action) error {
	switch a.Op {
	case "cancel":
		return m.cancel()
	case "mode":
		return m.setMode(a.ID)
	case "model":
		return m.setModel(a.ID)
	case "keepAll":
		return m.review(true)
	case "undoAll":
		return m.review(false)
	case "refresh":
		m.mu.Lock()
		m.lastState = ""
		m.mu.Unlock()
		m.modes, m.models, _ = m.store.options()
		return nil
	case "send":
		text := strings.TrimSpace(a.Text)
		if text == "" {
			return nil
		}
		m.setStatus(m.t("发送中…", "sending…"))
		return m.deliver(text)
	case "answer":
		return m.answer(a.ID, a.Answers)
	case "resync":
		m.resyncChat()
		return nil
	}
	return fmt.Errorf("unknown action %q", a.Op)
}

func (m *mirror) liveBridge() (bridgeReg, error) {
	r, ok := pickBridge(bridges(m.home), m.meta.Cwd)
	if !ok {
		return bridgeReg{}, errors.New(m.t(
			"电脑上的 Cursor 没在运行，或 KoKo 桥接插件还没加载（在 Cursor 里执行 Reload Window）",
			"Cursor is not running on the computer, or the KoKo bridge extension is not loaded (run Reload Window in Cursor)",
		))
	}
	return r, nil
}

func (m *mirror) commandBridge() (bridgeReg, error) {
	r, err := m.liveBridge()
	if err != nil {
		return r, err
	}
	if !bridgeSupports(r) {
		return r, errors.New(m.t(
			"Cursor 里的 KoKo 桥接插件是旧版本，请在电脑上的 Cursor 执行一次 Reload Window",
			"The KoKo bridge in Cursor is outdated; run Reload Window in Cursor on the computer",
		))
	}
	return r, nil
}

// frontChat opens this chat in its Cursor window, brings that window to the front and
// puts the caret in the chat input.
func (m *mirror) frontChat(r bridgeReg) error {
	if !m.desk.Trusted() {
		return errors.New(m.t("Agent 没有辅助功能权限，无法操作 Cursor", "The Agent lacks Accessibility permission, so it cannot drive Cursor"))
	}
	focus := map[string]string{"composerId": m.opt.ComposerID}
	if _, err := callBridge(r, "focus", focus); err != nil {
		return fmt.Errorf("%s%v", m.t("Cursor 打开会话失败：", "Cursor could not open the chat: "), err)
	}
	hint := ""
	if len(r.Folders) > 0 {
		hint = filepath.Base(r.Folders[0])
	}
	if err := m.desk.Raise(r.PPID, hint); err != nil {
		return err
	}
	for i := 0; i < 60; i++ {
		time.Sleep(100 * time.Millisecond)
		rep, err := callBridge(r, "ping", nil)
		if err == nil && rep.Focused && (r.PPID == 0 || m.desk.Frontmost() == r.PPID) {
			if _, err := callBridge(r, "focus", focus); err != nil {
				return err
			}
			time.Sleep(250 * time.Millisecond)
			return nil
		}
	}
	return errors.New(m.t("无法把 Cursor 窗口切到前台（电脑可能锁屏或休眠）", "Could not bring the Cursor window to the front (the computer may be locked or asleep)"))
}

// deliver pastes the line into the chat input, presses Return, then waits until the
// message shows up in Cursor's store.
func (m *mirror) deliver(line string) error {
	r, err := m.liveBridge()
	if err != nil {
		return err
	}
	m.mu.Lock()
	baseline := m.lastIdx
	m.mu.Unlock()
	if err := m.frontChat(r); err != nil {
		return err
	}
	m.desk.PasteAndSubmit(line)

	want := strings.Join(strings.Fields(line), " ")
	deadline := time.Now().Add(confirmWait)
	for time.Now().Before(deadline) {
		rows, err := m.store.bubbles(baseline + 1)
		if err == nil {
			for _, b := range rows {
				if b.Type == 1 && strings.Contains(strings.Join(strings.Fields(b.Text), " "), want) {
					return nil
				}
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	return errors.New(m.t("已发送，但没确认到 Cursor 收到消息，请在电脑上检查", "Sent, but Cursor did not confirm the message; check the computer"))
}

// answer replies to a Cursor questionnaire. Cursor exposes no command to submit one, so
// the selections go in as a chat message: Cursor skips the pending questionnaire and the
// model reads the reply.
func (m *mirror) answer(toolCallID string, answers []questionAnswer) error {
	m.mu.Lock()
	q, ok := m.questions[toolCallID]
	m.mu.Unlock()
	if !ok {
		return errors.New(m.t("找不到这个提问，可能已经过期", "That question is no longer available"))
	}
	text := answerText(q, answers)
	if text == "" {
		return errors.New(m.t("还没有选择任何选项", "No option selected"))
	}
	m.setStatus(m.t("发送回答…", "sending answer…"))
	if err := m.deliver(text); err != nil {
		return err
	}
	m.mu.Lock()
	m.answered[toolCallID] = answers
	m.mu.Unlock()
	m.poll()
	return nil
}

// resyncChat resends the recent conversation from scratch for a phone that (re)attached
// and has no messages yet.
func (m *mirror) resyncChat() {
	meta, err := m.store.meta()
	if err != nil {
		return
	}
	rows, err := m.store.bubbles(max(meta.Count-m.opt.History, 0))
	if err != nil {
		return
	}
	m.mu.Lock()
	m.lastState = ""
	m.publishChatLocked(rows, true)
	m.chatReset = false
	m.mu.Unlock()
	m.publishState()
}

func (m *mirror) cancel() error {
	r, err := m.liveBridge()
	if err != nil {
		return err
	}
	if _, err := callBridge(r, "cancel", map[string]string{"composerId": m.opt.ComposerID}); err != nil {
		return err
	}
	m.note("⏹ " + m.t("已请求停止生成", "Asked Cursor to stop generating"))
	return nil
}

const (
	confirmWait  = 20 * time.Second
	confirmLagZh = "，Cursor 还没写入确认（电脑繁忙时会延迟，上方状态会自动更新）"
	confirmLagEn = "; Cursor has not saved it yet (it lags when the computer is busy; the bar above updates by itself)"
)

func findOption(opts []option, id string) (option, bool) {
	for _, o := range opts {
		if o.ID == id {
			return o, true
		}
	}
	return option{}, false
}

// waitMeta polls the chat until ok(meta) or the timeout.
func (m *mirror) waitMeta(timeout time.Duration, ok func(composerMeta) bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if meta, err := m.store.meta(); err == nil && ok(meta) {
			return true
		}
		time.Sleep(400 * time.Millisecond)
	}
	return false
}

func (m *mirror) setMode(id string) error {
	mode, ok := findOption(m.modes, id)
	if !ok {
		return fmt.Errorf("unknown mode %q", id)
	}
	isMode := func(meta composerMeta) bool { return meta.Mode == mode.ID }
	if meta, err := m.store.meta(); err == nil && isMode(meta) {
		return nil
	}
	r, err := m.commandBridge()
	if err != nil {
		return err
	}
	// Cursor only applies a mode to the chat with keyboard focus, and the same command on an
	// already-switched focused chat closes the chat panel, so it must be sent exactly once.
	if err := m.frontChat(r); err != nil {
		return err
	}
	cmd := map[string]any{"id": "composerMode." + mode.ID, "composerId": m.opt.ComposerID}
	if _, err := callBridge(r, "command", cmd); err != nil {
		return err
	}
	// Cursor persists chat data lazily; seconds of lag are normal under load.
	if !m.waitMeta(confirmWait, isMode) {
		m.note(m.t("已请求切换为 ", "Asked Cursor to switch to ") + mode.Name + m.t(confirmLagZh, confirmLagEn))
		return nil
	}
	m.note(m.t("模式已切换为 ", "Mode switched to ") + mode.Name)
	return nil
}

func (m *mirror) setModel(id string) error {
	model, ok := findOption(m.models, id)
	if !ok {
		return fmt.Errorf("unknown model %q", id)
	}
	isModel := func(meta composerMeta) bool { return meta.Model == model.ID }
	if meta, err := m.store.meta(); err == nil && isModel(meta) {
		return nil
	}
	r, err := m.commandBridge()
	if err != nil {
		return err
	}
	if err := m.frontChat(r); err != nil {
		return err
	}
	if _, err := callBridge(r, "command", map[string]any{"id": "composer.openModelToggle"}); err != nil {
		return err
	}
	time.Sleep(400 * time.Millisecond)
	m.desk.PasteAndSubmit(model.Name)
	if !m.waitMeta(confirmWait, isModel) {
		m.note(m.t("已请求切换为 ", "Asked Cursor to switch to ") + model.Name + m.t(confirmLagZh, confirmLagEn))
		return nil
	}
	m.note(m.t("模型已切换为 ", "Model switched to ") + model.Name)
	return nil
}

// review presses Cursor's Keep All / Undo All (Undo asks for a second confirming click).
func (m *mirror) review(keep bool) error {
	r, err := m.liveBridge()
	if err != nil {
		return err
	}
	if err := m.frontChat(r); err != nil {
		return err
	}
	labels := []string{"Undo All", "Undo"}
	if keep {
		labels = []string{"Keep All", "Keep"}
	}
	pressed := false
	for _, l := range labels {
		if m.desk.PressButton(r.PPID, l) {
			pressed = true
			break
		}
	}
	switch {
	case !pressed:
		return errors.New(m.t("Cursor 里没有待保留或撤销的改动", "No pending changes to keep or undo in Cursor"))
	case !keep:
		// "Undo All" asks for a confirming click; the per-chat review bar's "Undo" does not.
		time.Sleep(400 * time.Millisecond)
		m.desk.PressButton(r.PPID, "Confirm")
	}
	if keep {
		m.note(m.t("已在 Cursor 中保留全部改动", "Kept all changes in Cursor"))
	} else {
		m.note(m.t("已在 Cursor 中撤销全部改动", "Undid all changes in Cursor"))
	}
	return nil
}

// createComposer opens a new Cursor IDE chat in folder (opening the folder in Cursor
// first when no window has it) and returns its composer id.
func (m *mirror) createComposer(folder string) (string, error) {
	folder = filepath.Clean(folder)
	r, ok := bridgeForFolder(bridges(m.home), folder)
	if !ok {
		cli := cursorCLI()
		if cli == "" {
			return "", errors.New(m.t("电脑上找不到 Cursor", "Cursor was not found on the computer"))
		}
		if !bridgeInstalled(m.home) {
			if err := installBridge(m.home); err != nil {
				return "", err
			}
		}
		m.note(m.t("正在电脑上用 Cursor 打开 ", "Opening in Cursor on the computer: ") + folder)
		if out, err := exec.Command(cli, folder).CombinedOutput(); err != nil {
			return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		for i := 0; i < 50 && !ok; i++ {
			time.Sleep(500 * time.Millisecond)
			r, ok = bridgeForFolder(bridges(m.home), folder)
		}
		if !ok {
			return "", errors.New(m.t("Cursor 打开了项目，但桥接插件没有响应", "Cursor opened the folder but the bridge did not respond"))
		}
	}
	if !bridgeSupports(r) {
		return "", errors.New(m.t(
			"这个项目所在的 Cursor 窗口需要先执行一次 Reload Window",
			"Run Reload Window in the Cursor window of this project first",
		))
	}
	store := &cursorStore{db: m.opt.StateDB}
	sqlite, err := exec.LookPath("sqlite3")
	if err != nil {
		return "", errors.New("sqlite3 not found")
	}
	store.sqlite = sqlite
	since := time.Now().UnixMilli() - 1000
	if _, err := callBridge(r, "command", map[string]any{"id": "composer.createNew"}); err != nil {
		return "", err
	}
	for i := 0; i < 20; i++ {
		time.Sleep(400 * time.Millisecond)
		for _, f := range append([]string{folder}, r.Folders...) {
			if id, _ := store.newComposerSince(f, since); id != "" {
				return id, nil
			}
		}
	}
	return "", errors.New(m.t("Cursor 没有创建新会话", "Cursor did not create a new chat"))
}

// ensureBridge installs the bridge extension on first use and waits for a window to load it.
func (m *mirror) ensureBridge() {
	regs := bridges(m.home)
	if !bridgeInstalled(m.home) {
		if len(regs) == 0 {
			m.note(m.t("正在为 Cursor 安装 KoKo 桥接插件…", "Installing the KoKo bridge extension into Cursor…"))
		}
		if err := installBridge(m.home); err != nil {
			m.fail(err)
			return
		}
	}
	if len(regs) > 0 {
		if r, ok := pickBridge(regs, m.meta.Cwd); ok && !bridgeSupports(r) {
			m.note(m.t(
				"提示：Cursor 里的桥接插件需要更新，在电脑上的 Cursor 执行一次 Reload Window 后才能切换模式、模型和 Keep / Undo。",
				"Tip: the Cursor bridge needs an update; run Reload Window in Cursor to enable mode, model and Keep / Undo.",
			))
		}
		return
	}
	for i := 0; i < 20; i++ {
		if len(bridges(m.home)) > 0 {
			m.publishState()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	m.note(m.t(
		"Cursor 还没加载桥接插件：现在只能查看。确认电脑上 Cursor 已打开（必要时执行 Reload Window）后即可发送。",
		"Cursor has not loaded the bridge yet: view-only for now. Open Cursor on the computer (Reload Window if needed) to send.",
	))
}

// Package agentchat lists AI agent conversations on the host (Cursor / Claude / Codex / Gemini).
// Data-only — no desktop capture, mouse, or PTY.
package agentchat

import (
	"bufio"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Info is one remote AI conversation (aligned with KoKo SSH AgentSessionSync rows).
type Info struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Cwd         string `json:"cwd,omitempty"`
	CreatedAtMs int64  `json:"created_at_ms,omitempty"`
	UpdatedAtMs int64  `json:"updated_at_ms,omitempty"`
	ScreenName  string `json:"screen_name,omitempty"`
	ScreenAlive bool   `json:"screen_alive,omitempty"`
	// Source is "cli" (terminal CLI) or "client" (desktop app / IDE); Client names the latter.
	Source string `json:"source,omitempty"`
	Client string `json:"client,omitempty"`
}

// Origin tells KoKo whether a conversation came from a CLI or a GUI client.
type Origin struct {
	Source string
	Client string
}

var cliOrigin = Origin{Source: "cli"}

type addFunc func(o Origin, kind, sid, title, cwd string, createdMs, updatedMs int64, screenName string)

// List scans the current user's AI session stores. projectPath filters Cursor chats by cwd when set.
func List(projectPath string) ([]Info, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	project := ""
	if strings.TrimSpace(projectPath) != "" {
		project = realPath(projectPath)
	}

	alive := screenAlive()
	var out []Info
	seen := map[string]struct{}{}

	add := func(o Origin, kind, sid, title, cwd string, createdMs, updatedMs int64, screenName string) {
		key := kind + "\x00" + sid
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		if title == "" {
			title = trunc(sid, 8)
		}
		if screenName == "" {
			screenName = "koko-" + kind + "-" + trunc(sid, 8)
		}
		if updatedMs == 0 {
			updatedMs = createdMs
		}
		_, isAlive := alive[screenName]
		out = append(out, Info{
			Kind: kind, ID: sid, Title: title, Cwd: cwd,
			CreatedAtMs: createdMs, UpdatedAtMs: updatedMs,
			ScreenName: screenName, ScreenAlive: isAlive,
			Source: o.Source, Client: o.Client,
		})
	}

	listCursor(home, project, add)
	listCursorIDE(home, project, add)
	listClaude(home, add)
	listCodex(home, project, add)
	var knownCwds []string
	for _, row := range out {
		if row.Cwd != "" {
			knownCwds = append(knownCwds, row.Cwd)
		}
	}
	listGemini(home, project, knownCwds, add)

	for name := range alive {
		parts := strings.SplitN(name, "-", 3)
		if len(parts) < 3 || parts[0] != "koko" {
			continue
		}
		kind := parts[1]
		switch kind {
		case "cursor", "claude", "codex", "gemini":
		default:
			continue
		}
		sid := parts[2]
		add(cliOrigin, kind, sid, "Screen "+name, "", 0, time.Now().UnixMilli(), name)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAtMs > out[j].UpdatedAtMs
	})
	return out, nil
}

func listCursor(home, project string, add addFunc) {
	chatsRoot := filepath.Join(home, ".cursor", "chats")
	addDir := func(dir string, requireCwdMatch bool) {
		metaPath := filepath.Join(dir, "meta.json")
		raw, err := os.ReadFile(metaPath)
		if err != nil {
			return
		}
		var meta map[string]any
		if json.Unmarshal(raw, &meta) != nil {
			return
		}
		if b, _ := meta["isSubagent"].(bool); b {
			return
		}
		if hc, ok := meta["hasConversation"].(bool); ok && !hc {
			return
		}
		sid := filepath.Base(dir)
		cwd, _ := meta["cwd"].(string)
		if requireCwdMatch && project != "" && realPath(cwd) != project {
			return
		}
		title, _ := meta["title"].(string)
		created := msField(meta["createdAtMs"])
		updated := msField(meta["updatedAtMs"])
		if updated == 0 {
			updated = created
		}
		add(cliOrigin, "cursor", sid, title, cwd, created, updated, "")
	}

	if project != "" {
		sum := md5.Sum([]byte(project))
		primary := filepath.Join(chatsRoot, hex.EncodeToString(sum[:]))
		entries, _ := os.ReadDir(primary)
		for _, e := range entries {
			if e.IsDir() {
				addDir(filepath.Join(primary, e.Name()), false)
			}
		}
		entries, _ = os.ReadDir(chatsRoot)
		for _, ws := range entries {
			if !ws.IsDir() || filepath.Join(chatsRoot, ws.Name()) == primary {
				continue
			}
			wsPath := filepath.Join(chatsRoot, ws.Name())
			chats, _ := os.ReadDir(wsPath)
			for _, d := range chats {
				if d.IsDir() {
					addDir(filepath.Join(wsPath, d.Name()), true)
				}
			}
		}
		return
	}

	// No project filter — list all Cursor chats.
	entries, _ := os.ReadDir(chatsRoot)
	for _, ws := range entries {
		if !ws.IsDir() {
			continue
		}
		wsPath := filepath.Join(chatsRoot, ws.Name())
		chats, _ := os.ReadDir(wsPath)
		for _, d := range chats {
			if d.IsDir() {
				addDir(filepath.Join(wsPath, d.Name()), false)
			}
		}
	}
}

func listClaude(home string, add addFunc) {
	root := filepath.Join(home, ".claude", "projects")
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}
		st, err := os.Stat(path)
		if err != nil {
			return nil
		}
		sid := strings.TrimSuffix(d.Name(), ".jsonl")
		ms := st.ModTime().UnixMilli()
		entrypoint, cwd := claudeJSONLMeta(path)
		add(claudeOrigin(entrypoint), "claude", sid, trunc(sid, 8), cwd, 0, ms, "")
		return nil
	})
}

func listCodex(home, project string, add addFunc) {
	for _, root := range []string{
		filepath.Join(home, ".codex", "sessions"),
		filepath.Join(home, ".codex", "archived_sessions"),
	} {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() || filepath.Ext(path) != ".jsonl" {
				return nil
			}
			sid, title, cwd, created, o := codexJSONLMeta(path)
			if o.Source == "" {
				return nil
			}
			if sid == "" {
				name := strings.TrimSuffix(d.Name(), ".jsonl")
				if m := regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F-]{27,}`).FindString(name); m != "" {
					sid = m
				} else {
					sid = name
				}
			}
			if project != "" && cwd != "" && realPath(cwd) != project {
				return nil
			}
			st, statErr := os.Stat(path)
			if statErr != nil {
				return nil
			}
			updated := st.ModTime().UnixMilli()
			if created == 0 {
				created = updated
			}
			add(o, "codex", sid, title, cwd, created, updated, "")
			return nil
		})
	}
}

func codexJSONLMeta(path string) (sid, title, cwd string, createdMs int64, o Origin) {
	o = cliOrigin
	f, err := os.Open(path)
	if err != nil {
		return "", "", "", 0, o
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for lines := 0; lines < 80 && scanner.Scan(); lines++ {
		var row struct {
			Timestamp string         `json:"timestamp"`
			Type      string         `json:"type"`
			Payload   map[string]any `json:"payload"`
		}
		if json.Unmarshal(scanner.Bytes(), &row) != nil {
			continue
		}
		if row.Type == "session_meta" {
			if v, _ := row.Payload["id"].(string); v != "" {
				sid = v
			}
			cwd, _ = row.Payload["cwd"].(string)
			originator, _ := row.Payload["originator"].(string)
			if sub, ok := row.Payload["source"].(map[string]any); ok {
				if _, isSub := sub["subagent"]; isSub {
					return "", "", "", 0, Origin{}
				}
			}
			source, _ := row.Payload["source"].(string)
			o = codexOrigin(originator, source)
			if ts, _ := row.Payload["timestamp"].(string); ts != "" {
				createdMs = parseTimeMs(ts)
			} else {
				createdMs = parseTimeMs(row.Timestamp)
			}
		}
		if title == "" {
			if row.Type == "event_msg" {
				if typ, _ := row.Payload["type"].(string); typ == "user_message" {
					message, _ := row.Payload["message"].(string)
					title = codexUserTitle(message)
				}
			}
			if row.Type == "response_item" {
				if role, _ := row.Payload["role"].(string); role == "user" {
					if content, ok := row.Payload["content"].([]any); ok {
						for _, item := range content {
							part, _ := item.(map[string]any)
							if text, _ := part["text"].(string); text != "" {
								if candidate := codexUserTitle(text); candidate != "" {
									title = candidate
									break
								}
							}
						}
					}
				}
			}
			title = cleanTitle(title)
		}
		if sid != "" && title != "" && cwd != "" {
			break
		}
	}
	return sid, title, cwd, createdMs, o
}

// codexOrigin maps rollout session_meta to CLI vs client. The Codex app and IDE
// extensions (VS Code / Cursor / Windsurf) write the same rollout store, so all of
// them stay resumable by `codex resume`. The app-server records source "vscode" for
// every GUI client, so the originator is checked first.
func codexOrigin(originator, source string) Origin {
	orig := strings.ToLower(originator)
	src := strings.ToLower(source)
	switch {
	case strings.Contains(orig, "desktop") || strings.Contains(orig, "app"):
		return Origin{Source: "client", Client: "Codex App"}
	case strings.Contains(orig, "vscode") || strings.Contains(orig, "ide"):
		return Origin{Source: "client", Client: "Codex IDE extension"}
	case src == "cli" || src == "exec" || strings.Contains(orig, "cli") || strings.Contains(orig, "exec"):
		return cliOrigin
	case src == "vscode":
		return Origin{Source: "client", Client: "Codex IDE extension"}
	case orig == "" && (src == "" || src == "unknown"):
		return cliOrigin
	default:
		name := originator
		if name == "" {
			name = source
		}
		return Origin{Source: "client", Client: name}
	}
}

// claudeJSONLMeta reads the first rows of a Claude Code transcript for its entrypoint and cwd.
func claudeJSONLMeta(path string) (entrypoint, cwd string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for lines := 0; lines < 40 && scanner.Scan(); lines++ {
		var row struct {
			Entrypoint string `json:"entrypoint"`
			Cwd        string `json:"cwd"`
		}
		if json.Unmarshal(scanner.Bytes(), &row) != nil {
			continue
		}
		if entrypoint == "" {
			entrypoint = row.Entrypoint
		}
		if cwd == "" {
			cwd = row.Cwd
		}
		if entrypoint != "" && cwd != "" {
			break
		}
	}
	return entrypoint, cwd
}

func claudeOrigin(entrypoint string) Origin {
	e := strings.ToLower(entrypoint)
	switch {
	case e == "" || e == "cli" || strings.HasPrefix(e, "sdk"):
		return cliOrigin
	case strings.Contains(e, "vscode") || strings.Contains(e, "jetbrains") || strings.Contains(e, "ide"):
		return Origin{Source: "client", Client: "Claude Code IDE extension"}
	case strings.Contains(e, "desktop"):
		return Origin{Source: "client", Client: "Claude Desktop"}
	default:
		return Origin{Source: "client", Client: entrypoint}
	}
}

// listCursorIDE reads Cursor IDE (GUI) composer conversations from its global state DB.
// These live outside ~/.cursor/chats and are not resumable by the Cursor CLI.
func listCursorIDE(home, project string, add addFunc) {
	db := CursorIDEStateDB(home)
	if db == "" {
		return
	}
	if _, err := os.Stat(db); err != nil {
		return
	}
	sqlite, err := exec.LookPath("sqlite3")
	if err != nil {
		return
	}
	const query = `select json_object(
  'id', substr(key, 14),
  'name', json_extract(value, '$.name'),
  'created', json_extract(value, '$.createdAt'),
  'updated', json_extract(value, '$.lastUpdatedAt'),
  'cwd', json_extract(value, '$.workspaceIdentifier.uri.fsPath'))
from cursorDiskKV
where key like 'composerData:%'
  and json_valid(value)
  and json_extract(value, '$.name') is not null
  and json_type(value, '$.subagentInfo') is null
  and coalesce(json_extract(value, '$.isBestOfNSubcomposer'), 0) = 0
  and coalesce(json_array_length(value, '$.fullConversationHeadersOnly'), 0) > 0`
	cmd := exec.Command(sqlite, "-readonly", "file:"+db+"?mode=ro", query)
	raw, err := cmd.Output()
	if err != nil {
		return
	}
	ide := Origin{Source: "client", Client: "Cursor IDE"}
	for _, line := range strings.Split(string(raw), "\n") {
		var row struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Created int64  `json:"created"`
			Updated int64  `json:"updated"`
			Cwd     string `json:"cwd"`
		}
		if json.Unmarshal([]byte(line), &row) != nil || row.ID == "" {
			continue
		}
		if project != "" && row.Cwd != "" && realPath(row.Cwd) != project {
			continue
		}
		add(ide, "cursor", row.ID, cleanTitle(row.Name), row.Cwd, row.Created, row.Updated, "")
	}
}

// CursorIDEStateDB is Cursor's global state DB holding IDE composer conversations.
func CursorIDEStateDB(home string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "state.vscdb")
	case "linux":
		return filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "state.vscdb")
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "Cursor", "User", "globalStorage", "state.vscdb")
		}
	}
	return ""
}

// listGemini reads Gemini CLI session recordings (~/.gemini/tmp/<project>/chats/session-*.json).
// `gemini --resume` only finds sessions of the current project, so the project root is
// recovered from .project_root / projects.json, or by matching projectHash (sha256 of the
// root) against cwds known from other providers.
func listGemini(home, project string, knownCwds []string, add addFunc) {
	geminiDir := filepath.Join(home, ".gemini")
	slugRoots := map[string]string{}
	if raw, err := os.ReadFile(filepath.Join(geminiDir, "projects.json")); err == nil {
		var reg struct {
			Projects map[string]string `json:"projects"`
		}
		if json.Unmarshal(raw, &reg) == nil {
			for root, slug := range reg.Projects {
				slugRoots[slug] = root
			}
		}
	}
	hashRoots := map[string]string{}
	for _, c := range append(knownCwds, project) {
		if c == "" {
			continue
		}
		for _, v := range []string{c, realPath(c)} {
			sum := sha256.Sum256([]byte(v))
			hashRoots[hex.EncodeToString(sum[:])] = v
		}
	}

	for _, sub := range []string{"chats", "tmp"} {
		root := filepath.Join(geminiDir, sub)
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".json" {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			var data map[string]any
			if json.Unmarshal(raw, &data) != nil {
				return nil
			}
			sid, _ := data["sessionId"].(string)
			if sid == "" {
				sid, _ = data["id"].(string)
			}
			if sid == "" {
				// Not a session recording (settings, logs, checkpoints).
				if _, ok := data["messages"]; !ok {
					return nil
				}
				sid = strings.TrimSuffix(d.Name(), ".json")
			}
			if kind, _ := data["kind"].(string); kind == "subagent" {
				return nil
			}
			if msgs, ok := data["messages"].([]any); ok && len(msgs) == 0 {
				return nil
			}

			cwd := geminiProjectRoot(path, geminiDir, data, slugRoots, hashRoots)
			if project != "" && cwd != "" && realPath(cwd) != project {
				return nil
			}
			title := geminiTitle(data)
			created := parseTimeMs(stringField(data, "startTime"))
			updated := parseTimeMs(stringField(data, "lastUpdated"))
			if updated == 0 {
				if st, _ := os.Stat(path); st != nil {
					updated = st.ModTime().UnixMilli()
				}
			}
			add(geminiOrigin(data), "gemini", sid, title, cwd, created, updated, "")
			return nil
		})
	}
}

func geminiProjectRoot(path, geminiDir string, data map[string]any, slugRoots, hashRoots map[string]string) string {
	if c := stringField(data, "cwd"); c != "" {
		return c
	}
	dir := filepath.Dir(path)
	if filepath.Base(dir) == "chats" {
		dir = filepath.Dir(dir)
	}
	if filepath.Dir(dir) == filepath.Join(geminiDir, "tmp") {
		if raw, err := os.ReadFile(filepath.Join(dir, ".project_root")); err == nil {
			if c := strings.TrimSpace(string(raw)); c != "" {
				return c
			}
		}
		if c := slugRoots[filepath.Base(dir)]; c != "" {
			return c
		}
		if c := hashRoots[filepath.Base(dir)]; c != "" {
			return c
		}
	}
	return hashRoots[stringField(data, "projectHash")]
}

func geminiTitle(data map[string]any) string {
	for _, key := range []string{"title", "summary"} {
		if t := cleanTitle(stringField(data, key)); t != "" {
			return t
		}
	}
	msgs, _ := data["messages"].([]any)
	for _, item := range msgs {
		m, _ := item.(map[string]any)
		if typ, _ := m["type"].(string); typ != "user" {
			continue
		}
		text := ""
		switch c := m["content"].(type) {
		case string:
			text = c
		case []any:
			for _, p := range c {
				part, _ := p.(map[string]any)
				if s, _ := part["text"].(string); s != "" {
					text = s
					break
				}
			}
		}
		if t := cleanTitle(text); t != "" && !strings.HasPrefix(t, "/") {
			return t
		}
	}
	return ""
}

// geminiOrigin: Gemini CLI recordings carry no client marker today (IDE companion mode is
// still the CLI in an IDE terminal). Honor one if a future version adds it.
func geminiOrigin(data map[string]any) Origin {
	for _, key := range []string{"source", "client", "entrypoint"} {
		v := strings.ToLower(stringField(data, key))
		switch {
		case v == "" || v == "cli" || v == "terminal":
			continue
		case strings.Contains(v, "vscode") || strings.Contains(v, "jetbrains") || strings.Contains(v, "ide") || strings.Contains(v, "codeassist"):
			return Origin{Source: "client", Client: "Gemini Code Assist"}
		default:
			return Origin{Source: "client", Client: stringField(data, key)}
		}
	}
	return cliOrigin
}

func stringField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func screenAlive() map[string]struct{} {
	out := map[string]struct{}{}
	cmd := exec.Command("screen", "-ls")
	raw, _ := cmd.CombinedOutput()
	re := regexp.MustCompile(`\t(\d+)\.(koko-[a-z]+-[^\s\t]+)`)
	for _, line := range strings.Split(string(raw), "\n") {
		m := re.FindStringSubmatch(line)
		if len(m) >= 3 {
			out[m[2]] = struct{}{}
		}
	}
	return out
}

func realPath(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	return abs
}

func msField(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case json.Number:
		i, _ := t.Int64()
		return i
	default:
		return 0
	}
}

func parseTimeMs(value string) int64 {
	if value == "" {
		return 0
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UnixMilli()
		}
	}
	return 0
}

func cleanTitle(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	const maxRunes = 80
	runes := []rune(value)
	if len(runes) > maxRunes {
		value = string(runes[:maxRunes]) + "…"
	}
	return value
}

var codexUserQueryRE = regexp.MustCompile(`(?is)<user_query>\s*(.*?)\s*</user_query>`)

func codexUserTitle(value string) string {
	value = strings.TrimSpace(value)
	if match := codexUserQueryRE.FindStringSubmatch(value); len(match) > 1 {
		return cleanTitle(match[1])
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "<environment_context") ||
		strings.HasPrefix(lower, "# agents.md instructions") ||
		strings.Contains(lower, "<instructions>") ||
		strings.Contains(lower, "<approval_policy>") ||
		strings.Contains(lower, "<cwd>") {
		return ""
	}
	return cleanTitle(value)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

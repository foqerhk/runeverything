package agentchat

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestCodexJSONLMetaCurrentRolloutFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-2026-10-07-636f1421.jsonl")
	body := `{"timestamp":"2026-10-07T01:02:03Z","type":"session_meta","payload":{"id":"636f1421-3a0e-44b3-9eb2-cc3a694efc97","timestamp":"2026-10-07T01:02:03Z","cwd":"/tmp/project","source":"cli"}}` + "\n" +
		`{"timestamp":"2026-10-07T01:02:04Z","type":"event_msg","payload":{"type":"user_message","message":"Fix the current project tests"}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	id, title, cwd, created, origin := codexJSONLMeta(path)
	if origin.Source != "cli" {
		t.Fatalf("origin=%+v", origin)
	}
	if id != "636f1421-3a0e-44b3-9eb2-cc3a694efc97" {
		t.Fatalf("id=%q", id)
	}
	if title != "Fix the current project tests" {
		t.Fatalf("title=%q", title)
	}
	if cwd != "/tmp/project" {
		t.Fatalf("cwd=%q", cwd)
	}
	if created <= 0 {
		t.Fatalf("created=%d", created)
	}
}

func TestCodexJSONLMetaSkipsInjectedContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-context.jsonl")
	body := `{"type":"session_meta","payload":{"id":"session-1","cwd":"/tmp/project"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"user_message","message":"<environment_context><cwd>/tmp/project</cwd></environment_context>"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"user_message","message":"Please fix the login flow"}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, title, _, _, _ := codexJSONLMeta(path)
	if title != "Please fix the login flow" {
		t.Fatalf("title=%q", title)
	}
}

func TestListCurrentHomeHasUniqueProviderIDs(t *testing.T) {
	rows, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, row := range rows {
		key := row.Kind + "\x00" + row.ID
		if seen[key] {
			t.Fatalf("duplicate provider/id %q", key)
		}
		seen[key] = true
		counts[row.Kind]++
	}
	t.Logf("provider counts: %#v total=%d", counts, len(rows))
}

func TestOriginMapping(t *testing.T) {
	cases := []struct {
		got  Origin
		want Origin
	}{
		{codexOrigin("codex_cli_rs", "cli"), Origin{Source: "cli"}},
		{codexOrigin("codex_exec", "exec"), Origin{Source: "cli"}},
		{codexOrigin("codex_vscode", "vscode"), Origin{Source: "client", Client: "Codex IDE extension"}},
		{codexOrigin("Codex Desktop", "vscode"), Origin{Source: "client", Client: "Codex App"}},
		{codexOrigin("", "vscode"), Origin{Source: "client", Client: "Codex IDE extension"}},
		{codexOrigin("", ""), Origin{Source: "cli"}},
		{claudeOrigin("cli"), Origin{Source: "cli"}},
		{claudeOrigin("sdk-ts"), Origin{Source: "cli"}},
		{claudeOrigin("claude-vscode"), Origin{Source: "client", Client: "Claude Code IDE extension"}},
		{claudeOrigin("claude-desktop"), Origin{Source: "client", Client: "Claude Desktop"}},
		{geminiOrigin(map[string]any{}), Origin{Source: "cli"}},
		{geminiOrigin(map[string]any{"source": "vscode"}), Origin{Source: "client", Client: "Gemini Code Assist"}},
	}
	for i, c := range cases {
		if c.got != c.want {
			t.Fatalf("case %d: got %+v want %+v", i, c.got, c.want)
		}
	}
}

func TestCodexSkipsSubagentRollouts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-sub.jsonl")
	body := `{"type":"session_meta","payload":{"id":"sub-1","cwd":"/tmp/p","originator":"codex_cli_rs","source":{"subagent":"review"}}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, o := codexJSONLMeta(path); o.Source != "" {
		t.Fatalf("subagent rollout should be skipped, got %+v", o)
	}
}

func TestListGeminiRecoversProjectAndTitle(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "proj")
	slugDir := filepath.Join(home, ".gemini", "tmp", "proj-slug", "chats")
	hashSum := sha256.Sum256([]byte(project))
	hashDir := filepath.Join(home, ".gemini", "tmp", hex.EncodeToString(hashSum[:]), "chats")
	for _, d := range []string{project, slugDir, hashDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(home, ".gemini", "tmp", "proj-slug", ".project_root"), project+"\n")
	write(filepath.Join(slugDir, "session-a.json"), `{"sessionId":"g-a","startTime":"2026-10-07T01:00:00Z","lastUpdated":"2026-10-07T02:00:00Z","messages":[{"type":"user","content":"/help"},{"type":"user","content":[{"text":"Refactor the parser"}]}]}`)
	write(filepath.Join(hashDir, "session-b.json"), `{"sessionId":"g-b","projectHash":"x","messages":[{"type":"user","content":"Hi"}]}`)
	write(filepath.Join(hashDir, "session-sub.json"), `{"sessionId":"g-sub","kind":"subagent","messages":[{"type":"user","content":"x"}]}`)
	write(filepath.Join(home, ".gemini", "tmp", "proj-slug", "logs.json"), `[]`)

	rows := map[string]Info{}
	listGemini(home, "", []string{project}, func(o Origin, kind, sid, title, cwd string, createdMs, updatedMs int64, screenName string) {
		rows[sid] = Info{Kind: kind, ID: sid, Title: title, Cwd: cwd, CreatedAtMs: createdMs, UpdatedAtMs: updatedMs, Source: o.Source}
	})
	if len(rows) != 2 {
		t.Fatalf("rows=%+v", rows)
	}
	if a := rows["g-a"]; a.Cwd != project || a.Title != "Refactor the parser" || a.Source != "cli" || a.CreatedAtMs == 0 || a.UpdatedAtMs <= a.CreatedAtMs {
		t.Fatalf("g-a=%+v", a)
	}
	if b := rows["g-b"]; b.Cwd != project || b.Title != "Hi" {
		t.Fatalf("g-b=%+v", b)
	}
}

func TestListCursorIDEUsesStateDB(t *testing.T) {
	home, _ := os.UserHomeDir()
	if _, err := os.Stat(CursorIDEStateDB(home)); err != nil {
		t.Skip("no Cursor IDE state DB")
	}
	var rows []Info
	listCursorIDE(home, "", func(o Origin, kind, sid, title, cwd string, createdMs, updatedMs int64, screenName string) {
		rows = append(rows, Info{Kind: kind, ID: sid, Title: title, Cwd: cwd, Source: o.Source, Client: o.Client})
	})
	if len(rows) == 0 {
		t.Fatal("expected Cursor IDE conversations")
	}
	for _, r := range rows {
		if r.Source != "client" || r.Client != "Cursor IDE" || r.Title == "" {
			t.Fatalf("bad row %+v", r)
		}
	}
	t.Logf("cursor IDE rows=%d first=%q", len(rows), rows[0].Title)
}

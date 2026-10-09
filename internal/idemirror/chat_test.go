package idemirror

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

const testQParams = `{"title":"Pick","questions":[{"id":"q1","prompt":"Render how?","options":[{"id":"native","label":"Native view"},{"id":"term","label":"Terminal"}]},{"id":"q2","prompt":"Answer on phone?","allowMultiple":true,"options":[{"id":"yes","label":"Yes"},{"id":"view","label":"View only"}]}]}`

func TestChatMessageForQuestion(t *testing.T) {
	b := bubble{Idx: 3, ID: "b3", Type: 2, Tool: "ask_question", Have: true, ToolCallID: "tc1",
		QParams: testQParams, QStatus: "submitted",
		QResult: `{"answers":[{"questionId":"q1","selectedOptionIds":["native"],"freeformText":""}]}`}
	msg, ok := chatMessageFor(b)
	if !ok || msg.Role != "question" || msg.Question == nil {
		t.Fatalf("got %+v ok=%v", msg, ok)
	}
	q := msg.Question
	if q.ToolCallID != "tc1" || q.Status != "submitted" || len(q.Questions) != 2 || !q.Questions[1].AllowMultiple {
		t.Fatalf("question = %+v", q)
	}
	if len(q.Answers) != 1 || q.Answers[0].SelectedOptionIDs[0] != "native" {
		t.Fatalf("answers = %+v", q.Answers)
	}
}

func TestChatMessageForKinds(t *testing.T) {
	cases := []struct {
		b    bubble
		role string
		ok   bool
	}{
		{bubble{Type: 1, Text: "  hi  ", Have: true}, "user", true},
		{bubble{Type: 1, Text: "   ", Have: true}, "", false},
		{bubble{Type: 2, Text: "# Title\n- a", Have: true}, "assistant", true},
		{bubble{Type: 2, Text: "", Have: true}, "", false},
		{bubble{Type: 2, Tool: "run_terminal_command_v2", Args: "go  test ./...", Status: "completed", Have: true}, "tool", true},
	}
	for i, c := range cases {
		msg, ok := chatMessageFor(c.b)
		if ok != c.ok || (ok && msg.Role != c.role) {
			t.Fatalf("case %d: role=%q ok=%v", i, msg.Role, ok)
		}
	}
	msg, _ := chatMessageFor(cases[4].b)
	if msg.Tool.Label != "Shell" || msg.Tool.Summary != "go test ./..." {
		t.Fatalf("tool = %+v", msg.Tool)
	}
}

func TestAnswerText(t *testing.T) {
	b := bubble{Tool: "ask_question", ToolCallID: "tc1", QParams: testQParams}
	q, ok := parseQuestion(b)
	if !ok {
		t.Fatal("parse failed")
	}
	got := answerText(q, []questionAnswer{
		{QuestionID: "q2", SelectedOptionIDs: []string{"yes", "view"}},
		{QuestionID: "q1", SelectedOptionIDs: []string{"native"}, FreeformText: "with tables"},
	})
	want := "Render how?\n→ Native view; with tables\nAnswer on phone?\n→ Yes; View only"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if answerText(q, nil) != "" {
		t.Fatal("empty answers must give empty text")
	}
}

func decodeChatFrames(t *testing.T, out string) []chatFrame {
	t.Helper()
	var frames []chatFrame
	for _, part := range strings.Split(out, oscChatTag)[1:] {
		end := strings.IndexByte(part, 0x07)
		raw, err := base64.StdEncoding.DecodeString(part[:end])
		if err != nil {
			t.Fatal(err)
		}
		var f chatFrame
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, f)
	}
	return frames
}

func TestPublishChatSendsOnlyChanges(t *testing.T) {
	var out bytes.Buffer
	m := &mirror{out: &out, sentChat: map[int]string{}, pendingQ: -1, answered: map[string][]questionAnswer{}}
	rows := []bubble{
		{Idx: 0, ID: "a", Type: 1, Text: "hello", Have: true},
		{Idx: 1, ID: "b", Type: 2, Text: "partial", Have: true},
		{Idx: 2, ID: "c", Type: 2, Tool: "ask_question", ToolCallID: "tc1", QParams: testQParams, Have: true},
	}
	m.publishChatLocked(rows, true)
	rows[1].Text = "partial and more"
	m.publishChatLocked(rows, false)
	m.publishChatLocked(rows, false)
	m.answered["tc1"] = []questionAnswer{{QuestionID: "q1", SelectedOptionIDs: []string{"term"}}}
	m.publishChatLocked(rows, false)

	frames := decodeChatFrames(t, out.String())
	if len(frames) != 3 {
		t.Fatalf("frames = %d, want 3 (reset, grown text, answered)", len(frames))
	}
	if !frames[0].Reset || len(frames[0].Messages) != 3 {
		t.Fatalf("reset frame = %+v", frames[0])
	}
	if len(frames[1].Messages) != 1 || frames[1].Messages[0].Text != "partial and more" {
		t.Fatalf("delta frame = %+v", frames[1])
	}
	if q := frames[2].Messages[0].Question; q == nil || q.Status != "answered" {
		t.Fatalf("answered frame = %+v", frames[2])
	}
	if m.pendingQ != 2 {
		t.Fatalf("pendingQ = %d, want 2", m.pendingQ)
	}
}

package idemirror

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestRendererStreamsLinesAndSettles(t *testing.T) {
	var r renderer
	t0 := time.Unix(0, 0)
	user := bubble{Idx: 0, ID: "u", Type: 1, Text: "fix it", Have: true}
	ai := bubble{Idx: 1, ID: "a", Type: 2, Text: "Looking", Have: true}

	out := r.feed([]bubble{user, ai}, t0)
	if !strings.Contains(out, "❯ fix it") || strings.Contains(out, "Looking") {
		t.Fatalf("partial line must wait: %q", out)
	}
	ai.Text = "Looking now\nsecond"
	out = r.feed([]bubble{ai}, t0.Add(100*time.Millisecond))
	if out != "\r\nLooking now\r\n" {
		t.Fatalf("complete line: %q", out)
	}
	if out = r.feed([]bubble{ai}, t0.Add(2*time.Second)); out != "second\r\n" {
		t.Fatalf("idle flush: %q", out)
	}
	tool := bubble{Idx: 2, ID: "t", Type: 2, Tool: "run_terminal_command_v2", Args: "go test\n  ./...", Have: true}
	out = r.feed([]bubble{ai, tool}, t0.Add(3*time.Second))
	if !strings.Contains(out, "Shell  go test ./...") {
		t.Fatalf("tool line: %q", out)
	}
	if out = r.feed([]bubble{tool}, t0.Add(4*time.Second)); out != "" {
		t.Fatalf("tool printed twice: %q", out)
	}
	if r.next != 2 {
		t.Fatalf("next=%d", r.next)
	}
}

func TestRendererSettledBubblePrintsRemainder(t *testing.T) {
	var r renderer
	t0 := time.Unix(0, 0)
	ai := bubble{Idx: 0, ID: "a", Type: 2, Text: "done", Have: true}
	next := bubble{Idx: 1, ID: "b", Type: 2, Have: false}
	out := r.feed([]bubble{ai, next}, t0)
	if out != "\r\ndone\r\n" || r.next != 1 {
		t.Fatalf("out=%q next=%d", out, r.next)
	}
}

func TestEscapeSeq(t *testing.T) {
	if s, ok := escapeSeq([]byte("\x1b[200~hi")); !ok || s != "\x1b[200~" {
		t.Fatalf("%q %v", s, ok)
	}
	if _, ok := escapeSeq([]byte("\x1b[2")); ok {
		t.Fatal("incomplete sequence")
	}
	if _, ok := escapeSeq([]byte("\x1b]7789;abc")); ok {
		t.Fatal("OSC without terminator is incomplete")
	}
	if s, ok := escapeSeq([]byte("\x1b]7789;abc\x07rest")); !ok || s != "\x1b]7789;abc\x07" {
		t.Fatalf("%q %v", s, ok)
	}
}

func TestDecodeAction(t *testing.T) {
	seq := oscActionTag + base64.StdEncoding.EncodeToString([]byte(`{"op":"mode","id":"chat"}`)) + "\x07"
	a, err := decodeAction(seq)
	if err != nil || a.Op != "mode" || a.ID != "chat" {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := decodeAction(oscActionTag + "!!\x07"); err == nil {
		t.Fatal("bad base64 accepted")
	}
}

func TestStateFiles(t *testing.T) {
	meta := composerMeta{
		Files:        []changedFile{{URI: "file:///Users/me/My%20App/a.go"}, {URI: "file:///Users/me/b.go", New: true}},
		CreatedFiles: []string{"file:///Users/me/b.go", "file:///tmp/c.txt"},
	}
	got := stateFiles(meta)
	if len(got) != 3 || got[0].Path != "/Users/me/My App/a.go" || !got[1].New || got[2].Path != "/tmp/c.txt" {
		t.Fatalf("%+v", got)
	}
}

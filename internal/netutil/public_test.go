package netutil

import "testing"

func TestIsLoopbackHost(t *testing.T) {
	cases := map[string]bool{
		"":                  true,
		"localhost":         true,
		"127.0.0.1":         true,
		"127.0.0.1:8787":    true,
		"[::1]":             true,
		"[::1]:8787":        true,
		"8.8.8.8":           false,
		"8.8.8.8:8787":      false,
		"relay.example.com": false,
	}
	for in, want := range cases {
		if got := IsLoopbackHost(in); got != want {
			t.Fatalf("IsLoopbackHost(%q)=%v want %v", in, got, want)
		}
	}
}

func TestRewriteLoopbackRelayHost(t *testing.T) {
	got := RewriteLoopbackRelayHost("ws://127.0.0.1:8787/re2", "203.0.113.10")
	want := "ws://203.0.113.10:8787/re2"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	keep := "wss://relay.example/re2"
	if RewriteLoopbackRelayHost(keep, "203.0.113.10") != keep {
		t.Fatalf("non-loopback should stay unchanged")
	}
	if RewriteLoopbackRelayHost("ws://127.0.0.1:8787/re2", "") != "ws://127.0.0.1:8787/re2" {
		t.Fatalf("empty public host should keep original")
	}
}

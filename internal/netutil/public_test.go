package netutil

import (
	"context"
	"net"
	"os"
	"testing"
)

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

func TestAdvertiseRelayURL(t *testing.T) {
	got := AdvertiseRelayURL("ws://127.0.0.1:8787/re2", "203.0.113.10")
	want := "ws://203.0.113.10:8787/re2"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDirectPublicIP_NATEgressRejected(t *testing.T) {
	// Home NAT: only private NICs, egress is shared public IP.
	lookup := func(context.Context) (string, error) { return "203.0.113.50", nil }
	locals := func() []net.IP { return nil }
	ip, ok := directPublicIP(context.Background(), lookup, locals)
	if ok || ip != "" {
		t.Fatalf("NAT egress must not count as direct public IP; got %q ok=%v", ip, ok)
	}
}

func TestDirectPublicIP_MatchingLocal(t *testing.T) {
	lookup := func(context.Context) (string, error) { return "203.0.113.10", nil }
	locals := func() []net.IP { return []net.IP{net.ParseIP("203.0.113.10")} }
	ip, ok := directPublicIP(context.Background(), lookup, locals)
	if !ok || ip != "203.0.113.10" {
		t.Fatalf("got %q ok=%v", ip, ok)
	}
}

func TestDirectPublicIP_EchoFailLocalPublicFallback(t *testing.T) {
	lookup := func(context.Context) (string, error) { return "", os.ErrNotExist }
	locals := func() []net.IP { return []net.IP{net.ParseIP("198.51.100.7")} }
	ip, ok := directPublicIP(context.Background(), lookup, locals)
	if !ok || ip != "198.51.100.7" {
		t.Fatalf("got %q ok=%v", ip, ok)
	}
}

func TestDirectPublicIP_PrivateEgressRejected(t *testing.T) {
	lookup := func(context.Context) (string, error) { return "10.0.0.1", nil }
	locals := func() []net.IP { return []net.IP{net.ParseIP("10.0.0.1")} }
	ip, ok := directPublicIP(context.Background(), lookup, locals)
	if ok || ip != "" {
		t.Fatalf("private egress must be rejected; got %q ok=%v", ip, ok)
	}
}

func TestEchoURLsOrdered(t *testing.T) {
	t.Setenv("RE_IP_ECHO_CN", "")
	t.Setenv("RE_IP_ECHO_INTL", "")
	SetEchoURLOverrides(nil, nil)
	cn := echoURLsOrdered("cn")
	if len(cn) != 4 || cn[0] != echoURLsCN[0] || cn[1] != echoURLsCN[1] ||
		cn[2] != echoURLsIntl[0] || cn[3] != echoURLsIntl[1] {
		t.Fatalf("cn order: %v", cn)
	}
	intl := echoURLsOrdered("intl")
	if len(intl) != 4 || intl[0] != echoURLsIntl[0] || intl[1] != echoURLsIntl[1] ||
		intl[2] != echoURLsCN[0] || intl[3] != echoURLsCN[1] {
		t.Fatalf("intl order: %v", intl)
	}
}

func TestEchoURLOverrides(t *testing.T) {
	t.Setenv("RE_IP_ECHO_CN", "")
	t.Setenv("RE_IP_ECHO_INTL", "")
	SetEchoURLOverrides([]string{"https://cn.example/ip"}, []string{"https://intl.example/ip"})
	defer SetEchoURLOverrides(nil, nil)
	cn, intl := ActiveEchoURLs()
	if len(cn) != 1 || cn[0] != "https://cn.example/ip" {
		t.Fatalf("cn=%v", cn)
	}
	if len(intl) != 1 || intl[0] != "https://intl.example/ip" {
		t.Fatalf("intl=%v", intl)
	}
	t.Setenv("RE_IP_ECHO_CN", "https://env-cn.example/ip")
	cn, _ = ActiveEchoURLs()
	if len(cn) != 1 || cn[0] != "https://env-cn.example/ip" {
		t.Fatalf("env should win: %v", cn)
	}
}


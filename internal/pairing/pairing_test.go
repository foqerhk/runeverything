package pairing

import (
	"strings"
	"testing"

	"github.com/foqerhk/runeverything/internal/protocol"
)

func TestNewPayloadOptsRE2(t *testing.T) {
	p, token, err := NewPayloadOpts("ws://127.0.0.1:8787/re2", "dev1", "box", DefaultTTL, Options{
		NoisePub: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		LAN:      []string{"192.168.1.8:41234"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	if p.V != protocol.VersionRE2 {
		t.Fatalf("v=%d", p.V)
	}
	if p.NoisePub == "" {
		t.Fatal("missing noise_pub")
	}
	if len(p.LAN) != 1 || p.LAN[0] != "192.168.1.8:41234" {
		t.Fatalf("lan=%v", p.LAN)
	}
	link := p.DeepLink()
	if !strings.Contains(link, "noise_pub=") {
		t.Fatalf("deep link missing noise_pub: %s", link)
	}
	if !strings.Contains(link, "lan=") {
		t.Fatalf("deep link missing lan: %s", link)
	}
}

func TestNewPayloadIgnoresV1(t *testing.T) {
	p, _, err := NewPayloadOpts("ws://127.0.0.1:8787/re2", "dev1", "box", DefaultTTL, Options{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if p.V != protocol.VersionRE2 {
		t.Fatalf("v=%d want %d (v1 must be ignored)", p.V, protocol.VersionRE2)
	}
}


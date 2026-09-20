package pairing

import (
	"strings"
	"testing"

	"github.com/foqerhk/runeverything/internal/protocol"
)

func TestNewPayloadOptsRE2(t *testing.T) {
	p, token, err := NewPayloadOpts("ws://127.0.0.1:8787/re2", "dev1", "box", DefaultTTL, Options{
		NoisePub: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
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
	link := p.DeepLink()
	if !strings.Contains(link, "noise_pub=") {
		t.Fatalf("deep link missing noise_pub: %s", link)
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


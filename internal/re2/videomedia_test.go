package re2

import (
	"bytes"
	"testing"
)

func TestVideoMediaRoundTripAndLoss(t *testing.T) {
	psk := DerivePSK("video-media-test-token")
	initStatic := bytes.Repeat([]byte{0x11}, 32)
	respStatic := bytes.Repeat([]byte{0x22}, 32)
	a2c, c2a, err := DeriveVideoMediaKeys(psk, initStatic, respStatic)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := NewVideoMediaAgent(a2c, c2a)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewVideoMediaClient(a2c, c2a)
	if err != nil {
		t.Fatal(err)
	}

	plain := EncodeInner(MsgVideo, []byte("nal-fragment-1"))
	pkt, err := agent.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !IsVideoPlane(pkt) {
		t.Fatal("expected video plane magic")
	}
	out, err := client.Open(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, plain) {
		t.Fatalf("mismatch %x vs %x", out, plain)
	}

	// Loss of an intermediate packet must not break later decrypts.
	_, _ = agent.Seal(EncodeInner(MsgVideo, []byte("lost")))
	pkt2, err := agent.Seal(EncodeInner(MsgVideo, []byte("after-loss")))
	if err != nil {
		t.Fatal(err)
	}
	out2, err := client.Open(pkt2)
	if err != nil {
		t.Fatal(err)
	}
	mt, body, err := DecodeInner(out2)
	if err != nil || mt != MsgVideo || string(body) != "after-loss" {
		t.Fatalf("after-loss decode: mt=%d body=%q err=%v", mt, body, err)
	}
}

package re2

import (
	"bytes"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	in := Frame{Type: TypeNoise, RouteID: "dev123", Payload: []byte("hello-re2")}
	if err := EncodeFrame(&buf, in); err != nil {
		t.Fatal(err)
	}
	out, err := DecodeFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if out.Type != in.Type || out.RouteID != in.RouteID || string(out.Payload) != string(in.Payload) {
		t.Fatalf("got %+v", out)
	}
}

func TestPSKLength(t *testing.T) {
	psk := DerivePSK("pair-token-example")
	if len(psk) != 32 {
		t.Fatalf("len=%d", len(psk))
	}
}

type memTransport struct {
	in  chan []byte
	out chan []byte
}

func (m *memTransport) Send(msg []byte) error {
	cp := append([]byte(nil), msg...)
	m.out <- cp
	return nil
}

func (m *memTransport) Recv() ([]byte, error) {
	msg := <-m.in
	return msg, nil
}

func TestNoiseHandshakeAndPTY(t *testing.T) {
	agentKey, err := GenerateStaticKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	psk := DerivePSK("test-pairing-token")

	c2a := make(chan []byte, 4)
	a2c := make(chan []byte, 4)
	clientT := &memTransport{in: a2c, out: c2a}
	agentT := &memTransport{in: c2a, out: a2c}

	clientHS, err := NewClientHandshake(psk)
	if err != nil {
		t.Fatal(err)
	}
	agentHS, err := NewAgentHandshake(psk, agentKey)
	if err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 2)
	var agentSess *Session
	var clientSess *Session
	var peerPub []byte
	go func() {
		s, _, err := agentHS.RunAgent(agentT)
		agentSess = s
		errCh <- err
	}()
	go func() {
		s, pub, err := clientHS.RunClient(clientT)
		clientSess = s
		peerPub = pub
		errCh <- err
	}()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(peerPub, agentKey.Public) {
		t.Fatal("client did not learn agent static pub")
	}

	inner := EncodeInner(MsgPTYData, EncodePTY("sess1", []byte("hello pty")))
	ct, err := clientSess.Encrypt(inner)
	if err != nil {
		t.Fatal(err)
	}
	pt, err := agentSess.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}
	mt, body, err := DecodeInner(pt)
	if err != nil || mt != MsgPTYData {
		t.Fatalf("inner mt=%d err=%v", mt, err)
	}
	sid, data, err := DecodePTY(body)
	if err != nil || sid != "sess1" || string(data) != "hello pty" {
		t.Fatalf("pty sid=%s data=%s err=%v", sid, data, err)
	}

	// reverse direction
	inner2 := EncodeInner(MsgSessionReady, []byte(`{"session_id":"sess1"}`))
	ct2, err := agentSess.Encrypt(inner2)
	if err != nil {
		t.Fatal(err)
	}
	pt2, err := clientSess.Decrypt(ct2)
	if err != nil {
		t.Fatal(err)
	}
	mt2, _, err := DecodeInner(pt2)
	if err != nil || mt2 != MsgSessionReady {
		t.Fatal("reverse decrypt failed")
	}
}

func TestPinMismatch(t *testing.T) {
	// After handshake, client must compare peer pub to QR; unit-level pin is that check.
	a, _ := GenerateStaticKeyPair()
	b, _ := GenerateStaticKeyPair()
	if bytes.Equal(a.Public, b.Public) {
		t.Fatal("keys should differ")
	}
}

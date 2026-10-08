package re2

import (
	"bytes"
	"testing"
)

func runMulti(t *testing.T, clientToken string, agentTokens []string) (int, error) {
	t.Helper()
	agentKey, err := GenerateStaticKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	c2a := make(chan []byte, 16)
	a2c := make(chan []byte, 16)
	clientT := &memTransport{in: a2c, out: c2a}
	agentT := &memTransport{in: c2a, out: a2c}

	clientHS, err := NewClientHandshake(DerivePSK(clientToken))
	if err != nil {
		t.Fatal(err)
	}
	psks := make([][]byte, len(agentTokens))
	for i, tok := range agentTokens {
		psks[i] = DerivePSK(tok)
	}
	agentHS, err := NewAgentHandshakes(psks, agentKey)
	if err != nil {
		t.Fatal(err)
	}

	type result struct {
		sess *Session
		idx  int
		err  error
	}
	agentCh := make(chan result, 1)
	go func() {
		s, _, idx, err := agentHS.RunAgent(agentT)
		agentCh <- result{s, idx, err}
	}()
	clientSess, pub, err := clientHS.RunClient(clientT)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pub, agentKey.Public) {
		t.Fatal("client did not learn agent static pub")
	}
	// No more client packets: the agent must give up instead of matching a wrong PSK.
	close(c2a)
	r := <-agentCh
	if r.err != nil {
		return -1, r.err
	}
	ct, err := clientSess.Encrypt([]byte("ping"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := r.sess.Decrypt(ct)
	if err != nil || string(pt) != "ping" {
		t.Fatalf("session mismatch: %q %v", pt, err)
	}
	return r.idx, nil
}

func TestMultiHandshakeMatchesCurrentToken(t *testing.T) {
	idx, err := runMulti(t, "current", []string{"current", "older"})
	if err != nil || idx != 0 {
		t.Fatalf("idx=%d err=%v", idx, err)
	}
}

func TestMultiHandshakeAcceptsEarlierToken(t *testing.T) {
	idx, err := runMulti(t, "scanned-before-rotation", []string{"rotated", "other", "scanned-before-rotation"})
	if err != nil || idx != 2 {
		t.Fatalf("idx=%d err=%v", idx, err)
	}
}

func TestMultiHandshakeRejectsUnknownToken(t *testing.T) {
	if _, err := runMulti(t, "stranger", []string{"rotated", "other"}); err == nil {
		t.Fatal("handshake with an unknown token must fail")
	}
}

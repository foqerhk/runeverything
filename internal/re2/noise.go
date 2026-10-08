package re2

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"

	"github.com/flynn/noise"
	"golang.org/x/crypto/curve25519"
)

const Prologue = "runeverything-re2-v2"

// StaticKeyPair is a long-term Curve25519 identity for Noise.
type StaticKeyPair struct {
	Private []byte // 32 bytes
	Public  []byte // 32 bytes
}

// GenerateStaticKeyPair creates a new identity key.
func GenerateStaticKeyPair() (*StaticKeyPair, error) {
	var priv, pub [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return nil, err
	}
	// clamp
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	curve25519.ScalarBaseMult(&pub, &priv)
	return &StaticKeyPair{Private: priv[:], Public: pub[:]}, nil
}

func cipherSuite() noise.CipherSuite {
	return noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256)
}

// Session is a post-handshake bidirectional AEAD channel.
type Session struct {
	// Independent directions — do NOT share one mutex. Input Decrypt used to
	// block video Encrypt under mouse floods → frozen picture with live tunnel.
	encMu sync.Mutex
	decMu sync.Mutex
	enc   *noise.CipherState
	dec   *noise.CipherState
}

// Encrypt seals an inner plaintext message for the peer.
func (s *Session) Encrypt(plain []byte) ([]byte, error) {
	if s == nil || s.enc == nil {
		return nil, errors.New("re2: session not ready")
	}
	s.encMu.Lock()
	defer s.encMu.Unlock()
	return s.enc.Encrypt(nil, nil, plain)
}

// Decrypt opens a peer ciphertext.
func (s *Session) Decrypt(cipher []byte) ([]byte, error) {
	if s == nil || s.dec == nil {
		return nil, errors.New("re2: session not ready")
	}
	s.decMu.Lock()
	defer s.decMu.Unlock()
	return s.dec.Decrypt(nil, nil, cipher)
}

// Handshake runs Noise_XXpsk3. Client=initiator, Agent=responder.
type Handshake struct {
	hs *noise.HandshakeState
}

type Transport interface {
	Send(msg []byte) error
	Recv() ([]byte, error)
}

// NewClientHandshake initiator side (ephemeral static for this session).
func NewClientHandshake(psk []byte) (*Handshake, error) {
	if len(psk) != 32 {
		return nil, errors.New("re2: psk must be 32 bytes")
	}
	kp, err := GenerateStaticKeyPair()
	if err != nil {
		return nil, err
	}
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:           cipherSuite(),
		Pattern:               noise.HandshakeXX,
		Initiator:             true,
		Prologue:              []byte(Prologue),
		PresharedKey:          psk,
		PresharedKeyPlacement: 3,
		StaticKeypair:         noise.DHKey{Private: kp.Private, Public: kp.Public},
	})
	if err != nil {
		return nil, err
	}
	return &Handshake{hs: hs}, nil
}

// NewAgentHandshake responder side with long-term static key.
func NewAgentHandshake(psk []byte, static *StaticKeyPair) (*Handshake, error) {
	if len(psk) != 32 {
		return nil, errors.New("re2: psk must be 32 bytes")
	}
	if static == nil || len(static.Private) != 32 {
		return nil, errors.New("re2: agent static key required")
	}
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:           cipherSuite(),
		Pattern:               noise.HandshakeXX,
		Initiator:             false,
		Prologue:              []byte(Prologue),
		PresharedKey:          psk,
		PresharedKeyPlacement: 3,
		StaticKeypair:         noise.DHKey{Private: static.Private, Public: static.Public},
	})
	if err != nil {
		return nil, err
	}
	return &Handshake{hs: hs}, nil
}

// RunClient performs initiator handshake over Transport.
// Returns session and peer (agent) static public key for QR pinning.
func (h *Handshake) RunClient(t Transport) (*Session, []byte, error) {
	msg, _, _, err := h.hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := t.Send(msg); err != nil {
		return nil, nil, err
	}
	reply, err := t.Recv()
	if err != nil {
		return nil, nil, err
	}
	if _, _, _, err := h.hs.ReadMessage(nil, reply); err != nil {
		return nil, nil, fmt.Errorf("re2 handshake read: %w", err)
	}
	msg, cs0, cs1, err := h.hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := t.Send(msg); err != nil {
		return nil, nil, err
	}
	peer := append([]byte(nil), h.hs.PeerStatic()...)
	return &Session{enc: cs0, dec: cs1}, peer, nil
}

// RunAgent performs responder handshake over Transport.
func (h *Handshake) RunAgent(t Transport) (*Session, []byte, error) {
	msg, err := t.Recv()
	if err != nil {
		return nil, nil, err
	}
	if _, _, _, err := h.hs.ReadMessage(nil, msg); err != nil {
		return nil, nil, err
	}
	msg, _, _, err = h.hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := t.Send(msg); err != nil {
		return nil, nil, err
	}
	// Unreliable UDP often retransmits msg1 while we await msg3. A single bad
	// ReadMessage used to abort the whole XX → App background→foreground fell
	// back to WSS / stuck reconnect. Skip AEAD failures and keep reading.
	var cs0, cs1 *noise.CipherState
	for attempt := 0; attempt < 12; attempt++ {
		msg, err = t.Recv()
		if err != nil {
			return nil, nil, err
		}
		_, cs0, cs1, err = h.hs.ReadMessage(nil, msg)
		if err == nil {
			peer := append([]byte(nil), h.hs.PeerStatic()...)
			return &Session{enc: cs1, dec: cs0}, peer, nil
		}
	}
	if err == nil {
		err = errors.New("no msg3")
	}
	return nil, nil, fmt.Errorf("re2 handshake finish: %w", err)
}

// MultiHandshake is a responder that accepts any of several PSKs. XXpsk3 mixes the
// PSK only into msg3, so every candidate shares one ephemeral key and sends the same
// msg2; msg3 then authenticates against exactly one of them.
type MultiHandshake struct {
	hs []*noise.HandshakeState
}

// NewAgentHandshakes prepares one responder per candidate PSK (most likely first).
func NewAgentHandshakes(psks [][]byte, static *StaticKeyPair) (*MultiHandshake, error) {
	if len(psks) == 0 {
		return nil, errors.New("re2: no psk")
	}
	if static == nil || len(static.Private) != 32 {
		return nil, errors.New("re2: agent static key required")
	}
	// flynn/noise draws the responder ephemeral from Config.Random when writing msg2
	// (EphemeralKeypair is ignored there), so every candidate reads the same seed.
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}
	m := &MultiHandshake{}
	for _, psk := range psks {
		if len(psk) != 32 {
			return nil, errors.New("re2: psk must be 32 bytes")
		}
		hs, err := noise.NewHandshakeState(noise.Config{
			CipherSuite:           cipherSuite(),
			Pattern:               noise.HandshakeXX,
			Initiator:             false,
			Prologue:              []byte(Prologue),
			PresharedKey:          psk,
			PresharedKeyPlacement: 3,
			StaticKeypair:         noise.DHKey{Private: static.Private, Public: static.Public},
			Random:                bytes.NewReader(seed),
		})
		if err != nil {
			return nil, err
		}
		m.hs = append(m.hs, hs)
	}
	return m, nil
}

// RunAgent performs the responder handshake and returns the session, the peer static
// key and the index of the PSK the client used.
func (m *MultiHandshake) RunAgent(t Transport) (*Session, []byte, int, error) {
	msg, err := t.Recv()
	if err != nil {
		return nil, nil, -1, err
	}
	for _, hs := range m.hs {
		if _, _, _, err := hs.ReadMessage(nil, msg); err != nil {
			return nil, nil, -1, err
		}
	}
	var reply []byte
	live := make([]*noise.HandshakeState, 0, len(m.hs))
	idx := make([]int, 0, len(m.hs))
	for i, hs := range m.hs {
		out, _, _, err := hs.WriteMessage(nil, nil)
		if err != nil {
			return nil, nil, -1, err
		}
		if reply == nil {
			reply = out
		} else if string(out) != string(reply) {
			continue
		}
		live = append(live, hs)
		idx = append(idx, i)
	}
	if err := t.Send(reply); err != nil {
		return nil, nil, -1, err
	}
	// Same retransmit tolerance as Handshake.RunAgent: skip packets that do not
	// authenticate (duplicate msg1, wrong PSK candidate) and keep reading.
	err = nil
	for attempt := 0; attempt < 12; attempt++ {
		msg, err = t.Recv()
		if err != nil {
			return nil, nil, -1, err
		}
		for j, hs := range live {
			_, cs0, cs1, rerr := hs.ReadMessage(nil, msg)
			if rerr == nil {
				peer := append([]byte(nil), hs.PeerStatic()...)
				return &Session{enc: cs1, dec: cs0}, peer, idx[j], nil
			}
			err = rerr
		}
	}
	if err == nil {
		err = errors.New("no msg3")
	}
	return nil, nil, -1, fmt.Errorf("re2 handshake finish: %w", err)
}

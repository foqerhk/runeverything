package re2

import (
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
	mu  sync.Mutex
	enc *noise.CipherState
	dec *noise.CipherState
}

// Encrypt seals an inner plaintext message for the peer.
func (s *Session) Encrypt(plain []byte) ([]byte, error) {
	if s == nil || s.enc == nil {
		return nil, errors.New("re2: session not ready")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enc.Encrypt(nil, nil, plain)
}

// Decrypt opens a peer ciphertext.
func (s *Session) Decrypt(cipher []byte) ([]byte, error) {
	if s == nil || s.dec == nil {
		return nil, errors.New("re2: session not ready")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
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
	msg, err = t.Recv()
	if err != nil {
		return nil, nil, err
	}
	_, cs0, cs1, err := h.hs.ReadMessage(nil, msg)
	if err != nil {
		return nil, nil, fmt.Errorf("re2 handshake finish: %w", err)
	}
	peer := append([]byte(nil), h.hs.PeerStatic()...)
	return &Session{enc: cs1, dec: cs0}, peer, nil
}
